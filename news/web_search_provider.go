package news

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

type WebSearchProvider struct {
	query    string
	interval time.Duration
	client   interface {
		CallWithMessages(systemPrompt, userPrompt string) (string, error)
	}
	provider string
	model    string
}

type webSearchDigestPayload struct {
	Items []webSearchDigestItem `json:"items"`
}

type webSearchDigestItem struct {
	Headline    string   `json:"headline"`
	Summary     string   `json:"summary"`
	Source      string   `json:"source"`
	URL         string   `json:"url"`
	PublishedAt string   `json:"published_at"`
	Tags        []string `json:"tags"`
}

func NewWebSearchProvider(opts WebSearchOptions) Provider {
	return &WebSearchProvider{
		query:    strings.TrimSpace(opts.Query),
		interval: opts.Interval,
		client:   opts.Client,
		provider: strings.TrimSpace(opts.Provider),
		model:    strings.TrimSpace(opts.Model),
	}
}

func (p *WebSearchProvider) Name() string {
	name := "web-search"
	if p.provider != "" {
		name += "-" + sanitize(p.provider)
	}
	return name
}

func (p *WebSearchProvider) Run(ctx context.Context, svc *Service) {
	if p == nil || p.client == nil {
		return
	}
	if p.interval <= 0 {
		p.interval = 15 * time.Minute
	}

	p.fetchOnce(ctx, svc)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.fetchOnce(ctx, svc)
		}
	}
}

func (p *WebSearchProvider) fetchOnce(ctx context.Context, svc *Service) {
	query := p.query
	if query == "" {
		query = "latest crypto market macro news BTC ETH regulation ETF exchange security"
	}

	systemPrompt := `你是加密市场新闻聚合器。请使用你所连接模型的实时联网搜索能力，检索最近24小时内最值得量化交易系统关注的加密市场与宏观新闻。

必须输出 JSON，格式如下：
{
  "items": [
    {
      "headline": "中文标题",
      "summary": "中文摘要，不超过90字",
      "source": "原始媒体或公告主体",
      "url": "https://...",
      "published_at": "2026-03-09T08:30:00Z",
      "tags": ["btc", "etf", "regulation"]
    }
  ]
}

要求：
1. 只保留最近24小时内的 3-8 条高价值新闻。
2. 尽量优先官方公告、主流财经媒体、交易所公告。
3. 不要输出 markdown，不要输出解释文字，不要包裹代码块。
4. 如果无法联网搜索，返回 {"items":[]}`

	userPrompt := fmt.Sprintf("搜索主题：%s", query)
	raw, err := p.client.CallWithMessages(systemPrompt, userPrompt)
	if err != nil {
		log.Printf("%s: WebSearch provider 调用失败: %v", svc.loggerPrefix, err)
		return
	}

	items, err := parseWebSearchItems(raw)
	if err != nil {
		log.Printf("%s: WebSearch provider 解析失败: %v", svc.loggerPrefix, err)
		return
	}

	added := 0
	for idx, item := range items {
		newsItem := NewsItem{
			ID:          buildSearchNewsID(item, idx),
			Headline:    cleanFallbackContent(sanitize(item.Headline)),
			Content:     cleanFallbackContent(sanitize(item.Summary)),
			Source:      sanitize(item.Source),
			SourceType:  "web_search",
			URL:         strings.TrimSpace(item.URL),
			Tags:        sanitizeStringSlice(item.Tags),
			PublishedAt: parsePublishedAt(item.PublishedAt),
			ReceivedAt:  time.Now(),
		}

		if newsItem.Headline == "" && newsItem.Content == "" {
			continue
		}
		if newsItem.PublishedAt.IsZero() {
			newsItem.PublishedAt = time.Now()
		}
		if svc.ingestItem(newsItem) {
			added++
		}
	}

	if added > 0 {
		log.Printf("%s: WebSearch 新增/合并 %d 条新闻", svc.loggerPrefix, added)
	}
}

func parseWebSearchItems(raw string) ([]webSearchDigestItem, error) {
	clean := strings.TrimSpace(raw)
	if strings.HasPrefix(clean, "```") {
		clean = stripCodeFence(clean)
	}
	start := strings.Index(clean, "{")
	end := strings.LastIndex(clean, "}")
	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("web search 响应缺少 JSON")
	}

	jsonPart := normalizeJSON(clean[start : end+1])
	var payload webSearchDigestPayload
	if err := json.Unmarshal([]byte(jsonPart), &payload); err != nil {
		return nil, fmt.Errorf("解析 web search JSON 失败: %w", err)
	}
	return payload.Items, nil
}

func buildSearchNewsID(item webSearchDigestItem, idx int) string {
	base := chooseNonEmpty(strings.TrimSpace(item.URL), strings.TrimSpace(item.Headline))
	base = normalizeDedupKey(base)
	if base == "" {
		base = fmt.Sprintf("search-%d", idx)
	}
	return "search-" + base
}

func parsePublishedAt(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts
		}
	}
	return time.Time{}
}

func sanitizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		clean := sanitize(value)
		if clean == "" {
			continue
		}
		key := strings.ToLower(clean)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, clean)
	}
	return result
}
