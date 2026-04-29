package news

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"aitrade/mcp"
)

// MacroOutlook 宏观基本面研判报告
type MacroOutlook struct {
	GeneratedAt     time.Time           `json:"generated_at"`
	ValidUntil      time.Time           `json:"valid_until"`
	OverallBias     string              `json:"overall_bias"`    // "bullish" / "bearish" / "neutral"
	BiasScore       int                 `json:"bias_score"`      // -100 ~ +100
	RiskLevel       string              `json:"risk_level"`      // "low" / "medium" / "high" / "extreme"
	Summary         string              `json:"summary"`         // 一段话总结当前全球金融基本面
	KeyFactors      []MacroFactor       `json:"key_factors"`     // 影响基本面的关键因素
	Recommendations MacroRecommendation `json:"recommendations"` // 对交易策略的建议
	DigestIDs       []string            `json:"digest_ids"`      // 基于哪些新闻摘要生成
}

// MacroFactor 影响基本面的关键因素
type MacroFactor struct {
	Category    string `json:"category"`    // "fed_policy" / "geopolitics" / "crypto_regulation" / "stock_market" / "commodities"
	Title       string `json:"title"`       // 因素标题
	Impact      string `json:"impact"`      // "bullish" / "bearish" / "neutral"
	Importance  int    `json:"importance"`  // 1-5
	Description string `json:"description"` // 简要描述
}

// MacroRecommendation 对交易策略的建议
type MacroRecommendation struct {
	PreferredDirection string   `json:"preferred_direction"` // "long" / "short" / "neutral"
	PositionSizeAdj    float64  `json:"position_size_adj"`   // 仓位调整系数 0.5~1.5
	MaxLeverageAdj     float64  `json:"max_leverage_adj"`    // 杠杆调整系数 0.5~1.0
	AvoidSymbols       []string `json:"avoid_symbols"`       // 建议回避的币种
	FocusSymbols       []string `json:"focus_symbols"`       // 建议关注的币种
	Reasoning          string   `json:"reasoning"`           // 建议理由
}

// OutlookAnalyzer 宏观基本面分析器
type OutlookAnalyzer struct {
	client *mcp.Client
}

// NewOutlookAnalyzer 创建基本面分析器
func NewOutlookAnalyzer(client *mcp.Client) *OutlookAnalyzer {
	if client == nil {
		return nil
	}
	return &OutlookAnalyzer{client: client}
}

// Interval 返回宏观基本面报告的刷新周期。
func (a *OutlookAnalyzer) Interval() time.Duration {
	if a == nil || a.client == nil {
		return 30 * time.Minute
	}
	return 30 * time.Minute
}

// Analyze 分析新闻生成宏观基本面报告
func (a *OutlookAnalyzer) Analyze(ctx context.Context, digests []Digest) (*MacroOutlook, error) {
	if a == nil || a.client == nil {
		return nil, fmt.Errorf("outlook analyzer: client is not configured")
	}

	if len(digests) == 0 {
		return a.defaultOutlook(), nil
	}

	systemPrompt := `你是资深的全球宏观金融分析师，专注于加密货币市场。
你的任务是综合分析近期所有新闻，输出一份结构化的全球金融基本面研判报告。

分析维度（按重要性排序）：
1. 美联储/央行政策（利率、QE/QT、通胀预期）
2. 地缘政治（战争、制裁、贸易摩擦）
3. 加密货币监管（SEC、各国立法、交易所合规）
4. 传统金融市场（美股、美元指数、债券）
5. 加密行业事件（交易所、项目、链上数据）
6. 大宗商品（黄金、原油）

输出要求：
- 必须使用 JSON 格式
- overall_bias: "bullish"（看多）/ "bearish"（看空）/ "neutral"（中性）
- bias_score: -100（极度看空）到 +100（极度看多）
- risk_level: "low" / "medium" / "high" / "extreme"
- summary: 中文，不超过150字，概括当前全球金融基本面对加密市场的影响
- key_factors: 最多5个关键因素，每个包含 category、title、impact、importance(1-5)、description
- recommendations: 
  - preferred_direction: "long" / "short" / "neutral"
  - position_size_adj: 0.5~1.5 的仓位调整系数
  - max_leverage_adj: 0.5~1.0 的杠杆调整系数
  - avoid_symbols: 建议回避的币种列表（可为空）
  - focus_symbols: 建议关注的币种列表（可为空）
  - reasoning: 中文，不超过100字

JSON 示例：
{
  "overall_bias": "bullish",
  "bias_score": 35,
  "risk_level": "medium",
  "summary": "美联储释放鸽派信号，市场降息预期升温，风险资产受益。加密监管趋于明朗化，机构入场意愿增强。",
  "key_factors": [
    {"category": "fed_policy", "title": "美联储鸽派转向", "impact": "bullish", "importance": 5, "description": "鲍威尔暗示降息周期临近"}
  ],
  "recommendations": {
    "preferred_direction": "long",
    "position_size_adj": 1.2,
    "max_leverage_adj": 1.0,
    "avoid_symbols": [],
    "focus_symbols": ["BTC", "ETH"],
    "reasoning": "宏观环境转暖，可适度加大多头敞口"
  }
}`

	var newsContent strings.Builder
	newsContent.WriteString("以下是近期新闻摘要，请综合分析并输出基本面研判报告：\n\n")

	for i, d := range digests {
		if i >= 20 {
			break
		}
		newsContent.WriteString(fmt.Sprintf("【%d】%s\n", i+1, d.Headline))
		if d.Summary != "" {
			newsContent.WriteString(fmt.Sprintf("   摘要: %s\n", d.Summary))
		}
		newsContent.WriteString(fmt.Sprintf("   影响: %s | 情绪: %s | 置信度: %d%%\n", d.Impact, d.Sentiment, d.Confidence))
		newsContent.WriteString(fmt.Sprintf("   时间: %s\n\n", d.PublishedAt.Format("2006-01-02 15:04")))
	}

	response, err := a.client.CallWithMessages(systemPrompt, newsContent.String())
	if err != nil {
		log.Printf("📰 基本面分析: AI 调用失败: %v", err)
		return a.defaultOutlook(), nil
	}

	outlook, err := parseOutlookResponse(response)
	if err != nil {
		log.Printf("📰 基本面分析: 解析响应失败: %v", err)
		return a.defaultOutlook(), nil
	}

	outlook.GeneratedAt = time.Now()
	outlook.ValidUntil = time.Now().Add(30 * time.Minute)

	digestIDs := make([]string, 0, len(digests))
	for i, d := range digests {
		if i >= 20 {
			break
		}
		digestIDs = append(digestIDs, d.ID)
	}
	outlook.DigestIDs = digestIDs

	return outlook, nil
}

func (a *OutlookAnalyzer) defaultOutlook() *MacroOutlook {
	return &MacroOutlook{
		GeneratedAt: time.Now(),
		ValidUntil:  time.Now().Add(30 * time.Minute),
		OverallBias: "neutral",
		BiasScore:   0,
		RiskLevel:   "medium",
		Summary:     "暂无足够新闻数据进行基本面分析，建议以技术面为主进行交易决策。",
		KeyFactors:  []MacroFactor{},
		Recommendations: MacroRecommendation{
			PreferredDirection: "neutral",
			PositionSizeAdj:    1.0,
			MaxLeverageAdj:     1.0,
			AvoidSymbols:       []string{},
			FocusSymbols:       []string{},
			Reasoning:          "数据不足，维持常规交易策略",
		},
		DigestIDs: []string{},
	}
}

func parseOutlookResponse(raw string) (*MacroOutlook, error) {
	clean := strings.TrimSpace(raw)
	if strings.HasPrefix(clean, "```") {
		clean = stripCodeFence(clean)
	}

	start := strings.Index(clean, "{")
	end := strings.LastIndex(clean, "}")
	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("响应缺少JSON: %s", clean[:min(len(clean), 200)])
	}

	jsonPart := normalizeJSON(clean[start : end+1])

	var outlook MacroOutlook
	if err := json.Unmarshal([]byte(jsonPart), &outlook); err != nil {
		return nil, fmt.Errorf("解析基本面报告失败: %w | 原始: %s", err, jsonPart[:min(len(jsonPart), 500)])
	}

	outlook.OverallBias = normalizeBias(outlook.OverallBias)
	outlook.RiskLevel = normalizeRiskLevel(outlook.RiskLevel)

	if outlook.BiasScore < -100 {
		outlook.BiasScore = -100
	}
	if outlook.BiasScore > 100 {
		outlook.BiasScore = 100
	}

	if outlook.Recommendations.PositionSizeAdj < 0.3 {
		outlook.Recommendations.PositionSizeAdj = 0.3
	}
	if outlook.Recommendations.PositionSizeAdj > 2.0 {
		outlook.Recommendations.PositionSizeAdj = 2.0
	}
	if outlook.Recommendations.MaxLeverageAdj < 0.3 {
		outlook.Recommendations.MaxLeverageAdj = 0.3
	}
	if outlook.Recommendations.MaxLeverageAdj > 1.0 {
		outlook.Recommendations.MaxLeverageAdj = 1.0
	}

	outlook.Recommendations.PreferredDirection = normalizeDirection(outlook.Recommendations.PreferredDirection)

	return &outlook, nil
}

func normalizeBias(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	switch lower {
	case "bullish", "bull", "看多", "多", "long":
		return "bullish"
	case "bearish", "bear", "看空", "空", "short":
		return "bearish"
	default:
		return "neutral"
	}
}

func normalizeRiskLevel(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	switch lower {
	case "low", "低":
		return "low"
	case "medium", "中", "moderate":
		return "medium"
	case "high", "高":
		return "high"
	case "extreme", "极高", "very high":
		return "extreme"
	default:
		return "medium"
	}
}

func normalizeDirection(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	switch lower {
	case "long", "多", "做多", "bullish":
		return "long"
	case "short", "空", "做空", "bearish":
		return "short"
	default:
		return "neutral"
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
