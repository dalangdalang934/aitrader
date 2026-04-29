"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import {
  usePositions,
  type ExitPlan,
  type RawPositionRow,
} from "@/lib/api/hooks/usePositions";
import {
  useAccountTotals,
  type AccountTotalsRow,
} from "@/lib/api/hooks/useAccountTotals";
import { fmtUSD, pnlClass } from "@/lib/utils/formatters";
import clsx from "clsx";
import ErrorBanner from "@/components/ui/ErrorBanner";
import { SkeletonRow } from "@/components/ui/Skeleton";
import PositionsFilter from "@/components/positions/PositionsFilter";
import { getModelColor, getModelName } from "@/lib/model/meta";
import { ModelLogoChip } from "@/components/shared/ModelLogo";
import CoinIcon from "@/components/shared/CoinIcon";
import StrategyOverview from "@/components/overview/StrategyOverview";

type SortKey =
  | "symbol"
  | "leverage"
  | "entry_price"
  | "current_price"
  | "unrealized_pnl"
  | "side";

type PositionWithSide = RawPositionRow & {
  side: "LONG" | "SHORT";
};

type TotalsSnapshot = {
  equity?: number;
  realizedPnL?: number;
};

export function PositionsPanel() {
  const { positionsByModel, isLoading, isError } = usePositions();
  const { data: totalsData } = useAccountTotals();
  const [qModel, setQModel] = useState("ALL");
  const [qSymbol, setQSymbol] = useState("ALL");
  const [qSide, setQSide] = useState("ALL");
  const sortKey: SortKey = "unrealized_pnl";
  const sortDir: "asc" | "desc" = "desc";
  const totalsRows = totalsData?.accountTotals ?? [];

  if (isLoading) {
    return (
      <div className="tab-surface">
        <div className="tab-empty">加载持仓中…</div>
        <div className="tab-table-wrap">
          <table className="tab-table">
            <tbody>
              <SkeletonRow cols={7} />
              <SkeletonRow cols={7} />
              <SkeletonRow cols={7} />
            </tbody>
          </table>
        </div>
      </div>
    );
  }

  if (!positionsByModel.length) {
    return <div className="tab-surface tab-empty">暂无持仓。</div>;
  }

  return (
    <div className="space-y-3">
      <StrategyOverview />
      <ErrorBanner
        message={isError ? "上游持仓接口暂时不可用，请稍后重试。" : undefined}
      />
      <div className="tab-surface">
        <div className="tab-filterbar">
          <div>
            <div className="tab-toolbar-label">Position Matrix</div>
            <div className="tab-toolbar-title">模型持仓总览</div>
          </div>
          <PositionsFilter
            models={positionsByModel.map((model) => model.id)}
            symbols={Array.from(
              new Set(
                positionsByModel.flatMap((model) =>
                  Object.keys(model.positions || {}),
                ),
              ),
            )}
            model={qModel}
            symbol={qSymbol}
            side={qSide}
            onChange={(next) => {
              if (next.model !== undefined) setQModel(next.model);
              if (next.symbol !== undefined) setQSymbol(next.symbol);
              if (next.side !== undefined) setQSide(next.side);
            }}
          />
        </div>
      </div>

      {positionsByModel
        .filter((model) =>
          qModel === "ALL" ? true : model.id.toLowerCase() === qModel.toLowerCase(),
        )
        .map((model) => {
          const positions = sortPositions(
            Object.values(model.positions || {}),
            sortKey,
            sortDir,
          );
          const filtered = positions
            .filter((position) =>
              qSymbol === "ALL"
                ? true
                : position.symbol?.toUpperCase() === qSymbol,
            )
            .filter((position) =>
              qSide === "ALL" ? true : position.side === qSide,
            );

          if (!filtered.length) return null;

          const totalUnreal = filtered.reduce(
            (acc, position) => acc + (position.unrealized_pnl || 0),
            0,
          );
          const sumMargin = filtered.reduce(
            (acc, position) => acc + (position.margin || 0),
            0,
          );
          const sumRisk = filtered.reduce(
            (acc, position) => acc + (position.risk_usd || 0),
            0,
          );
          const avgConf = filtered.length
            ? filtered.reduce((acc, position) => acc + (position.confidence || 0), 0) /
              filtered.length
            : 0;
          const { equity, realizedPnL } = findTotalsSnapshot(totalsRows, model.id);
          const availableCash = equity != null ? equity - sumMargin : undefined;
          const color = getModelColor(model.id);

          return (
            <section
              key={model.id}
              className="tab-surface px-4 py-4"
              style={{
                borderColor: `${color}50`,
                background: `linear-gradient(180deg, ${color}12, transparent 26%), var(--panel-bg)`,
              }}
            >
              <div className="flex flex-col gap-4">
                <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                  <div className="flex items-center gap-3">
                    <ModelLogoChip modelId={model.id} size="sm" />
                    <div>
                      <div className="tab-toolbar-label" style={{ color }}>
                        Model Allocation
                      </div>
                      <div className="tab-toolbar-title">
                        {getModelName(model.id)}
                      </div>
                    </div>
                  </div>
                  <div className="rounded-full border px-3 py-1 text-[11px] ui-sans">
                    <span style={{ color: "var(--muted-text)" }}>
                      未实现盈亏合计：
                    </span>
                    <span
                      className={totalUnreal >= 0 ? "text-green-400" : "text-red-400"}
                    >
                      {fmtUSD(totalUnreal)}
                    </span>
                  </div>
                </div>

                <div className="tab-summary-grid">
                  <MetricCard label="净值" value={fmtUSD(equity)} />
                  <MetricCard label="已实现盈亏" value={fmtUSD(realizedPnL)} />
                  <MetricCard label="可用现金" value={fmtUSD(availableCash)} />
                  <MetricCard label="风险金额合计" value={fmtUSD(sumRisk)} />
                  <MetricCard
                    label="平均置信度"
                    value={avgConf ? `${(avgConf * 100).toFixed(1)}%` : "—"}
                  />
                </div>

                <div className="space-y-2">
                  {filtered.map((position) => (
                    <PositionDetail key={position.entry_oid} position={position} />
                  ))}
                </div>

                <div className="tab-table-wrap">
                  <table className="tab-table terminal-text text-[12px]">
                    <thead className="ui-sans">
                      <tr>
                        <th>方向</th>
                        <th>币种</th>
                        <th>杠杆</th>
                        <th>名义金额</th>
                        <th>退出计划</th>
                        <th>未实现盈亏</th>
                      </tr>
                    </thead>
                    <tbody style={{ color: "var(--foreground)" }}>
                      {filtered.map((position) => {
                        const isLong = position.quantity > 0;
                        const notional =
                          Math.abs(position.quantity) * (position.current_price ?? 0);
                        return (
                          <tr key={`table-${position.entry_oid}`}>
                            <td style={{ color: isLong ? "#16a34a" : "#ef4444" }}>
                              {isLong ? "做多" : "做空"}
                            </td>
                            <td>
                              <span className="inline-flex items-center gap-2">
                                <CoinIcon symbol={getBase(position.symbol)} size={16} />
                                <span className="ui-sans">
                                  {renderSymbolParts(position.symbol)}
                                </span>
                              </span>
                            </td>
                            <td>{position.leverage}x</td>
                            <td className="tabular-nums">{fmtUSD(notional)}</td>
                            <td>
                              <ExitPlanPeek plan={position.exit_plan} />
                            </td>
                            <td
                              className={clsx(
                                "tabular-nums",
                                pnlClass(position.unrealized_pnl),
                              )}
                            >
                              {fmtUSD(position.unrealized_pnl)}
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              </div>
            </section>
          );
        })}
    </div>
  );
}

function PositionDetail({ position }: { position: PositionWithSide }) {
  const qtyValue = Number(position.quantity) || 0;
  const isLong = qtyValue > 0;
  const qtyAbs = Math.abs(qtyValue);
  const marginType = position.margin_type ?? "isolated";
  const marginTypeText = marginType === "cross" ? "全仓" : "逐仓";
  const breakeven = buildBreakeven(position, isLong, qtyAbs);
  const baseSymbol = getBase(position.symbol);
  const qtyDigits = qtyAbs >= 1 ? 2 : qtyAbs >= 0.1 ? 3 : 4;
  const qtyFormatted = Number.isFinite(qtyValue) ? qtyValue.toFixed(qtyDigits) : "—";

  return (
    <div className="tab-section">
      <div className="tab-section-body">
        <div className="mb-2 flex items-center gap-2">
          <CoinIcon symbol={baseSymbol} size={18} />
          <div className="ui-sans text-sm font-semibold">
            {renderSymbolParts(position.symbol)}
          </div>
          <span
            className="rounded-full px-2 py-0.5 text-[11px] font-semibold"
            style={{
              color: isLong ? "#16a34a" : "#ef4444",
              background: isLong
                ? "color-mix(in oklab, #16a34a 12%, transparent)"
                : "color-mix(in oklab, #ef4444 12%, transparent)",
            }}
          >
            {isLong ? "LONG" : "SHORT"}
          </span>
        </div>
        <div
          className="grid grid-cols-2 gap-x-4 gap-y-2 text-[11px] ui-sans md:grid-cols-3 xl:grid-cols-4"
          style={{ color: "var(--muted-text)" }}
        >
          <MetricItem
            label="数量"
            value={`${qtyFormatted}${baseSymbol ? ` ${baseSymbol}` : ""}`}
            accent={isLong ? "#16a34a" : "#ef4444"}
          />
          <MetricItem label="开仓价格" value={fmtUSD(position.entry_price)} />
          <MetricItem label="标记价格" value={fmtUSD(position.current_price)} />
          <MetricItem
            label="损益两平价"
            value={typeof breakeven === "number" ? fmtUSD(breakeven) : breakeven}
          />
          <MetricItem
            label="强平价格"
            value={
              position.liquidation_price
                ? fmtUSD(position.liquidation_price)
                : "—"
            }
            accent="#f97316"
          />
          <MetricItem label="保证金模式" value={marginTypeText} />
          <MetricItem label="保证金" value={fmtUSD(position.margin)} />
          {position.exit_plan ? (
            <>
              <MetricItem
                label="止盈"
                value={
                  position.exit_plan.profit_target
                    ? fmtUSD(position.exit_plan.profit_target)
                    : "—"
                }
              />
              <MetricItem
                label="止损"
                value={
                  position.exit_plan.stop_loss
                    ? fmtUSD(position.exit_plan.stop_loss)
                    : "—"
                }
              />
            </>
          ) : null}
        </div>
      </div>
    </div>
  );
}

function MetricCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="tab-stat-card">
      <div className="tab-stat-label">{label}</div>
      <div className="tab-stat-value">{value}</div>
    </div>
  );
}

function MetricItem({
  label,
  value,
  accent,
}: {
  label: string;
  value: string;
  accent?: string;
}) {
  return (
    <div>
      {label}：
      <span className="tabular-nums" style={{ color: accent || "var(--foreground)" }}>
        {value}
      </span>
    </div>
  );
}

function ExitPlanPeek({ plan }: { plan?: ExitPlan }) {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null);
  const btnRef = useRef<HTMLButtonElement | null>(null);
  const popRef = useRef<HTMLDivElement | null>(null);
  const hasPlan = Boolean(
    plan && (plan.profit_target || plan.stop_loss || plan.invalidation_condition),
  );

  useEffect(() => {
    if (!hasPlan || !open) return;

    const place = () => {
      const rect = btnRef.current?.getBoundingClientRect();
      if (!rect) return;
      const margin = 8;
      const width = 320;
      let left = Math.min(window.innerWidth - width - margin, rect.right - width);
      if (left < margin) left = Math.max(margin, rect.left);
      const top = Math.max(
        margin,
        Math.min(window.innerHeight - 160, rect.bottom + 6),
      );
      setPos({ top, left });
    };

    const handleDocumentClick = (event: MouseEvent) => {
      const target = event.target as Node;
      if (popRef.current?.contains(target) || btnRef.current?.contains(target)) {
        return;
      }
      setOpen(false);
    };

    const handleKeydown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };

    place();
    window.addEventListener("resize", place);
    window.addEventListener("scroll", place, true);
    document.addEventListener("mousedown", handleDocumentClick);
    document.addEventListener("keydown", handleKeydown);
    return () => {
      window.removeEventListener("resize", place);
      window.removeEventListener("scroll", place, true);
      document.removeEventListener("mousedown", handleDocumentClick);
      document.removeEventListener("keydown", handleKeydown);
    };
  }, [hasPlan, open]);

  if (!hasPlan) {
    return <span style={{ color: "var(--muted-text)" }}>—</span>;
  }

  return (
    <>
      <button
        ref={btnRef}
        type="button"
        className="tab-chip"
        onClick={() => setOpen((value) => !value)}
      >
        查看
      </button>
      {open && pos && typeof document !== "undefined"
        ? createPortal(
            <div
              ref={popRef}
              className="tab-section w-80 shadow-xl"
              style={{
                position: "fixed",
                top: pos.top,
                left: pos.left,
                zIndex: 9999,
              }}
            >
              <div className="tab-section-header">退出计划</div>
              <div className="tab-section-body terminal-text text-xs leading-relaxed">
                <div>
                  目标价：
                  <span className="tabular-nums">{plan?.profit_target ?? "—"}</span>
                </div>
                <div>
                  止损价：
                  <span className="tabular-nums">{plan?.stop_loss ?? "—"}</span>
                </div>
                <div className="mt-2">失效条件：</div>
                <div className="whitespace-pre-wrap">
                  {plan?.invalidation_condition || "—"}
                </div>
              </div>
            </div>,
            document.body,
          )
        : null}
    </>
  );
}

function sortPositions(
  positions: RawPositionRow[],
  sortKey: SortKey,
  sortDir: "asc" | "desc",
): PositionWithSide[] {
  const direction = sortDir === "asc" ? 1 : -1;
  return positions
    .map((position) => ({
      ...position,
      side: position.quantity > 0 ? ("LONG" as const) : ("SHORT" as const),
    }))
    .sort((left, right) => {
      const leftValue = sortKey === "side" ? left.side : left[sortKey];
      const rightValue = sortKey === "side" ? right.side : right[sortKey];
      if (leftValue == null && rightValue == null) return 0;
      if (leftValue == null) return 1;
      if (rightValue == null) return -1;
      if (typeof leftValue === "string" && typeof rightValue === "string") {
        return leftValue.localeCompare(rightValue) * direction;
      }
      return (Number(leftValue) - Number(rightValue)) * direction;
    });
}

function findTotalsSnapshot(rows: AccountTotalsRow[], modelId: string): TotalsSnapshot {
  for (let i = rows.length - 1; i >= 0; i--) {
    const row = rows[i];
    if (row?.model_id === modelId || row?.id === modelId) {
      return {
        equity: row.dollar_equity ?? row.equity ?? row.account_value,
        realizedPnL: row.realized_pnl,
      };
    }
  }
  return {};
}

function buildBreakeven(
  position: PositionWithSide,
  isLong: boolean,
  qtyAbs: number,
): number | string {
  if (typeof position.entry_price !== "number" || qtyAbs <= 0) {
    return position.entry_price ?? "—";
  }

  const feeRate = 0.0005; // taker 0.05%
  // 开仓+平仓两次手续费
  const totalFee = position.entry_price * feeRate * 2;
  const estimate = isLong
    ? position.entry_price + totalFee
    : position.entry_price - totalFee;
  return Number(estimate.toFixed(2));
}

function splitSymbol(symbol?: string | null): [string, string] {
  const value = String(symbol || "").toUpperCase();
  if (!value) return ["", ""];
  const suffixes = ["USDT", "USDC", "USD", "BTC", "ETH", "PERP", "SOL", "BNB"];
  for (const suffix of suffixes) {
    if (value.endsWith(suffix) && value.length > suffix.length) {
      return [value.slice(0, -suffix.length), suffix];
    }
  }
  return [value, ""];
}

function getBase(symbol?: string | null): string {
  return splitSymbol(symbol)[0];
}

function renderSymbolParts(symbol?: string | null): ReactNode {
  const [base, quote] = splitSymbol(symbol);
  if (!base && !quote) return "—";
  return quote ? (
    <span>
      {base}
      <span className="ml-1 text-[11px] text-zinc-500">/{quote}</span>
    </span>
  ) : (
    base
  );
}
