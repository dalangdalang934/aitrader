"use client";
import { useMemo, useState } from "react";
import { useExchangeTrades } from "@/lib/api/hooks/useExchangeTrades";
import ErrorBanner from "@/components/ui/ErrorBanner";
import { SkeletonRow } from "@/components/ui/Skeleton";
import { getModelName } from "@/lib/model/meta";
import { ExchangeTradeItem } from "@/components/trades/ExchangeTradeItemRow";

export default function ExchangeTradesTable() {
  const { trades, isLoading, isError } = useExchangeTrades();
  const [qModel, setQModel] = useState("all");

  const all = useMemo(() => {
    const arr = [...trades];
    arr.sort(
      (a, b) =>
        Number(b.exit_time || b.entry_time) -
        Number(a.exit_time || a.entry_time),
    );
    return arr.slice(0, 100);
  }, [trades]);

  const rows = useMemo(() => {
    return all.filter((t) =>
      qModel === "all" ? true : (t.model_id || "").toLowerCase() === qModel,
    );
  }, [all, qModel]);

  const models = useMemo(() => {
    const ids = Array.from(new Set(trades.map((t) => t.model_id))).filter(Boolean) as string[];
    return ids.sort((a, b) => a.localeCompare(b));
  }, [trades]);

  return (
    <div
      className="tab-surface overflow-hidden terminal-text text-[13px] leading-relaxed sm:text-xs"
    >
      <div className="tab-filterbar">
        <div>
          <div className="tab-toolbar-label">Exchange Tape</div>
          <div className="tab-toolbar-title">交易所成交快照</div>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <label className="tab-filter-chip">
            <span className="tab-filter-label">模型</span>
            <select
              className="tab-select"
              value={qModel === "all" ? "ALL" : qModel}
              onChange={(e) => setQModel(e.target.value === "ALL" ? "all" : e.target.value.toLowerCase())}
            >
              <option value="ALL">全部模型</option>
              {models.map((m) => (
                <option key={m} value={m}>
                  {getModelName(m)}
                </option>
              ))}
            </select>
          </label>
          <div className="tab-toolbar-note">最近 100 笔交易所成交</div>
        </div>
      </div>

      <ErrorBanner
        message={isError ? "交易所成交数据暂不可用，请稍后重试。" : undefined}
      />

      <div className="divide-y" style={{ borderColor: "color-mix(in oklab, var(--panel-border) 50%, transparent)" }}>
        {isLoading ? (
          <div className="p-3 space-y-2">
            <SkeletonRow cols={1} as="div" />
            <SkeletonRow cols={1} as="div" />
            <SkeletonRow cols={1} as="div" />
          </div>
        ) : rows.length ? (
          rows.map((t) => <ExchangeTradeItem key={`exchange-${t.id}`} t={t} />)
        ) : (
          <div className="tab-empty">暂无交易所成交数据</div>
        )}
      </div>
    </div>
  );
}
