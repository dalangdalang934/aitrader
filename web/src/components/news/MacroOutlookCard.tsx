"use client";
import { useMemo, useState } from "react";
import {
  useMacroOutlook,
  type MacroOutlook,
  type MacroFactor,
} from "@/lib/api/hooks/useMacroOutlook";

const BIAS_CONFIG: Record<string, { label: string; color: string; bg: string }> = {
  bullish: { label: "看多", color: "text-green-400", bg: "bg-green-500/15" },
  bearish: { label: "看空", color: "text-red-400", bg: "bg-red-500/15" },
  neutral: { label: "中性", color: "text-zinc-400", bg: "bg-zinc-500/15" },
};

const RISK_CONFIG: Record<string, { label: string; color: string; bg: string }> = {
  low: { label: "低", color: "text-green-400", bg: "bg-green-500/20" },
  medium: { label: "中", color: "text-yellow-400", bg: "bg-yellow-500/20" },
  high: { label: "高", color: "text-orange-400", bg: "bg-orange-500/20" },
  extreme: { label: "极高", color: "text-red-400", bg: "bg-red-500/20" },
};

const DIRECTION_LABEL: Record<string, string> = {
  long: "偏多",
  short: "偏空",
  neutral: "中性",
};

const CATEGORY_LABEL: Record<string, string> = {
  fed_policy: "美联储",
  geopolitics: "地缘政治",
  crypto_regulation: "加密监管",
  stock_market: "美股",
  commodities: "大宗商品",
};

function BiasBar({ score }: { score: number }) {
  const pct = Math.min(100, Math.max(0, (score + 100) / 2));
  const barColor =
    score > 15 ? "bg-green-500" : score < -15 ? "bg-red-500" : "bg-zinc-500";

  return (
    <div className="flex items-center gap-2">
      <span className="shrink-0 text-xs tabular-nums" style={{ color: "var(--muted-text)" }}>
        -100
      </span>
      <div
        className="relative h-1.5 flex-1 rounded-full overflow-hidden"
        style={{ background: "var(--panel-border)" }}
      >
        <div
          className={`absolute left-0 top-0 h-full rounded-full transition-all ${barColor}`}
          style={{ width: `${pct}%` }}
        />
        <div
          className="absolute top-1/2 h-3 w-px -translate-y-1/2 bg-zinc-400"
          style={{ left: "50%" }}
        />
      </div>
      <span className="shrink-0 text-xs tabular-nums" style={{ color: "var(--muted-text)" }}>
        +100
      </span>
      <span className="ml-1 text-xs font-semibold tabular-nums" style={{ color: "var(--foreground)" }}>
        {score > 0 ? "+" : ""}
        {score}
      </span>
    </div>
  );
}

function FactorTag({ factor }: { factor: MacroFactor }) {
  const impactCfg = BIAS_CONFIG[factor.impact] ?? BIAS_CONFIG.neutral;
  const label = CATEGORY_LABEL[factor.category] ?? factor.category;

  return (
    <span
      className={`inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium ${impactCfg.bg} ${impactCfg.color}`}
      title={factor.description}
    >
      {label}: {factor.title}
    </span>
  );
}

function ValidityCountdown({ validUntil }: { validUntil: string }) {
  const label = useMemo(() => {
    const end = new Date(validUntil);
    if (Number.isNaN(end.getTime())) return "";
    return end.toLocaleString("zh-CN", {
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    });
  }, [validUntil]);

  if (!label) {
    return (
      <span className="text-xs text-orange-400">
        已过期，等待更新
      </span>
    );
  }
  return (
    <span className="text-xs tabular-nums" style={{ color: "var(--muted-text)" }}>
      有效至 {label}
    </span>
  );
}

function OutlookContent({ outlook }: { outlook: MacroOutlook }) {
  const bias = BIAS_CONFIG[outlook.overall_bias] ?? BIAS_CONFIG.neutral;
  const risk = RISK_CONFIG[outlook.risk_level] ?? RISK_CONFIG.medium;
  const rec = outlook.recommendations;
  const direction = DIRECTION_LABEL[rec?.preferred_direction] ?? "中性";

  const genTime = new Date(outlook.generated_at).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });

  const topFactors = (outlook.key_factors ?? [])
    .sort((a, b) => b.importance - a.importance)
    .slice(0, 5);

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-start gap-3">
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <span className={`rounded-full px-3 py-1 text-[11px] font-bold tracking-[0.18em] uppercase ${bias.bg} ${bias.color}`}>
              {bias.label}
            </span>
            <span className={`rounded-full px-3 py-1 text-[11px] font-medium tracking-[0.18em] uppercase ${risk.bg} ${risk.color}`}>
              风险 {risk.label}
            </span>
          </div>
          <span
            className="inline-flex rounded-full px-3 py-1 text-xs"
            style={{ background: "var(--logo-chip-bg)", color: "var(--muted-text)" }}
          >
            方向 {direction}
          </span>
        </div>
        <div className="ml-auto text-right">
          <div className="text-[11px] uppercase tracking-[0.18em]" style={{ color: "var(--muted-text)" }}>
            更新时间
          </div>
          <div className="mt-1 text-sm font-semibold tabular-nums" style={{ color: "var(--foreground)" }}>
            {genTime}
          </div>
          <div className="mt-1">
            <ValidityCountdown validUntil={outlook.valid_until} />
          </div>
        </div>
      </div>

      <div
        className="rounded-2xl border p-3"
        style={{
          background:
            "linear-gradient(135deg, color-mix(in srgb, var(--logo-chip-bg) 82%, transparent), transparent 72%)",
          borderColor: "color-mix(in srgb, var(--panel-border) 86%, transparent)",
        }}
      >
        <div className="mb-2 text-[11px] uppercase tracking-[0.18em]" style={{ color: "var(--muted-text)" }}>
          市场偏向
        </div>
        <BiasBar score={outlook.bias_score} />
      </div>

      <div
        className="rounded-2xl border p-4"
        style={{
          background: "color-mix(in srgb, var(--logo-chip-bg) 82%, transparent)",
          borderColor: "color-mix(in srgb, var(--panel-border) 82%, transparent)",
        }}
      >
        <div className="mb-2 text-[11px] uppercase tracking-[0.18em]" style={{ color: "var(--muted-text)" }}>
          宏观结论
        </div>
        <p className="text-sm leading-7" style={{ color: "var(--foreground)" }}>
          {outlook.summary}
        </p>
      </div>

      {topFactors.length > 0 && (
        <div>
          <div className="mb-2 text-[11px] uppercase tracking-[0.18em]" style={{ color: "var(--muted-text)" }}>
            关键因子
          </div>
          <div className="flex flex-wrap gap-1.5">
            {topFactors.map((f, i) => (
              <FactorTag key={`${f.category}-${i}`} factor={f} />
            ))}
          </div>
        </div>
      )}

      {rec && (
        <div
          className="rounded-2xl border p-3 text-xs leading-relaxed"
          style={{
            background: "linear-gradient(135deg, rgba(16,163,127,0.08), transparent 55%), var(--logo-chip-bg)",
            borderColor: "color-mix(in srgb, var(--panel-border) 82%, transparent)",
            color: "var(--muted-text)",
          }}
        >
          <div className="mb-2 text-[11px] uppercase tracking-[0.18em]" style={{ color: "var(--muted-text)" }}>
            执行建议
          </div>
          <div className="mb-2 flex flex-wrap gap-3">
            {rec.position_size_adj != null && rec.position_size_adj !== 1 && (
              <span>
                仓位系数{" "}
                <strong style={{ color: "var(--foreground)" }}>
                  {rec.position_size_adj.toFixed(2)}x
                </strong>
              </span>
            )}
            {rec.max_leverage_adj != null && rec.max_leverage_adj !== 1 && (
              <span>
                杠杆系数{" "}
                <strong style={{ color: "var(--foreground)" }}>
                  {rec.max_leverage_adj.toFixed(2)}x
                </strong>
              </span>
            )}
            {rec.focus_symbols?.length > 0 && (
              <span>
                关注{" "}
                <strong className="text-green-400">
                  {rec.focus_symbols.join(", ")}
                </strong>
              </span>
            )}
            {rec.avoid_symbols?.length > 0 && (
              <span>
                回避{" "}
                <strong className="text-red-400">
                  {rec.avoid_symbols.join(", ")}
                </strong>
              </span>
            )}
          </div>
          {rec.reasoning && <p>{rec.reasoning}</p>}
        </div>
      )}
    </div>
  );
}

export default function MacroOutlookCard({ compact = false }: { compact?: boolean }) {
  const { outlook, isLoading, isError } = useMacroOutlook();
  const [expanded, setExpanded] = useState(false);

  if (isLoading) {
    return (
      <div className="rounded-xl border px-4 py-3 animate-pulse" style={{ borderColor: "var(--panel-border)", background: "var(--panel-bg)" }}>
        <div className="h-3 w-48 rounded skeleton-bg" />
      </div>
    );
  }

  if (isError || !outlook) return null;

  // 完整模式（新闻 Tab 内）
  if (!compact) {
    return (
      <div
        className="dashboard-panel rounded-[20px] p-5"
        style={{ background: "var(--panel-bg)", borderColor: "var(--panel-border)", color: "var(--foreground)" }}
      >
        <div className="mb-3 flex items-center justify-between gap-3">
          <div>
            <div className="text-[10px] font-bold tracking-[0.22em] uppercase" style={{ color: "var(--muted-text)" }}>Macro Outlook</div>
            <div className="mt-1 text-base font-semibold">宏观基本面研判</div>
          </div>
          <div className="rounded-full border px-3 py-1 text-[11px] uppercase tracking-[0.18em]"
            style={{ borderColor: "color-mix(in srgb, var(--panel-border) 80%, transparent)", background: "color-mix(in srgb, var(--logo-chip-bg) 80%, transparent)", color: "var(--muted-text)" }}>
            AI Digest
          </div>
        </div>
        <OutlookContent outlook={outlook} />
      </div>
    );
  }

  // 紧凑模式（左栏顶部）
  const bias = BIAS_CONFIG[outlook.overall_bias] ?? BIAS_CONFIG.neutral;
  const risk = RISK_CONFIG[outlook.risk_level] ?? RISK_CONFIG.medium;
  const score = outlook.bias_score ?? 0;
  const pct = Math.min(100, Math.max(0, (score + 100) / 2));
  const barColor = score > 15 ? "bg-green-500" : score < -15 ? "bg-red-500" : "bg-zinc-500";
  const genTime = new Date(outlook.generated_at).toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" });

  return (
    <div
      className="rounded-xl border cursor-pointer select-none"
      style={{ borderColor: "color-mix(in srgb, var(--panel-border) 86%, transparent)", background: "color-mix(in srgb, var(--logo-chip-bg) 60%, transparent)" }}
      onClick={() => setExpanded((v) => !v)}
    >
      {/* 紧凑头条行 */}
      <div className="flex items-center gap-3 px-4 py-2.5">
        <span className="text-[10px] font-bold tracking-[0.2em] uppercase shrink-0" style={{ color: "var(--muted-text)" }}>宏观</span>
        <span className={`rounded-full px-2.5 py-0.5 text-[11px] font-bold tracking-[0.14em] shrink-0 ${bias.bg} ${bias.color}`}>{bias.label}</span>
        <span className={`rounded-full px-2 py-0.5 text-[11px] shrink-0 ${risk.bg} ${risk.color}`}>风险 {risk.label}</span>
        {/* 偏向分条 */}
        <div className="flex-1 flex items-center gap-1.5 min-w-0">
          <div className="relative h-1 flex-1 rounded-full overflow-hidden" style={{ background: "var(--panel-border)" }}>
            <div className={`absolute left-0 top-0 h-full rounded-full ${barColor}`} style={{ width: `${pct}%` }} />
            <div className="absolute top-1/2 h-2 w-px -translate-y-1/2 bg-zinc-500/60" style={{ left: "50%" }} />
          </div>
          <span className="text-[11px] font-semibold tabular-nums shrink-0" style={{ color: "var(--foreground)" }}>
            {score > 0 ? "+" : ""}{score}
          </span>
        </div>
        <span className="text-[11px] tabular-nums shrink-0" style={{ color: "var(--muted-text)" }}>{genTime}</span>
        <span className="text-[11px] shrink-0" style={{ color: "var(--muted-text)" }}>{expanded ? "▲" : "▼"}</span>
      </div>
      {/* 展开内容 */}
      {expanded && (
        <div className="border-t px-4 pb-4 pt-3" style={{ borderColor: "color-mix(in srgb, var(--panel-border) 60%, transparent)" }}
          onClick={(e) => e.stopPropagation()}>
          <OutlookContent outlook={outlook} />
        </div>
      )}
    </div>
  );
}

export { OutlookContent };
