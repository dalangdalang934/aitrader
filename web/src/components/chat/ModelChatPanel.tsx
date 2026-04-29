"use client";

import {
  useConversations,
  type ExecutedOrder,
} from "@/lib/api/hooks/useConversations";
import { useMemo, useState } from "react";
import { getModelName, getModelColor } from "@/lib/model/meta";
import { ModelLogoChip } from "@/components/shared/ModelLogo";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { useTraderRegistry } from "@/lib/api/hooks/useTraderRegistry";

type ModelOption = { id: string; label: string };

type ChatRow = {
  model_id: string;
  timestamp: number | string;
  cot_trace?: unknown;
  decisions?: ExecutedOrder[];
};

function hasRealOrders(row: ChatRow) {
  return (row.decisions ?? []).some((d) => d.action && d.action !== "hold" && d.action !== "wait");
}

const markdownComponents: Components = {
  a: (props) => <a {...props} style={{ color: "var(--brand-accent)" }} />,
  code: ({ className, children, ...props }) => (
    <code className={className} style={!className ? {
      background: "var(--logo-chip-bg)", border: "1px solid var(--logo-chip-ring)",
      borderRadius: 4, padding: "0 0.3rem",
    } : undefined} {...props}>{children}</code>
  ),
  pre: ({ children, ...props }) => (
    <pre className="overflow-x-auto rounded-lg border p-3" style={{
      background: "color-mix(in srgb, var(--logo-chip-bg) 62%, transparent)",
      borderColor: "var(--panel-border)",
    }} {...props}>{children}</pre>
  ),
  li: (props) => <li {...props} className="ml-4 list-disc" />,
  ul: (props) => <ul {...props} className="my-2" />,
  ol: (props) => <ol {...props} className="my-2 ml-4 list-decimal" />,
  p: (props) => <p {...props} className="my-2" />,
};

export default function ModelChatPanel() {
  const { items, isLoading, isError } = useConversations();
  const { traders } = useTraderRegistry();
  const [qModel, setQModel] = useState("ALL");
  const [onlyTrades, setOnlyTrades] = useState(false);

  const all = useMemo(() => {
    const arr: ChatRow[] = [];
    const seen = new Set<string>();
    for (const it of items) {
      const id = it.model_id;
      if (!id) continue;
      const ts = it.timestamp ?? it.inserted_at ?? 0;
      const key = `${id}::${ts}`;
      if (seen.has(key)) continue;
      seen.add(key);
      arr.push({ model_id: id, timestamp: ts, cot_trace: it.cot_trace ?? {}, decisions: it.decisions });
    }
    arr.sort((a, b) => Number(b.timestamp) - Number(a.timestamp));
    return arr;
  }, [items]);

  const list = useMemo(() => {
    let filtered = qModel === "ALL" ? all : all.filter((r) => r.model_id === qModel);
    if (onlyTrades) filtered = filtered.filter(hasRealOrders);
    return filtered;
  }, [all, qModel, onlyTrades]);

  const options = useMemo(() => {
    const seen = new Set<string>();
    const opts: ModelOption[] = traders
      .map((t) => {
        if (!t.model_id) return null;
        seen.add(t.model_id);
        return { id: t.model_id, label: t.model_name || getModelName(t.model_id) };
      })
      .filter((o): o is ModelOption => o !== null);
    for (const c of items) {
      if (!c.model_id || seen.has(c.model_id)) continue;
      seen.add(c.model_id);
      opts.push({ id: c.model_id, label: getModelName(c.model_id) });
    }
    opts.sort((a, b) => a.label.localeCompare(b.label));
    return [{ id: "ALL", label: "全部模型" }, ...opts];
  }, [traders, items]);

  if (isLoading) return <div className="tab-surface tab-empty">加载中…</div>;
  if (isError) return <div className="tab-surface tab-empty" style={{ color: "#ef4444" }}>接口暂不可用</div>;

  return (
    <div className="flex flex-col gap-3" style={{ height: "100%" }}>
      <div className="tab-surface shrink-0">
        <div className="tab-filterbar">
          <div>
            <div className="tab-toolbar-label">Model Feed</div>
            <div className="tab-toolbar-title">决策日志</div>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              className="tab-chip"
              style={onlyTrades ? {
                background: "color-mix(in oklab, #16a34a 15%, transparent)",
                color: "#16a34a",
                border: "1px solid color-mix(in oklab, #16a34a 40%, transparent)",
              } : undefined}
              onClick={() => setOnlyTrades((v) => !v)}
            >
              {onlyTrades ? "有交易 ✓" : "全部"}
            </button>
            <label className="tab-filter-chip">
              <span className="tab-filter-label">模型</span>
              <select className="tab-select" value={qModel} onChange={(e) => setQModel(e.target.value || "ALL")}>
                {options.map((o) => <option key={o.id} value={o.id}>{o.label}</option>)}
              </select>
            </label>
          </div>
        </div>
      </div>

      <div className="flex-1 min-h-0 overflow-y-auto space-y-2 pr-1" style={{ maxHeight: "calc(100vh - 220px)" }}>
        {!list.length && (
          <div className="tab-surface tab-empty">{onlyTrades ? "暂无含交易的决策" : "暂无决策记录"}</div>
        )}
        {list.map((row, idx) => (
          <ChatCard key={`${row.model_id}:${row.timestamp}:${idx}`} row={row} />
        ))}
      </div>
    </div>
  );
}

function ChatCard({ row }: { row: ChatRow }) {
  const [open, setOpen] = useState(false);
  const color = getModelColor(row.model_id);
  const hasOrders = orders.length > 0;

  return (
    <article className="tab-surface px-4 py-3">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-3 min-w-0">
          <ModelLogoChip modelId={row.model_id} size="md" />
          <div className="min-w-0">
            <div className="ui-sans text-[11px] font-semibold uppercase tracking-[0.22em]" style={{ color }}>
              {getModelName(row.model_id)}
            </div>
            <time className="tab-toolbar-note">{fmtTime(row.timestamp)}</time>
          </div>
        </div>
        <button type="button" className="tab-chip shrink-0" onClick={() => setOpen((v) => !v)}>
          {open ? "收起" : "思维链"}
        </button>
      </div>

      {open && (
        <div className="mt-3 border-t pt-3 max-h-[400px] overflow-y-auto" style={{ borderColor: "color-mix(in srgb, var(--panel-border) 50%, transparent)" }}>
          <CotBlock cot={row.cot_trace} />
        </div>
      )}
    </article>
  );
}

function OrderChip({ order }: { order: ExecutedOrder }) {
  const isLong = order.action?.includes("long") || order.action?.includes("buy");
  const isClose = order.action?.includes("close");
  const success = order.success !== false;

  const label = actionZh(order.action);
  const fg = isClose ? "var(--muted-text)" : isLong ? "#16a34a" : "#ef4444";
  const bg = isClose
    ? "color-mix(in srgb, var(--logo-chip-bg) 80%, transparent)"
    : isLong ? "color-mix(in oklab, #16a34a 12%, transparent)" : "color-mix(in oklab, #ef4444 12%, transparent)";
  const border = isClose
    ? "color-mix(in srgb, var(--panel-border) 70%, transparent)"
    : isLong ? "color-mix(in oklab, #16a34a 40%, transparent)" : "color-mix(in oklab, #ef4444 40%, transparent)";

  return (
    <div className="flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-[11px]" style={{ border: `1px solid ${border}`, background: bg }}>
      <span className="font-semibold" style={{ color: fg }}>{label}</span>
      <span className="font-mono font-medium" style={{ color: "var(--foreground)" }}>{order.symbol?.replace("USDT", "")}</span>
      {order.leverage != null && <span style={{ color: "var(--muted-text)" }}>{order.leverage}x</span>}
      {!success && <span className="rounded px-1 text-[10px]" style={{ background: "rgba(239,68,68,0.15)", color: "#ef4444" }}>失败</span>}
    </div>
  );
}

function CotBlock({ cot }: { cot?: unknown }) {
  const text = useMemo(() => {
    if (!cot) return "";
    if (typeof cot === "string") return normalizeMd(cot);
    try { return normalizeMd(JSON.stringify(cot, null, 2)); } catch { return ""; }
  }, [cot]);

  if (!text) return <span style={{ color: "var(--muted-text)" }}>—</span>;

  return (
    <div className="terminal-text text-xs leading-relaxed" style={{ color: "var(--foreground)" }}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>{text}</ReactMarkdown>
    </div>
  );
}

function actionZh(action?: string) {
  if (!action) return "—";
  const k = action.toLowerCase();
  if (k.includes("open_long") || k === "buy" || k === "long") return "开多";
  if (k.includes("open_short") || k === "sell" || k === "short") return "开空";
  if (k.includes("close_long")) return "平多";
  if (k.includes("close_short")) return "平空";
  return action;
}

function fmtTime(value?: number | string) {
  if (!value) return "";
  const numeric = typeof value === "string" ? Number(value) : value;
  const ms = numeric > 1e12 ? numeric : numeric * 1000;
  const d = new Date(ms);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getMonth() + 1)}/${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

function normalizeMd(source?: string): string {
  if (!source) return "";
  let text = String(source);
  if (/^"[\s\S]*"$/.test(text) && /\\n|\\t|\\r/.test(text)) {
    try { text = JSON.parse(text); } catch { }
  }
  text = text.replace(/\\n/g, "\n").replace(/\\t/g, "\t").replace(/\\r/g, "\r");
  if (text.length > 1 && text.startsWith('"') && text.endsWith('"')) text = text.slice(1, -1);
  return text;
}
