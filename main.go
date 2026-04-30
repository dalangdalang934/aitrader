package main

import (
	"aitrade/api"
	"aitrade/config"
	"aitrade/db"
	"aitrade/manager"
	"aitrade/mcp"
	"aitrade/news"
	"aitrade/pool"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║       🤖 基于AI大模型的加密货币量化交易系统               ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// 加载配置文件
	configFile := "config.json"
	if len(os.Args) > 1 {
		configFile = os.Args[1]
	}

	log.Printf("📋 加载配置文件: %s", configFile)
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		log.Fatalf("❌ 加载配置失败: %v", err)
	}

	log.Printf("✓ 配置加载成功，共%d个交易策略已启用", len(cfg.Traders))
	fmt.Println()

	// 设置默认主流币种列表
	pool.SetDefaultCoins(cfg.DefaultCoins)

	// 设置是否使用默认主流币种
	pool.SetUseDefaultCoins(cfg.UseDefaultCoins)
	if cfg.UseDefaultCoins {
		log.Printf("✓ 已启用默认主流币种列表（共%d个币种）: %v", len(cfg.DefaultCoins), cfg.DefaultCoins)
	}

	// 设置币种池API URL
	if cfg.CoinPoolAPIURL != "" {
		pool.SetCoinPoolAPI(cfg.CoinPoolAPIURL)
		log.Printf("✓ 已配置AI500币种池API")
	}
	if cfg.OITopAPIURL != "" {
		pool.SetOITopAPI(cfg.OITopAPIURL)
		log.Printf("✓ 已配置OI Top API")
	}

	// 初始化单点 SQLite（整个进程唯一 Open，必须在所有模块之前）
	var store *db.Store
	if s, sErr := db.NewStore("data/trading.db"); sErr != nil {
		log.Printf("⚠️  打开 SQLite 失败: %v", sErr)
	} else {
		store = s
		defer store.Close()
	}

	// 初始化新闻服务
	var newsSvc *news.Service
	var stopNews context.CancelFunc
	log.Printf("📰 初始化新闻服务...")
	newsOpts := news.Options{
		StorageDir: cfg.NewsStorageDir,
	}

	// 解析时间配置
	if cfg.NewsMaxAge != "" {
		if d, err := time.ParseDuration(cfg.NewsMaxAge); err == nil {
			newsOpts.MaxAge = d
		}
	}
	if cfg.NewsPersistCooldown != "" {
		if d, err := time.ParseDuration(cfg.NewsPersistCooldown); err == nil {
			newsOpts.PersistCooldown = d
		}
	}
	if cfg.NewsReconnectDelay != "" {
		if d, err := time.ParseDuration(cfg.NewsReconnectDelay); err == nil {
			newsOpts.ReconnectDelay = d
		}
	}
	if cfg.NewsPingInterval != "" {
		if d, err := time.ParseDuration(cfg.NewsPingInterval); err == nil {
			newsOpts.PingInterval = d
		}
	}

	newsAIClient, newsAIName := buildNewsAIClient(cfg)
	if newsAIClient != nil {
		newsOpts.Summarizer = news.NewMCPSummarizer(newsAIClient)
		log.Printf("📰 使用 %s 进行新闻摘要/宏观研判", newsAIName)
	} else {
		log.Printf("⚠️  未找到可用 AI 配置，新闻将使用本地 fallback 摘要")
	}

	if cfg.NewsWebSearchEnabled {
		searchClient, providerName := buildNewsSearchClient(cfg, newsAIClient)
		if searchClient != nil {
			newsOpts.WebSearch.Enabled = true
			newsOpts.WebSearch.Query = cfg.NewsWebSearchQuery
			newsOpts.WebSearch.Client = searchClient
			newsOpts.WebSearch.Provider = providerName
			newsOpts.WebSearch.Model = cfg.NewsWebSearchModel
			if d, err := time.ParseDuration(cfg.NewsWebSearchInterval); err == nil {
				newsOpts.WebSearch.Interval = d
			}
			log.Printf("📰 已启用 Web Search 新闻补源 (%s)", providerName)
		} else {
			log.Printf("⚠️  已开启 news_web_search_enabled，但未找到可用搜索 API，跳过 Web Search provider")
		}
	}

	// OpenNews 配置
	if cfg.NewsOpenNewsEnabled {
		newsOpts.OpenNews.Enabled = true
		newsOpts.OpenNews.APIURL = cfg.NewsOpenNewsAPIURL
		newsOpts.OpenNews.WSURL = cfg.NewsOpenNewsWSURL
		newsOpts.OpenNews.APIKey = cfg.NewsOpenNewsAPIKey
		if d, err := time.ParseDuration(cfg.NewsOpenNewsPollInterval); err == nil {
			newsOpts.OpenNews.PollInterval = d
		}
		log.Printf("📰 已启用 OpenNews 新闻源 (%s)", cfg.NewsOpenNewsAPIURL)
	}

	newsSvc, err = news.NewService(newsOpts)
	if err != nil {
		log.Printf("⚠️  新闻服务初始化失败: %v，将不使用新闻功能", err)
	} else {
		news.SetDefaultService(newsSvc)

		// 挂载 SQLite（可选，失败时降级到文件存储）
		if store != nil {
			if dbErr := newsSvc.SetDB(store); dbErr != nil {
				log.Printf("⚠️  新闻 SQLite 挂载失败: %v，使用文件存储", dbErr)
			}
		}

		providerNames := newsSvc.ProviderNames()
		if len(providerNames) > 0 {
			log.Printf("✓ 新闻服务初始化成功，活跃 provider: %s", strings.Join(providerNames, ", "))
		} else {
			log.Printf("✓ 新闻服务初始化成功（未启用 provider，将仅提供缓存数据）")
		}

		if newsAIClient != nil {
			newsSvc.SetOutlookAnalyzer(news.NewOutlookAnalyzer(newsAIClient))
			log.Printf("✓ Outlook 宏观研判分析器已启用")
		}

		newsCtx, cancelNews := context.WithCancel(context.Background())
		stopNews = cancelNews
		go newsSvc.Run(newsCtx)
		go newsSvc.RunOutlookLoop(newsCtx)
		log.Printf("✓ 新闻服务后台任务已启动")
	}

	// 创建TraderManager
	traderManager := manager.NewTraderManager(store)

	// 添加所有启用的trader
	enabledCount := 0
	for i, traderCfg := range cfg.Traders {
		// 跳过未启用的trader
		if !traderCfg.Enabled {
			log.Printf("⏭️  [%d/%d] 跳过未启用的 %s", i+1, len(cfg.Traders), traderCfg.Name)
			continue
		}

		enabledCount++
		log.Printf("📦 [%d/%d] 初始化 %s (%s模型)...",
			i+1, len(cfg.Traders), traderCfg.Name, strings.ToUpper(traderCfg.AIModel))

		err := traderManager.AddTrader(
			traderCfg,
			cfg.CoinPoolAPIURL,
			cfg.MaxDailyLoss,
			cfg.MaxDrawdown,
			cfg.StopTradingMinutes,
			cfg.Leverage, // 传递杠杆配置
		)
		if err != nil {
			log.Fatalf("❌ 初始化trader失败: %v", err)
		}
	}

	// 检查是否至少有一个启用的trader
	if enabledCount == 0 {
		log.Fatalf("❌ 没有启用的trader，请在config.json中设置至少一个trader的enabled=true")
	}

	fmt.Println()
	fmt.Println("📊 已启用的交易策略:")
	for _, traderCfg := range cfg.Traders {
		// 只显示启用的trader
		if !traderCfg.Enabled {
			continue
		}
		fmt.Printf("  • %s (%s) - 初始资金: %.0f USDT\n",
			traderCfg.Name, strings.ToUpper(traderCfg.AIModel), traderCfg.InitialBalance)
	}

	fmt.Println()
	fmt.Println("🤖 AI全权决策模式:")
	fmt.Printf("  • AI将自主决定每笔交易的杠杆倍数（山寨币最高%d倍，BTC/ETH最高%d倍）\n",
		cfg.Leverage.AltcoinLeverage, cfg.Leverage.BTCETHLeverage)
	fmt.Println("  • AI将自主决定每笔交易的仓位大小")
	fmt.Println("  • AI将自主设置止损和止盈价格")
	fmt.Println("  • AI将基于市场数据、技术指标、账户状态做出全面分析")
	fmt.Println()
	fmt.Println("⚠️  风险提示: AI自动交易有风险，建议小额资金测试！")
	fmt.Println()
	fmt.Println("按 Ctrl+C 停止运行")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	// 创建并启动API服务器
	apiServer := api.NewServer(traderManager, cfg.APIServerPort, store)
	go func() {
		if err := apiServer.Start(); err != nil {
			log.Printf("❌ API服务器错误: %v", err)
		}
	}()

	// 设置优雅退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// 启动所有trader
	traderManager.StartAll()

	// 等待退出信号
	<-sigChan
	fmt.Println()
	fmt.Println()
	log.Println("📛 收到退出信号，正在停止所有trader...")
	if stopNews != nil {
		stopNews()
	}
	traderManager.StopAll()

	fmt.Println()
	fmt.Println("👋 感谢使用基于AI大模型的量化交易系统！")
}

func buildNewsAIClient(cfg *config.Config) (*mcp.Client, string) {
	for _, trader := range cfg.Traders {
		if !trader.Enabled {
			continue
		}
		client := mcp.New()
		switch trader.AIModel {
		case "custom":
			if trader.CustomAPIURL != "" && trader.CustomAPIKey != "" && trader.CustomModelName != "" {
				client.SetCustomAPI(trader.CustomAPIURL, trader.CustomAPIKey, trader.CustomModelName)
				return client, fmt.Sprintf("自定义AI %s", trader.Name)
			}
		case "qwen":
			if trader.QwenKey != "" {
				client.SetQwenAPIKey(trader.QwenKey, "")
				return client, fmt.Sprintf("Qwen %s", trader.Name)
			}
		case "deepseek":
			if trader.DeepSeekKey != "" {
				client.SetDeepSeekAPIKey(trader.DeepSeekKey)
				return client, fmt.Sprintf("DeepSeek %s", trader.Name)
			}
		}
	}
	return nil, ""
}

func buildNewsSearchClient(cfg *config.Config, fallback *mcp.Client) (*mcp.Client, string) {
	if cfg.NewsWebSearchAPIURL != "" && cfg.NewsWebSearchAPIKey != "" && cfg.NewsWebSearchModel != "" {
		client := mcp.New()
		client.SetCustomAPI(cfg.NewsWebSearchAPIURL, cfg.NewsWebSearchAPIKey, cfg.NewsWebSearchModel)
		return client, "custom-search"
	}
	if fallback != nil {
		return fallback, "shared-ai"
	}
	return nil, ""
}
