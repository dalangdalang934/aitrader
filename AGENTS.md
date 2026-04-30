#  项目开发文档

> 本科毕业设计：基于大语言模型的加密货币量化交易系统
>
> **维护规则：每次修改代码后必须同步更新本文档对应章节。**

---

## 系统概述

**本系统** 是一个让多个 AI 大模型（DeepSeek、Qwen、自定义模型）在真实交易所（Binance Futures、Hyperliquid、Aster）上进行自动量化交易的智能系统。

- AI 全权决策（开多/开空/平仓/止损/止盈），Go 层只做合法性校验
- 多 Trader 并行运行，每个 Trader 独立账户、独立 AI 模型
- 实时收集市场数据（K 线 + 技术指标 + OI + 资金费率）
- 新闻抓取 + AI 摘要 + 宏观基本面研判注入决策 Prompt
- AI 自我学习闭环：历史表现 → 结构化指令 → 下轮 Prompt
- Web 监控面板实时展示各 AI 表现对比

**技术栈：** Go 1.25 / Gin / SQLite（modernc，纯 Go）/ Next.js + TypeScript / Docker Compose / Nginx

---

## 目录结构

```
项目根目录/
├── main.go                   # 程序入口，组装并启动所有服务
├── start.py                  # 终端管理脚本（编辑配置/提示词、启动与维护）
├── config/                   # 配置加载与验证（config.json → Config 结构体）
├── manager/                  # TraderManager：管理多个 AutoTrader 实例的生命周期
├── trader/                   # 核心交易层
│   ├── interface.go          # Trader 统一接口（多交易所抽象）
│   ├── auto_trader.go        # AutoTrader：AI 决策主循环（核心文件）
│   ├── binance_futures.go    # Binance 合约实现
│   ├── hyperliquid_trader.go # Hyperliquid 实现
│   ├── aster_trader.go       # Aster 实现
│   └── position_tracker.go   # 仓位生命周期追踪（UUID + 持久化）
├── decision/                 # AI 决策引擎
│   ├── engine.go             # GetFullDecision()：Prompt 构建 + AI 调用 + 解析验证
│   └── templates/
│       └── system_prompt.tmpl  # System Prompt Go 模板（固定规则）
├── market/                   # 市场数据采集 + 技术指标本地计算
├── mcp/                      # AI API 客户端（OpenAI 兼容格式，支持三个 provider）
├── news/                     # 新闻服务（WebSocket/RSS/OpenNews/WebSearch + AI 摘要 + 宏观研判）
├── pool/                     # 币种候选池（AI500 + OI Top，三层降级缓存）
├── learning/                 # AI 自学习模块（历史表现 → 结构化指令持久化）
├── logger/                   # 决策日志（JSON 文件）+ 性能分析（AnalyzePerformance）
├── api/                      # HTTP API 服务器（Gin，只读，前端数据源）
├── db/                       # SQLite 封装（db.Open：WAL 模式 + 外键约束）
├── web/                      # Next.js 前端监控面板
├── nginx/                    # Nginx 反向代理配置
├── data/                     # 运行时数据持久化（JSON 文件，规划迁移至 SQLite）
├── decision_logs/            # 每轮决策完整记录（每轮一个 JSON 文件）
├── config.json               # 运行时配置（含密钥，不入库，见 config.json.example）
├── Dockerfile.backend        # Go 后端镜像
└── docker-compose.yml        # 容器编排
```

---

## 核心模块职责

### `config/` — 配置层

唯一数据结构：`Config`（全局）和 `TraderConfig`（每个 Trader 实例）。

**关键字段：**
- `TraderConfig.AIModel`：`"deepseek"` / `"qwen"` / `"custom"`
- `TraderConfig.Exchange`：`"binance"` / `"hyperliquid"` / `"aster"`
- `LeverageConfig`：`BTCETHLeverage`（BTC/ETH 杠杆上限）、`AltcoinLeverage`（山寨币上限）

**修改配置时：** 只改 `config.json`，不改代码。新增字段需同步更新 `config/config.go` 对应结构体和 `config.json.example`。

---

### `trader/` — 交易执行层

**`Trader` 接口（interface.go）** — 多交易所抽象，所有实现必须满足：

```go
GetBalance() map[string]interface{}
GetPositions() []map[string]interface{}
OpenLong/OpenShort(symbol, qty, leverage) error
CloseLong/CloseShort(symbol, qty) error
SetStopLoss/SetTakeProfit(symbol, side, qty, price) error
CancelAllOrders(symbol) error
GetMarketPrice(symbol) (float64, error)
```

**`AutoTrader`（auto_trader.go）** — 每个 Trader 实例的主循环，每 `ScanIntervalMinutes` 分钟执行一轮：

1. `pool.GetMergedCoinPool()` → 候选币种
2. `trader.GetBalance()` / `GetPositions()` → 账户快照
3. `news.GetDigests()` / `GetOutlook()` → 新闻摘要 + 宏观研判
4. `learning.Manager.Load()` → 历史学习状态
5. `logger.AnalyzePerformance()` → 近期表现统计
6. `decision.GetFullDecision()` → AI 决策（核心）
7. 执行每条 `Decision`（开/平仓、止损止盈）
8. `logger.SaveDecision()` + `learningManager.Update()` → 持久化

**`PositionTracker`（position_tracker.go）** — 追踪仓位完整生命周期：
- 每个仓位用 UUID 标识
- 活跃仓位常驻内存 + 写入 `data/positions/<trader_id>/positions.json`
- 平仓时写入 `data/positions/<trader_id>/history/`

---

### `decision/` — AI 决策引擎

**核心函数：** `GetFullDecision(ctx Context, client *mcp.Client) (*FullDecision, error)`

**执行流程：**
```
并发拉取市场数据（每个候选币种：3m/1h/4h K线 + OI + 资金费率）
    ↓
流动性过滤（OI 价值 < 15M USD 的币种跳过）
    ↓
buildSystemPrompt()  ← Go template 渲染（固定约束规则）
buildUserPrompt()    ← 动态拼接（账户、持仓、市场数据、历史表现、学习状态、新闻）
    ↓
mcp.CallWithMessages(system, user) → AI API（temperature=0.5，max_tokens=2000，超时 120s）
    ↓
parseFullDecision()  → 提取 CoT 思维链 + JSON 决策数组
validateDecisions()  → 硬约束校验
```

**硬约束（`validateDecisions`，AI 无法绕过）：**
- 山寨币杠杆 ≤ `AltcoinLeverage`，BTC/ETH ≤ `BTCETHLeverage`
- 山寨币单币种仓位 ≤ 账户净值 × 1.5
- BTC/ETH 单币种仓位 ≤ 账户净值 × 10
- 风险回报比强制 ≥ 3:1（以当前市场价为入场参考校验）
- 止损/止盈方向必须与多空方向一致

**Decision 结构：**
```go
Decision {
    Symbol          string   // 币种，如 "BTCUSDT"
    Action          string   // "open_long" / "open_short" / "close_long" / "close_short" / "hold"
    Leverage        int
    PositionSizeUSD float64
    StopLoss        float64
    TakeProfit      float64
    Confidence      int      // 0-100
    Reasoning       string   // AI 的决策理由
}
```

---

### `market/` — 市场数据层

全部调用 Binance Futures **公共 API**（无需密钥），本地计算技术指标：

| 指标 | 时间框架 |
|------|---------|
| EMA(20/50/100/200) | 3m / 1h / 4h |
| MACD(12,26,9) | 3m / 1h / 4h |
| RSI(7/14) | 3m / 1h / 4h |
| 布林带(20, 2σ) | 3m / 1h |
| VWAP | 3m / 1h / 4h |
| OBV | 3m |
| ATR(3/14) | 4h |
| ADX + ±DI(14) | 4h |
| Fibonacci 回撤/扩展 | 4h（近20根K线） |

`market.Format()` 将数据序列化为结构化文本，直接拼入 AI Prompt。

---

### `mcp/` — AI 客户端

| Provider | 端点 | 默认模型 |
|----------|------|---------|
| `deepseek` | `api.deepseek.com/v1` | `deepseek-chat` |
| `qwen` | `dashscope.aliyuncs.com/compatible-mode/v1` | `qwen-plus` |
| `custom` | 用户自定义 | 用户自定义 |

统一 OpenAI Chat Completions 格式，最多重试 3 次（可重试：网络超时、EOF、连接重置）。

---

### `news/` — 新闻服务

**数据源：** OpenNews API（WebSocket / HTTP）/ Web Search Provider（周期性 AI 主动搜索）；未启用 provider 时仅提供本地缓存读取

**OpenNews API：** 84+ 实时数据源（Bloomberg、Reuters、CoinDesk 等），支持 REST API 和 WebSocket，自带 AI 评分（0-100）和交易信号（long/short/neutral）。配置字段：`news_opennews_enabled`、`news_opennews_api_url`、`news_opennews_ws_url`、`news_opennews_api_key`。HTTP 轮询使用官方 `POST /open/news_search` 最新新闻接口；若 `news_opennews_ws_url` 不是合法的 `ws/wss` 地址，或 WebSocket 因 `401/403`、套餐权限不足等原因不可用，则自动降级为 `news_opennews_api_url` 的 HTTP 轮询模式。

**后台 goroutine：**
- `Run(ctx)`：持续收新闻 → AI 生成摘要（`Digest`）→ 内存 + SQLite 持久化（降级文件）
- `RunOutlookLoop(ctx)`：定期用近期新闻生成宏观基本面研判（`MacroOutlook`）

**`MacroOutlook` 字段：** `OverallBias`（多/空/中性）、`BiasScore`（-100 到 100）、`RiskLevel`、`KeyFactors`、`Recommendations`

**⚠️ 重要设计注意：** `GetDigests()` 和 `pruneLocked()` 用 **`CreatedAt`（接收时间）** 而非 `PublishedAt` 做过期判断。原因：RSS 数据源的 `pubDate` 是新闻原始发布时间（可能是数天前），若用 `PublishedAt` 过滤会导致所有 RSS 新闻被 `MaxAge` 误杀显示"暂无新闻"。

---

### `pool/` — 币种候选池

合并两个数据源，三层降级策略：
```
AI500 API / OI Top API  →（失败）→  文件缓存 (coin_pool_cache/)  →（失败）→  默认18个主流币种
```

候选币种标记来源：`ai500` / `oi_top` / 双重信号。

---

### `learning/` — AI 自学习

**数据流（学习闭环）：**
```
decision_logs/ → logger.AnalyzePerformance() → learning.Manager.Update() → data/learning/<id>.json
                                                                                  ↓
                                                           下轮决策 buildUserPrompt() 注入
```

**`State` 核心字段：**
- `Risk`：`ConfidenceThreshold`、`MaxConcurrentPositions`、`PositionSizeMultiplier`、`CooldownMinutes`
- `Symbols`：每个币种的 `focus` / `avoid` / `watch` 指令
- `Insights`：关键反思要点列表

**当前行为：** 学习状态主要作为 Prompt 中的历史经验参考，不再直接按 `ConfidenceThreshold` / `MaxConcurrentPositions` / `PositionSizeMultiplier` 对 AI 开仓做硬拦截；执行层仅在累计收益过高或回撤过深时启用保护性缩仓与持仓收紧。

当前存储：`data/learning/<trader_id>.json`（规划迁移至 SQLite `learning_states` 表）。

---

### `logger/` — 决策日志

每轮决策写一个 JSON 文件：`decision_logs/<trader_id>/decision_YYYYMMDD_HHMMSS.json`

**`DecisionRecord` 包含：** 完整 System/User Prompt、AI CoT 思维链、执行情况、账户快照、持仓快照。

`AnalyzePerformance(traderID)` 扫描所有历史 JSON，配对开平仓计算：胜率、盈亏比、夏普比率、按币种统计、最近20笔明细。

---

### `api/` — HTTP API

框架：Gin（端口 `api_server_port`，默认 8080），全局 CORS `*`，只读接口。

| 端点 | 说明 |
|------|------|
| `GET /api/competition` | 所有 Trader 对比（净值/盈亏/持仓数） |
| `GET /api/traders` | Trader 列表 |
| `GET /api/status?trader_id=` | 运行状态 |
| `GET /api/account?trader_id=` | 账户信息（净值/余额/盈亏率） |
| `GET /api/positions?trader_id=` | 当前持仓 |
| `GET /api/decisions?trader_id=` | 决策日志（最多10000条） |
| `GET /api/decisions/latest?trader_id=` | 最新5条决策 |
| `GET /api/equity-history?trader_id=` | 净值历史曲线 |
| `GET /api/performance?trader_id=` | AI 表现分析 |
| `GET /api/positions/active?trader_id=` | 活跃仓位（PositionTracker） |
| `GET /api/positions/history?trader_id=` | 历史仓位 |
| `GET /api/news/digests` | 最新10条新闻摘要 |
| `GET /api/news/outlook` | 宏观基本面研判 |

---

### `db/` — SQLite 封装

```go
db.Open(path string) (*sql.DB, error)
// 启用 WAL 模式 + 外键约束，MaxOpenConns=1（SQLite 单写者）
```

统一数据库文件：`data/trading.db`，各模块自行建表（`CREATE TABLE IF NOT EXISTS`）：

| 表 | 模块 | 用途 |
|----|------|------|
| `learning_states` | `learning/` | `trader_id → JSON State`（已迁移） |
| `news_cache` | `news/` | 新闻原始条目 + 摘要，单行 JSON blob（已迁移，文件降级保留） |

---

## 数据流总览

```
config.json
    ↓
main.go
    ├── news.Service  ─────────────────────→ data/news/（raw_news.json / digests.json）
    ├── api.Server    ←── 读取所有模块状态
    └── manager.TraderManager
            └── AutoTrader × N（goroutine，每 3 分钟一轮）
                    ↓
            pool ──────────────→ 候选币种（AI500 + OI Top）
            trader.GetBalance/Positions → 账户快照
            news.GetDigests/Outlook → 新闻上下文
            learning.Load → 历史约束指令
            logger.AnalyzePerformance → 近期表现
                    ↓
            decision.GetFullDecision
                ├── market.Get(symbol) × N（并发）← Binance fapi（公共 API）
                ├── buildSystemPrompt（Go template）
                ├── buildUserPrompt（动态拼接）
                ├── mcp.Call → AI API（DeepSeek / Qwen / Custom）
                └── parse + validate
                    ↓
            执行决策（trader.Open/Close/SetSL/SetTP）
                    ↓
            logger.SaveDecision → decision_logs/
            learning.Update    → data/learning/
            positionTracker    → data/positions/
```

---

## 存储层现状

| 路径 | 内容 | 状态 |
|------|------|------|
| `decision_logs/<id>/decision_*.json` | 每轮决策完整记录 | 稳定，保持文件 |
| `data/positions/<id>/positions.json` | 活跃仓位 | 稳定，保持文件 |
| `data/positions/<id>/history/` | 历史仓位 | 稳定，保持文件 |
| `data/trading.db` (SQLite) | learning_states 表：AI 学习状态 | ✅ 已迁移 SQLite |
| `data/trading.db` (SQLite) | news_cache 表：新闻条目 + 摘要 | ✅ 已迁移 SQLite（文件降级保留） |
| `coin_pool_cache/latest.json` | AI500 币种池缓存 | 稳定，保持文件 |

---

## 并发模式

```
main goroutine
  ├── go news.Run(ctx)            # 新闻采集主循环
  ├── go news.RunOutlookLoop(ctx) # 宏观研判生成循环
  ├── go api.Start()             # Gin HTTP 服务器
  └── go trader.Run() × N        # 每个 AutoTrader 独立 goroutine
```

| 锁 | 类型 | 保护对象 |
|----|------|---------|
| `TraderManager.mu` | `sync.RWMutex` | `traders` map |
| `PositionTracker.mu` | `sync.RWMutex` | `activePositions` map |
| `DecisionLogger.metaMu` | `sync.RWMutex` | 日志元数据 |
| `news.Service`（内部） | `sync.RWMutex` | 新闻条目 + 摘要列表 |
| `learning.Manager.mu` | `sync.Mutex` | 学习状态文件读写 |

优雅退出：`SIGINT`/`SIGTERM` → `StopAll()` + `stopNews()`。每个 Trader goroutine 有 `recover()` 兜底，单个 panic 不影响整体。

---

## 部署

### Docker Compose（推荐）

```bash
cp config.json.example config.json  # 填入密钥
docker compose up -d
# backend → :8080，frontend → :3000，nginx → :80
```

### PM2（本地开发）

```bash
cp config.json.example config.json
go build -o main .
pm2 start pm2.config.js
```

### 环境变量（前端）

| 变量 | 用途 |
|------|------|
| `NOF1_API_BASE_URL` | 服务端请求后端（容器内部地址） |
| `NEXT_PUBLIC_NOF1_API_BASE_URL` | 浏览器端请求后端（公网地址或 /api/nof1） |

---

## 开发规范

### 新增 Trader 交易所支持

1. 在 `trader/` 下新建 `<exchange>_trader.go`，实现 `Trader` 接口
2. 在 `trader/auto_trader.go` 的 `newExchangeTrader()` 中注册 `case "<exchange>"`
3. 在 `config/config.go` 的 `TraderConfig` 中添加对应密钥字段
4. 更新 `config.json.example`

### 新增 AI Provider

1. 在 `mcp/client.go` 的 `NewClient()` 中添加 `case "<provider>"`，设置 BaseURL 和模型名
2. 在 `config/config.go` 的 `TraderConfig` 中添加 `<Provider>Key` 字段
3. 更新 `config.json.example`

### 修改决策逻辑

- **Prompt 内容** → 改 `decision/templates/system_prompt.tmpl`（固定约束）或 `decision/engine.go` 的 `buildUserPrompt()`（动态数据）
- **验证规则** → 改 `decision/engine.go` 的 `validateDecisions()`
- **新增市场指标** → 改 `market/` 下的计算函数 + `market.Format()` 序列化
- **提示词部署注意** → `decision/templates/system_prompt.tmpl` 通过 `go:embed` 编译进 backend，修改后必须重建 backend（仅 `restart` 不会生效）

### 修改 API 端点

路由在 `api/server.go` 的 `setupRoutes()`，Handler 与路由在同文件。新增端点需同步更新本文档「HTTP API」章节。

### 数据库操作

使用 `db.Open(path)` 获取 `*sql.DB`，在各业务模块内部自行管理表结构（`CREATE TABLE IF NOT EXISTS` 在模块 `New` 函数中执行）。

---

## 关键设计决策

| 决策 | 原因 |
|------|------|
| AI 全权决策，Go 只做合法性校验 | 最大化 AI 自主性，同时防止灾难性操作（爆仓等） |
| System Prompt（固定）+ User Prompt（动态）双层结构 | 分离角色定义与实时数据，便于独立调优 |
| Binance 公共 API 获取行情（无需密钥） | 降低配置复杂度，多交易所共用同一行情源 |
| `modernc.org/sqlite`（纯 Go，无 CGO） | 简化 Docker 构建，无需交叉编译工具链 |
| 学习状态注入 Prompt | 实现 AI 自我修正的正反馈闭环，不修改模型权重 |
| 三层降级的币种池 | 确保 API 故障时系统仍能正常运行 |
| 每 Trader 独立 goroutine + `recover()` | 单个 AI 策略崩溃不影响其他策略正常运行 |
