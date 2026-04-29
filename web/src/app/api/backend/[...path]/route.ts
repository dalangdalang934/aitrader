import { NextRequest, NextResponse } from "next/server";

export const runtime = "nodejs";

const RAW_BASE = process.env.API_BASE_URL || "http://localhost:8080";
const BASE = RAW_BASE.endsWith("/") ? RAW_BASE.slice(0, -1) : RAW_BASE;
const API_BASE = BASE.endsWith("/api") ? BASE : `${BASE}/api`;

type CacheEntry = {
  data: unknown;
  timestamp: number;
};

// 添加缓存配置
const CACHE_DURATION = 3000; // 3秒缓存（更短，更实时）
const cache = new Map<string, CacheEntry>();

// 请求去重：防止相同请求并发
const pendingRequests = new Map<string, Promise<unknown>>();

function getCached<T>(key: string): T | null {
  const cached = cache.get(key);
  if (!cached) return null;
  if (Date.now() - cached.timestamp > CACHE_DURATION) {
    cache.delete(key);
    return null;
  }
  return cached.data as T;
}

function setCache<T>(key: string, data: T): void {
  cache.set(key, { data, timestamp: Date.now() });
  // 限制缓存大小，防止内存泄漏
  if (cache.size > 100) {
    const firstKey = cache.keys().next().value;
    if (firstKey) cache.delete(firstKey);
  }
}

const DEFAULT_HEADERS = {
  "access-control-allow-origin": "*",
  "cache-control": "no-store",
};

function jsonResponse(data: unknown, init?: ResponseInit) {
  return NextResponse.json(data, {
    ...init,
    headers: {
      ...DEFAULT_HEADERS,
      ...(init?.headers || {}),
    },
  });
}

async function fetchJSON<T>(path: string, init?: RequestInit, timeout = 8000): Promise<T> {
  const cacheKey = `fetch:${path}`;
  
  // 检查缓存
  const cached = getCached<T>(cacheKey);
  if (cached) return cached;
  
  // 检查是否有正在进行的相同请求（请求去重）
  const pending = pendingRequests.get(cacheKey);
  if (pending) return pending as Promise<T>;
  
  // 创建新请求
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), timeout);
  
  const requestPromise = (async () => {
    try {
      const res = await fetch(`${API_BASE}${path}`, {
        cache: "no-store",
        signal: controller.signal,
        headers: {
          'Connection': 'keep-alive', // 保持连接
          'Accept': 'application/json',
        },
        ...init,
      });
      clearTimeout(timeoutId);
      
      if (!res.ok) {
        throw new Error(`Request failed ${res.status}`);
      }
      const data = (await res.json()) as T;
      
      // 缓存成功的响应
      setCache(cacheKey, data);
      return data;
    } catch (error) {
      clearTimeout(timeoutId);
      if ((error as Error).name === 'AbortError') {
        console.error(`⏱️ Request timeout (${timeout}ms) for ${path}`);
      }
      throw error;
    } finally {
      pendingRequests.delete(cacheKey);
    }
  })();
  
  pendingRequests.set(cacheKey, requestPromise);
  return requestPromise;
}

interface StrategiesResponse {
  count: number;
  traders: Array<{
    trader_id: string;
    trader_name: string;
    ai_model: string;
    total_equity?: number;
    total_pnl?: number;
    total_pnl_pct?: number;
    position_count?: number;
    margin_used_pct?: number;
    call_count?: number;
    is_running?: boolean;
  }>;
}

interface AccountResponse {
  total_equity?: number;
  total_unrealized_pnl?: number;
  total_pnl?: number;
  total_pnl_pct?: number;
  available_balance?: number;
  wallet_balance?: number;
  position_count?: number;
  margin_used?: number;
  margin_used_pct?: number;
  initial_balance?: number;
}

interface PositionResponse {
  symbol: string;
  side?: string;
  entry_price?: number;
  mark_price?: number;
  quantity?: number;
  leverage?: number;
  unrealized_pnl?: number;
  unrealized_pnl_pct?: number;
  liquidation_price?: number;
  margin_used?: number;
  margin_type?: string;
  order_id?: number;
  risk_usd?: number;
  confidence?: number;
  entry_time?: number;
  exit_plan?: Record<string, unknown>;
  stop_loss?: number;
  take_profit?: number;
}

interface DecisionAction {
  action?: string;
  signal?: string;
  coin?: string;
  symbol?: string;
  price?: number;
  quantity?: number;
  leverage?: number;
  position_id?: string | number;
  profit_target?: number;
  target?: number;
  take_profit?: number;
  take_profit_price?: number;
  stop_loss?: number;
  stop_loss_price?: number;
  risk_usd?: number;
  risk?: number;
  invalidation_condition?: string;
  confidence?: number;
}

interface DecisionRecord {
  timestamp?: string;
  trader_id?: string;
  trader_name?: string;
  input_prompt?: string;
  prompt?: string;
  cot_trace?: string | Record<string, unknown>;
  cot_trace_summary?: string;
  summary?: string;
  decision_json?: string;
  decisions?: DecisionAction[];
  execution_log?: string[];
  cycle_number?: number;
}

interface StatisticsRecord {
  total_cycles?: number;
  successful_cycles?: number;
  failed_cycles?: number;
  total_open_positions?: number;
  total_close_positions?: number;
}

interface TradersResponse {
  traders?: Array<{
    trader_id: string;
    trader_name?: string;
    ai_model?: string;
  }>;
}

interface TradeOutcome {
  symbol?: string;
  side?: string;
  quantity?: number;
  leverage?: number;
  open_price?: number;
  close_price?: number;
  entry_price?: number;
  exit_price?: number;
  position_value?: number;
  margin_used?: number;
  pn_l?: number;
  pn_l_pct?: number;
  pnl_pct?: number;
  realized_net_pnl?: number;
  realized_gross_pnl?: number;
  total_commission_dollars?: number;
  duration?: string;
  open_time?: string | number;
  close_time?: string | number;
  entry_time?: string | number;
  exit_time?: string | number;
  was_stop_loss?: boolean;
  is_partial_close?: boolean; // 是否部分平仓
  open_quantity?: number; // 原始开仓数量
  close_note?: string;
  position_id?: string | number;
}

interface ExchangeTradesAPIResponse {
  exchange_trades?: TradeOutcome[];
}

interface SymbolPerformanceEntry {
  symbol?: string;
  total_trades?: number;
  winning_trades?: number;
  losing_trades?: number;
  win_rate?: number;
  total_pn_l?: number;
  avg_pn_l?: number;
}

interface PerformanceAPIResponse {
  total_trades?: number;
  winning_trades?: number;
  losing_trades?: number;
  win_rate?: number;
  avg_win?: number;
  avg_loss?: number;
  profit_factor?: number;
  sharpe_ratio?: number;
  recent_trades?: TradeOutcome[];
  symbol_stats?: Record<string, SymbolPerformanceEntry>;
  best_symbol?: string;
  worst_symbol?: string;
}

interface StatusPayload {
  exchange?: string;
  ai_provider?: string;
  is_running?: boolean;
  runtime_minutes?: number | string;
  call_count?: number | string;
  scan_interval?: string;
  stop_until?: string;
  last_reset_time?: string;
  position_count?: number | string;
}

interface AccountTotalsPositionRow {
  entry_oid: number;
  risk_usd: number;
  confidence: number;
  exit_plan: Record<string, unknown>;
  entry_time: number;
  symbol: string;
  entry_price: number;
  margin: number;
  leverage: number;
  quantity: number;
  current_price: number;
  unrealized_pnl: number;
  liquidation_price?: number;
  margin_type: string;
  side: string;
}

interface AccountTotalsSnapshotRow {
  model_id: string;
  id: string;
  timestamp: number;
  dollar_equity: number;
  account_value: number;
  return_pct: number;
  total_pnl: number;
  unrealized_pnl: number;
  positions: Record<string, AccountTotalsPositionRow>;
  initial_balance?: number;
  error?: boolean;
}

interface AggregatedTradeRow {
  id: string;
  symbol: string;
  model_id: string;
  side: "long" | "short";
  entry_price: number | null;
  exit_price: number | null;
  quantity: number | null;
  leverage: number | null;
  entry_time: number;
  exit_time: number;
  realized_net_pnl: number | null;
  realized_gross_pnl: number | null;
  total_commission_dollars: number | null;
  pnl_pct: number | null;
  margin_used: number | null;
  position_value: number | null;
  duration: string | null;
  was_stop_loss: boolean;
  is_partial_close: boolean;
  open_quantity: number | null;
  close_note: string | null;
  position_id?: string | number | null;
}

interface DecisionMapEntry {
  signal?: string;
  leverage?: number;
  profit_target?: number;
  stop_loss?: number;
  risk_usd?: number;
  invalidation_condition?: string;
  confidence?: number;
  quantity?: number;
  price?: number;
}

interface ConversationCard {
  model_id: string;
  timestamp: number;
  summary: string;
  user_prompt: string;
  cot_trace: string | Record<string, unknown>;
  llm_response: Record<string, DecisionMapEntry>;
  decisions: DecisionAction[];
  execution_log: string[];
}

interface StatusRow {
  model_id: string;
  model_name: string;
  ai_model: string;
  exchange: string | null;
  ai_provider: string | null;
  is_running: boolean;
  runtime_minutes: number;
  call_count: number;
  scan_interval: string | null;
  stop_until: string | null;
  last_reset_time: string | null;
  total_equity: number | null;
  total_pnl: number | null;
  total_pnl_pct: number | null;
  position_count: number;
  margin_used_pct: number | null;
}

interface LatestDecisionRecordRow {
  timestamp: number;
  cycle_number: number | null;
  summary: string;
  actions: DecisionAction[];
  execution_log: string[];
}

interface LatestDecisionsRow {
  model_id: string;
  model_name: string;
  ai_model: string;
  records: LatestDecisionRecordRow[];
}

interface LearningRow {
  model_id: string;
  model_name: string;
  ai_model: string;
  totals: {
    total_trades: number;
    winning_trades: number;
    losing_trades: number;
    win_rate: number;
    profit_factor: number;
    sharpe_ratio: number;
    avg_win: number;
    avg_loss: number;
  };
  symbol_stats: NormalizedSymbolStat[];
  recent_trades: TradeOutcome[];
  best_symbol: string | null;
  worst_symbol: string | null;
}

interface NormalizedSymbolStat {
  symbol: string;
  total_trades: number;
  winning_trades: number;
  losing_trades: number;
  win_rate: number;
  total_pn_l: number;
  avg_pn_l: number;
}

async function handleAccountTotals() {
  const startTime = Date.now();
  
  // 检查缓存
  const cacheKey = 'account-totals';
  const cached = getCached<{ accountTotals: AccountTotalsSnapshotRow[] }>(cacheKey);
  if (cached) {
    console.log(`✅ Cache hit for account-totals (${Date.now() - startTime}ms)`);
    return jsonResponse(cached);
  }

  const competition = await fetchJSON<StrategiesResponse>(`/competition`, undefined, 5000).catch(() => ({ traders: [], count: 0 }));
  const traders = competition.traders ?? [];

  const rows: AccountTotalsSnapshotRow[] = [];
  console.log(`⏱️ Fetching data for ${traders.length} traders...`);

  await Promise.all(
    traders.map(async (trader) => {
      const traderId = trader.trader_id;
      try {
        // 优化：并行但独立获取，减少超时影响
        const [account, positions] = await Promise.all([
          fetchJSON<AccountResponse>(`/account?trader_id=${encodeURIComponent(traderId)}`, undefined, 6000).catch<AccountResponse>(() => ({})),
          fetchJSON<PositionResponse[]>(`/positions?trader_id=${encodeURIComponent(traderId)}`, undefined, 6000).catch<PositionResponse[]>(() => []),
        ]);

        const nowTs = Date.now();
        
        // 只返回当前快照，不加载历史数据（减少延迟）
        const positionMap: Record<string, AccountTotalsPositionRow> = {};
        const posList = Array.isArray(positions) ? positions : [];
        posList.forEach((pos, index) => {
          if (!pos || !pos.symbol) return;
          const rawQty = Number(pos.quantity ?? 0);
          const symbol = String(pos.symbol).toUpperCase();
          const sideText = String(pos.side ?? "").toUpperCase();
          const qty = sideText.includes("SHORT")
            ? -Math.abs(rawQty)
            : Math.abs(rawQty);
          const entryPrice = Number(pos.entry_price ?? pos.mark_price ?? 0);
          const markPrice = Number(pos.mark_price ?? pos.entry_price ?? 0);
          
          const stopLossFromExchange = pos.stop_loss ?? undefined;
          const takeProfitFromExchange = pos.take_profit ?? undefined;
          
          const exitPlan: Record<string, unknown> = {
            ...(pos.exit_plan ?? {}),
          };
          if (stopLossFromExchange) {
            exitPlan.stop_loss = stopLossFromExchange;
          }
          if (takeProfitFromExchange) {
            exitPlan.profit_target = takeProfitFromExchange;
          }
          
          positionMap[symbol] = {
            entry_oid: pos.order_id ?? index,
            risk_usd: pos.risk_usd ?? Math.abs(rawQty) * Math.abs(markPrice),
            confidence: pos.confidence ?? 0,
            exit_plan: exitPlan,
            entry_time: pos.entry_time ?? Math.floor(nowTs / 1000),
            symbol,
            entry_price: entryPrice,
            margin: pos.margin_used ?? 0,
            leverage: pos.leverage ?? 0,
            quantity: qty,
            current_price: markPrice,
            unrealized_pnl: pos.unrealized_pnl ?? 0,
            liquidation_price: pos.liquidation_price ?? undefined,
            margin_type: pos.margin_type ?? "isolated",
            side: sideText || (qty >= 0 ? "LONG" : "SHORT"),
          };
        });

        rows.push({
          model_id: traderId,
          id: traderId,
          timestamp: nowTs,
          dollar_equity: account.total_equity ?? trader.total_equity ?? 0,
          account_value: account.total_equity ?? trader.total_equity ?? 0,
          return_pct: account.total_pnl_pct ?? trader.total_pnl_pct ?? 0,
          total_pnl: account.total_pnl ?? trader.total_pnl ?? 0,
          unrealized_pnl: account.total_unrealized_pnl ?? 0,
          positions: positionMap,
          initial_balance: account.initial_balance, // 传递初始余额用于百分比计算
        });
      } catch (error) {
        console.error(`❌ Failed to build account row for ${traderId}:`, error);
        // 即使失败也添加基本信息，避免丢失trader
        rows.push({
          model_id: traderId,
          id: traderId,
          timestamp: Date.now(),
          dollar_equity: trader.total_equity ?? 0,
          account_value: trader.total_equity ?? 0,
          return_pct: trader.total_pnl_pct ?? 0,
          total_pnl: trader.total_pnl ?? 0,
          unrealized_pnl: 0,
          positions: {},
          error: true,
        });
      }
    }),
  );

  const result = { accountTotals: rows };
  setCache(cacheKey, result); // 设置缓存
  
  const elapsed = Date.now() - startTime;
  console.log(`✅ account-totals completed in ${elapsed}ms (${traders.length} traders, ${rows.length} rows)`);
  
  return jsonResponse(result);
}

async function handleTrades() {
  const competition = await fetchJSON<StrategiesResponse>(`/competition`).catch(() => ({ traders: [], count: 0 }));
  const traders = competition.traders ?? [];
  const trades: AggregatedTradeRow[] = [];

  await Promise.all(
    traders.map(async (trader) => {
      const traderId = trader.trader_id;
      const perf = await fetchJSON<PerformanceAPIResponse>(`/performance?trader_id=${encodeURIComponent(traderId)}`).catch<PerformanceAPIResponse | null>(() => null);
      const recent = perf?.recent_trades ?? [];

      recent.forEach((item, idx) => {
        const row = buildTradeRow(traderId, item, idx);
        if (row) trades.push(row);
      });
    }),
  );

  trades.sort((a, b) => {
    const ta = Number(a.exit_time || a.entry_time || 0);
    const tb = Number(b.exit_time || b.entry_time || 0);
    return tb - ta;
  });

  return jsonResponse({ trades });
}

async function handleExchangeTrades(req: NextRequest) {
  const search = req.nextUrl.searchParams;
  const traderId = search.get("trader_id");
  const traders = await fetchStrategyTraders(traderId || undefined);
  if (!traders.length) {
    return jsonResponse({ exchange_trades: [] });
  }

  const rows: AggregatedTradeRow[] = [];

  await Promise.all(
    traders.map(async (trader) => {
      const id = trader.trader_id;
      const resp = await fetchJSON<ExchangeTradesAPIResponse>(`/exchange-trades?trader_id=${encodeURIComponent(id)}`).catch<ExchangeTradesAPIResponse | null>(() => null);
      const history = resp?.exchange_trades ?? [];

      history.forEach((item, idx) => {
        const row = buildTradeRow(id, item, idx, "exchange");
        if (row) rows.push(row);
      });
    }),
  );

  rows.sort((a, b) => {
    const ta = Number(a.exit_time || a.entry_time || 0);
    const tb = Number(b.exit_time || b.entry_time || 0);
    return tb - ta;
  });

  return jsonResponse({ exchange_trades: rows });
}

async function handleConversations() {
  const competition = await fetchJSON<StrategiesResponse>(`/competition`).catch(() => ({ traders: [], count: 0 }));
  const traders = competition.traders ?? [];
  const cards: ConversationCard[] = [];

  await Promise.all(
    traders.map(async (trader) => {
      const traderId = trader.trader_id;
      const decisions = await fetchJSON<DecisionRecord[]>(`/decisions?trader_id=${encodeURIComponent(traderId)}`).catch<DecisionRecord[]>(() => []);
      if (!decisions || !Array.isArray(decisions)) {
        return;
      }
      decisions.forEach((decision, idx) => {
        const ts = parseTimestamp(decision.timestamp, idx);
        const summary = pickSummary(decision);
        const publicPrompt = decision.input_prompt ?? decision.user_prompt ?? "";
        const cotTrace = decision.cot_trace ?? decision.cot_trace_summary ?? "";
        const llmResponse = buildDecisionMap(decision);

        if (!summary && !cotTrace && !Object.keys(llmResponse).length) {
          return;
        }

        cards.push({
          model_id: traderId,
          timestamp: ts,
          summary,
          user_prompt: publicPrompt,
          cot_trace: cotTrace,
          llm_response: llmResponse,
          decisions: decision.decisions ?? [],
          execution_log: decision.execution_log ?? [],
        });
      });
    }),
  );

  cards.sort((a, b) => Number(b.timestamp || 0) - Number(a.timestamp || 0));

  return jsonResponse({ conversations: cards });
}

async function fetchStrategyTraders(targetId?: string) {
  const competition = await fetchJSON<StrategiesResponse>(`/competition`).catch(() => ({ traders: [], count: 0 }));
  const traders = competition.traders ?? [];
  if (!targetId) return traders;
  return traders.filter((t) => t.trader_id === targetId);
}

async function handleStatus(req: NextRequest) {
  const search = req.nextUrl.searchParams;
  const traderId = search.get("trader_id");
  const traders = await fetchStrategyTraders(traderId || undefined);
  if (!traders.length) return jsonResponse({ status: [] });

  const rows: StatusRow[] = [];

  await Promise.all(
    traders.map(async (trader) => {
      const id = trader.trader_id;
      try {
        const status = await fetchJSON<StatusPayload>(`/status?trader_id=${encodeURIComponent(id)}`).catch<StatusPayload | null>(() => null);
        if (!status) return;
        rows.push({
          model_id: id,
          model_name: trader.trader_name,
          ai_model: trader.ai_model,
          exchange: status.exchange ?? null,
          ai_provider: status.ai_provider ?? null,
          is_running: !!status.is_running,
          runtime_minutes: Number(status.runtime_minutes ?? 0),
          call_count: Number(status.call_count ?? trader.call_count ?? 0),
          scan_interval: status.scan_interval ?? null,
          stop_until: status.stop_until ?? null,
          last_reset_time: status.last_reset_time ?? null,
          total_equity: numberOrNull(trader.total_equity),
          total_pnl: numberOrNull(trader.total_pnl),
          total_pnl_pct: numberOrNull(trader.total_pnl_pct),
          position_count: Number(trader.position_count ?? status.position_count ?? 0),
          margin_used_pct: numberOrNull(trader.margin_used_pct),
        });
      } catch (error) {
        console.error("Failed to load status", error);
      }
    }),
  );

  return jsonResponse({ status: rows });
}

async function handleStatistics(req: NextRequest) {
  const search = req.nextUrl.searchParams;
  const traderId = search.get("trader_id");
  const traders = await fetchStrategyTraders(traderId || undefined);
  if (!traders.length) return jsonResponse({ statistics: [] });

  const rows: Array<{
    model_id: string;
    model_name: string;
    ai_model: string;
  } & StatisticsRecord> = [];

  await Promise.all(
    traders.map(async (trader) => {
      const id = trader.trader_id;
      try {
        const stats = await fetchJSON<StatisticsRecord>(`/statistics?trader_id=${encodeURIComponent(id)}`).catch<StatisticsRecord | null>(() => null);
        if (!stats) return;
        rows.push({
          model_id: id,
          model_name: trader.trader_name,
          ai_model: trader.ai_model,
          ...stats,
        });
      } catch (error) {
        console.error("Failed to load statistics", error);
      }
    }),
  );

  return jsonResponse({ statistics: rows });
}

async function handleLatestDecisions(req: NextRequest) {
  const search = req.nextUrl.searchParams;
  const traderId = search.get("trader_id");
  const traders = await fetchStrategyTraders(traderId || undefined);
  if (!traders.length) return jsonResponse({ latest: [] });

  const rows: LatestDecisionsRow[] = [];

  await Promise.all(
    traders.map(async (trader) => {
      const id = trader.trader_id;
      try {
        const decisions = await fetchJSON<DecisionRecord[]>(`/decisions/latest?trader_id=${encodeURIComponent(id)}`).catch<DecisionRecord[]>(() => []);
        if (!decisions.length) return;
        rows.push({
          model_id: id,
          model_name: trader.trader_name,
          ai_model: trader.ai_model,
          records: decisions.map((decision) => ({
            timestamp: parseTimestamp(decision.timestamp),
            cycle_number: decision.cycle_number ?? null,
            summary: pickSummary(decision),
            actions: decision.decisions ?? [],
            execution_log: decision.execution_log ?? [],
          })),
        });
      } catch (error) {
        console.error("Failed to load latest decisions", error);
      }
    }),
  );

  return jsonResponse({ latest: rows });
}

async function handleTraders() {
  try {
    const traders = await fetchJSON<TradersResponse>(`/traders`).catch(() => ({ traders: [] }));
    return jsonResponse(traders ?? { traders: [] });
  } catch (error) {
    console.error("Failed to load traders", error);
    return jsonResponse({ traders: [] }, { status: 500 });
  }
}

async function handlePerformance(req: NextRequest) {
  const search = req.nextUrl.searchParams;
  const traderId = search.get("trader_id");
  if (traderId) {
    const target = `${API_BASE}/performance?trader_id=${encodeURIComponent(traderId)}`;
    const upstream = await fetch(target, { cache: "no-store" });
    const data = await safeJson<unknown>(upstream, {});
    return jsonResponse(data, { status: upstream.status });
  }

  const competition = await fetchJSON<StrategiesResponse>(`/competition`).catch(() => ({ traders: [], count: 0 }));
  const traders = competition.traders ?? [];
  const rows: LearningRow[] = [];

  await Promise.all(
    traders.map(async (trader) => {
      const traderId = trader.trader_id;
      try {
        const perf = await fetchJSON<PerformanceAPIResponse>(`/performance?trader_id=${encodeURIComponent(traderId)}`).catch<PerformanceAPIResponse | null>(() => null);
        if (!perf) return;
        rows.push({
          model_id: traderId,
          model_name: trader.trader_name,
          ai_model: trader.ai_model,
          totals: {
            total_trades: perf.total_trades ?? 0,
            winning_trades: perf.winning_trades ?? 0,
            losing_trades: perf.losing_trades ?? 0,
            win_rate: perf.win_rate ?? 0,
            profit_factor: perf.profit_factor ?? 0,
            sharpe_ratio: perf.sharpe_ratio ?? 0,
            avg_win: perf.avg_win ?? 0,
            avg_loss: perf.avg_loss ?? 0,
          },
          symbol_stats: normalizeSymbolStats(perf.symbol_stats),
          recent_trades: (perf.recent_trades ?? []).slice(0, 15),
          best_symbol: perf.best_symbol ?? null,
          worst_symbol: perf.worst_symbol ?? null,
        });
      } catch (error) {
        console.error("Failed to load performance", error);
      }
    }),
  );

  return jsonResponse({ learning: rows });
}

async function handleEquityHistory(req: NextRequest) {
  const traderId = req.nextUrl.searchParams.get("trader_id");
  if (!traderId) return jsonResponse({ error: "trader_id required" }, { status: 400 });
  interface RawEquityPoint { equity?: number; balance?: number; pnl?: number; recorded_at?: string; }
  const raw = await fetchJSON<RawEquityPoint[]>(`/equity-history?trader_id=${encodeURIComponent(traderId)}`).catch(() => [] as RawEquityPoint[]);
  const mapped = raw.map((p) => ({
    timestamp: p.recorded_at ?? new Date().toISOString(),
    total_equity: p.equity ?? 0,
    total_pnl: p.pnl ?? 0,
    total_pnl_pct: 0,
    position_count: 0,
    cycle_number: 0,
  }));
  return jsonResponse(mapped);
}

function parseTimestamp(value?: string, fallbackIdx?: number): number {
  if (!value) return Date.now() - (fallbackIdx ?? 0) * 1000;
  const parsed = Date.parse(value);
  if (Number.isFinite(parsed)) return parsed;
  const num = Number(value);
  if (Number.isFinite(num)) return num > 1e12 ? num : num * 1000;
  return Date.now();
}

function cleanString(input?: string): string {
  if (!input) return "";
  return String(input).replace(/\r/g, "").trim();
}


function pickSummary(decision: DecisionRecord): string {
  const raw =
    decision.cot_trace_summary ||
    decision.summary ||
    (decision.cot_trace && typeof decision.cot_trace === "string" && decision.cot_trace) ||
    (decision.execution_log && decision.execution_log.join("\n")) ||
    "";
  const text = cleanString(raw as string);
  if (!text) return "";
  if (text.length <= 360) return text;
  return `${text.slice(0, 357)}...`;
}

function buildDecisionMap(decision: DecisionRecord): Record<string, DecisionMapEntry> {
  const map: Record<string, DecisionMapEntry> = {};

  const parseArray = (): DecisionAction[] => {
    const raw = decision.decision_json;
    if (!raw) return [];
    try {
      const parsed = JSON.parse(raw) as unknown;
      return Array.isArray(parsed) ? parsed : [];
    } catch {
      return [];
    }
  };

  const fromJson = parseArray();
  const source = fromJson.length ? fromJson : (decision.decisions ?? []);

  source.forEach((item) => {
    if (!item) return;
    const k = String(item.symbol || item.coin || "").trim();
    if (!k) return;
    const symbol = k.toUpperCase();
    const signal = normalizeSignal(item.action || item.signal);
    const entry: DecisionMapEntry = {
      signal,
      leverage: item.leverage != null ? Number(item.leverage) : undefined,
      profit_target:
        item.profit_target ?? item.target ?? item.take_profit ?? item.take_profit_price,
      stop_loss: item.stop_loss ?? item.stop_loss_price,
      risk_usd: item.risk_usd ?? item.risk,
      invalidation_condition: item.invalidation_condition,
      confidence: item.confidence != null ? Number(item.confidence) : undefined,
      quantity: item.quantity != null ? Number(item.quantity) : undefined,
      price: item.price != null ? Number(item.price) : undefined,
    };
    map[symbol] = entry;
  });

  return map;
}

function normalizeSignal(signal?: string): string | undefined {
  if (!signal) return undefined;
  const k = signal.toString().toLowerCase();
  if (k.includes("long") || k.includes("buy")) return "long";
  if (k.includes("short") || k.includes("sell")) return "short";
  if (k.includes("hold") || k.includes("wait")) return "hold";
  return signal;
}

// 获取默认traderId
async function getDefaultTraderId() {
  // 先尝试从环境变量获取
  const envTraderId = process.env.NEXT_PUBLIC_DEFAULT_TRADER_ID;
  if (envTraderId) return envTraderId;
  
  // 否则从traders列表获取第一个
  const fallbackId = "binance_deepseek";
  try {
    const traders = await fetchJSON<TradersResponse | Array<{ trader_id?: string }>>("/traders");
    if (Array.isArray(traders) && traders.length > 0) {
      return traders[0]?.trader_id || fallbackId;
    }
    if (
      !Array.isArray(traders) &&
      traders &&
      Array.isArray(traders.traders) &&
      traders.traders.length > 0
    ) {
      return traders.traders[0]?.trader_id || fallbackId;
    }
  } catch {
    // ignore and fallback
  }
  return fallbackId;
}

async function safeJson<T>(resp: Response, fallback: T): Promise<T> {
  try {
    return (await resp.json()) as T;
  } catch {
    return fallback;
  }
}

// 处理活跃仓位
async function handleActivePositions(req: NextRequest) {
  const traderId = req.nextUrl.searchParams.get("trader_id") || await getDefaultTraderId();
  const url = `${API_BASE}/positions/active?trader_id=${traderId}`;
  
  const resp = await fetch(url, {
    cache: "no-store",
  });

  if (!resp.ok) {
    return NextResponse.json(
      { error: "Failed to fetch active positions" },
      { status: resp.status }
    );
  }

  const data = await safeJson<unknown[]>(resp, []);
  return NextResponse.json(data);
}

// 处理仓位历史
async function handlePositionHistory(req: NextRequest) {
  const traderId = req.nextUrl.searchParams.get("trader_id") || await getDefaultTraderId();
  const url = `${API_BASE}/positions/history?trader_id=${traderId}`;
  
  const resp = await fetch(url, {
    cache: "no-store",
  });

  if (!resp.ok) {
    return NextResponse.json(
      { error: "Failed to fetch position history" },
      { status: resp.status }
    );
  }

  const data = await safeJson<unknown[]>(resp, []);
  return NextResponse.json(data);
}

// 处理仓位详情
async function handlePositionDetail(req: NextRequest, positionId: string) {
  const traderId = req.nextUrl.searchParams.get("trader_id") || await getDefaultTraderId();
  const url = `${API_BASE}/positions/${positionId}?trader_id=${traderId}`;
  
  const resp = await fetch(url, {
    cache: "no-store",
  });

  if (!resp.ok) {
    return NextResponse.json(
      { error: "Failed to fetch position detail" },
      { status: resp.status }
    );
  }

  const data = await safeJson<unknown>(resp, {});
  return NextResponse.json(data);
}

export async function GET(req: NextRequest, ctx: { params: Promise<{ path: string[] }> }) {
  const { path } = await ctx.params;
  const parts = (path || []).filter(Boolean);
  const subpath = parts.join("/");

  try {
    if (parts[0] === "account-totals") {
      return await handleAccountTotals();
    }
    if (parts[0] === "trades") {
      return await handleTrades();
    }
    if (parts[0] === "exchange-trades") {
      return await handleExchangeTrades(req);
    }
    if (parts[0] === "positions" && parts[1] === "active") {
      return await handleActivePositions(req);
    }
    if (parts[0] === "positions" && parts[1] === "history") {
      return await handlePositionHistory(req);
    }
    if (parts[0] === "positions" && parts[1]) {
      return await handlePositionDetail(req, parts[1]);
    }
    if (parts[0] === "conversations") {
      return await handleConversations();
    }
    if (parts[0] === "status") {
      return await handleStatus(req);
    }
    if (parts[0] === "statistics") {
      return await handleStatistics(req);
    }
    if (parts[0] === "traders") {
      return await handleTraders();
    }
    if (parts[0] === "decisions" && parts[1] === "latest") {
      return await handleLatestDecisions(req);
    }
    if (parts[0] === "performance") {
      return await handlePerformance(req);
    }
    if (parts[0] === "equity-history") {
      return await handleEquityHistory(req);
    }

    const STUB_ROUTES: Record<string, unknown> = {
      leaderboard: { leaderboard: [] },
      analytics: {},
      "since-inception-values": { values: [] },
      "crypto-prices": { prices: [] },
    };
    if (STUB_ROUTES[subpath] !== undefined) {
      return jsonResponse(STUB_ROUTES[subpath]);
    }
  } catch (error) {
    console.error(`Aggregate handler failed for ${subpath}`, error);
    return jsonResponse({ error: (error as Error).message ?? "internal error" }, { status: 500 });
  }

  // Fallback proxy behaviour for any other paths
  const target = `${API_BASE}/${subpath}${req.nextUrl.search}`;
  try {
  const upstream = await fetch(target, {
    cache: "no-store",
    headers: {
        Accept: "application/json",
    },
  });
    const data = await safeJson<unknown>(upstream, {});
    return jsonResponse(data, { status: upstream.status });
  } catch (error) {
    console.error(`Proxy fetch failed for ${target}`, error);
    return jsonResponse({ error: 'failed to proxy request' }, { status: 502 });
  }
}

export async function OPTIONS() {
  return new NextResponse(null, {
    headers: {
      ...DEFAULT_HEADERS,
      "access-control-allow-methods": "GET,OPTIONS",
      "access-control-allow-headers": "*",
    },
  });
}

function buildTradeRow(
  modelId: string,
  item: TradeOutcome,
  index: number,
  prefix?: string,
): AggregatedTradeRow | null {
  const symbol = String(item.symbol ?? "").toUpperCase();
  if (!symbol) return null;
  const entryTs = toUnixSeconds(item.entry_time ?? item.open_time, index);
  const exitTs = toUnixSeconds(item.exit_time ?? item.close_time, index);
  const idPrefix = prefix ? `${modelId}-${prefix}` : modelId;
  return {
    id: `${idPrefix}-${exitTs || entryTs || Date.now()}-${index}`,
    symbol,
    model_id: modelId,
    side: normalizeTradeSide(item.side),
    entry_price: numberOrNull(item.entry_price ?? item.open_price),
    exit_price: numberOrNull(item.exit_price ?? item.close_price),
    quantity: numberOrNull(item.quantity),
    leverage: numberOrNull(item.leverage),
    entry_time: entryTs,
    exit_time: exitTs,
    realized_net_pnl: numberOrNull(item.realized_net_pnl ?? item.pn_l),
    realized_gross_pnl: numberOrNull(
      item.realized_gross_pnl ?? item.realized_net_pnl ?? item.pn_l,
    ),
    total_commission_dollars: numberOrNull(item.total_commission_dollars),
    pnl_pct: numberOrNull(item.pnl_pct ?? item.pn_l_pct),
    margin_used: numberOrNull(item.margin_used),
    position_value: numberOrNull(item.position_value),
    duration: item.duration || null,
    was_stop_loss: !!item.was_stop_loss,
    is_partial_close: item.is_partial_close ?? false,
    open_quantity: numberOrNull(item.open_quantity),
    close_note: item.close_note ?? null,
    position_id: item.position_id ?? null,
  };
}

function normalizeTradeSide(side?: string): "long" | "short" {
  const k = (side || "long").toString().toLowerCase();
  if (k.includes("short")) return "short";
  return "long";
}

function toUnixSeconds(value?: string | number, fallbackIndex?: number): number {
  if (value == null) {
    // ensure unique sorting fallback
    return Math.floor((Date.now() - (fallbackIndex ?? 0)) / 1000);
  }
  if (typeof value === "number") {
    return value > 1e12 ? Math.floor(value / 1000) : Math.floor(value);
  }
  const parsed = Date.parse(value);
  if (!Number.isNaN(parsed)) {
    return Math.floor(parsed / 1000);
  }
  const num = Number(value);
  if (Number.isFinite(num)) {
    return num > 1e12 ? Math.floor(num / 1000) : Math.floor(num);
  }
  return Math.floor((Date.now() - (fallbackIndex ?? 0)) / 1000);
}

function numberOrNull(v: unknown): number | null {
  const n = typeof v === "string" ? Number(v) : (v as number);
  if (Number.isFinite(n)) return n;
  return null;
}

function normalizeSymbolStats(
  stats?: Record<string, SymbolPerformanceEntry | undefined>,
): NormalizedSymbolStat[] {
  if (!stats) return [];
  const arr: NormalizedSymbolStat[] = [];
  for (const [symbol, entry] of Object.entries(stats)) {
    if (!entry) continue;
    const baseSymbol = entry.symbol ?? symbol;
    const normSymbol = baseSymbol ? String(baseSymbol).toUpperCase() : "";
    arr.push({
      symbol: normSymbol,
      total_trades: entry.total_trades ?? 0,
      winning_trades: entry.winning_trades ?? 0,
      losing_trades: entry.losing_trades ?? 0,
      win_rate: entry.win_rate ?? 0,
      total_pn_l: entry.total_pn_l ?? 0,
      avg_pn_l: entry.avg_pn_l ?? 0,
    });
  }
  arr.sort((a, b) => Number(b.total_pn_l || 0) - Number(a.total_pn_l || 0));
  return arr;
}
