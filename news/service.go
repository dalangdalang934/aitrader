package news

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// newsStore 私有接口：仅需 SaveNewsCache/LoadNewsCache
type newsStore interface {
	SaveNewsCache(rawData []byte) error
	LoadNewsCache() ([]byte, error)
}

// Options 定义新闻服务的启动参数
type Options struct {
	WebsocketURL    string        // WebSocket地址，如果以http://或https://开头则使用HTTP轮询
	RSSURL          string        // RSS地址（如果使用RSS模式）
	StorageDir      string        // 存储目录
	RawFilename     string        // 原始新闻文件名
	DigestFilename  string        // 摘要文件名
	MaxAge          time.Duration // 数据保留时长
	PersistCooldown time.Duration // 持久化冷却时间
	ReconnectDelay  time.Duration // WebSocket重连延迟
	PingInterval    time.Duration // WebSocket心跳间隔（HTTP模式用作轮询间隔）
	Summarizer      Summarizer    // 摘要生成器
	WebSearch       WebSearchOptions
	OpenNews        OpenNewsOptions // OpenNews API 配置
}

func (o *Options) applyDefaults() {
	if o.WebsocketURL == "" && o.RSSURL == "" {
		// 默认使用RSS模式，使用BWE新闻RSS接口
		o.RSSURL = "https://rss-public.bwe-ws.com/"
	}
	if o.StorageDir == "" {
		o.StorageDir = filepath.Join("data", "news")
	}
	if o.RawFilename == "" {
		o.RawFilename = "raw_news.json"
	}
	if o.DigestFilename == "" {
		o.DigestFilename = "digests.json"
	}
	if o.MaxAge <= 0 {
		o.MaxAge = 2 * time.Hour
	}
	if o.PersistCooldown <= 0 {
		o.PersistCooldown = 5 * time.Second
	}
	if o.ReconnectDelay <= 0 {
		o.ReconnectDelay = 10 * time.Second
	}
	if o.PingInterval <= 0 {
		o.PingInterval = 40 * time.Second
	}
	if o.WebSearch.Interval <= 0 {
		o.WebSearch.Interval = 15 * time.Minute
	}
}

func ensureWritableStorage(preferred string) (string, bool, error) {
	target := strings.TrimSpace(preferred)
	if target == "" {
		target = filepath.Join("data", "news")
	}

	primaryErr := ensureDirWritable(target)
	if primaryErr == nil {
		return target, false, nil
	}

	fallback := fallbackStorageDir(target)
	if fallback == "" || fallback == target {
		fallback = filepath.Join(os.TempDir(), "trading-news")
	}

	if ensureErr := ensureDirWritable(fallback); ensureErr != nil {
		return "", false, fmt.Errorf("primary news storage %q not writable: %v; fallback %q failed: %v", target, primaryErr, fallback, ensureErr)
	}

	return fallback, true, nil
}

func ensureDirWritable(dir string) error {
	if dir == "" {
		return fmt.Errorf("storage directory is empty")
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	testFile := filepath.Join(dir, fmt.Sprintf(".write-check-%d", time.Now().UnixNano()))
	if err := os.WriteFile(testFile, []byte("ok"), 0o600); err != nil {
		return err
	}
	if err := os.Remove(testFile); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// Service 负责与新闻WebSocket/RSS交互并生成本地摘要
type Service struct {
	opts              Options
	summarizer        Summarizer
	providers         []Provider
	storageDir        string
	primaryStorageDir string
	db                newsStore // 可选，非 nil 时优先用 SQLite 持久化
	mu                sync.RWMutex
	items             []NewsItem
	digests           []Digest
	lastPersist       time.Time
	lastPing          time.Time
	startedAt         time.Time
	loggerPrefix      string

	outlookMu       sync.RWMutex
	outlook         *MacroOutlook
	outlookAnalyzer *OutlookAnalyzer
}

// NewsItem 表示从WebSocket/RSS收到的原始快讯
type NewsItem struct {
	ID          string          `json:"id"`
	Headline    string          `json:"headline"`
	Content     string          `json:"content"`
	Source      string          `json:"source"`
	SourceType  string          `json:"source_type,omitempty"`
	URL         string          `json:"url"`
	Tags        []string        `json:"tags"`
	PublishedAt time.Time       `json:"published_at"`
	ReceivedAt  time.Time       `json:"received_at"`
	Raw         json.RawMessage `json:"raw"`
}

// Digest 表示经过提炼后的观点摘要
type Digest struct {
	ID              string    `json:"id"`
	Headline        string    `json:"headline"`
	Summary         string    `json:"summary"`
	Impact          string    `json:"impact"`
	Sentiment       string    `json:"sentiment"`
	Confidence      int       `json:"confidence"`
	Reasoning       string    `json:"reasoning,omitempty"`
	Source          string    `json:"source"`
	SourceType      string    `json:"source_type,omitempty"`
	SourceRank      int       `json:"source_rank,omitempty"`
	Sources         []string  `json:"sources,omitempty"`
	ConfidenceBasis string    `json:"confidence_basis,omitempty"`
	URL             string    `json:"url"`
	PublishedAt     time.Time `json:"published_at"`
	CreatedAt       time.Time `json:"created_at"`
	ItemIDs         []string  `json:"item_ids"`
}

// persistedContainer 用于序列化到磁盘
type persistedContainer struct {
	UpdatedAt time.Time  `json:"updated_at"`
	Items     []NewsItem `json:"items"`
	Digests   []Digest   `json:"digests"`
}

type rssFeed struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title string    `xml:"title"`
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	GUID           string   `xml:"guid"`
	Title          string   `xml:"title"`
	Description    string   `xml:"description"`
	Link           string   `xml:"link"`
	PubDate        string   `xml:"pubDate"`
	Source         string   `xml:"source"`
	Categories     []string `xml:"category"`
	ContentEncoded string   `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
}

var (
	defaultService *Service
	defaultMu      sync.RWMutex
)

// SetDefaultService 设置全局默认服务实例
func SetDefaultService(s *Service) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultService = s
}

// GetDefaultService 获取全局默认服务实例
func GetDefaultService() *Service {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultService
}

// NewService 创建新闻服务
func NewService(opts Options) (*Service, error) {
	opts.applyDefaults()

	loggerPrefix := "📰 新闻"
	primaryDir := strings.TrimSpace(opts.StorageDir)
	actualDir, usedFallback, err := ensureWritableStorage(primaryDir)
	if err != nil {
		return nil, fmt.Errorf("初始化新闻数据目录失败: %w", err)
	}
	if usedFallback && actualDir != primaryDir && primaryDir != "" {
		log.Printf("%s: 存储目录 %s 不可写，回退到 %s", loggerPrefix, primaryDir, actualDir)
	}
	opts.StorageDir = actualDir

	svc := &Service{
		opts:              opts,
		storageDir:        actualDir,
		primaryStorageDir: primaryDir,
		items:             make([]NewsItem, 0, 128),
		digests:           make([]Digest, 0, 64),
		lastPersist:       time.Time{},
		startedAt:         time.Now(),
		loggerPrefix:      loggerPrefix,
	}

	if err := svc.loadFromDisk(); err != nil {
		log.Printf("%s: 加载本地新闻缓存失败: %v", svc.loggerPrefix, err)
	}

	if opts.Summarizer != nil {
		svc.SetSummarizer(opts.Summarizer)
	}
	svc.providers = buildProviders(opts)

	return svc, nil
}

// SetDB 设置 SQLite 数据库（建表由 db.NewStore 统一完成），启动时从 SQLite 加载缓存。
// 在 NewService 之后、Run 之前调用。
func (s *Service) SetDB(store newsStore) error {
	s.db = store
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadFromDisk(); err != nil {
		log.Printf("%s: 从 SQLite 加载新闻缓存失败: %v", s.loggerPrefix, err)
	}
	return nil
}

// Run 启动与新闻源的同步循环
func (s *Service) Run(ctx context.Context) {
	if len(s.providers) > 0 {
		s.runProviders(ctx)
		return
	}

	// 保留旧逻辑作为兜底，兼容历史缓存和配置。
	if s.opts.RSSURL != "" {
		s.runHTTP(ctx, s.opts.RSSURL)
		return
	}

	target := strings.TrimSpace(s.opts.WebsocketURL)
	switch {
	case strings.HasPrefix(target, "http://"), strings.HasPrefix(target, "https://"):
		s.runHTTP(ctx, target)
	default:
		s.runWebsocket(ctx, target)
	}
}

func (s *Service) runWebsocket(ctx context.Context, target string) {
	log.Printf("%s: 服务启动（WebSocket），目标: %s", s.loggerPrefix, target)

	backoff := s.opts.ReconnectDelay

	for {
		if ctx.Err() != nil {
			return
		}

		if err := s.consume(ctx); err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Printf("%s: 连接中断: %v，%v后重试", s.loggerPrefix, err, backoff)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

func (s *Service) runHTTP(ctx context.Context, target string) {
	interval := s.opts.PingInterval
	if interval <= 0 {
		interval = 40 * time.Second
	}

	log.Printf("%s: 服务启动（HTTP轮询），目标: %s，间隔: %s", s.loggerPrefix, target, interval)

	if err := s.fetchRSS(ctx, target); err != nil {
		log.Printf("%s: 首次拉取RSS失败: %v", s.loggerPrefix, err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.fetchRSS(ctx, target); err != nil {
				if !errors.Is(err, context.Canceled) {
					log.Printf("%s: 拉取RSS失败: %v", s.loggerPrefix, err)
				}
			}
		}
	}
}

// consume 建立连接并读取数据直到出错或上下文取消
func (s *Service) consume(ctx context.Context) error {
	d := websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
	}

	conn, resp, err := d.DialContext(ctx, s.opts.WebsocketURL, nil)
	if err != nil {
		var detail string
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			detail = fmt.Sprintf(" (status=%s, body=%s)", resp.Status, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("websocket连接失败: %w%s", err, detail)
	}
	defer conn.Close()

	log.Printf("%s: 已连接新闻源", s.loggerPrefix)

	pingTicker := time.NewTicker(s.opts.PingInterval)
	defer pingTicker.Stop()

	errCh := make(chan error, 1)

	go func() {
		for {
			_, message, readErr := conn.ReadMessage()
			if readErr != nil {
				errCh <- readErr
				return
			}
			s.handleMessage(message)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		case err := <-errCh:
			return err
		case <-pingTicker.C:
			_ = conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second))
		}
	}
}

func (s *Service) fetchRSS(ctx context.Context, target string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("构建RSS请求失败: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求RSS失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("RSS响应状态异常: %s | %s", resp.Status, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取RSS响应失败: %w", err)
	}

	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return fmt.Errorf("解析RSS失败: %w", err)
	}

	if err := s.ingestRSSItems(feed.Channel.Items, feed.Channel.Title); err != nil {
		return err
	}

	return nil
}

func (s *Service) ingestRSSItems(items []rssItem, channelTitle string) error {
	if len(items) == 0 {
		return nil
	}

	newsItems := make([]NewsItem, 0, len(items))
	digests := make([]Digest, 0, len(items))

	for _, entry := range items {
		item := s.buildNewsItemFromRSS(entry, channelTitle)
		if item == nil {
			continue
		}
		newsItems = append(newsItems, *item)
		digests = append(digests, s.createDigest(*item))
	}

	if len(newsItems) == 0 {
		return nil
	}

	added := 0
	for i, item := range newsItems {
		if s.ingestItemWithDigest(item, digests[i]) {
			added++
		}
	}

	if added > 0 {
		log.Printf("%s: RSS新增 %d 条新闻", s.loggerPrefix, added)
	}

	return nil
}

func (s *Service) buildNewsItemFromRSS(entry rssItem, channelTitle string) *NewsItem {
	// RSS的Title字段可能包含完整内容（标题+正文+<br/>标签等）
	// 需要提取真正的标题（第一行或第一个<br/>之前的内容）
	fullTitle := chooseNonEmpty(entry.Title)
	headline := extractHeadlineFromTitle(fullTitle)

	// Content可能是空的，或者Title已经包含了所有内容
	content := chooseNonEmpty(entry.ContentEncoded, entry.Description)

	// 如果content为空且title包含很多内容，则从title中提取正文部分
	if content == "" && fullTitle != "" {
		// 提取标题后的内容作为正文（跳过第一行）
		var contentParts []string
		if parts := strings.Split(fullTitle, "<br/>"); len(parts) > 1 {
			contentParts = parts[1:]
		} else if idx := strings.Index(fullTitle, "\n"); idx > 0 {
			contentParts = strings.Split(fullTitle[idx+1:], "\n")
		}

		// 过滤掉明显不是正文的内容（时间戳、source URL、分隔线、市场数据等）
		filteredParts := make([]string, 0, len(contentParts))
		for _, part := range contentParts {
			cleaned := strings.TrimSpace(part)
			// 跳过空行、分隔线、时间戳格式、source URL、市场数据等
			if cleaned == "" ||
				strings.HasPrefix(cleaned, "————————————") ||
				strings.HasPrefix(cleaned, "---") ||
				strings.HasPrefix(cleaned, "source:") ||
				strings.HasPrefix(cleaned, "http://") ||
				strings.HasPrefix(cleaned, "https://") ||
				strings.HasPrefix(cleaned, "t.co/") ||
				strings.HasPrefix(cleaned, "x.com/") ||
				regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}`).MatchString(cleaned) ||
				// 跳过市场数据行
				regexp.MustCompile(`^\$[A-Z0-9]+\s*(MarketCap|市值)[:：\s]*\$?\d+[KMkmB]?$`).MatchString(cleaned) ||
				// 跳过Auto match提示行
				strings.Contains(cleaned, "Auto match") || strings.Contains(cleaned, "自动匹配") {
				continue
			}
			filteredParts = append(filteredParts, cleaned)
		}
		content = strings.Join(filteredParts, " ")
	}

	if headline == "" && content == "" {
		return nil
	}

	var published time.Time
	if ts, err := tryParseTime(entry.PubDate); err == nil {
		published = ts
	} else {
		published = time.Now()
	}

	source := chooseNonEmpty(entry.Source, channelTitle, "BWEnews RSS")
	url := strings.TrimSpace(entry.Link)

	var tags []string
	for _, cat := range entry.Categories {
		if trimmed := sanitize(cat); trimmed != "" {
			tags = append(tags, trimmed)
		}
	}

	id := sanitize(entry.GUID)
	if id == "" {
		id = strings.TrimSpace(entry.Link)
	}
	if id == "" {
		id = fmt.Sprintf("rss-%s-%s", sanitize(entry.Title), entry.PubDate)
	}
	if id == "" {
		id = fmt.Sprintf("rss-%d", time.Now().UnixNano())
	}

	rawEntry, _ := json.Marshal(entry)

	// 对headline和content进行深度清理（移除URL、市场数据等），但保留英文和中文
	headline = sanitize(headline)
	headline = cleanFallbackContent(headline)

	content = sanitize(content)
	content = cleanFallbackContent(content)

	return &NewsItem{
		ID:          id,
		Headline:    headline, // 已清理：HTML标签、URL、市场数据等
		Content:     content,  // 已清理：HTML标签、URL、市场数据等
		Source:      sanitize(source),
		SourceType:  "rss",
		URL:         url,
		Tags:        tags,
		PublishedAt: published,
		ReceivedAt:  time.Now(),
		Raw:         rawEntry,
	}
}

func (s *Service) hasNewsIDLocked(id string) bool {
	if id == "" {
		return false
	}
	for _, item := range s.items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func (s *Service) ingestItem(item NewsItem) bool {
	return s.ingestItemWithDigest(item, s.createDigest(item))
}

func (s *Service) ingestItemWithDigest(item NewsItem, digest Digest) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.hasNewsIDLocked(item.ID) {
		return false
	}

	s.items = append(s.items, item)

	if match := s.findDigestMatchLocked(item, digest); match >= 0 {
		s.mergeDigestLocked(match, item, digest)
	} else {
		s.digests = append(s.digests, digest)
	}

	s.pruneLocked()
	s.persistLocked()
	return true
}

func (s *Service) findDigestMatchLocked(item NewsItem, digest Digest) int {
	newURL := normalizeDedupKey(chooseNonEmpty(item.URL, digest.URL))
	newHeadline := normalizeDedupKey(chooseNonEmpty(item.Headline, digest.Headline))

	for idx, existing := range s.digests {
		existingURL := normalizeDedupKey(existing.URL)
		if newURL != "" && existingURL != "" && newURL == existingURL {
			return idx
		}

		existingHeadline := normalizeDedupKey(existing.Headline)
		if newHeadline == "" || existingHeadline == "" || newHeadline != existingHeadline {
			continue
		}

		if sameDay(existing.PublishedAt, chooseDigestTime(digest)) {
			return idx
		}
	}
	return -1
}

func (s *Service) mergeDigestLocked(index int, item NewsItem, digest Digest) {
	existing := &s.digests[index]
	existing.ItemIDs = compactUniqueStrings(append(existing.ItemIDs, item.ID)...)
	existing.Sources = compactUniqueStrings(append(existing.Sources, digest.Sources...)...)
	existing.Sources = compactUniqueStrings(append(existing.Sources, item.Source, digest.Source)...)

	if existing.Source == "" || scoreNewsSource(digest.SourceType, digest.Source) > existing.SourceRank {
		existing.Source = chooseNonEmpty(digest.Source, item.Source, existing.Source)
		existing.SourceType = chooseNonEmpty(digest.SourceType, item.SourceType, existing.SourceType)
		existing.SourceRank = scoreNewsSource(existing.SourceType, existing.Source)
	}

	if existing.URL == "" {
		existing.URL = chooseNonEmpty(digest.URL, item.URL)
	}
	if existing.Summary == "" {
		existing.Summary = chooseNonEmpty(digest.Summary, item.Content)
	}
	if existing.Reasoning == "" {
		existing.Reasoning = digest.Reasoning
	}
	if existing.Confidence < digest.Confidence {
		existing.Confidence = digest.Confidence
		existing.Impact = digest.Impact
		existing.Sentiment = digest.Sentiment
		existing.ConfidenceBasis = digest.ConfidenceBasis
	}

	existing.PublishedAt = chooseLaterTime(existing.PublishedAt, chooseDigestTime(digest), item.PublishedAt)
	if existing.CreatedAt.IsZero() {
		existing.CreatedAt = time.Now()
	}
	if existing.SourceRank <= 0 {
		existing.SourceRank = scoreNewsSource(existing.SourceType, existing.Source)
	}
	if existing.ConfidenceBasis == "" {
		existing.ConfidenceBasis = defaultConfidenceBasis(existing.SourceType, existing.Confidence > 50)
	}
}

func normalizeDedupKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"https://", "",
		"http://", "",
		"www.", "",
		"/", "",
		"?", "",
		"&", "",
		"=", "",
		"-", "",
		"_", "",
		" ", "",
		".", "",
	)
	return replacer.Replace(value)
}

func sameDay(a, b time.Time) bool {
	if a.IsZero() || b.IsZero() {
		return false
	}
	return a.UTC().Format("2006-01-02") == b.UTC().Format("2006-01-02")
}

func chooseDigestTime(digest Digest) time.Time {
	if !digest.PublishedAt.IsZero() {
		return digest.PublishedAt
	}
	return digest.CreatedAt
}

func chooseLaterTime(values ...time.Time) time.Time {
	var latest time.Time
	for _, value := range values {
		if value.IsZero() {
			continue
		}
		if latest.IsZero() || value.After(latest) {
			latest = value
		}
	}
	return latest
}

func compactUniqueStrings(values ...string) []string {
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

func scoreNewsSource(sourceType, source string) int {
	lowerSource := strings.ToLower(strings.TrimSpace(source))
	switch {
	case strings.Contains(lowerSource, "binance"), strings.Contains(lowerSource, "coinbase"),
		strings.Contains(lowerSource, "okx"), strings.Contains(lowerSource, "sec"),
		strings.Contains(lowerSource, "federal reserve"), strings.Contains(lowerSource, "cftc"),
		strings.Contains(lowerSource, "blackrock"), strings.Contains(lowerSource, "bloomberg"),
		strings.Contains(lowerSource, "reuters"):
		return 95
	case sourceType == "web_search":
		return 82
	case sourceType == "websocket":
		return 72
	case sourceType == "rss":
		return 68
	default:
		return 60
	}
}

func defaultConfidenceBasis(sourceType string, usedAI bool) string {
	switch {
	case sourceType == "web_search" && usedAI:
		return "search_provider_and_ai_summary"
	case usedAI:
		return "provider_feed_and_ai_summary"
	case sourceType == "web_search":
		return "search_provider"
	default:
		return "provider_feed"
	}
}

// handleMessage 解析并处理新闻消息
func (s *Service) handleMessage(raw []byte) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return
	}

	// 消息可能是数组或对象
	if trimmed[0] == '[' {
		var arr []map[string]interface{}
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			log.Printf("%s: 解析数组消息失败: %v", s.loggerPrefix, err)
			return
		}
		for _, m := range arr {
			s.handleEntry(m)
		}
		return
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		log.Printf("%s: 解析消息失败: %v", s.loggerPrefix, err)
		return
	}

	// 某些结构可能把新闻放在 data/list 中
	if data, ok := obj["data"].(map[string]interface{}); ok {
		if list, ok := data["list"].([]interface{}); ok {
			for _, item := range list {
				if entry, ok := item.(map[string]interface{}); ok {
					s.handleEntry(entry)
				}
			}
			return
		}
	}

	s.handleEntry(obj)
}

func (s *Service) handleEntry(entry map[string]interface{}) {
	rawEntry, _ := json.Marshal(entry)
	item := s.buildNewsItem(entry, rawEntry)
	if item == nil {
		return
	}

	s.ingestItem(*item)
}

func (s *Service) buildNewsItem(entry map[string]interface{}, raw json.RawMessage) *NewsItem {
	now := time.Now()
	headline := pickString(entry, "title", "headline", "newsTitle", "name")
	content := pickString(entry, "content", "summary", "desc", "description", "digest", "text")

	if headline == "" && content == "" {
		return nil
	}

	publishedAt := parseTime(entry, now)
	source := pickString(entry, "source", "from", "provider", "channel")
	url := pickString(entry, "url", "link", "pageUrl")
	tags := pickStringArray(entry, "tags", "category", "categories")

	id := pickString(entry, "id", "newsId", "uuid")
	if id == "" {
		id = fmt.Sprintf("%d", now.UnixNano())
	}

	return &NewsItem{
		ID:          id,
		Headline:    sanitize(headline),
		Content:     sanitize(content),
		Source:      sanitize(source),
		SourceType:  "websocket",
		URL:         strings.TrimSpace(url),
		Tags:        tags,
		PublishedAt: publishedAt,
		ReceivedAt:  now,
		Raw:         raw,
	}
}

func (s *Service) fallbackDigest(item NewsItem) Digest {
	text := item.Content
	if text == "" {
		text = item.Headline
	}
	text = sanitize(text)
	// 清理URL、市场数据、时间戳等
	text = cleanFallbackContent(text)
	if len([]rune(text)) > 240 {
		text = string([]rune(text)[:240]) + "..."
	}

	impact, sentiment := classifyImpact(strings.ToLower(item.Headline + " " + text))

	digestID := fmt.Sprintf("digest-%s", item.ID)

	return Digest{
		ID:              digestID,
		Headline:        item.Headline,
		Summary:         text,
		Impact:          impact,
		Sentiment:       sentiment,
		Confidence:      50,
		Source:          item.Source,
		SourceType:      item.SourceType,
		SourceRank:      scoreNewsSource(item.SourceType, item.Source),
		Sources:         compactUniqueStrings(item.Source),
		ConfidenceBasis: defaultConfidenceBasis(item.SourceType, false),
		URL:             item.URL,
		PublishedAt:     item.PublishedAt,
		CreatedAt:       time.Now(),
		ItemIDs:         []string{item.ID},
	}
}

func (s *Service) currentSummarizer() Summarizer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.summarizer
}

func (s *Service) createDigest(item NewsItem) Digest {
	if summarizer := s.currentSummarizer(); summarizer != nil {
		if aiDigest, err := summarizer.Summarize(context.Background(), item); err == nil && aiDigest != nil {
			d := *aiDigest
			s.normalizeDigest(&d, item)
			d.ConfidenceBasis = defaultConfidenceBasis(d.SourceType, true)
			return d
		} else if err != nil {
			log.Printf("%s: AI摘要失败 [%s]: %v", s.loggerPrefix, item.ID, err)
		}
	}

	d := s.fallbackDigest(item)
	s.normalizeDigest(&d, item)
	d.ConfidenceBasis = defaultConfidenceBasis(d.SourceType, false)
	return d
}

func (s *Service) normalizeDigest(digest *Digest, item NewsItem) {
	if digest.ID == "" {
		digest.ID = fmt.Sprintf("digest-%s", item.ID)
	}

	// Headline清理：移除URL、市场数据、时间戳等，但保留英文和中文
	if digest.Headline == "" {
		digest.Headline = item.Headline // item.Headline已经在buildNewsItemFromRSS中清理过了
	} else {
		// 如果digest有headline，也需要清理（可能来自AI返回）
		digest.Headline = cleanFallbackContent(digest.Headline)
	}
	if digest.Headline == "" {
		digest.Headline = item.Headline
	}

	// Summary清理：移除URL、市场数据、时间戳等，但保留英文和中文
	if digest.Summary == "" {
		contentText := sanitize(item.Content)
		contentText = cleanFallbackContent(contentText)
		if len([]rune(contentText)) > 240 {
			digest.Summary = string([]rune(contentText)[:240]) + "..."
		} else {
			digest.Summary = contentText
		}
	} else {
		// 清理已有的summary
		digest.Summary = cleanFallbackContent(digest.Summary)
	}

	// 限制summary长度在90字以内
	if len([]rune(digest.Summary)) > 90 {
		digest.Summary = string([]rune(digest.Summary)[:90]) + "..."
	}
	if len(digest.ItemIDs) == 0 {
		digest.ItemIDs = []string{item.ID}
	}
	if digest.PublishedAt.IsZero() {
		digest.PublishedAt = item.PublishedAt
	}
	if digest.CreatedAt.IsZero() {
		digest.CreatedAt = time.Now()
	}
	if digest.Source == "" {
		digest.Source = item.Source
	}
	if digest.SourceType == "" {
		digest.SourceType = item.SourceType
	}
	if digest.URL == "" {
		digest.URL = item.URL
	}
	if len(digest.Sources) == 0 {
		digest.Sources = compactUniqueStrings(digest.Source, item.Source)
	} else {
		digest.Sources = compactUniqueStrings(append(digest.Sources, item.Source)...)
	}
	if digest.SourceRank <= 0 {
		digest.SourceRank = scoreNewsSource(digest.SourceType, digest.Source)
	}
	if digest.ConfidenceBasis == "" {
		digest.ConfidenceBasis = defaultConfidenceBasis(digest.SourceType, digest.Confidence > 50)
	}
	// 完全信任 AI 返回的置信度，不进行重置
	// 只有在 Fallback 机制（没有 AI）时才使用固定值 50
	// 如果 AI 返回的置信度 > 100，已经在上层 clampConfidence 中被限制为 100
	// 如果置信度 <= 0，可能是 AI 的真实判断（虽然少见），也可能是解析失败
	// 无论如何，我们都保留 AI 的原始判断，不强制覆盖
	if digest.Sentiment == "" {
		digest.Sentiment = "neutral"
	}
	if digest.Impact == "" {
		digest.Impact = "中性"
	}
}

// SetSummarizer 设置用于生成摘要的AI
func (s *Service) SetSummarizer(sum Summarizer) {
	s.mu.Lock()
	s.summarizer = sum
	items := append([]NewsItem(nil), s.items...)
	s.mu.Unlock()

	if sum == nil || len(items) == 0 {
		return
	}

	go s.rebuildDigests(items)
}

func (s *Service) rebuildDigests(items []NewsItem) {
	digests := make([]Digest, 0, len(items))
	for _, item := range items {
		digests = append(digests, s.createDigest(item))
	}

	s.mu.Lock()
	s.digests = digests
	s.pruneLocked()
	s.persistLocked()
	s.mu.Unlock()
}

// GetDigests 返回最近有效期内的新闻摘要（按时间倒序）
func (s *Service) GetDigests() []Digest {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-s.opts.MaxAge)
	result := make([]Digest, 0, len(s.digests))
	for _, d := range s.digests {
		// 优先用 CreatedAt（接收时间）判断是否过期，避免 RSS 历史时间误杀
		pivot := d.CreatedAt
		if pivot.IsZero() {
			pivot = d.PublishedAt
		}
		if !pivot.IsZero() && pivot.Before(cutoff) {
			continue
		}
		result = append(result, d)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].PublishedAt.After(result[j].PublishedAt)
	})

	return result
}

func (s *Service) pruneLocked() {
	cutoff := time.Now().Add(-s.opts.MaxAge)

	filteredItems := s.items[:0]
	for _, item := range s.items {
		// 优先用 ReceivedAt（接收时间），避免 RSS 历史 pubDate 误剪
		pivot := item.ReceivedAt
		if pivot.IsZero() {
			pivot = item.PublishedAt
		}
		if pivot.After(cutoff) {
			filteredItems = append(filteredItems, item)
		}
	}
	s.items = filteredItems

	filteredDigests := s.digests[:0]
	for _, digest := range s.digests {
		// 优先用 CreatedAt（接收时间）
		pivot := digest.CreatedAt
		if pivot.IsZero() {
			pivot = digest.PublishedAt
		}
		if pivot.After(cutoff) {
			filteredDigests = append(filteredDigests, digest)
		}
	}
	s.digests = filteredDigests
}

func (s *Service) persistLocked() {
	if !s.lastPersist.IsZero() && time.Since(s.lastPersist) < s.opts.PersistCooldown {
		return
	}

	container := persistedContainer{
		UpdatedAt: time.Now(),
		Items:     s.items,
		Digests:   s.digests,
	}

	rawData, err := json.MarshalIndent(container, "", "  ")
	if err != nil {
		log.Printf("%s: 序列化新闻缓存失败: %v", s.loggerPrefix, err)
		return
	}

	digestEnvelope := struct {
		UpdatedAt time.Time `json:"updated_at"`
		Digests   []Digest  `json:"digests"`
	}{
		UpdatedAt: time.Now(),
		Digests:   s.digests,
	}

	digestData, err := json.MarshalIndent(digestEnvelope, "", "  ")
	if err != nil {
		log.Printf("%s: 序列化摘要失败: %v", s.loggerPrefix, err)
		return
	}

	if err := s.persistToDir(s.storageDir, rawData, digestData); err != nil {
		if !s.recoverStorage(rawData, digestData, err) {
			log.Printf("%s: 写入新闻缓存失败: %v", s.loggerPrefix, err)
			return
		}
	}

	s.lastPersist = time.Now()
}

func (s *Service) persistToDir(dir string, rawData, digestData []byte) error {
	// 优先 SQLite
	if s.db != nil {
		return s.db.SaveNewsCache(rawData)
	}
	// 降级到文件
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, s.opts.RawFilename), rawData, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, s.opts.DigestFilename), digestData, 0o644)
}

func (s *Service) recoverStorage(rawData, digestData []byte, cause error) bool {
	preferred := s.primaryStorageDir
	if preferred == "" {
		preferred = s.storageDir
	}

	dir, usedFallback, err := ensureWritableStorage(preferred)
	if err != nil {
		log.Printf("%s: 无法恢复新闻存储目录: %v (原始错误: %v)", s.loggerPrefix, err, cause)
		return false
	}

	if usedFallback && dir != preferred {
		log.Printf("%s: 新闻存储目录不可写，已回退到 %s (原因: %v)", s.loggerPrefix, dir, cause)
	} else if dir != s.storageDir {
		log.Printf("%s: 新闻存储目录调整为 %s", s.loggerPrefix, dir)
	}

	s.storageDir = dir
	s.opts.StorageDir = dir

	if err := s.persistToDir(dir, rawData, digestData); err != nil {
		log.Printf("%s: 使用目录 %s 写入新闻数据失败: %v", s.loggerPrefix, dir, err)
		return false
	}

	return true
}

func (s *Service) loadFromDisk() error {
	// 优先 SQLite
	if s.db != nil {
		rawData, err := s.db.LoadNewsCache()
		if err != nil {
			return err
		}
		if rawData == nil {
			return nil
		}
		return s.applyPersistedContainer(rawData)
	}
	// 降级到文件
	rawPath := filepath.Join(s.storageDir, s.opts.RawFilename)
	data, err := os.ReadFile(rawPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return s.applyPersistedContainer(data)
}

func (s *Service) applyPersistedContainer(data []byte) error {
	var container persistedContainer
	if err := json.Unmarshal(data, &container); err != nil {
		return err
	}
	s.items = container.Items
	s.digests = container.Digests
	for i := range s.items {
		if s.items[i].SourceType == "" {
			s.items[i].SourceType = "rss"
		}
	}
	for i := range s.digests {
		item := NewsItem{
			ID:          chooseNonEmpty(firstOrEmpty(s.digests[i].ItemIDs), s.digests[i].ID),
			Headline:    s.digests[i].Headline,
			Content:     s.digests[i].Summary,
			Source:      s.digests[i].Source,
			SourceType:  s.digests[i].SourceType,
			URL:         s.digests[i].URL,
			PublishedAt: s.digests[i].PublishedAt,
			ReceivedAt:  s.digests[i].CreatedAt,
		}
		if item.SourceType == "" {
			item.SourceType = "rss"
		}
		s.normalizeDigest(&s.digests[i], item)
	}
	return nil
}

func firstOrEmpty(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func fallbackStorageDir(path string) string {
	base := sanitize(filepath.Base(path))
	if base == "" || base == "." {
		base = "news"
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("trading-%s", base))
}

// pickString 选择第一个存在的字符串字段
func pickString(entry map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := entry[key]; ok {
			if str, ok := value.(string); ok {
				return str
			}
		}
	}
	return ""
}

func pickStringArray(entry map[string]interface{}, keys ...string) []string {
	for _, key := range keys {
		if value, ok := entry[key]; ok {
			switch typed := value.(type) {
			case []interface{}:
				arr := make([]string, 0, len(typed))
				for _, v := range typed {
					if str, ok := v.(string); ok {
						arr = append(arr, sanitize(str))
					}
				}
				if len(arr) > 0 {
					return arr
				}
			case []string:
				return typed
			case string:
				if typed != "" {
					parts := strings.Split(typed, ",")
					result := make([]string, 0, len(parts))
					for _, part := range parts {
						trimmed := strings.TrimSpace(part)
						if trimmed != "" {
							result = append(result, trimmed)
						}
					}
					if len(result) > 0 {
						return result
					}
				}
			}
		}
	}
	return nil
}

func parseTime(entry map[string]interface{}, fallback time.Time) time.Time {
	candidates := []string{"published_at", "publishTime", "time", "timestamp", "created_at", "createdAt"}
	for _, key := range candidates {
		if value, ok := entry[key]; ok {
			if str, ok := value.(string); ok {
				if ts, err := tryParseTime(str); err == nil {
					return ts
				}
			}
			if f, ok := value.(float64); ok {
				sec := int64(f)
				if f > 1e12 {
					sec = int64(f / 1000)
				}
				if sec > 0 {
					return time.Unix(sec, 0)
				}
			}
		}
	}
	return fallback
}

func tryParseTime(value string) (time.Time, error) {
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05",
		time.RFC3339Nano,
		"Mon, 02 Jan 2006 15:04:05 MST",
		"Mon, 02 Jan 2006 15:04:05 -0700",
	}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析时间: %s", value)
}

func sanitize(input string) string {
	trimmed := strings.TrimSpace(input)
	// 移除HTML标签
	trimmed = removeHTMLTags(trimmed)
	trimmed = strings.ReplaceAll(trimmed, "\n", " ")
	trimmed = strings.ReplaceAll(trimmed, "\r", " ")
	trimmed = strings.Join(strings.Fields(trimmed), " ")
	return trimmed
}

// removeHTMLTags 移除HTML标签，将常见的HTML标签转换为空格
func removeHTMLTags(text string) string {
	// 移除所有HTML标签 <...>
	htmlTagRegex := regexp.MustCompile(`<[^>]*>`)
	text = htmlTagRegex.ReplaceAllString(text, " ")

	// 转换常见的HTML实体
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")

	return text
}

// cleanFallbackContent 清理fallback摘要内容，移除URL、市场数据、时间戳等（用于service包）
func cleanFallbackContent(text string) string {
	if text == "" {
		return ""
	}

	// 移除所有URL（http://, https://, t.co/, x.com/等）- 更全面的模式
	urlRegex := regexp.MustCompile(`(https?://[^\s\)]+|t\.co/[^\s\)]+|x\.com/[^\s\)]+|www\.[^\s\)]+|telegram\.me/[^\s\)]+)`)
	text = urlRegex.ReplaceAllString(text, "")

	// 移除市场数据模式（$XXX MarketCap: $数字, $符号等）- 更全面的匹配
	marketCapRegex := regexp.MustCompile(`\$[A-Z0-9]+\s*(MarketCap|市值)[:：\s]*\$?\d+[KMkmB]?`)
	text = marketCapRegex.ReplaceAllString(text, "")

	// 移除所有代币符号和市场数据（$MONPRO, $MON, $KITE等，包括后面跟数字的）
	dollarSymbolRegex := regexp.MustCompile(`\$[A-Z0-9]{2,15}\s*(MarketCap|市值|:)?`)
	text = dollarSymbolRegex.ReplaceAllString(text, "")

	// 移除单独的市场数据行（如 "$MONPRO MarketCap: $10M" 或 "$MON MarketCap: $6600M"）
	marketDataLineRegex := regexp.MustCompile(`\$[A-Z0-9]+\s+MarketCap:\s*\$?\d+[KMkmB]?`)
	text = marketDataLineRegex.ReplaceAllString(text, "")

	// 移除时间戳格式（2025-11-04 01:57:52等）
	timeRegex := regexp.MustCompile(`\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}`)
	text = timeRegex.ReplaceAllString(text, "")

	// 移除日期格式（November 2025, 3 November等）
	dateRegex := regexp.MustCompile(`\d{1,2}\s+(January|February|March|April|May|June|July|August|September|October|November|December)\s+\d{4}`)
	text = dateRegex.ReplaceAllString(text, "")

	// 移除分隔线
	separatorRegex := regexp.MustCompile(`[—\-]{4,}`)
	text = separatorRegex.ReplaceAllString(text, "")

	// 移除括号内的市场数据提示（如 "(Auto match could be wrong, 自动匹配可能不准确)"）
	// 但保留代币名称（如 "(MON)", "(KITE)" 等）
	autoMatchRegex := regexp.MustCompile(`\([^)]*(Auto match|自动匹配)[^)]*\)`)
	text = autoMatchRegex.ReplaceAllString(text, "")

	// 移除括号内只包含英文描述性内容的（如 "(Auto match could be wrong)"）
	// 但保留简短的代币符号（2-8个字符，如 "(MON)", "(KITE)"）
	text = regexp.MustCompile(`\([^)]+\)`).ReplaceAllStringFunc(text, func(match string) string {
		inner := match[1 : len(match)-1] // 移除括号
		// 如果是Auto match相关，移除
		if strings.Contains(inner, "Auto match") || strings.Contains(inner, "自动匹配") ||
			strings.Contains(inner, "could be wrong") || strings.Contains(inner, "可能不准确") {
			return ""
		}
		// 如果是简短的代币符号（2-8个字符，只包含字母数字），保留
		if len(inner) >= 2 && len(inner) <= 8 && regexp.MustCompile(`^[A-Z0-9]+$`).MatchString(inner) {
			return match // 保留代币符号如 (MON), (KITE)
		}
		// 如果包含空格或特殊字符且很长，可能是描述性内容，移除
		if len(inner) > 15 && (strings.Contains(inner, " ") || strings.Contains(inner, ",")) {
			return ""
		}
		return match // 其他情况保留
	})

	// 移除 "Auto match could be wrong" 等提示文本（包括中英文）- 更全面的匹配
	text = strings.ReplaceAll(text, "Auto match could be wrong", "")
	text = strings.ReplaceAll(text, "自动匹配可能不准确", "")
	text = regexp.MustCompile(`\(Auto match[^)]*\)`).ReplaceAllString(text, "")
	text = regexp.MustCompile(`\([^)]*Auto match[^)]*\)`).ReplaceAllString(text, "")
	text = regexp.MustCompile(`\([^)]*自动匹配[^)]*\)`).ReplaceAllString(text, "")

	// 移除所有只包含市场数据的行（如 "$MONPRO MarketCap: $10M"）
	lines := strings.Split(text, "\n")
	var cleanedLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// 跳过只包含市场数据的行
		if regexp.MustCompile(`^\$[A-Z0-9]+\s*(MarketCap|市值)[:：\s]*\$?\d+[KMkmB]?$`).MatchString(trimmed) {
			continue
		}
		// 跳过只包含代币符号的行
		if regexp.MustCompile(`^\$[A-Z0-9]{2,15}\s*$`).MatchString(trimmed) {
			continue
		}
		// 跳过只包含Auto match提示的行
		if strings.Contains(trimmed, "Auto match") || strings.Contains(trimmed, "自动匹配") {
			continue
		}
		if trimmed != "" {
			cleanedLines = append(cleanedLines, trimmed)
		}
	}
	text = strings.Join(cleanedLines, " ")

	// 移除 "source:" 及其后面的URL
	sourceRegex := regexp.MustCompile(`source:\s*[^\s]+`)
	text = sourceRegex.ReplaceAllString(text, "")

	// 如果文本主要是英文且包含"COINBASE LISTING:"，尝试只保留中文部分
	if strings.Contains(text, "COINBASE LISTING:") && !containsChinese(text) {
		// 这种情况在extractHeadlineFromTitle中应该已经处理了，这里做兜底
		return ""
	}

	// 移除多余的标点符号（多个连续标点）
	punctuationRegex := regexp.MustCompile(`[。，、；：]{2,}`)
	text = punctuationRegex.ReplaceAllString(text, "，")

	// 清理多余空格和换行
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.Join(strings.Fields(text), " ")

	return strings.TrimSpace(text)
}

func classifyImpact(text string) (impact, sentiment string) {
	positiveKeywords := []string{"利好", "上涨", "上升", "上涨", "增加", "突破", "bull", "surge", "up", "growth", "positive"}
	negativeKeywords := []string{"利空", "下跌", "下降", "暴跌", "bear", "down", "drop", "decrease", "negative"}
	neutral := "中性"

	text = strings.ToLower(text)

	score := 0
	for _, keyword := range positiveKeywords {
		if keyword == "" {
			continue
		}
		if strings.Contains(text, strings.ToLower(keyword)) {
			score++
		}
	}
	for _, keyword := range negativeKeywords {
		if keyword == "" {
			continue
		}
		if strings.Contains(text, strings.ToLower(keyword)) {
			score--
		}
	}

	switch {
	case score > 0:
		return "利好", "positive"
	case score < 0:
		return "利空", "negative"
	default:
		return neutral, "neutral"
	}
}

// SetOutlookAnalyzer 设置宏观基本面分析器并启动定时生成
func (s *Service) SetOutlookAnalyzer(analyzer *OutlookAnalyzer) {
	if analyzer == nil {
		return
	}
	s.outlookMu.Lock()
	s.outlookAnalyzer = analyzer
	s.outlookMu.Unlock()

	if err := s.loadOutlookFromDisk(); err != nil {
		log.Printf("%s: 加载本地 Outlook 缓存失败: %v", s.loggerPrefix, err)
	}
}

// RunOutlookLoop 启动定时 Outlook 生成循环（应在独立 goroutine 中调用）
func (s *Service) RunOutlookLoop(ctx context.Context) {
	s.outlookMu.RLock()
	analyzer := s.outlookAnalyzer
	s.outlookMu.RUnlock()
	if analyzer == nil {
		return
	}

	interval := analyzer.Interval()
	log.Printf("%s: Outlook 分析器启动，间隔: %s", s.loggerPrefix, interval)

	// 首次运行前等待一段时间，让新闻源至少拉到第一批数据
	initialDelay := 90 * time.Second
	if s.outlook != nil {
		initialDelay = 10 * time.Second
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(initialDelay):
	}

	s.generateOutlook(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.generateOutlook(ctx)
		}
	}
}

func (s *Service) generateOutlook(ctx context.Context) {
	s.outlookMu.RLock()
	analyzer := s.outlookAnalyzer
	s.outlookMu.RUnlock()
	if analyzer == nil {
		return
	}

	digests := s.GetDigests()
	if len(digests) == 0 {
		log.Printf("%s: 无新闻摘要，跳过 Outlook 生成", s.loggerPrefix)
		return
	}

	outlook, err := analyzer.Analyze(ctx, digests)
	if err != nil {
		log.Printf("%s: Outlook 生成失败: %v", s.loggerPrefix, err)
		return
	}

	s.outlookMu.Lock()
	s.outlook = outlook
	s.outlookMu.Unlock()

	s.persistOutlook(outlook)
	log.Printf("%s: Outlook 已更新 | 倾向: %s(%+d) | 风险: %s | 方向: %s",
		s.loggerPrefix, outlook.OverallBias, outlook.BiasScore,
		outlook.RiskLevel, outlook.Recommendations.PreferredDirection)
}

// GetOutlook 返回最新的宏观基本面报告（线程安全）
func (s *Service) GetOutlook() *MacroOutlook {
	s.outlookMu.RLock()
	defer s.outlookMu.RUnlock()
	return s.outlook
}

func (s *Service) persistOutlook(outlook *MacroOutlook) {
	data, err := json.MarshalIndent(outlook, "", "  ")
	if err != nil {
		log.Printf("%s: 序列化 Outlook 失败: %v", s.loggerPrefix, err)
		return
	}
	path := filepath.Join(s.storageDir, "outlook.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("%s: 写入 Outlook 文件失败: %v", s.loggerPrefix, err)
	}
}

func (s *Service) loadOutlookFromDisk() error {
	path := filepath.Join(s.storageDir, "outlook.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	var outlook MacroOutlook
	if err := json.Unmarshal(data, &outlook); err != nil {
		return err
	}

	if time.Now().Before(outlook.ValidUntil) {
		s.outlookMu.Lock()
		s.outlook = &outlook
		s.outlookMu.Unlock()
		log.Printf("%s: 已加载本地 Outlook 缓存 (生成于 %s)", s.loggerPrefix, outlook.GeneratedAt.Format("15:04:05"))
	}

	return nil
}

// ProviderNames 返回当前活跃 provider 名称列表
func (s *Service) ProviderNames() []string {
	names := make([]string, 0, len(s.providers))
	for _, p := range s.providers {
		names = append(names, p.Name())
	}
	return names
}

// Stats 返回基础统计数据
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-s.opts.MaxAge)
	freshCount := 0
	for _, item := range s.items {
		if item.PublishedAt.After(cutoff) || item.ReceivedAt.After(cutoff) {
			freshCount++
		}
	}

	return map[string]interface{}{
		"total_items":      len(s.items),
		"total_digests":    len(s.digests),
		"fresh_item_count": freshCount,
		"max_age_minutes":  math.Round(s.opts.MaxAge.Minutes()),
		"started_at":       s.startedAt.Format(time.RFC3339),
	}
}

func chooseNonEmpty(values ...string) string {
	for _, v := range values {
		candidate := sanitize(v)
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

// extractHeadlineFromTitle 从RSS Title中提取真正的标题（优先提取中文行）
func extractHeadlineFromTitle(fullTitle string) string {
	if fullTitle == "" {
		return ""
	}

	// 先移除HTML实体（但保留<br/>用于分割）
	fullTitle = strings.ReplaceAll(fullTitle, "&lt;", "<")
	fullTitle = strings.ReplaceAll(fullTitle, "&gt;", ">")
	fullTitle = strings.ReplaceAll(fullTitle, "&amp;", "&")

	// 按 <br/> 和换行符分割所有行
	var allLines []string
	if strings.Contains(fullTitle, "<br/>") {
		for _, part := range strings.Split(fullTitle, "<br/>") {
			allLines = append(allLines, strings.Split(part, "\n")...)
		}
	} else {
		allLines = strings.Split(fullTitle, "\n")
	}

	// 优先查找包含中文的行作为标题
	var headline string
	for _, line := range allLines {
		cleaned := strings.TrimSpace(removeHTMLTags(line))
		if cleaned == "" {
			continue
		}
		// 如果包含中文，优先使用
		if containsChinese(cleaned) {
			headline = cleaned
			break
		}
		// 如果没有找到中文行，使用第一行非空行
		if headline == "" {
			headline = cleaned
		}
	}

	// 如果都失败，返回原始内容的前100个字符作为标题（清理HTML后）
	if headline == "" {
		cleaned := removeHTMLTags(fullTitle)
		if len([]rune(cleaned)) > 100 {
			headline = string([]rune(cleaned)[:100]) + "..."
		} else {
			headline = cleaned
		}
	}

	// 对标题进行深度清理：移除URL、市场数据、时间戳等，但保留代币名称
	headline = cleanFallbackContent(headline)

	return headline
}

// extractChineseFromTitle 从标题中提取中文部分
func extractChineseFromTitle(fullTitle string) string {
	// 查找包含中文的部分（使用 \p{Han} 匹配所有中文字符）
	chinesePattern := regexp.MustCompile(`\p{Han}+[^\p{Han}]*\p{Han}+`)
	if matches := chinesePattern.FindAllString(fullTitle, -1); len(matches) > 0 {
		// 合并所有中文片段
		return strings.Join(matches, " ")
	}
	return ""
}

// containsChinese 检查字符串是否包含中文字符
func containsChinese(text string) bool {
	// 使用 \p{Han} 匹配所有中文字符（CJK统一汉字）
	chinesePattern := regexp.MustCompile(`\p{Han}`)
	return chinesePattern.MatchString(text)
}

// extractPureChinese 提取纯中文内容，保留数字和必要的标点，移除英文、URL等
func extractPureChinese(text string) string {
	if text == "" {
		return ""
	}

	// 先进行基础清理（移除URL、市场数据等）
	text = cleanFallbackContent(text)

	// 如果没有中文，返回空字符串
	if !containsChinese(text) {
		return ""
	}

	// 提取中文内容，保留：
	// 1. 中文字符 (\p{Han})
	// 2. 中文标点（，。、；：？！）
	// 3. 数字（用于日期等：2025年11月3日）
	// 4. 必要的空格和换行
	chinesePattern := regexp.MustCompile(`[\p{Han}，。、；：？！0-9年月日\s]+`)
	matches := chinesePattern.FindAllString(text, -1)
	if len(matches) > 0 {
		result := strings.Join(matches, "")
		// 清理多余空格，但保留中文字符之间的单个空格
		result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")
		// 移除数字之间不必要的空格（如"2025 年 11 月" -> "2025年11月"）
		result = regexp.MustCompile(`(\d+)\s+([年月日])`).ReplaceAllString(result, "$1$2")
		result = regexp.MustCompile(`([年月日])\s+(\d+)`).ReplaceAllString(result, "$1$2")
		return strings.TrimSpace(result)
	}

	// 如果正则匹配失败，尝试手动提取中文片段（保留数字）
	var chineseParts []string
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		// 检查是否是中文字符或数字
		isChinese := r >= 0x4e00 && r <= 0x9fff
		isDigit := r >= '0' && r <= '9'
		isChinesePunct := r == '，' || r == '。' || r == '、' || r == '；' ||
			r == '：' || r == '？' || r == '！' || r == '年' ||
			r == '月' || r == '日'

		if isChinese || isDigit || isChinesePunct {
			// 提取连续的中文字符、数字和中文标点
			var seq []rune
			for j := i; j < len(runes); j++ {
				r2 := runes[j]
				isChinese2 := r2 >= 0x4e00 && r2 <= 0x9fff
				isDigit2 := r2 >= '0' && r2 <= '9'
				isChinesePunct2 := r2 == '，' || r2 == '。' || r2 == '、' ||
					r2 == '；' || r2 == '：' || r2 == '？' ||
					r2 == '！' || r2 == '年' || r2 == '月' ||
					r2 == '日' || r2 == ' ' || r2 == '\t'

				if isChinese2 || isDigit2 || isChinesePunct2 {
					seq = append(seq, r2)
				} else {
					// 如果遇到英文或其他字符，检查是否应该继续
					// 如果前面有中文，且当前字符是常见的中文间分隔符，可以继续
					if r2 == '(' || r2 == ')' || r2 == '（' || r2 == '）' {
						break // 括号通常包含英文，跳过
					}
					break
				}
			}
			if len(seq) > 0 {
				chineseParts = append(chineseParts, string(seq))
				i += len(seq) - 1
			}
		}
	}

	if len(chineseParts) > 0 {
		result := strings.Join(chineseParts, " ")
		result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")
		result = regexp.MustCompile(`(\d+)\s+([年月日])`).ReplaceAllString(result, "$1$2")
		result = regexp.MustCompile(`([年月日])\s+(\d+)`).ReplaceAllString(result, "$1$2")
		return strings.TrimSpace(result)
	}

	return ""
}
