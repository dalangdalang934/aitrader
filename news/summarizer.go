package news

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"aitrade/mcp"
)

// Summarizer 定义新闻摘要器接口
type Summarizer interface {
	Summarize(ctx context.Context, item NewsItem) (*Digest, error)
}

// MCPSummarizer 使用 mcp.Client 委托大模型进行摘要
type MCPSummarizer struct {
	client *mcp.Client
}

// NewMCPSummarizer 创建基于 MCP 的新闻摘要器
func NewMCPSummarizer(client *mcp.Client) Summarizer {
	if client == nil {
		return nil
	}
	return &MCPSummarizer{client: client}
}

func (m *MCPSummarizer) Summarize(_ context.Context, item NewsItem) (*Digest, error) {
	if m == nil || m.client == nil {
		return nil, errors.New("mcp summarizer: client is not configured")
	}

	systemPrompt := "你是资深的加密市场情报分析师。针对每条新闻快讯，给量化交易系统输出结构化判断。务必使用JSON返回，字段包含 headline、summary、impact、sentiment、confidence、reasoning、source、url。\n\n重要要求：\n1. headline 和 summary 必须是纯中文，简洁明了\n2. summary 只保留核心信息，不要包含URL、链接、市场数据（如MarketCap、$符号）、时间戳、英文原文\n3. 过滤掉所有标点符号中的冗余内容（如括号内的英文、URL等）\n4. summary 中文不超过90字，reasoning 不超过60字\n5. impact 只能是 '利好'、'利空' 或 '中性'，sentiment 只能是 'positive'、'negative' 或 'neutral'\n\n判断原则（请自主应用，不要硬编码）：\n- 根据交易所的全球影响力、用户规模、流动性来判断上线消息的重要性\n- 正式上线通常比测试/Alpha上线影响更大\n- USD交易对通常比区域性货币交易对（如KRW）影响更广\n- 大型主流交易所的上线消息通常比小型区域性交易所更重要\n- 综合考虑新闻的具体表述（如\"不代表会正式上市\"等限制性条件）\n- 根据新闻内容的实际影响程度动态调整置信度（confidence）"

	var published string
	if !item.PublishedAt.IsZero() {
		published = item.PublishedAt.Format("2006-01-02 15:04:05 MST")
	}

	var builder strings.Builder
	builder.WriteString("【标题】\n")
	builder.WriteString(item.Headline)
	builder.WriteString("\n\n")
	builder.WriteString("【正文】\n")
	builder.WriteString(item.Content)
	builder.WriteString("\n\n")
	builder.WriteString("【来源】\n")
	builder.WriteString(item.Source)
	builder.WriteString("\n\n")
	builder.WriteString("【标签】\n")
	builder.WriteString(strings.Join(item.Tags, ", "))
	builder.WriteString("\n\n")
	builder.WriteString("【链接】\n")
	builder.WriteString(item.URL)
	builder.WriteString("\n\n")
	builder.WriteString("【发布时间】\n")
	builder.WriteString(published)

	response, err := m.client.CallWithMessages(systemPrompt, builder.String())
	if err != nil {
		return nil, err
	}

	payload, err := parseMCPDigest(response)
	if err != nil {
		return nil, err
	}

	// 清理AI返回的摘要，移除HTML标签、URL、市场数据等，但保留英文和中文
	summary := removeHTMLTagsInSummarizer(payload.Summary)
	summary = cleanSummaryContent(summary)
	summary = strings.TrimSpace(summary)
	
	if summary == "" {
		// 如果AI没有返回摘要，从Content中提取摘要
		contentText := removeHTMLTagsInSummarizer(item.Content)
		contentText = cleanSummaryContent(contentText)
		contentText = strings.TrimSpace(contentText)
		if len([]rune(contentText)) > 240 {
			summary = string([]rune(contentText)[:240]) + "..."
		} else {
			summary = contentText
		}
		if summary == "" {
			summary = item.Headline
		}
	}
	
	// 限制摘要长度在90字以内（按AI要求）
	if len([]rune(summary)) > 90 {
		summary = string([]rune(summary)[:90]) + "..."
	}

	// 清理AI返回的推理，移除HTML标签、URL等
	reasoning := removeHTMLTagsInSummarizer(payload.Reasoning)
	reasoning = cleanSummaryContent(reasoning)
	reasoning = strings.TrimSpace(reasoning)
	
	// 限制推理长度在60字以内（按AI要求）
	if len([]rune(reasoning)) > 60 {
		reasoning = string([]rune(reasoning)[:60]) + "..."
	}

	// 清理AI返回的headline，保留英文和中文
	aiHeadline := removeHTMLTagsInSummarizer(payload.Headline)
	aiHeadline = cleanSummaryContent(aiHeadline)
	aiHeadline = strings.TrimSpace(aiHeadline)
	
	// 优先使用清理后的AI headline，如果没有或为空，使用原始headline（也已清理过）
	finalHeadline := chooseNonEmpty(aiHeadline, item.Headline)
	if finalHeadline == "" {
		finalHeadline = item.Headline
	}
	
	digest := &Digest{
		ID:          fmt.Sprintf("digest-%s", item.ID),
		Headline:    finalHeadline,  // 使用清理后的headline
		Summary:     summary,
		Impact:      normalizeImpact(payload.Impact),
		Sentiment:   normalizeSentiment(payload.Sentiment),
		Confidence:  clampConfidence(payload.Confidence),
		Reasoning:   reasoning,
		Source:      chooseNonEmpty(payload.Source, item.Source),
		URL:         chooseNonEmpty(payload.URL, item.URL),
		PublishedAt: item.PublishedAt,
		CreatedAt:   time.Now(),
		ItemIDs:     []string{item.ID},
	}

	return digest, nil
}

type mcpDigestPayload struct {
	Headline   string          `json:"headline"`
	Summary    string          `json:"summary"`
	Impact     string          `json:"impact"`
	Sentiment  string          `json:"sentiment"`
	Confidence confidenceValue `json:"confidence"`
	Reasoning  string          `json:"reasoning"`
	Source     string          `json:"source"`
	URL        string          `json:"url"`
}

type confidenceValue float64

func (c *confidenceValue) UnmarshalJSON(data []byte) error {
	clean := strings.TrimSpace(string(data))
	if clean == "" || clean == "null" {
		*c = 0
		return nil
	}

	var numeric float64
	if err := json.Unmarshal([]byte(clean), &numeric); err == nil {
		*c = confidenceValue(numeric)
		return nil
	}

	var str string
	if err := json.Unmarshal([]byte(clean), &str); err == nil {
		trimmed := strings.TrimSpace(str)
		trimmed = strings.TrimSuffix(trimmed, "%")
		if trimmed == "" {
			*c = 0
			return nil
		}
		if parsed, err := strconv.ParseFloat(trimmed, 64); err == nil {
			*c = confidenceValue(parsed)
			return nil
		}
	}

	return fmt.Errorf("invalid confidence value: %s", clean)
}

func parseMCPDigest(raw string) (mcpDigestPayload, error) {
	clean := strings.TrimSpace(raw)
	if strings.HasPrefix(clean, "```") {
		clean = stripCodeFence(clean)
	}
	start := strings.Index(clean, "{")
	end := strings.LastIndex(clean, "}")
	if start == -1 || end == -1 || end <= start {
		return mcpDigestPayload{}, fmt.Errorf("AI响应缺少JSON: %s", clean)
	}

	jsonPart := normalizeJSON(clean[start : end+1])
	var payload mcpDigestPayload
	if err := json.Unmarshal([]byte(jsonPart), &payload); err != nil {
		return mcpDigestPayload{}, fmt.Errorf("解析AI摘要失败: %w | 原始: %s", err, jsonPart)
	}
	return payload, nil
}

func stripCodeFence(input string) string {
	trimmed := strings.TrimSpace(input)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	return strings.TrimSpace(trimmed)
}

func normalizeJSON(input string) string {
	replacer := strings.NewReplacer(
		"\u201c", "\"",
		"\u201d", "\"",
		"\u2018", "'",
		"\u2019", "'",
		"\u201c", "\"",
		"\u201d", "\"",
		"：", ":",
		"，", ",",
	)
	return replacer.Replace(input)
}

// removeHTMLTagsInSummarizer 移除HTML标签（用于summarizer包）
func removeHTMLTagsInSummarizer(text string) string {
	if text == "" {
		return ""
	}
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
	text = strings.ReplaceAll(text, "&apos;", "'")
	
	return strings.TrimSpace(text)
}

// cleanSummaryContent 清理摘要内容，移除URL、市场数据、时间戳等（与service包的cleanFallbackContent逻辑一致）
func cleanSummaryContent(text string) string {
	if text == "" {
		return ""
	}
	
	// 移除所有URL（http://, https://, t.co/, x.com/等）- 更全面的模式
	urlRegex := regexp.MustCompile(`(https?://[^\s\)]+|t\.co/[^\s\)]+|x\.com/[^\s\)]+|www\.[^\s\)]+|telegram\.me/[^\s\)]+)`)
	text = urlRegex.ReplaceAllString(text, "")
	
	// 移除市场数据模式（$XXX MarketCap: $数字, $符号等）- 更全面的匹配
	marketCapRegex := regexp.MustCompile(`\$[A-Z0-9]+\s*(MarketCap|市值)[:：\s]*\$?\d+[KMkmB]?`)
	text = marketCapRegex.ReplaceAllString(text, "")
	
	// 移除所有代币符号和市场数据（$MONPRO, $MON, $KITE等）
	dollarSymbolRegex := regexp.MustCompile(`\$[A-Z0-9]{2,15}\s*(MarketCap|市值|:)?`)
	text = dollarSymbolRegex.ReplaceAllString(text, "")
	
	// 移除单独的市场数据行
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
	
	// 移除所有只包含市场数据的行
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
	
	// 移除多余的标点符号（多个连续标点）
	punctuationRegex := regexp.MustCompile(`[。，、；：]{2,}`)
	text = punctuationRegex.ReplaceAllString(text, "，")
	
	// 清理多余空格和换行
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.Join(strings.Fields(text), " ")
	
	return strings.TrimSpace(text)
}

// extractPureChineseInSummarizer 提取纯中文内容，保留数字和必要的标点（用于summarizer包）
func extractPureChineseInSummarizer(text string) string {
	if text == "" {
		return ""
	}
	
	// 先进行基础清理
	text = cleanSummaryContent(text)
	
	// 如果没有中文，返回空字符串
	chinesePattern := regexp.MustCompile(`\p{Han}`)
	if !chinesePattern.MatchString(text) {
		return ""
	}
	
	// 提取中文内容，保留数字（用于日期等）
	chineseContentPattern := regexp.MustCompile(`[\p{Han}，。、；：？！0-9年月日\s]+`)
	matches := chineseContentPattern.FindAllString(text, -1)
	if len(matches) > 0 {
		result := strings.Join(matches, "")
		// 清理多余空格
		result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")
		// 移除数字和年月日之间的空格
		result = regexp.MustCompile(`(\d+)\s+([年月日])`).ReplaceAllString(result, "$1$2")
		result = regexp.MustCompile(`([年月日])\s+(\d+)`).ReplaceAllString(result, "$1$2")
		return strings.TrimSpace(result)
	}
	
	// 如果正则匹配失败，尝试手动提取（保留数字）
	var chineseParts []string
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		isChinese := r >= 0x4e00 && r <= 0x9fff
		isDigit := r >= '0' && r <= '9'
		isChinesePunct := r == '，' || r == '。' || r == '、' || r == '；' || 
		                  r == '：' || r == '？' || r == '！' || r == '年' || 
		                  r == '月' || r == '日'
		
		if isChinese || isDigit || isChinesePunct {
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

func normalizeImpact(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	switch lower {
	case "bullish", "positive", "看多", "利好", "long", "看涨":
		return "利好"
	case "bearish", "negative", "看空", "利空", "short", "看跌":
		return "利空"
	case "neutral", "中性", "neutrality":
		return "中性"
	default:
		if lower == "" {
			return "中性"
		}
		return sanitize(raw)
	}
}

func normalizeSentiment(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	switch lower {
	case "positive", "bullish", "看多", "利好", "up", "long":
		return "positive"
	case "negative", "bearish", "看空", "利空", "down", "short":
		return "negative"
	default:
		return "neutral"
	}
}

func clampConfidence(raw confidenceValue) int {
	value := float64(raw)
	
	// 如果 AI 返回无效值（NaN/Inf），返回 0，让系统知道这是无效的
	// 但不会强制重置为默认值，而是保留让 normalizeDigest 处理
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}

	// 处理 0-1 范围的百分比（如 0.85 表示 85%）
	if value >= 0 && value <= 1 {
		value *= 100
	}

	// 负数直接设为 0（无效值）
	if value < 0 {
		value = 0
	}

	value = math.Round(value)

	// 超过 100 的，限制为 100（但保留 AI 的判断意图）
	if value > 100 {
		value = 100
	}

	// 完全信任 AI 返回的值，包括 0（可能是 AI 的真实判断）
	return int(value)
}

