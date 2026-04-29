"use client";
import { useCallback, useMemo, useState } from "react";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
  ReferenceLine,
  ResponsiveContainer,
} from "recharts";
import {
  useAccountValueSeries,
} from "@/lib/api/hooks/useAccountValueSeries";
import {
  useAccountTotals,
  type AccountTotalsRow,
} from "@/lib/api/hooks/useAccountTotals";
import { format } from "date-fns";
import {
  getModelColor,
  getModelName,
  getModelIcon,
  resolveCanonicalId,
} from "@/lib/model/meta";
import { adjustLuminance } from "@/lib/ui/useDominantColors";
import ErrorBanner from "@/components/ui/ErrorBanner";
import { SkeletonBlock } from "@/components/ui/Skeleton";

type Range = "ALL" | "24H";
type Mode = "$" | "%";

interface ChartDataPoint {
  timestamp: Date;
  [modelId: string]: Date | number | undefined;
}

interface EndDotProps {
  cx?: number;
  cy?: number;
  index?: number;
}

const MAX_POINTS_24H = 600;

/* ── helpers ── */

function hexToRGB(hex: string) {
  const m = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(hex.trim());
  if (!m) return null;
  return { r: parseInt(m[1], 16), g: parseInt(m[2], 16), b: parseInt(m[3], 16) };
}

function relativeLuminance(hex: string) {
  const rgb = hexToRGB(hex);
  if (!rgb) return null;
  const toLin = (c: number) => { const s = c / 255; return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4); };
  return 0.2126 * toLin(rgb.r) + 0.7152 * toLin(rgb.g) + 0.0722 * toLin(rgb.b);
}

function getStrokeColor(id: string) {
  const base = getModelColor(id);
  const lum = relativeLuminance(base);
  if (lum == null) return base;
  if (resolveCanonicalId(id) === "grok-4") return `var(--brand-grok-4-stroke, ${base})`;
  if (lum < 0.06) return `var(--line-too-dark-fallback, ${base})`;
  if (lum > 0.94) return `var(--line-too-light-fallback, ${base})`;
  return base;
}

const cssSafe = (s: string) => s.toLowerCase().replace(/[^a-z0-9_-]+/g, "-");

/* ── Component ── */

export default function AccountValueChart() {
  const { series, modelIds, isLoading, isError } = useAccountValueSeries();
  const { data: totalsData } = useAccountTotals();
  const [range, setRange] = useState<Range>("ALL");
  const [mode, setMode] = useState<Mode>("$");
  const [cutoffTime, setCutoffTime] = useState<number | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const ids = useMemo(() => Array.from(new Set(modelIds)), [modelIds]);

  const vw = typeof window !== "undefined" ? window.innerWidth : 1024;
  const endLogoBaseSize = vw < 380 ? 21 : vw < 640 ? 27 : vw < 1024 ? 42 : 44;
  const endLogoSize = Math.round((endLogoBaseSize * 2) / 3);
  const marginFactor = vw < 380 ? 1.2 : vw < 640 ? 1.35 : vw < 1024 ? 1.6 : 1.7;
  const chartRightMargin = Math.max(64, Math.round(endLogoSize * marginFactor));

  const dataRows = useMemo<ChartDataPoint[]>(() => {
    const sorted = [...series].sort((a, b) => a.timestamp - b.timestamp);
    return sorted.map((point) => {
      const row: ChartDataPoint = { timestamp: new Date(point.timestamp) };
      for (const [key, value] of Object.entries(point)) {
        if (key !== "timestamp" && typeof value === "number") row[key] = value;
      }
      return row;
    });
  }, [series]);

  const { data, models } = useMemo(() => {
    let points = dataRows;
    if (range === "24H" && points.length) {
      points = points.filter((p) => p.timestamp.getTime() >= (cutoffTime ?? 0));
    }
    if (mode === "%") {
      const rows: AccountTotalsRow[] = totalsData?.accountTotals ?? [];
      const initialBalanceByModel = new Map<string, { balance: number; timestamp: number }>();
      for (const row of rows) {
        const modelId = row?.model_id ?? row?.id;
        if (modelId && typeof row.initial_balance === "number" && row.initial_balance > 0) {
          const rowTs = Number(row.timestamp ?? 0);
          const existing = initialBalanceByModel.get(modelId);
          if (!existing || rowTs > existing.timestamp) {
            initialBalanceByModel.set(modelId, { balance: row.initial_balance, timestamp: rowTs });
          }
        }
      }
      const bases: Record<string, number> = {};
      for (const id of ids) {
        const d = initialBalanceByModel.get(id);
        if (d && d.balance > 0) { bases[id] = d.balance; }
        else {
          const fb = points.find((p) => typeof p[id] === "number")?.[id];
          bases[id] = typeof fb === "number" && fb > 0 ? fb : 1;
        }
      }
      points = points.map((p) => {
        const cp: ChartDataPoint = { ...p };
        for (const id of ids) {
          const v = p[id];
          if (typeof v === "number" && bases[id] > 0) cp[id] = (v / bases[id] - 1) * 100;
        }
        return cp;
      });
    }
    if (range === "24H" && points.length > MAX_POINTS_24H) {
      const step = Math.ceil(points.length / MAX_POINTS_24H);
      const sampled: ChartDataPoint[] = [];
      for (let i = 0; i < points.length; i += step) sampled.push(points[i]);
      if (sampled[sampled.length - 1] !== points[points.length - 1]) sampled.push(points[points.length - 1]);
      points = sampled;
    }
    return { data: points, models: ids };
  }, [dataRows, ids, range, mode, totalsData, cutoffTime]);

  const isSeriesVisible = (id: string) => !selectedId || selectedId === id;

  const lastIdxById = useMemo(() => {
    const m: Record<string, number> = {};
    for (const id of models) {
      for (let i = data.length - 1; i >= 0; i--) {
        if (typeof data[i]?.[id] === "number") { m[id] = i; break; }
      }
    }
    return m;
  }, [models, data]);

  const lastValById = useMemo(() => {
    const m: Record<string, number | undefined> = {};
    for (const id of models) {
      const idx = lastIdxById[id];
      if (typeof idx === "number") {
        const v = data[idx]?.[id];
        if (typeof v === "number") m[id] = v;
      }
    }
    return m;
  }, [models, data, lastIdxById]);

  const formatValue = useCallback((v: number | undefined) => {
    if (typeof v !== "number") return "--";
    if (mode === "%") return `${v >= 0 ? "+" : ""}${v.toFixed(1)}%`;
    try {
      return `$${Number(v).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
    } catch {
      return `$${Math.round(Number(v) * 100) / 100}`;
    }
  }, [mode]);

  const renderEndDot = (id: string) => {
    const EndDot = ({ cx, cy, index }: EndDotProps) => {
      if (cx == null || cy == null) return <g key={`e-${id}-${index}`} />;
      if (typeof lastIdxById[id] !== "number" || index !== lastIdxById[id]) return <g key={`e-${id}-${index}`} />;
      if (!isSeriesVisible(id)) return <g key={`e-${id}-${index}`} />;
      const icon = getModelIcon(id);
      const color = getModelColor(id);
      const bg = color || "var(--chart-logo-bg)";
      const ring = typeof bg === "string" && bg.startsWith("#") ? adjustLuminance(bg, -0.15) : "var(--chart-logo-ring)";
      const size = endLogoSize;
      const haloR = Math.round((endLogoBaseSize / 3) * (2 / 3));
      const valueStr = formatValue(lastValById[id]);
      const fontSize = vw < 380 ? 11 : vw < 640 ? 12 : 13;
      const chipPadX = 8;
      const charW = Math.round(fontSize * 0.62);
      const chipH = fontSize + 8;
      const chipW = valueStr.length * charW + chipPadX * 2;
      return (
        <g key={`${id}-dot-${index}`} transform={`translate(${cx}, ${cy})`} style={{ cursor: "pointer" }}>
          <g style={{ transform: "scale(1)", transformBox: "fill-box", transformOrigin: "50% 50%", transition: "transform 160ms ease" }}>
            <circle r={haloR} className="animate-ping" fill={color} opacity={0.075} />
            <circle r={Math.round(size * 0.55)} fill={bg} stroke={ring} strokeWidth={1} />
            {icon ? (
              <image href={icon} x={-size / 2} y={-size / 2} width={size} height={size}
                focusable="false" tabIndex={-1} preserveAspectRatio="xMidYMid meet"
                style={{ filter: "drop-shadow(0 0 2px rgba(0,0,0,0.6))", pointerEvents: "none" }} />
            ) : (
              <circle r={Math.max(6, Math.round(size * 0.38))} fill={color} />
            )}
          </g>
          {isSeriesVisible(id) && (
            <g transform={`translate(${Math.round(size * 0.7) + 8}, ${-Math.round(chipH / 2)})`} style={{ pointerEvents: "none" }}>
              <rect rx={6} ry={6} width={chipW} height={chipH} fill={color} opacity={0.9} />
              <text x={chipW / 2} y={Math.round(chipH * 0.68)} textAnchor="middle" fontSize={fontSize}
                className="tabular-nums" fill="#fff" fontWeight={700}>{valueStr}</text>
            </g>
          )}
        </g>
      );
    };
    EndDot.displayName = `EndDot_${cssSafe(id)}`;
    return EndDot;
  };

  const tickFill = "var(--axis-tick)" as const;
  const gridStroke = "var(--grid-stroke)" as const;
  const refLine = "var(--ref-line)" as const;

  const rightMargin = useMemo(() => {
    const visibleIds = models.filter((m) => isSeriesVisible(m));
    let maxW = 0;
    for (const m of visibleIds) {
      const s = formatValue(lastValById[m]);
      const fs = vw < 380 ? 11 : vw < 640 ? 12 : 13;
      const est = s.length * Math.round(fs * 0.62) + 16 + Math.round(endLogoSize * 0.7) + 10;
      if (est > maxW) maxW = est;
    }
    return Math.max(chartRightMargin, maxW);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [models, selectedId, lastValById, formatValue]);

  return (
    <div
      className="grid h-full min-w-0 rounded-md border p-3"
      style={{
        gridTemplateRows: "auto 1fr auto",
        background: "var(--panel-bg)",
        borderColor: "var(--panel-border)",
      }}
    >
      {/* row-1: header */}
      <div className="mb-2 flex items-center justify-between">
        <div className="text-xs font-semibold tracking-wider" style={{ color: "var(--muted-text)" }}>
          账户总资产
        </div>
        <div className="flex items-center gap-1 sm:gap-2 text-[10px] sm:text-[11px]">
          <div className="flex overflow-hidden rounded border" style={{ borderColor: "var(--chip-border)" }}>
            {(["ALL", "24H"] as Range[]).map((r) => (
              <button key={r} className="px-2 py-1 chip-btn"
                style={range === r ? { background: "var(--btn-active-bg)", color: "var(--btn-active-fg)" } : { color: "var(--btn-inactive-fg)" }}
                onClick={() => { setRange(r); if (r === "24H") setCutoffTime(Date.now() - 24 * 3600 * 1000); }}>
                {r}
              </button>
            ))}
          </div>
          <div className="flex overflow-hidden rounded border" style={{ borderColor: "var(--chip-border)" }}>
            {(["$", "%"] as Mode[]).map((m) => (
              <button key={m} className="px-2 py-1 chip-btn"
                style={mode === m ? { background: "var(--btn-active-bg)", color: "var(--btn-active-fg)" } : { color: "var(--btn-inactive-fg)" }}
                onClick={() => setMode(m)}>
                {m}
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* row-2: chart (1fr locks height) */}
      <div className="min-h-0 min-w-0 overflow-hidden h-full w-full relative">
        <ErrorBanner message={isError ? "账户价值数据源暂时不可用，请稍后重试。" : undefined} />
        {isLoading ? (
          <SkeletonBlock className="h-full" />
        ) : (
          <div
            className="chart-container no-tap-highlight select-none w-full h-full absolute inset-0"
            tabIndex={-1}
            onMouseDown={(e) => e.preventDefault()}
          >
            <ResponsiveContainer width="100%" height="100%">
              <LineChart
                data={data}
                margin={{ top: 8, right: rightMargin, bottom: 8, left: 0 }}
              >
              <CartesianGrid stroke={gridStroke} strokeDasharray="3 3" />
              <XAxis dataKey="timestamp" tickFormatter={(v: Date) => format(v, "MM-dd HH:mm")}
                tick={{ fill: tickFill, fontSize: 11 }} />
              <YAxis tickFormatter={(v: number) => mode === "%" ? `${v.toFixed(1)}%` : `$${Math.round(v).toLocaleString()}`}
                tick={{ fill: tickFill, fontSize: 11 }} width={60} domain={["auto", "auto"]} />
              <Tooltip
                contentStyle={{ background: "var(--tooltip-bg)", border: "1px solid var(--tooltip-border)", color: "var(--tooltip-fg)" }}
                labelFormatter={(v) => v instanceof Date ? format(v, "yyyy-MM-dd HH:mm") : String(v)}
                formatter={(val: number) => mode === "%" ? `${Number(val).toFixed(2)}%` : `$${Number(val).toFixed(2)}`}
              />
              {mode === "$"
                ? <ReferenceLine y={10000} stroke={refLine} strokeDasharray="4 4" />
                : <ReferenceLine y={0} stroke={refLine} strokeDasharray="4 4" />}
              {models.map((id) => (
                <Line key={id} type="monotone" dataKey={id} dot={renderEndDot(id)} connectNulls
                  className={`series series-${cssSafe(id)}`} stroke={getStrokeColor(id)} strokeWidth={1.8}
                  isAnimationActive={false} animationDuration={0} animationEasing="ease-out"
                  name={getModelName(id)} hide={!isSeriesVisible(id)} />
              ))}
            </LineChart>
            </ResponsiveContainer>
          </div>
        )}
      </div>

      {/* row-3: legend */}
      {!isLoading && models.length > 0 && (
        <div className="pt-2">
          <div className="hidden md:grid gap-2" style={{ gridTemplateColumns: `repeat(${models.length}, minmax(0, 1fr))` }}>
            {models.map((id) => (
              <LegendBtn key={id} id={id} active={isSeriesVisible(id)} value={formatValue(lastValById[id])}
                onClick={() => setSelectedId((p) => (p === id ? null : id))} />
            ))}
          </div>
          <div className="flex md:hidden flex-nowrap gap-2 overflow-x-auto pr-1" style={{ WebkitOverflowScrolling: "touch" }}>
            {models.map((id) => (
              <LegendBtn key={id} id={id} active={isSeriesVisible(id)} value={formatValue(lastValById[id])}
                onClick={() => setSelectedId((p) => (p === id ? null : id))} mobile />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function LegendBtn({ id, active, value, onClick, mobile }: {
  id: string; active: boolean; value: string; onClick: () => void; mobile?: boolean;
}) {
  const icon = getModelIcon(id);
  const color = getModelColor(id);
  return (
    <button
      className={`${mobile ? "inline-flex min-w-[120px] flex-shrink-0" : "w-full"} group flex-col items-center justify-center gap-1 rounded border px-2.5 py-2 text-[12px] sm:text-[13px] chip-btn inline-flex`}
      style={{
        borderColor: "var(--chip-border)",
        background: active ? "var(--btn-active-bg)" : "transparent",
        color: active ? "var(--btn-active-fg)" : "var(--btn-inactive-fg)",
      }}
      onClick={onClick}
    >
      <div className="flex items-center gap-1 text-[11px] opacity-90">
        {icon ? (
          <span className="logo-chip logo-chip-sm" style={{ background: color, borderColor: adjustLuminance(color, -0.2) }}>
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src={icon} alt="" className="h-3 w-3 object-contain" />
          </span>
        ) : (
          <span className="inline-block h-3 w-3 rounded-full" style={{ background: color }} />
        )}
        <span className={`truncate ${mobile ? "max-w-[9ch]" : "max-w-[9ch] sm:max-w-none"}`}>
          {getModelName(id)}
        </span>
      </div>
      <div className="font-semibold leading-tight" style={{ color: active ? "var(--btn-active-fg)" : "var(--btn-inactive-fg)" }}>
        {value}
      </div>
    </button>
  );
}
