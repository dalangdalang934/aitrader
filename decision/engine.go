package decision

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"aitrade/learning"
	"aitrade/market"
	"aitrade/mcp"
	"aitrade/pool"
	"strings"
	"text/template"
	"time"
)

//go:embed templates/system_prompt.tmpl
var systemPromptTemplate string

var systemPromptTpl = template.Must(template.New("systemPrompt").Parse(systemPromptTemplate))

type systemPromptTemplateData struct {
	AccountEquity         string
	AltRangeLow           string
	AltRangeHigh          string
	BtcRangeLow           string
	BtcRangeHigh          string
	AltcoinLeverage       int
	BtcEthLeverage        int
	SampleBTCLeverage     int
	SampleAltcoinLeverage int
	SamplePositionUSD     string
}

// PositionInfo 持仓信息
type PositionInfo struct {
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"` // "long" or "short"
	EntryPrice       float64 `json:"entry_price"`
	MarkPrice        float64 `json:"mark_price"`
	Quantity         float64 `json:"quantity"`
	Leverage         int     `json:"leverage"`
	UnrealizedPnL    float64 `json:"unrealized_pnl"`
	UnrealizedPnLPct float64 `json:"unrealized_pnl_pct"`
	LiquidationPrice float64 `json:"liquidation_price"`
	MarginUsed       float64 `json:"margin_used"`
	UpdateTime       int64   `json:"update_time"`           // 持仓更新时间戳（毫秒）
	PositionID       string  `json:"position_id,omitempty"` // 关联的仓位ID
}

// AccountInfo 账户信息
type AccountInfo struct {
	TotalEquity      float64 `json:"total_equity"`      // 账户净值
	AvailableBalance float64 `json:"available_balance"` // 可用余额
	TotalPnL         float64 `json:"total_pnl"`         // 总盈亏
	TotalPnLPct      float64 `json:"total_pnl_pct"`     // 总盈亏百分比
	MarginUsed       float64 `json:"margin_used"`       // 已用保证金
	MarginUsedPct    float64 `json:"margin_used_pct"`   // 保证金使用率
	PositionCount    int     `json:"position_count"`    // 持仓数量
}

// CandidateCoin 候选币种（来自币种池）
type CandidateCoin struct {
	Symbol  string   `json:"symbol"`
	Sources []string `json:"sources"` // 来源: "ai500" 和/或 "oi_top"
}

// OITopData 持仓量增长Top数据（用于AI决策参考）
type OITopData struct {
	Rank              int     // OI Top排名
	OIDeltaPercent    float64 // 持仓量变化百分比（1小时）
	OIDeltaValue      float64 // 持仓量变化价值
	PriceDeltaPercent float64 // 价格变化百分比
	NetLong           float64 // 净多仓
	NetShort          float64 // 净空仓
}

// Context 交易上下文（传递给AI的完整信息）
type Context struct {
	CurrentTime     string                  `json:"current_time"`
	RuntimeMinutes  int                     `json:"runtime_minutes"`
	CallCount       int                     `json:"call_count"`
	Account         AccountInfo             `json:"account"`
	Positions       []PositionInfo          `json:"positions"`
	CandidateCoins  []CandidateCoin         `json:"candidate_coins"`
	MarketDataMap   map[string]*market.Data `json:"-"` // 不序列化，但内部使用
	OITopDataMap    map[string]*OITopData   `json:"-"` // OI Top数据映射
	Performance     interface{}             `json:"-"` // 历史表现分析（logger.PerformanceAnalysis）
	NewsDigests     interface{}             `json:"-"` // 新闻摘要列表（[]news.Digest）
	MacroOutlook    interface{}             `json:"-"` // 宏观基本面研判（*news.MacroOutlook）
	BTCETHLeverage  int                     `json:"-"` // BTC/ETH杠杆倍数（从配置读取）
	AltcoinLeverage int                     `json:"-"` // 山寨币杠杆倍数（从配置读取）
	LearningState   *learning.State         `json:"-"`
	HistoryWarmup   bool                    `json:"-"`
}

// LearningStateCopy 返回学习状态的深拷贝，用于跨 goroutine 安全使用
func (ctx *Context) LearningStateCopy() (*learning.State, bool) {
	if ctx == nil || ctx.LearningState == nil {
		return nil, false
	}
	return ctx.LearningState.Copy(), true
}

// Decision AI的交易决策
type Decision struct {
	Symbol          string  `json:"symbol"`
	Action          string  `json:"action"` // "open_long", "open_short", "close_long", "close_short", "hold", "wait"
	Leverage        int     `json:"leverage,omitempty"`
	PositionSizeUSD float64 `json:"position_size_usd,omitempty"`
	StopLoss        float64 `json:"stop_loss,omitempty"`
	TakeProfit      float64 `json:"take_profit,omitempty"`
	Confidence      int     `json:"confidence,omitempty"` // 信心度 (0-100)
	RiskUSD         float64 `json:"risk_usd,omitempty"`   // 最大美元风险
	Reasoning       string  `json:"reasoning"`
	PositionID      string  `json:"position_id,omitempty"` // 关联的仓位ID
}

// FullDecision AI的完整决策（包含思维链）
type FullDecision struct {
	UserPrompt string     `json:"user_prompt"` // 发送给AI的输入prompt
	CoTTrace   string     `json:"cot_trace"`   // 思维链分析（AI输出）
	Decisions  []Decision `json:"decisions"`   // 具体决策列表
	Timestamp  time.Time  `json:"timestamp"`
}

// GetFullDecision 获取AI的完整交易决策（批量分析所有币种和持仓）
func GetFullDecision(ctx *Context, mcpClient *mcp.Client) (*FullDecision, error) {
	// 1. 为所有币种获取市场数据
	if err := fetchMarketDataForContext(ctx); err != nil {
		return nil, fmt.Errorf("获取市场数据失败: %w", err)
	}

	// 2. 构建 System Prompt（固定规则）和 User Prompt（动态数据）
	systemPrompt := buildSystemPrompt(ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage)
	userPrompt := buildUserPrompt(ctx)

	// 3. 调用AI API（使用 system + user prompt）
	aiResponse, err := mcpClient.CallWithMessages(systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("调用AI API失败: %w", err)
	}

	// 4. 解析AI响应
	decision, err := parseFullDecisionResponse(aiResponse, ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage)
	if err != nil {
		return nil, fmt.Errorf("解析AI响应失败: %w", err)
	}

	decision.Timestamp = time.Now()
	decision.UserPrompt = userPrompt // 保存输入prompt
	return decision, nil
}

// fetchMarketDataForContext 为上下文中的所有币种获取市场数据和OI数据
func fetchMarketDataForContext(ctx *Context) error {
	ctx.MarketDataMap = make(map[string]*market.Data)
	ctx.OITopDataMap = make(map[string]*OITopData)

	// 收集所有需要获取数据的币种
	symbolSet := make(map[string]bool)

	// 1. 优先获取持仓币种的数据（这是必须的）
	for _, pos := range ctx.Positions {
		symbolSet[pos.Symbol] = true
	}

	// 2. 候选币种数量根据账户状态动态调整
	maxCandidates := calculateMaxCandidates(ctx)
	for i, coin := range ctx.CandidateCoins {
		if i >= maxCandidates {
			break
		}
		symbolSet[coin.Symbol] = true
	}

	// 并发获取市场数据
	// 持仓币种集合（用于判断是否跳过OI检查）
	positionSymbols := make(map[string]bool)
	for _, pos := range ctx.Positions {
		positionSymbols[pos.Symbol] = true
	}

	for symbol := range symbolSet {
		data, err := market.Get(symbol)
		if err != nil {
			// 单个币种失败不影响整体，只记录错误
			continue
		}

		// ⚠️ 流动性过滤：持仓价值低于15M USD的币种不做（多空都不做）
		// 持仓价值 = 持仓量 × 当前价格
		// 但现有持仓必须保留（需要决策是否平仓）
		isExistingPosition := positionSymbols[symbol]
		if !isExistingPosition && data.OpenInterest != nil && data.CurrentPrice > 0 {
			// 计算持仓价值（USD）= 持仓量 × 当前价格
			oiValue := data.OpenInterest.Latest * data.CurrentPrice
			oiValueInMillions := oiValue / 1_000_000 // 转换为百万美元单位
			if oiValueInMillions < 15 {
				log.Printf("⚠️  %s 持仓价值过低(%.2fM USD < 15M)，跳过此币种 [持仓量:%.0f × 价格:%.4f]",
					symbol, oiValueInMillions, data.OpenInterest.Latest, data.CurrentPrice)
				continue
			}
		}

		ctx.MarketDataMap[symbol] = data
	}

	// 加载OI Top数据（不影响主流程）
	oiPositions, err := pool.GetOITopPositions()
	if err == nil {
		for _, pos := range oiPositions {
			// 标准化符号匹配
			symbol := pos.Symbol
			ctx.OITopDataMap[symbol] = &OITopData{
				Rank:              pos.Rank,
				OIDeltaPercent:    pos.OIDeltaPercent,
				OIDeltaValue:      pos.OIDeltaValue,
				PriceDeltaPercent: pos.PriceDeltaPercent,
				NetLong:           pos.NetLong,
				NetShort:          pos.NetShort,
			}
		}
	}

	return nil
}

// calculateMaxCandidates 根据账户状态计算需要分析的候选币种数量
func calculateMaxCandidates(ctx *Context) int {
	// 直接返回候选池的全部币种数量
	// 因为候选池已经在 auto_trader.go 中筛选过了
	// 固定分析前20个评分最高的币种（来自AI500）
	return len(ctx.CandidateCoins)
}

// buildSystemPrompt 构建 System Prompt（固定规则，可缓存）
func buildSystemPrompt(accountEquity float64, btcEthLeverage, altcoinLeverage int) string {
	sampleBTCLeverage := btcEthLeverage
	if sampleBTCLeverage > 5 {
		sampleBTCLeverage = 5
	}
	if sampleBTCLeverage < 2 {
		sampleBTCLeverage = 2
	}
	sampleAltcoinLeverage := altcoinLeverage
	if sampleAltcoinLeverage > 3 {
		sampleAltcoinLeverage = 3
	}
	if sampleAltcoinLeverage < 1 {
		sampleAltcoinLeverage = 1
	}

	data := systemPromptTemplateData{
		AccountEquity:         fmt.Sprintf("%.2f", accountEquity),
		AltRangeLow:           fmt.Sprintf("%.0f", accountEquity*0.8),
		AltRangeHigh:          fmt.Sprintf("%.0f", accountEquity*1.5),
		BtcRangeLow:           fmt.Sprintf("%.0f", accountEquity*5),
		BtcRangeHigh:          fmt.Sprintf("%.0f", accountEquity*10),
		AltcoinLeverage:       altcoinLeverage,
		BtcEthLeverage:        btcEthLeverage,
		SampleBTCLeverage:     sampleBTCLeverage,
		SampleAltcoinLeverage: sampleAltcoinLeverage,
		SamplePositionUSD:     fmt.Sprintf("%.0f", accountEquity*5),
	}

	var sb strings.Builder
	if err := systemPromptTpl.Execute(&sb, data); err != nil {
		log.Printf("⚠️ 渲染系统 Prompt 模板失败: %v", err)
		return ""
	}

	return sb.String()
}

// buildUserPrompt 构建 User Prompt（动态数据）
func buildUserPrompt(ctx *Context) string {
	var sb strings.Builder

	// 系统状态
	sb.WriteString(fmt.Sprintf("**时间**: %s | **周期**: #%d | **运行**: %d分钟\n\n",
		ctx.CurrentTime, ctx.CallCount, ctx.RuntimeMinutes))

	// BTC 市场
	if btcData, hasBTC := ctx.MarketDataMap["BTCUSDT"]; hasBTC {
		sb.WriteString(fmt.Sprintf("**BTC**: %.2f (1h: %+.2f%%, 4h: %+.2f%%) | MACD: %.4f | RSI: %.2f\n\n",
			btcData.CurrentPrice, btcData.PriceChange1h, btcData.PriceChange4h,
			btcData.CurrentMACD, btcData.CurrentRSI7))
	}

	// 账户
	balancePct := 0.0
	if ctx.Account.TotalEquity > 0 {
		balancePct = (ctx.Account.AvailableBalance / ctx.Account.TotalEquity) * 100
	}
	sb.WriteString(fmt.Sprintf("**账户**: 净值%.2f | 余额%.2f (%.1f%%) | 盈亏%+.2f%% | 保证金%.1f%% | 持仓%d个\n\n",
		ctx.Account.TotalEquity,
		ctx.Account.AvailableBalance,
		balancePct,
		ctx.Account.TotalPnLPct,
		ctx.Account.MarginUsedPct,
		ctx.Account.PositionCount))

	profitPct := ctx.Account.TotalPnLPct
	switch {
	case profitPct >= 15:
		sb.WriteString(fmt.Sprintf("🚨 **超级锁盈模式**：净值已领先 %.1f%%。所有新仓位≤原计划的25%%，如无多重共振信号，请优先 `hold/wait`，让收益冷却。任何做空前必须再次确认大周期与宏观环境。\n\n", profitPct))
	case profitPct >= 8:
		sb.WriteString(fmt.Sprintf("🔒 **锁盈提醒**：累计收益 %.1f%%。进入防守状态：新增仓位≤40%%，最多2仓，同方向不连击，优先考虑减仓或观望，避免情绪化反击。\n\n", profitPct))
	case profitPct <= -6:
		sb.WriteString(fmt.Sprintf("🧊 **止血提示**：当前回撤 %.1f%%。暂停追单，回顾失误，只有当风险回报重新≥1:3 且信心≥85 时才允许重启交易。\n\n", profitPct))
	}

	if ctx.HistoryWarmup {
		sb.WriteString("⚠️ 周期日志与学习数据刚被清空，当前绩效统计仅供参考，请专注于高质量信号重新积累样本。\n\n")
	}

	// 持仓（完整市场数据）
	if len(ctx.Positions) > 0 {
		sb.WriteString("## 当前持仓\n")
		for i, pos := range ctx.Positions {
			// 计算持仓时长
			holdingDuration := ""
			if pos.UpdateTime > 0 {
				durationMs := time.Now().UnixMilli() - pos.UpdateTime
				durationMin := durationMs / (1000 * 60) // 转换为分钟
				if durationMin < 60 {
					holdingDuration = fmt.Sprintf(" | 持仓时长%d分钟", durationMin)
				} else {
					durationHour := durationMin / 60
					durationMinRemainder := durationMin % 60
					holdingDuration = fmt.Sprintf(" | 持仓时长%d小时%d分钟", durationHour, durationMinRemainder)
				}
			}

			sb.WriteString(fmt.Sprintf("%d. %s %s | 入场价%.4f 当前价%.4f | 盈亏%+.2f%% | 杠杆%dx | 保证金%.0f | 强平价%.4f%s\n\n",
				i+1, pos.Symbol, strings.ToUpper(pos.Side),
				pos.EntryPrice, pos.MarkPrice, pos.UnrealizedPnLPct,
				pos.Leverage, pos.MarginUsed, pos.LiquidationPrice, holdingDuration))

			// 使用FormatMarketData输出完整市场数据
			if marketData, ok := ctx.MarketDataMap[pos.Symbol]; ok {
				sb.WriteString(market.Format(marketData))
				sb.WriteString("\n")
			}
		}
	} else {
		sb.WriteString("**当前持仓**: 无\n\n")
	}

	// 候选币种（完整市场数据）
	sb.WriteString(fmt.Sprintf("## 候选币种 (%d个)\n\n", len(ctx.MarketDataMap)))
	displayedCount := 0
	for _, coin := range ctx.CandidateCoins {
		marketData, hasData := ctx.MarketDataMap[coin.Symbol]
		if !hasData {
			continue
		}
		displayedCount++

		sourceTags := ""
		if len(coin.Sources) > 1 {
			sourceTags = " (AI500+OI_Top双重信号)"
		} else if len(coin.Sources) == 1 && coin.Sources[0] == "oi_top" {
			sourceTags = " (OI_Top持仓增长)"
		}

		// 使用FormatMarketData输出完整市场数据
		sb.WriteString(fmt.Sprintf("### %d. %s%s\n\n", displayedCount, coin.Symbol, sourceTags))
		sb.WriteString(market.Format(marketData))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// 历史表现摘要（精简版，但确保夏普比率始终显示）
	var sharpeRatio float64
	var hasFullData bool

	if ctx.Performance != nil {
		type PerformanceData struct {
			SharpeRatio   float64                `json:"sharpe_ratio"`
			TotalTrades   int                    `json:"total_trades"`
			WinningTrades int                    `json:"winning_trades"`
			LosingTrades  int                    `json:"losing_trades"`
			WinRate       float64                `json:"win_rate"`
			ProfitFactor  float64                `json:"profit_factor"`
			AvgWin        float64                `json:"avg_win"`
			AvgLoss       float64                `json:"avg_loss"`
			SymbolStats   map[string]interface{} `json:"symbol_stats"` // 改为interface{}以兼容*SymbolPerformance
		}
		var perfData PerformanceData
		if jsonData, err := json.Marshal(ctx.Performance); err == nil {
			if err := json.Unmarshal(jsonData, &perfData); err == nil {
				sharpeRatio = perfData.SharpeRatio
				hasFullData = perfData.TotalTrades > 0

				if hasFullData {
					sb.WriteString("## 📊 历史表现摘要\n\n")
					sb.WriteString(fmt.Sprintf("**夏普比率**: %.2f | **总交易**: %d | **胜/负**: %dW/%dL\n", perfData.SharpeRatio, perfData.TotalTrades, perfData.WinningTrades, perfData.LosingTrades))
					if perfData.WinRate > 0 {
						sb.WriteString(fmt.Sprintf("**胜率**: %.1f%% | **盈亏比**: %.2f\n", perfData.WinRate*100, perfData.ProfitFactor))
					}
					if perfData.AvgWin > 0 || perfData.AvgLoss < 0 {
						sb.WriteString(fmt.Sprintf("**平均盈利**: $%.2f | **平均亏损**: $%.2f\n", perfData.AvgWin, perfData.AvgLoss))
					}
					sb.WriteString("\n")

					// 显示各币种表现统计（帮助AI识别优势/劣势币种）
					if perfData.SymbolStats != nil && len(perfData.SymbolStats) > 0 {
						sb.WriteString("### 📈 各币种表现统计\n\n")
						// 按总盈亏排序，显示前10个币种
						type SymbolStat struct {
							Symbol        string
							TotalTrades   int
							WinningTrades int
							LosingTrades  int
							WinRate       float64
							TotalPnL      float64
							AvgPnL        float64
						}
						var symbolList []SymbolStat
						for symbol, stats := range perfData.SymbolStats {
							// SymbolStats 可能是 *SymbolPerformance 或 map[string]interface{}
							// 通过JSON序列化/反序列化统一处理
							statsJSON, _ := json.Marshal(stats)
							var statsMap map[string]interface{}
							if err := json.Unmarshal(statsJSON, &statsMap); err == nil {
								totalTrades, _ := statsMap["total_trades"].(float64)
								winningTrades, _ := statsMap["winning_trades"].(float64)
								losingTrades, _ := statsMap["losing_trades"].(float64)
								winRate, _ := statsMap["win_rate"].(float64)
								totalPnL, _ := statsMap["total_pn_l"].(float64)
								avgPnL, _ := statsMap["avg_pn_l"].(float64)
								symbolList = append(symbolList, SymbolStat{
									Symbol:        symbol,
									TotalTrades:   int(totalTrades),
									WinningTrades: int(winningTrades),
									LosingTrades:  int(losingTrades),
									WinRate:       winRate,
									TotalPnL:      totalPnL,
									AvgPnL:        avgPnL,
								})
							}
						}
						// 按总盈亏排序
						for i := 0; i < len(symbolList)-1; i++ {
							for j := i + 1; j < len(symbolList); j++ {
								if symbolList[i].TotalPnL < symbolList[j].TotalPnL {
									symbolList[i], symbolList[j] = symbolList[j], symbolList[i]
								}
							}
						}
						// 显示前10个
						displayCount := 10
						if len(symbolList) < displayCount {
							displayCount = len(symbolList)
						}
						for i := 0; i < displayCount; i++ {
							stat := symbolList[i]
							pnlSign := "+"
							if stat.TotalPnL < 0 {
								pnlSign = ""
							}
							sb.WriteString(fmt.Sprintf("- **%s**: %d笔 (%dW/%dL, 胜率%.1f%%) | 总盈亏: %s$%.2f | 平均: $%.2f\n",
								stat.Symbol, stat.TotalTrades, stat.WinningTrades, stat.LosingTrades,
								stat.WinRate, pnlSign, stat.TotalPnL, stat.AvgPnL))
						}
						sb.WriteString("\n")
					}
				} else {
					sb.WriteString(fmt.Sprintf("## 📊 夏普比率: %.2f\n\n", sharpeRatio))
				}
			}
		}
	}

	// 学习状态摘要
	if ls := ctx.LearningState; ls != nil {
		age := time.Since(ls.GeneratedAt)
		ageText := "刚刚"
		if age < 0 {
			age = 0
		}
		if age.Hours() >= 1 {
			hours := int(age.Hours())
			minutes := int(age.Minutes()) % 60
			if minutes > 0 {
				ageText = fmt.Sprintf("%d小时%d分钟", hours, minutes)
			} else {
				ageText = fmt.Sprintf("%d小时", hours)
			}
		} else if age.Minutes() >= 1 {
			ageText = fmt.Sprintf("%d分钟", int(age.Minutes()))
		}

		sb.WriteString("## 🧠 学习状态指令\n\n")
		sb.WriteString(fmt.Sprintf("生成时间：%s（约%s前）\n\n", ls.GeneratedAt.Format("2006-01-02 15:04"), ageText))
		sb.WriteString(fmt.Sprintf("- 概览：%s\n", ls.Summary))
		sb.WriteString(fmt.Sprintf("- 风险指引：信心≥%d | 最多持仓%d | 仓位系数%.2f | 冷静期%d分钟\n",
			ls.Risk.ConfidenceThreshold, ls.Risk.MaxConcurrentPositions, ls.Risk.PositionSizeMultiplier, ls.Risk.CooldownMinutes))
		if ls.Execution.MinHoldMinutes > 0 || ls.Execution.MaxTradesPerHour > 0 {
			sb.WriteString(fmt.Sprintf("- 执行约束：持仓不少于%d分钟 | 频率≤%.2f 笔/小时\n",
				ls.Execution.MinHoldMinutes, ls.Execution.MaxTradesPerHour))
		}

		var focusList, avoidList, watchList []string
		for _, directive := range ls.Symbols {
			item := fmt.Sprintf("%s (%s)", directive.Symbol, directive.Reason)
			switch directive.Action {
			case "focus":
				focusList = append(focusList, item)
			case "avoid":
				avoidList = append(avoidList, item)
			default:
				watchList = append(watchList, item)
			}
		}
		if len(focusList) > 0 {
			sb.WriteString(fmt.Sprintf("- 重点关注：%s\n", strings.Join(focusList, "；")))
		}
		if len(avoidList) > 0 {
			sb.WriteString(fmt.Sprintf("- 禁止开仓：%s\n", strings.Join(avoidList, "；")))
		}
		if len(watchList) > 0 {
			sb.WriteString(fmt.Sprintf("- 观察名单：%s\n", strings.Join(watchList, "；")))
		}

		if len(ls.Execution.Comments) > 0 {
			sb.WriteString("- 执行提示：\n")
			for _, comment := range ls.Execution.Comments {
				sb.WriteString(fmt.Sprintf("  • %s\n", comment))
			}
		}
		if len(ls.Insights) > 0 {
			sb.WriteString("- 反思重点：\n")
			for _, insight := range ls.Insights {
				sb.WriteString(fmt.Sprintf("  • %s\n", insight))
			}
		}
		sb.WriteString("\n")
	}

	// 如果只有部分数据或数据获取失败，至少显示夏普比率
	if ctx.Performance != nil && !hasFullData {
		if sharpeRatio != 0 {
			sb.WriteString(fmt.Sprintf("## 📊 夏普比率: %.2f\n\n", sharpeRatio))
		}
	}

	// 显示最近交易明细（帮助AI深度学习和反思）
	if ctx.Performance != nil {
		type PerformanceDataFull struct {
			RecentTrades []map[string]interface{} `json:"recent_trades"`
		}
		var perfDataFull PerformanceDataFull
		if jsonData, err := json.Marshal(ctx.Performance); err == nil {
			if err := json.Unmarshal(jsonData, &perfDataFull); err == nil {
				if len(perfDataFull.RecentTrades) > 0 {
					sb.WriteString("## 📋 最近交易明细（最近20笔，用于深度学习和反思）\n\n")
					sb.WriteString("**格式**: 币种 | 方向 | 开仓价→平仓价 | 数量 | 盈亏 | 持仓时长 | 盈亏%%\n\n")

					// 显示最近20笔交易
					displayCount := 20
					if len(perfDataFull.RecentTrades) < displayCount {
						displayCount = len(perfDataFull.RecentTrades)
					}

					for i := 0; i < displayCount; i++ {
						trade := perfDataFull.RecentTrades[i]
						symbol, _ := trade["symbol"].(string)
						side, _ := trade["side"].(string)
						openPrice, _ := trade["open_price"].(float64)
						closePrice, _ := trade["close_price"].(float64)
						quantity, _ := trade["quantity"].(float64)
						pnl, _ := trade["pn_l"].(float64)
						duration, _ := trade["duration"].(string)
						pnlPct, _ := trade["pn_l_pct"].(float64)

						pnlSign := "+"
						pnlColor := "🟢"
						if pnl < 0 {
							pnlSign = ""
							pnlColor = "🔴"
						}

						sideText := "做多"
						if side == "short" {
							sideText = "做空"
						}

						sb.WriteString(fmt.Sprintf("%d. %s **%s** %s | %.4f→%.4f | %.4f | %s%.2f | %s | %.2f%%\n",
							i+1, pnlColor, symbol, sideText, openPrice, closePrice, quantity,
							pnlSign, pnl, duration, pnlPct))
					}
					sb.WriteString("\n")
					sb.WriteString("**反思要点**:\n")
					sb.WriteString("- 哪些交易盈利/亏损？原因是什么？\n")
					sb.WriteString("- 持仓时间是否合理？（<30分钟可能过早平仓）\n")
					sb.WriteString("- 哪些币种表现好/差？应该重点关注哪些币种？\n")
					sb.WriteString("- 是否存在重复的错误模式？（如频繁交易、过早平仓等）\n\n")
				}
			}
		}
	}

	// 宏观基本面研判（优先于逐条新闻展示）
	if ctx.MacroOutlook != nil {
		type OutlookData struct {
			GeneratedAt time.Time `json:"generated_at"`
			ValidUntil  time.Time `json:"valid_until"`
			OverallBias string    `json:"overall_bias"`
			BiasScore   int       `json:"bias_score"`
			RiskLevel   string    `json:"risk_level"`
			Summary     string    `json:"summary"`
			KeyFactors  []struct {
				Category    string `json:"category"`
				Title       string `json:"title"`
				Impact      string `json:"impact"`
				Importance  int    `json:"importance"`
				Description string `json:"description"`
			} `json:"key_factors"`
			Recommendations struct {
				PreferredDirection string   `json:"preferred_direction"`
				PositionSizeAdj    float64  `json:"position_size_adj"`
				MaxLeverageAdj     float64  `json:"max_leverage_adj"`
				AvoidSymbols       []string `json:"avoid_symbols"`
				FocusSymbols       []string `json:"focus_symbols"`
				Reasoning          string   `json:"reasoning"`
			} `json:"recommendations"`
		}
		var outlook OutlookData
		if jsonData, err := json.Marshal(ctx.MacroOutlook); err == nil {
			if err := json.Unmarshal(jsonData, &outlook); err == nil && outlook.Summary != "" {
				sb.WriteString("## 📰 宏观基本面研判\n\n")

				biasEmoji := "⚪"
				switch outlook.OverallBias {
				case "bullish":
					biasEmoji = "🟢"
				case "bearish":
					biasEmoji = "🔴"
				}
				sb.WriteString(fmt.Sprintf("**整体倾向**: %s %s (偏向分数: %+d)\n", biasEmoji, outlook.OverallBias, outlook.BiasScore))

				riskEmoji := "🟢"
				switch outlook.RiskLevel {
				case "medium":
					riskEmoji = "🟡"
				case "high":
					riskEmoji = "🟠"
				case "extreme":
					riskEmoji = "🔴"
				}
				sb.WriteString(fmt.Sprintf("**风险等级**: %s %s\n", riskEmoji, outlook.RiskLevel))
				sb.WriteString(fmt.Sprintf("**基本面总结**: %s\n\n", outlook.Summary))

				if len(outlook.KeyFactors) > 0 {
					sb.WriteString("**关键因素**:\n")
					for _, f := range outlook.KeyFactors {
						impactEmoji := "⚪"
						switch f.Impact {
						case "bullish":
							impactEmoji = "🟢"
						case "bearish":
							impactEmoji = "🔴"
						}
						sb.WriteString(fmt.Sprintf("- %s [%s] %s (重要性:%d/5): %s\n", impactEmoji, f.Category, f.Title, f.Importance, f.Description))
					}
					sb.WriteString("\n")
				}

				rec := outlook.Recommendations
				sb.WriteString("**策略建议**:\n")
				sb.WriteString(fmt.Sprintf("- 方向: %s | 仓位系数: %.2f | 杠杆系数: %.2f\n", rec.PreferredDirection, rec.PositionSizeAdj, rec.MaxLeverageAdj))
				if len(rec.FocusSymbols) > 0 {
					sb.WriteString(fmt.Sprintf("- 关注币种: %s\n", strings.Join(rec.FocusSymbols, ", ")))
				}
				if len(rec.AvoidSymbols) > 0 {
					sb.WriteString(fmt.Sprintf("- 回避币种: %s\n", strings.Join(rec.AvoidSymbols, ", ")))
				}
				if rec.Reasoning != "" {
					sb.WriteString(fmt.Sprintf("- 理由: %s\n", rec.Reasoning))
				}

				sb.WriteString(fmt.Sprintf("\n**报告时间**: %s | **有效至**: %s\n\n",
					outlook.GeneratedAt.Format("2006-01-02 15:04"),
					outlook.ValidUntil.Format("2006-01-02 15:04")))
			}
		}
	}

	// 新闻摘要（最近2小时，最多5条）
	if ctx.NewsDigests != nil {
		type NewsDigest struct {
			Headline    string    `json:"headline"`
			Summary     string    `json:"summary"`
			Impact      string    `json:"impact"`
			Sentiment   string    `json:"sentiment"`
			Confidence  int       `json:"confidence"`
			PublishedAt time.Time `json:"published_at"`
		}
		var digests []NewsDigest
		if jsonData, err := json.Marshal(ctx.NewsDigests); err == nil {
			if err := json.Unmarshal(jsonData, &digests); err == nil {
				// 只显示最近5条
				maxNews := 5
				if len(digests) > maxNews {
					digests = digests[:maxNews]
				}
				if len(digests) > 0 {
					sb.WriteString("## 📰 市场新闻摘要（最近2小时）\n\n")
					for i, d := range digests {
						sb.WriteString(fmt.Sprintf("**%d. %s**\n", i+1, d.Headline))
						sb.WriteString(fmt.Sprintf("   - 影响: %s | 情绪: %s | 置信度: %d%%\n", d.Impact, d.Sentiment, d.Confidence))
						if d.Summary != "" {
							sb.WriteString(fmt.Sprintf("   - 摘要: %s\n", d.Summary))
						}
						sb.WriteString("\n")
					}
				}
			}
		}
	}

	sb.WriteString("---\n\n")
	sb.WriteString("现在请分析并输出决策（思维链 + JSON）\n")

	return sb.String()
}

// parseFullDecisionResponse 解析AI的完整决策响应
func parseFullDecisionResponse(aiResponse string, accountEquity float64, btcEthLeverage, altcoinLeverage int) (*FullDecision, error) {
	// 1. 提取思维链
	cotTrace := extractCoTTrace(aiResponse)

	// 2. 提取JSON决策列表
	decisions, err := extractDecisions(aiResponse)
	if err != nil {
		return &FullDecision{
			CoTTrace:  cotTrace,
			Decisions: []Decision{},
		}, fmt.Errorf("提取决策失败: %w\n\n=== AI思维链分析 ===\n%s", err, cotTrace)
	}

	// 3. 验证决策
	if err := validateDecisions(decisions, accountEquity, btcEthLeverage, altcoinLeverage); err != nil {
		return &FullDecision{
			CoTTrace:  cotTrace,
			Decisions: decisions,
		}, fmt.Errorf("决策验证失败: %w\n\n=== AI思维链分析 ===\n%s", err, cotTrace)
	}

	return &FullDecision{
		CoTTrace:  cotTrace,
		Decisions: decisions,
	}, nil
}

// extractCoTTrace 提取思维链分析
func extractCoTTrace(response string) string {
	// 查找JSON数组的开始位置
	jsonStart := strings.Index(response, "[")

	if jsonStart > 0 {
		// 思维链是JSON数组之前的内容
		return strings.TrimSpace(response[:jsonStart])
	}

	// 如果找不到JSON，整个响应都是思维链
	return strings.TrimSpace(response)
}

// extractDecisions 提取JSON决策列表
func extractDecisions(response string) ([]Decision, error) {
	// 直接查找JSON数组 - 找第一个完整的JSON数组
	arrayStart := strings.Index(response, "[")
	if arrayStart == -1 {
		return nil, fmt.Errorf("无法找到JSON数组起始")
	}

	// 从 [ 开始，匹配括号找到对应的 ]
	arrayEnd := findMatchingBracket(response, arrayStart)
	if arrayEnd == -1 {
		return nil, fmt.Errorf("无法找到JSON数组结束")
	}

	jsonContent := strings.TrimSpace(response[arrayStart : arrayEnd+1])

	// 🔧 修复常见的JSON格式错误：缺少引号的字段值
	// 匹配: "reasoning": 内容"}  或  "reasoning": 内容}  (没有引号)
	// 修复为: "reasoning": "内容"}
	// 使用简单的字符串扫描而不是正则表达式
	jsonContent = fixMissingQuotes(jsonContent)

	// 解析JSON
	var decisions []Decision
	if err := json.Unmarshal([]byte(jsonContent), &decisions); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %w\nJSON内容: %s", err, jsonContent)
	}

	return decisions, nil
}

// fixMissingQuotes 替换中文引号为英文引号（避免输入法自动转换）
func fixMissingQuotes(jsonStr string) string {
	jsonStr = strings.ReplaceAll(jsonStr, "\u201c", "\"") // "
	jsonStr = strings.ReplaceAll(jsonStr, "\u201d", "\"") // "
	jsonStr = strings.ReplaceAll(jsonStr, "\u2018", "'")  // '
	jsonStr = strings.ReplaceAll(jsonStr, "\u2019", "'")  // '
	return jsonStr
}

// validateDecisions 验证所有决策（需要账户信息和杠杆配置）
func validateDecisions(decisions []Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int) error {
	for i, decision := range decisions {
		if err := validateDecision(&decision, accountEquity, btcEthLeverage, altcoinLeverage); err != nil {
			return fmt.Errorf("决策 #%d 验证失败: %w", i+1, err)
		}
	}
	return nil
}

// findMatchingBracket 查找匹配的右括号
func findMatchingBracket(s string, start int) int {
	if start >= len(s) || s[start] != '[' {
		return -1
	}

	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return -1
}

// validateDecision 验证单个决策的有效性
func validateDecision(d *Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int) error {
	// 验证action
	validActions := map[string]bool{
		"open_long":   true,
		"open_short":  true,
		"close_long":  true,
		"close_short": true,
		"hold":        true,
		"wait":        true,
	}

	if !validActions[d.Action] {
		return fmt.Errorf("无效的action: %s", d.Action)
	}

	// 开仓操作必须提供完整参数
	if d.Action == "open_long" || d.Action == "open_short" {
		// 根据币种使用配置的杠杆上限
		maxLeverage := altcoinLeverage          // 山寨币使用配置的杠杆
		maxPositionValue := accountEquity * 1.5 // 山寨币最多1.5倍账户净值
		if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
			maxLeverage = btcEthLeverage          // BTC和ETH使用配置的杠杆
			maxPositionValue = accountEquity * 10 // BTC/ETH最多10倍账户净值
		}

		if d.Leverage <= 0 || d.Leverage > maxLeverage {
			return fmt.Errorf("杠杆必须在1-%d之间（%s，当前配置上限%d倍）: %d", maxLeverage, d.Symbol, maxLeverage, d.Leverage)
		}
		if d.PositionSizeUSD <= 0 {
			return fmt.Errorf("仓位大小必须大于0: %.2f", d.PositionSizeUSD)
		}
		// 验证仓位价值上限（加1%容差以避免浮点数精度问题）
		tolerance := maxPositionValue * 0.01 // 1%容差
		if d.PositionSizeUSD > maxPositionValue+tolerance {
			if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
				return fmt.Errorf("BTC/ETH单币种仓位价值不能超过%.0f USDT（10倍账户净值），实际: %.0f", maxPositionValue, d.PositionSizeUSD)
			} else {
				return fmt.Errorf("山寨币单币种仓位价值不能超过%.0f USDT（1.5倍账户净值），实际: %.0f", maxPositionValue, d.PositionSizeUSD)
			}
		}
		if d.StopLoss <= 0 || d.TakeProfit <= 0 {
			return fmt.Errorf("止损和止盈必须大于0")
		}

		// 验证止损止盈的合理性
		if d.Action == "open_long" {
			if d.StopLoss >= d.TakeProfit {
				return fmt.Errorf("做多时止损价必须小于止盈价")
			}
		} else {
			if d.StopLoss <= d.TakeProfit {
				return fmt.Errorf("做空时止损价必须大于止盈价")
			}
		}

		// 验证风险回报比（必须≥1:3）
		// 计算入场价（假设当前市价）
		var entryPrice float64
		if d.Action == "open_long" {
			// 做多：入场价在止损和止盈之间
			entryPrice = d.StopLoss + (d.TakeProfit-d.StopLoss)*0.2 // 假设在20%位置入场
		} else {
			// 做空：入场价在止损和止盈之间
			entryPrice = d.StopLoss - (d.StopLoss-d.TakeProfit)*0.2 // 假设在20%位置入场
		}

		var riskPercent, rewardPercent, riskRewardRatio float64
		if d.Action == "open_long" {
			riskPercent = (entryPrice - d.StopLoss) / entryPrice * 100
			rewardPercent = (d.TakeProfit - entryPrice) / entryPrice * 100
			if riskPercent > 0 {
				riskRewardRatio = rewardPercent / riskPercent
			}
		} else {
			riskPercent = (d.StopLoss - entryPrice) / entryPrice * 100
			rewardPercent = (entryPrice - d.TakeProfit) / entryPrice * 100
			if riskPercent > 0 {
				riskRewardRatio = rewardPercent / riskPercent
			}
		}

		// 硬约束：风险回报比必须≥3.0
		if riskRewardRatio < 3.0 {
			return fmt.Errorf("风险回报比过低(%.2f:1)，必须≥3.0:1 [风险:%.2f%% 收益:%.2f%%] [止损:%.2f 止盈:%.2f]",
				riskRewardRatio, riskPercent, rewardPercent, d.StopLoss, d.TakeProfit)
		}
	}

	return nil
}
