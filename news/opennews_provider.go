package news

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// OpenNewsOptions OpenNews API 配置
type OpenNewsOptions struct {
	Enabled bool
	APIURL  string // REST API 地址，如 https://ai.6551.io
	WSURL   string // WebSocket 地址，如 wss://ai.6551.io/open/news_wss
	APIKey  string // Bearer Token
}

// OpenNewsProvider OpenNews 新闻源
type OpenNewsProvider struct {
	APIURL string
	WSURL  string
	APIKey string
}

func (p OpenNewsProvider) Name() string {
	return "opennews"
}

// openNewsArticle OpenNews API 返回的文章结构
type openNewsArticle struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	NewsType   string `json:"newsType"`
	EngineType string `json:"engineType"`
	Link       string `json:"link"`
	Coins      []struct {
		Symbol     string `json:"symbol"`
		MarketType string `json:"market_type"`
		Match      string `json:"match"`
	} `json:"coins"`
	AIRating *struct {
		Score     int    `json:"score"`
		Grade     string `json:"grade"`
		Signal    string `json:"signal"`
		Status    string `json:"status"`
		Summary   string `json:"summary"`
		EnSummary string `json:"enSummary"`
	} `json:"aiRating,omitempty"`
	TS int64 `json:"ts"`
}

func (p OpenNewsProvider) Run(ctx context.Context, svc *Service) {
	ws := strings.TrimSpace(p.WSURL)
	if ws != "" {
		// WebSocket 模式：添加 token 到 URL
		if !strings.Contains(ws, "token=") && p.APIKey != "" {
			sep := "?"
			if strings.Contains(ws, "?") {
				sep = "&"
			}
			ws = ws + sep + "token=" + p.APIKey
		}
		svc.runWebsocket(ctx, ws)
		return
	}

	api := strings.TrimSpace(p.APIURL)
	if api != "" {
		// HTTP 轮询模式
		p.runHTTP(ctx, svc, api)
	}
}

func (p OpenNewsProvider) runHTTP(ctx context.Context, svc *Service, apiURL string) {
	interval := svc.opts.PingInterval
	if interval <= 0 {
		interval = 40 * time.Second
	}

	log.Printf("%s: OpenNews 服务启动（HTTP轮询），目标: %s，间隔: %s", svc.loggerPrefix, apiURL, interval)

	// 首次拉取
	if err := p.fetchOpenNews(ctx, svc, apiURL); err != nil {
		log.Printf("%s: OpenNews 首次拉取失败: %v", svc.loggerPrefix, err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.fetchOpenNews(ctx, svc, apiURL); err != nil {
				if !strings.Contains(err.Error(), "context canceled") {
					log.Printf("%s: OpenNews 拉取失败: %v", svc.loggerPrefix, err)
				}
			}
		}
	}
}

func (p OpenNewsProvider) fetchOpenNews(ctx context.Context, svc *Service, apiURL string) error {
	// 构建请求 URL：使用 /open/news 端点获取最新新闻
	target := strings.TrimSuffix(apiURL, "/") + "/open/news"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("构建 OpenNews 请求失败: %w", err)
	}

	// 添加认证 Header
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求 OpenNews 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("OpenNews 响应状态异常: %s | %s", resp.Status, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取 OpenNews 响应失败: %w", err)
	}

	// OpenNews 可能返回单条或多条文章
	var articles []openNewsArticle
	if err := json.Unmarshal(body, &articles); err != nil {
		// 尝试解析单条
		var single openNewsArticle
		if err2 := json.Unmarshal(body, &single); err2 != nil {
			return fmt.Errorf("解析 OpenNews JSON 失败: %w", err)
		}
		articles = []openNewsArticle{single}
	}

	if len(articles) == 0 {
		return nil
	}

	newsItems := make([]NewsItem, 0, len(articles))
	digests := make([]Digest, 0, len(articles))

	now := time.Now()
	for _, article := range articles {
		item := p.buildNewsItem(article, now)
		digest := p.buildDigest(article, now)
		newsItems = append(newsItems, item)
		digests = append(digests, digest)
	}

	added := 0
	for i, item := range newsItems {
		if svc.ingestItemWithDigest(item, digests[i]) {
			added++
		}
	}

	if added > 0 {
		log.Printf("%s: OpenNews 新增 %d 条新闻", svc.loggerPrefix, added)
	}

	return nil
}

func (p OpenNewsProvider) buildNewsItem(article openNewsArticle, now time.Time) NewsItem {
	// 从 coins 中提取标签
	tags := make([]string, 0, len(article.Coins))
	for _, coin := range article.Coins {
		tags = append(tags, coin.Symbol)
	}

	// 构建 raw 数据
	raw, _ := json.Marshal(article)

	// 解析时间戳（毫秒）
	publishedAt := now
	if article.TS > 0 {
		publishedAt = time.UnixMilli(article.TS)
	}

	return NewsItem{
		ID:          article.ID,
		Headline:    article.Text,
		Content:     article.Text,
		Source:      article.NewsType,
		SourceType:  article.EngineType,
		URL:         article.Link,
		Tags:        tags,
		PublishedAt: publishedAt,
		ReceivedAt:  now,
		Raw:         raw,
	}
}

func (p OpenNewsProvider) buildDigest(article openNewsArticle, now time.Time) Digest {
	summary := ""
	impact := "neutral"
	confidence := 50

	if article.AIRating != nil && article.AIRating.Status == "done" {
		// 使用 OpenNews 提供的 AI 分析结果
		if article.AIRating.Summary != "" {
			summary = article.AIRating.Summary
		} else if article.AIRating.EnSummary != "" {
			summary = article.AIRating.EnSummary
		}
		confidence = article.AIRating.Score
		// 转换 signal 到 impact
		switch strings.ToLower(article.AIRating.Signal) {
		case "long", "bullish":
			impact = "bullish"
		case "short", "bearish":
			impact = "bearish"
		default:
			impact = "neutral"
		}
	}

	if summary == "" {
		summary = article.Text
	}

	// 解析时间戳
	publishedAt := now
	if article.TS > 0 {
		publishedAt = time.UnixMilli(article.TS)
	}

	return Digest{
		ID:          article.ID,
		Headline:    article.Text,
		Summary:     summary,
		Impact:      impact,
		Sentiment:   impact,
		Confidence:  confidence,
		Reasoning:   "OpenNews AI Rating",
		Source:      article.NewsType,
		SourceType:  article.EngineType,
		URL:         article.Link,
		PublishedAt: publishedAt,
		CreatedAt:   now,
		ItemIDs:     []string{article.ID},
	}
}
