package learning

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"aitrade/logger"
)

// StateVersion 用于向前兼容未来的学习状态格式
const StateVersion = 1

// State 代表 AI 学习模块可持久化的结构化指令
type State struct {
	TraderID    string              `json:"trader_id"`
	Version     int                 `json:"version"`
	GeneratedAt time.Time           `json:"generated_at"`
	Summary     string              `json:"summary"`
	Stats       Stats               `json:"stats"`
	Risk        RiskDirectives      `json:"risk"`
	Symbols     []SymbolDirective   `json:"symbols"`
	Execution   ExecutionDirectives `json:"execution"`
	Insights    []string            `json:"insights"`
}

// Stats 是对 PerformanceAnalysis 的精简版摘要
type Stats struct {
	TotalTrades   int     `json:"total_trades"`
	WinningTrades int     `json:"winning_trades"`
	LosingTrades  int     `json:"losing_trades"`
	WinRate       float64 `json:"win_rate"` // 百分比
	ProfitFactor  float64 `json:"profit_factor"`
	SharpeRatio   float64 `json:"sharpe_ratio"`
	AvgWin        float64 `json:"avg_win"`
	AvgLoss       float64 `json:"avg_loss"`
}

// RiskDirectives 控制风险相关参数
type RiskDirectives struct {
	ConfidenceThreshold    int     `json:"confidence_threshold"`
	MaxConcurrentPositions int     `json:"max_concurrent_positions"`
	PositionSizeMultiplier float64 `json:"position_size_multiplier"`
	CooldownMinutes        int     `json:"cooldown_minutes"`
}

// SymbolDirective 给出对特定币种的处理建议
type SymbolDirective struct {
	Symbol   string  `json:"symbol"`
	Action   string  `json:"action"` // focus / avoid / watch
	Reason   string  `json:"reason"`
	Trades   int     `json:"trades"`
	WinRate  float64 `json:"win_rate"`
	TotalPnL float64 `json:"total_pn_l"`
	AvgPnL   float64 `json:"avg_pn_l"`
}

// ExecutionDirectives 约束执行层面的规则
type ExecutionDirectives struct {
	MinHoldMinutes   int      `json:"min_hold_minutes"`
	MaxTradesPerHour float64  `json:"max_trades_per_hour"`
	Comments         []string `json:"comments"`
}

// learningStore 私有接口：仅需 SaveLearning/LoadLearning
type learningStore interface {
	SaveLearning(traderID string, data []byte) error
	LoadLearning(traderID string) ([]byte, error)
}

// Manager 负责读写学习状态（SQLite）
type Manager struct {
	store learningStore
}

// NewManager 创建学习状态管理器
func NewManager(store learningStore) (*Manager, error) {
	if store == nil {
		return nil, fmt.Errorf("store cannot be nil")
	}
	return &Manager{store: store}, nil
}

// Update 根据最新 PerformanceAnalysis 生成并持久化学习状态
func (m *Manager) Update(traderID string, perf *logger.PerformanceAnalysis) (*State, error) {
	if perf == nil {
		return nil, fmt.Errorf("performance cannot be nil")
	}
	state := buildState(traderID, perf)
	buf, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("encode learning state: %w", err)
	}
	if err := m.store.SaveLearning(traderID, buf); err != nil {
		return nil, err
	}
	return state, nil
}

// Load 读取已保存的学习状态，如不存在返回 nil
func (m *Manager) Load(traderID string) (*State, error) {
	data, err := m.store.LoadLearning(traderID)
	if err != nil || data == nil {
		return nil, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decode learning state: %w", err)
	}
	return &state, nil
}

// Copy 深拷贝一个学习状态（用于跨 goroutine 安全使用）
func (s *State) Copy() *State {
	if s == nil {
		return nil
	}
	data, err := json.Marshal(s)
	if err != nil {
		return nil
	}
	var cloned State
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil
	}
	return &cloned
}

func buildState(traderID string, perf *logger.PerformanceAnalysis) *State {
	now := time.Now()
	stats := Stats{
		TotalTrades:   perf.TotalTrades,
		WinningTrades: perf.WinningTrades,
		LosingTrades:  perf.LosingTrades,
		WinRate:       perf.WinRate,
		ProfitFactor:  perf.ProfitFactor,
		SharpeRatio:   perf.SharpeRatio,
		AvgWin:        perf.AvgWin,
		AvgLoss:       perf.AvgLoss,
	}

	symbolDirectives := buildSymbolDirectives(perf)
	risk := buildRiskDirectives(stats, perf)
	execution := buildExecutionDirectives(perf)
	insights := buildInsights(stats, symbolDirectives, execution)

	summary := buildSummary(stats, symbolDirectives, risk)

	return &State{
		TraderID:    traderID,
		Version:     StateVersion,
		GeneratedAt: now,
		Summary:     summary,
		Stats:       stats,
		Risk:        risk,
		Symbols:     symbolDirectives,
		Execution:   execution,
		Insights:    insights,
	}
}

func buildSymbolDirectives(perf *logger.PerformanceAnalysis) []SymbolDirective {
	if len(perf.SymbolStats) == 0 {
		return nil
	}

	var directives []SymbolDirective
	for symbol, stat := range perf.SymbolStats {
		trades := stat.TotalTrades
		if trades == 0 {
			continue
		}

		directive := SymbolDirective{
			Symbol:   symbol,
			Trades:   trades,
			WinRate:  stat.WinRate,
			TotalPnL: stat.TotalPnL,
			AvgPnL:   stat.AvgPnL,
		}

		symbolUpper := strings.ToUpper(symbol)
		var reasons []string

		// "avoid" 条件：样本>=3 且胜率<35% 或持续亏损
		if trades >= 3 && (stat.WinRate < 35 || stat.TotalPnL < -0.8) {
			directive.Action = "avoid"
			reasons = append(reasons, fmt.Sprintf("%d 笔仅胜率 %.1f%%", trades, stat.WinRate))
			if stat.TotalPnL < 0 {
				reasons = append(reasons, fmt.Sprintf("累计亏损 %.2f USDT", stat.TotalPnL))
			}
		}

		// "focus" 条件：样本>=3 且胜率≥60% 且累计盈利
		if directive.Action == "" && trades >= 3 && stat.WinRate >= 60 && stat.TotalPnL > 0.8 {
			directive.Action = "focus"
			reasons = append(reasons, fmt.Sprintf("胜率 %.1f%% | 盈利 %.2f USDT", stat.WinRate, stat.TotalPnL))
		}

		// 其他情况：标记 watch 让 AI 自行决定
		if directive.Action == "" {
			directive.Action = "watch"
			reasons = append(reasons, fmt.Sprintf("样本 %d | 胜率 %.1f%%", trades, stat.WinRate))
		}

		directive.Reason = strings.Join(reasons, "；")
		directive.Symbol = symbolUpper
		directives = append(directives, directive)
	}

	// 将 avoid 排前面，便于展示
	sort.SliceStable(directives, func(i, j int) bool {
		prio := func(action string) int {
			switch action {
			case "avoid":
				return 0
			case "focus":
				return 1
			default:
				return 2
			}
		}
		pi := prio(directives[i].Action)
		pj := prio(directives[j].Action)
		if pi == pj {
			return directives[i].Symbol < directives[j].Symbol
		}
		return pi < pj
	})

	return directives
}

func buildRiskDirectives(stats Stats, perf *logger.PerformanceAnalysis) RiskDirectives {
	risk := RiskDirectives{
		ConfidenceThreshold:    72,
		MaxConcurrentPositions: 5,
		PositionSizeMultiplier: 1.0,
		CooldownMinutes:        0,
	}

	// 基于总体胜率和盈亏比调整
	if stats.TotalTrades >= 10 {
		if stats.SharpeRatio < -0.5 || stats.ProfitFactor < 0.8 {
			risk.ConfidenceThreshold = 83
			risk.MaxConcurrentPositions = 1
			risk.PositionSizeMultiplier = 0.55
			risk.CooldownMinutes = 9 // 相当于至少休息3个周期
		} else if stats.ProfitFactor < 1.0 || stats.WinRate < 45 {
			risk.ConfidenceThreshold = 78
			risk.MaxConcurrentPositions = 3
			risk.PositionSizeMultiplier = 0.75
		} else if stats.SharpeRatio > 0.8 && stats.WinRate > 55 && stats.ProfitFactor > 1.3 {
			risk.PositionSizeMultiplier = 1.15
			risk.MaxConcurrentPositions = 5
			risk.ConfidenceThreshold = 70
		}
	}

	return risk
}

func buildExecutionDirectives(perf *logger.PerformanceAnalysis) ExecutionDirectives {
	execution := ExecutionDirectives{
		MinHoldMinutes:   30,
		MaxTradesPerHour: 50.0,
	}

	if len(perf.RecentTrades) == 0 {
		execution.Comments = append(execution.Comments, "暂无近期交易，维持默认节奏")
		return execution
	}

	var totalDuration time.Duration
	var shortest time.Duration = math.MaxInt64
	var longest time.Duration
	var count int
	tradeTimes := make([]time.Time, 0, len(perf.RecentTrades))

	for _, trade := range perf.RecentTrades {
		if trade.Duration != "" {
			dur, err := time.ParseDuration(trade.Duration)
			if err == nil {
				totalDuration += dur
				if dur < shortest {
					shortest = dur
				}
				if dur > longest {
					longest = dur
				}
				count++
			}
		}
		if !trade.CloseTime.IsZero() {
			tradeTimes = append(tradeTimes, trade.CloseTime)
		}
	}

	if count > 0 {
		avg := totalDuration / time.Duration(count)
		avgMinutes := int(math.Round(avg.Minutes()))
		if avgMinutes > 5 {
			execution.MinHoldMinutes = avgMinutes
		}
		execution.Comments = append(execution.Comments,
			fmt.Sprintf("最近平均持仓 %d 分钟 (最短 %d 分钟 / 最长 %d 分钟)",
				avgMinutes, int(shortest.Minutes()), int(longest.Minutes())))
	} else {
		execution.Comments = append(execution.Comments, "无法计算平均持仓时长，默认保持 30 分钟以上")
	}

	if len(tradeTimes) >= 2 {
		sort.Slice(tradeTimes, func(i, j int) bool { return tradeTimes[i].Before(tradeTimes[j]) })
		first := tradeTimes[0]
		last := tradeTimes[len(tradeTimes)-1]
		spanHours := last.Sub(first).Hours()
		if spanHours > 0 {
			tradesPerHour := float64(len(tradeTimes)) / spanHours
			execution.Comments = append(execution.Comments,
				fmt.Sprintf("过去区间平均 %.2f 笔/小时", math.Round(tradesPerHour*100)/100))
		}
	}

	return execution
}

func buildInsights(stats Stats, symbols []SymbolDirective, execution ExecutionDirectives) []string {
	var insights []string
	if stats.TotalTrades == 0 {
		insights = append(insights, "暂无交易历史，等待新样本")
		return insights
	}
	if stats.SharpeRatio < 0 {
		insights = append(insights, fmt.Sprintf("夏普比率 %.2f < 0，需要降低仓位并专注强信号", stats.SharpeRatio))
	} else if stats.SharpeRatio > 0.8 {
		insights = append(insights, fmt.Sprintf("夏普比率 %.2f 显示策略表现稳定，可逐步放大优势", stats.SharpeRatio))
	}
	if stats.ProfitFactor < 1.0 {
		insights = append(insights, fmt.Sprintf("盈亏比 %.2f 偏低，需筛掉低质量交易", stats.ProfitFactor))
	}
	for _, directive := range symbols {
		if directive.Action == "avoid" {
			insights = append(insights, fmt.Sprintf("避开 %s：%s", directive.Symbol, directive.Reason))
		}
	}
	if len(execution.Comments) > 0 {
		insights = append(insights, execution.Comments[0])
	}
	return insights
}

func buildSummary(stats Stats, symbols []SymbolDirective, risk RiskDirectives) string {
	parts := []string{
		fmt.Sprintf("总交易 %d | 胜率 %.1f%% | 盈亏比 %.2f | 夏普 %.2f", stats.TotalTrades, stats.WinRate, stats.ProfitFactor, stats.SharpeRatio),
	}

	var focusList, avoidList []string
	for _, directive := range symbols {
		switch directive.Action {
		case "focus":
			focusList = append(focusList, directive.Symbol)
		case "avoid":
			avoidList = append(avoidList, directive.Symbol)
		}
	}
	if len(focusList) > 0 {
		parts = append(parts, fmt.Sprintf("重点：%s", strings.Join(focusList, ", ")))
	}
	if len(avoidList) > 0 {
		parts = append(parts, fmt.Sprintf("回避：%s", strings.Join(avoidList, ", ")))
	}
	parts = append(parts, fmt.Sprintf("最低信心 %d / 最大持仓 %d / 仓位系数 %.2f", risk.ConfidenceThreshold, risk.MaxConcurrentPositions, risk.PositionSizeMultiplier))
	return strings.Join(parts, " | ")
}
