package news

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
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
	ID         interface{} `json:"id"`
	Text       string      `json:"text"`
	Source     string      `json:"source"`
	NewsType   string      `json:"newsType"`
	EngineType string      `json:"engineType"`
	Link       string      `json:"link"`
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
	TS interface{} `json:"ts"`
}

func (p OpenNewsProvider) Run(ctx context.Context, svc *Service) {
	ws := strings.TrimSpace(p.WSURL)
	if ws != "" {
		parsedWS, err := url.Parse(ws)
		if err != nil || (parsedWS.Scheme != "ws" && parsedWS.Scheme != "wss") {
			api := strings.TrimSpace(p.APIURL)
			if api != "" {
				log.Printf("%s: OpenNews WS地址无效(%q)，降级为HTTP轮询", svc.loggerPrefix, ws)
				p.runHTTP(ctx, svc, api)
				return
			}

			log.Printf("%s: OpenNews WS地址无效(%q)，且未配置可用HTTP地址，跳过该provider", svc.loggerPrefix, ws)
			return
		}

		// WebSocket 模式：添加 token 到 URL
		if !strings.Contains(ws, "token=") && p.APIKey != "" {
			sep := "?"
			if strings.Contains(ws, "?") {
				sep = "&"
			}
			ws = ws + sep + "token=" + p.APIKey
		}

		api := strings.TrimSpace(p.APIURL)
		backoff := svc.opts.ReconnectDelay
		if backoff <= 0 {
			backoff = 10 * time.Second
		}

		for {
			if ctx.Err() != nil {
				return
			}

			err := svc.consume(ctx, ws)
			if err == nil || ctx.Err() != nil {
				return
			}

			if shouldFallbackToHTTP(err) {
				if api != "" {
					log.Printf("%s: OpenNews WebSocket不可用(%v)，降级为HTTP轮询", svc.loggerPrefix, err)
					p.runHTTP(ctx, svc, api)
					return
				}

				log.Printf("%s: OpenNews WebSocket不可用(%v)，且未配置HTTP地址，停止该provider", svc.loggerPrefix, err)
				return
			}

			log.Printf("%s: OpenNews WebSocket中断: %v，%v后重试", svc.loggerPrefix, err, backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}
	}

	api := strings.TrimSpace(p.APIURL)
	if api != "" {
		// HTTP 轮询模式
		p.runHTTP(ctx, svc, api)
	}
}

func shouldFallbackToHTTP(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "403 forbidden") ||
		strings.Contains(msg, "401 unauthorized") ||
		strings.Contains(msg, "upgrade to a higher plan") ||
		strings.Contains(msg, "unlock this content")
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
	// 官方 latest 接口走 /open/news_search，而不是 /open/news。
	target := strings.TrimSuffix(apiURL, "/") + "/open/news_search"
	requestBody := strings.NewReader(`{"limit":20,"page":1}`)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, requestBody)
	if err != nil {
		return fmt.Errorf("构建 OpenNews 请求失败: %w", err)
	}

	// 添加认证 Header
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求 OpenNews 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("OpenNews 响应状态异常: %s | %s", resp.Status, strings.TrimSpace(string(body)))
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取 OpenNews 响应失败: %w", err)
	}

	// OpenNews latest 接口返回 envelope: { data: [...], total, success, ... }
	var envelope struct {
		Data []openNewsArticle `json:"data"`
	}
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return fmt.Errorf("解析 OpenNews JSON 失败: %w", err)
	}

	var articles []openNewsArticle
	articles = envelope.Data

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

	publishedAt := parseOpenNewsTime(article.TS, now)

	return NewsItem{
		ID:          parseOpenNewsID(article.ID),
		Headline:    article.Text,
		Content:     article.Text,
		Source:      chooseOpenNewsSource(article),
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

	publishedAt := parseOpenNewsTime(article.TS, now)

	return Digest{
		ID:          parseOpenNewsID(article.ID),
		Headline:    article.Text,
		Summary:     summary,
		Impact:      impact,
		Sentiment:   impact,
		Confidence:  confidence,
		Reasoning:   "OpenNews AI Rating",
		Source:      chooseOpenNewsSource(article),
		SourceType:  article.EngineType,
		URL:         article.Link,
		PublishedAt: publishedAt,
		CreatedAt:   now,
		ItemIDs:     []string{parseOpenNewsID(article.ID)},
	}
}

func parseOpenNewsID(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func parseOpenNewsTime(value interface{}, fallback time.Time) time.Time {
	switch typed := value.(type) {
	case string:
		typed = strings.TrimSpace(typed)
		if typed == "" {
			return fallback
		}
		if ts, err := time.Parse(time.RFC3339Nano, typed); err == nil {
			return ts
		}
		if ts, err := time.Parse(time.RFC3339, typed); err == nil {
			return ts
		}
		if millis, err := strconv.ParseInt(typed, 10, 64); err == nil {
			if millis > 1e12 {
				return time.UnixMilli(millis)
			}
			if millis > 0 {
				return time.Unix(millis, 0)
			}
		}
	case float64:
		if typed > 1e12 {
			return time.UnixMilli(int64(typed))
		}
		if typed > 0 {
			return time.Unix(int64(typed), 0)
		}
	case int64:
		if typed > 1e12 {
			return time.UnixMilli(typed)
		}
		if typed > 0 {
			return time.Unix(typed, 0)
		}
	}
	return fallback
}

func chooseOpenNewsSource(article openNewsArticle) string {
	if source := strings.TrimSpace(article.Source); source != "" {
		return source
	}
	if newsType := strings.TrimSpace(article.NewsType); newsType != "" {
		return newsType
	}
	return strings.TrimSpace(article.EngineType)
}
