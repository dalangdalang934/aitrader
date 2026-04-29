package api

import (
	"aitrade/db"
	"aitrade/logger"
	"aitrade/manager"
	"aitrade/news"
	"fmt"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
)

// Server HTTP API服务器
type Server struct {
	router        *gin.Engine
	traderManager *manager.TraderManager
	port          int
	reader        db.APIReader
}

// NewServer 创建API服务器
func NewServer(traderManager *manager.TraderManager, port int, reader db.APIReader) *Server {
	// 设置为Release模式（减少日志输出）
	gin.SetMode(gin.ReleaseMode)

	router := gin.Default()

	// 启用CORS
	router.Use(corsMiddleware())

	s := &Server{
		router:        router,
		traderManager: traderManager,
		port:          port,
		reader:        reader,
	}

	// 设置路由
	s.setupRoutes()

	return s
}

// corsMiddleware CORS中间件
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}

		c.Next()
	}
}

// setupRoutes 设置路由
func (s *Server) setupRoutes() {
	// 健康检查
	s.router.Any("/health", s.handleHealth)

	// API路由组
	api := s.router.Group("/api")
	{
		// 多策略总览
		api.GET("/competition", s.handleCompetition)

		// Trader列表
		api.GET("/traders", s.handleTraderList)

		// 指定trader的数据（使用query参数 ?trader_id=xxx）
		api.GET("/status", s.handleStatus)
		api.GET("/account", s.handleAccount)
		api.GET("/positions", s.handlePositions)
		api.GET("/decisions", s.handleDecisions)
		api.GET("/decisions/latest", s.handleLatestDecisions)
		api.GET("/statistics", s.handleStatistics)
		api.GET("/equity-history", s.handleEquityHistory)
		api.GET("/performance", s.handlePerformance)
		api.GET("/exchange-trades", s.handleExchangeTrades)
		api.GET("/news/digests", s.handleNewsDigests)
		api.GET("/news/outlook", s.handleNewsOutlook)

		// 仓位相关
		api.GET("/positions/active", s.handleActivePositions)
		api.GET("/positions/history", s.handlePositionHistory)
		api.GET("/positions/:id", s.handlePositionDetail)
	}
}

// handleHealth 健康检查
func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"time":   c.Request.Context().Value("time"),
	})
}

// getTraderFromQuery 从query参数获取trader
func (s *Server) getTraderFromQuery(c *gin.Context) (*manager.TraderManager, string, error) {
	traderID := c.Query("trader_id")
	if traderID == "" {
		// 如果没有指定trader_id，返回第一个trader
		ids := s.traderManager.GetTraderIDs()
		if len(ids) == 0 {
			return nil, "", fmt.Errorf("没有可用的trader")
		}
		traderID = ids[0]
	}
	return s.traderManager, traderID, nil
}

// handleCompetition 多策略总览（对比所有交易策略）
func (s *Server) handleCompetition(c *gin.Context) {
	comparison, err := s.traderManager.GetComparisonData()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("获取对比数据失败: %v", err),
		})
		return
	}
	c.JSON(http.StatusOK, comparison)
}

// handleTraderList trader列表
func (s *Server) handleTraderList(c *gin.Context) {
	traders := s.traderManager.GetAllTraders()
	result := make([]map[string]interface{}, 0, len(traders))

	for _, t := range traders {
		result = append(result, map[string]interface{}{
			"trader_id":   t.GetID(),
			"trader_name": t.GetName(),
			"ai_model":    t.GetAIModel(),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"traders": result,
	})
}

// handleStatus 系统状态
func (s *Server) handleStatus(c *gin.Context) {
	_, traderID, err := s.getTraderFromQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	trader, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	status := trader.GetStatus()
	c.JSON(http.StatusOK, status)
}

// handleAccount 账户信息
func (s *Server) handleAccount(c *gin.Context) {
	_, traderID, err := s.getTraderFromQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	trader, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	log.Printf("📊 收到账户信息请求 [%s]", trader.GetName())
	account, err := trader.GetAccountInfo()
	if err != nil {
		log.Printf("❌ 获取账户信息失败 [%s]: %v", trader.GetName(), err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("获取账户信息失败: %v", err),
		})
		return
	}

	log.Printf("✓ 返回账户信息 [%s]: 净值=%.2f, 可用=%.2f, 盈亏=%.2f (%.2f%%)",
		trader.GetName(),
		account["total_equity"],
		account["available_balance"],
		account["total_pnl"],
		account["total_pnl_pct"])
	c.JSON(http.StatusOK, account)
}

// handlePositions 持仓列表
func (s *Server) handlePositions(c *gin.Context) {
	_, traderID, err := s.getTraderFromQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	trader, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	positions, err := trader.GetPositions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("获取持仓列表失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, positions)
}

// handleDecisions 决策日志列表
func (s *Server) handleDecisions(c *gin.Context) {
	_, traderID, err := s.getTraderFromQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	trader, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// 获取所有历史决策记录（无限制）
	records, err := trader.GetDecisionLogger().GetLatestRecords(10000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("获取决策日志失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, records)
}

// handleLatestDecisions 最新决策日志（最近5条，最新的在前）
func (s *Server) handleLatestDecisions(c *gin.Context) {
	_, traderID, err := s.getTraderFromQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	trader, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	records, err := trader.GetDecisionLogger().GetLatestRecords(5)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("获取决策日志失败: %v", err),
		})
		return
	}

	// 反转数组，让最新的在前面（用于列表显示）
	// GetLatestRecords返回的是从旧到新（用于图表），这里需要从新到旧
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}

	c.JSON(http.StatusOK, records)
}

// handleStatistics 统计信息
func (s *Server) handleStatistics(c *gin.Context) {
	_, traderID, err := s.getTraderFromQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	trader, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	stats, err := trader.GetDecisionLogger().GetStatistics()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("获取统计信息失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// handleEquityHistory 净值历史数据（从 SQLite 查询）
func (s *Server) handleEquityHistory(c *gin.Context) {
	_, traderID, err := s.getTraderFromQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if s.reader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "数据库未初始化"})
		return
	}

	points, err := s.reader.QueryEquityHistory(traderID, 10000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("查询净值历史失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, points)
}

// handlePerformance AI历史表现分析（用于展示AI学习和反思）
func (s *Server) handlePerformance(c *gin.Context) {
	_, traderID, err := s.getTraderFromQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	auto, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// 1. 先尝试从决策日志分析最新交易
	performance, err := auto.GetDecisionLogger().AnalyzePerformance(10000)
	if err != nil {
		log.Printf("⚠️ 分析决策日志失败: %v", err)
		performance = &logger.PerformanceAnalysis{
			RecentTrades: []logger.TradeOutcome{},
			SymbolStats:  make(map[string]*logger.SymbolPerformance),
		}
	}
	log.Printf("📊 从决策日志分析到 %d 条交易", len(performance.RecentTrades))

	// 2. 读取持久化的仓位历史
	persistedHistory, err := auto.GetPositionHistory()
	if err != nil {
		log.Printf("⚠️ 读取仓位历史失败: %v", err)
		persistedHistory = []logger.TradeOutcome{}
	}
	log.Printf("📊 读取到持久化历史 %d 条", len(persistedHistory))

	// 3. 合并两个数据源，去重
	tradeMap := make(map[string]logger.TradeOutcome)

	// 先添加持久化的历史记录
	for _, trade := range persistedHistory {
		// 使用更详细的key：币种_平仓时间_开仓时间_数量
		key := fmt.Sprintf("%s_%d_%d_%.4f", trade.Symbol, trade.CloseTime.Unix(), trade.OpenTime.Unix(), trade.Quantity)
		tradeMap[key] = trade
	}

	// 再添加最新的分析结果（会覆盖重复的）
	for _, trade := range performance.RecentTrades {
		// 使用更详细的key：币种_平仓时间_开仓时间_数量
		key := fmt.Sprintf("%s_%d_%d_%.4f", trade.Symbol, trade.CloseTime.Unix(), trade.OpenTime.Unix(), trade.Quantity)
		tradeMap[key] = trade
	}

	// 转换回切片并按时间排序
	allTrades := make([]logger.TradeOutcome, 0, len(tradeMap))
	for _, trade := range tradeMap {
		allTrades = append(allTrades, trade)
	}
	log.Printf("📊 合并去重后共 %d 条交易", len(allTrades))

	sort.Slice(allTrades, func(i, j int) bool {
		return allTrades[i].CloseTime.After(allTrades[j].CloseTime)
	})

	// 更新performance的交易记录
	performance.RecentTrades = allTrades

	// 对于仓位历史，我们显示所有保存的交易，不进行周期过滤
	// 因为这些是AI实际执行的交易，应该全部显示给用户
	periodStart, periodSource := auto.GetDecisionLogger().DetectPeriodStart(auto.GetStartTime())

	// 不过滤，直接使用所有交易
	performance.PeriodStart = periodStart
	performance.PeriodSource = periodSource

	performance.TotalTrades = len(allTrades)
	performance.WinningTrades = 0
	performance.LosingTrades = 0
	performance.AvgWin = 0
	performance.AvgLoss = 0
	performance.ProfitFactor = 0
	performance.WinRate = 0
	performance.BestSymbol = ""
	performance.WorstSymbol = ""

	if performance.TotalTrades == 0 {
		performance.SymbolStats = make(map[string]*logger.SymbolPerformance)
		c.JSON(http.StatusOK, performance)
		return
	}

	totalWinAmount := 0.0
	totalLossAmount := 0.0
	newSymbolStats := make(map[string]*logger.SymbolPerformance)

	for _, trade := range allTrades {
		if trade.PnL > 0 {
			performance.WinningTrades++
			totalWinAmount += trade.PnL
		} else if trade.PnL < 0 {
			performance.LosingTrades++
			totalLossAmount += trade.PnL
		}

		stats := newSymbolStats[trade.Symbol]
		if stats == nil {
			stats = &logger.SymbolPerformance{Symbol: trade.Symbol}
			newSymbolStats[trade.Symbol] = stats
		}
		stats.TotalTrades++
		stats.TotalPnL += trade.PnL
		if trade.PnL > 0 {
			stats.WinningTrades++
		} else if trade.PnL < 0 {
			stats.LosingTrades++
		}
		stats.AvgPnL = stats.TotalPnL / float64(stats.TotalTrades)
		if stats.TotalTrades > 0 {
			stats.WinRate = (float64(stats.WinningTrades) / float64(stats.TotalTrades)) * 100
		}
	}

	if performance.WinningTrades > 0 {
		performance.AvgWin = totalWinAmount / float64(performance.WinningTrades)
	}
	if performance.LosingTrades > 0 {
		performance.AvgLoss = totalLossAmount / float64(performance.LosingTrades)
	}
	if totalLossAmount != 0 {
		performance.ProfitFactor = totalWinAmount / -totalLossAmount
	}
	performance.SymbolStats = newSymbolStats
	if performance.TotalTrades > 0 {
		performance.WinRate = (float64(performance.WinningTrades) / float64(performance.TotalTrades)) * 100
	}

	bestPnL := 0.0
	worstPnL := 0.0
	for symbol, stats := range newSymbolStats {
		if performance.BestSymbol == "" || stats.TotalPnL > bestPnL {
			performance.BestSymbol = symbol
			bestPnL = stats.TotalPnL
		}
		if performance.WorstSymbol == "" || stats.TotalPnL < worstPnL {
			performance.WorstSymbol = symbol
			worstPnL = stats.TotalPnL
		}
	}

	c.JSON(http.StatusOK, performance)
}

// handleExchangeTrades 返回交易所成交汇总（仅用于展示）
func (s *Server) handleExchangeTrades(c *gin.Context) {
	_, traderID, err := s.getTraderFromQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	auto, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	exchangeTrades, err := auto.GetExchangeTrades()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("读取交易所成交记录失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"exchange_trades": exchangeTrades,
		"count":           len(exchangeTrades),
	})
}

// Start 启动服务器
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("🌐 API服务器启动在 http://localhost%s", addr)
	log.Printf("📊 API文档:")
	log.Printf("  • GET  /api/competition      - 多策略总览（实时对比各交易策略）")
	log.Printf("  • GET  /api/traders          - Trader列表")
	log.Printf("  • GET  /api/status?trader_id=xxx     - 指定trader的系统状态")
	log.Printf("  • GET  /api/account?trader_id=xxx    - 指定trader的账户信息")
	log.Printf("  • GET  /api/positions?trader_id=xxx  - 指定trader的持仓列表")
	log.Printf("  • GET  /api/decisions?trader_id=xxx  - 指定trader的决策日志")
	log.Printf("  • GET  /api/decisions/latest?trader_id=xxx - 指定trader的最新决策")
	log.Printf("  • GET  /api/statistics?trader_id=xxx - 指定trader的统计信息")
	log.Printf("  • GET  /api/equity-history?trader_id=xxx - 指定trader的收益率历史数据")
	log.Printf("  • GET  /api/performance?trader_id=xxx - 指定trader的AI学习表现分析")
	log.Printf("  • GET  /api/exchange-trades?trader_id=xxx - 交易所成交记录展示")
	log.Printf("  • GET  /api/news/digests  - 近期新闻快讯摘要")
	log.Printf("  • GET  /api/news/outlook  - 宏观基本面研判报告")
	log.Printf("  • GET  /health               - 健康检查")
	log.Println()

	return s.router.Run(addr)
}

// handleActivePositions 获取活跃仓位
func (s *Server) handleActivePositions(c *gin.Context) {
	_, traderID, err := s.getTraderFromQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	auto, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// 获取仓位追踪器
	positionTracker := auto.GetPositionTracker()
	if positionTracker == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "仓位追踪器未初始化"})
		return
	}

	activePositions := positionTracker.GetActivePositions()
	log.Printf("📊 [%s] 获取活跃仓位: %d 条", traderID, len(activePositions))
	c.JSON(http.StatusOK, activePositions)
}

// handleNewsDigests 获取新闻摘要
func (s *Server) handleNewsDigests(c *gin.Context) {
	newsSvc := news.GetDefaultService()
	if newsSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "news service unavailable",
		})
		return
	}
	digests := newsSvc.GetDigests()
	sort.Slice(digests, func(i, j int) bool {
		pi := digests[i].PublishedAt
		pj := digests[j].PublishedAt
		if pi.IsZero() {
			pi = digests[i].CreatedAt
		}
		if pj.IsZero() {
			pj = digests[j].CreatedAt
		}
		return pi.After(pj)
	})
	if len(digests) > 10 {
		digests = digests[:10]
	}
	c.JSON(http.StatusOK, gin.H{
		"digests":    digests,
		"count":      len(digests),
		"updated_at": time.Now().Format("2006-01-02 15:04:05"),
	})
}

// handleNewsOutlook 获取宏观基本面研判报告
func (s *Server) handleNewsOutlook(c *gin.Context) {
	newsSvc := news.GetDefaultService()
	if newsSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "news service unavailable",
		})
		return
	}
	outlook := newsSvc.GetOutlook()
	if outlook == nil {
		c.JSON(http.StatusOK, gin.H{
			"outlook":    nil,
			"available":  false,
			"updated_at": time.Now().Format("2006-01-02 15:04:05"),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"outlook":    outlook,
		"available":  true,
		"updated_at": time.Now().Format("2006-01-02 15:04:05"),
	})
}

// handlePositionHistory 获取历史仓位（从 SQLite 查询）
func (s *Server) handlePositionHistory(c *gin.Context) {
	_, traderID, err := s.getTraderFromQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if s.reader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "数据库未初始化"})
		return
	}

	records, err := s.reader.QueryPositionHistory(traderID, 1000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("查询历史仓位失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, records)
}

// handlePositionDetail 获取仓位详情
func (s *Server) handlePositionDetail(c *gin.Context) {
	positionID := c.Param("id")
	if positionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少仓位ID"})
		return
	}

	_, traderID, err := s.getTraderFromQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	auto, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// 获取仓位追踪器
	positionTracker := auto.GetPositionTracker()
	if positionTracker == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "仓位追踪器未初始化"})
		return
	}

	position, err := positionTracker.GetPosition(positionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, position)
}
