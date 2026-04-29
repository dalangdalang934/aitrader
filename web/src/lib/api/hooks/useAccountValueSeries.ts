"use client";

import { useMemo } from "react";
import useSWR from "swr";
import { fetcher } from "../api";
import { activityAwareRefresh } from "./activityAware";
import { useAccountTotals, type AccountTotalsRow } from "./useAccountTotals";

export interface SeriesPoint {
  timestamp: number; // ms epoch
  [modelId: string]: number | undefined;
}

interface EquityHistoryPoint {
  timestamp: string;
  total_equity: number;
  total_pnl: number;
  total_pnl_pct: number;
  position_count: number;
  cycle_number: number;
}

interface EquityHistoryByModel {
  model_id: string;
  points: EquityHistoryPoint[];
}

function toMs(t: number | string) {
  if (typeof t === "string") {
    const parsed = Date.parse(t);
    return Number.isFinite(parsed) ? parsed : Date.now();
  }
  return t > 1e12 ? Math.floor(t) : Math.floor(t * 1000);
}

// Round to nearest minute for cleaner x-axis
function roundToMinute(ms: number) {
  return Math.floor(ms / 60_000) * 60_000;
}

export function useAccountValueSeries() {
  const { data: totalsData, isLoading: totalsLoading, isError: totalsError } = useAccountTotals();

  // Get trader IDs from account-totals
  const traderIds = useMemo(() => {
    const rows = (totalsData?.accountTotals ?? []) as AccountTotalsRow[];
    return [...new Set(rows.map((r) => r.model_id ?? r.id).filter(Boolean))] as string[];
  }, [totalsData]);

  // Fetch equity history for each trader
  const historyKey = traderIds.length
    ? `/api/backend/equity-history?trader_id=${traderIds[0]}`
    : null;

  const { data: historyData, isLoading: historyLoading, error: historyError } = useSWR<EquityHistoryPoint[]>(
    historyKey,
    fetcher,
    { ...activityAwareRefresh(30_000), revalidateOnFocus: false },
  );

  const series = useMemo(() => {
    const map = new Map<number, SeriesPoint>();
    const modelId = traderIds[0];
    if (!modelId) return [];

    // Ingest history points
    const points = historyData ?? [];
    for (const p of points) {
      if (!p.timestamp || typeof p.total_equity !== "number" || p.total_equity <= 0) continue;
      const ts = roundToMinute(toMs(p.timestamp));
      const existing = map.get(ts) || { timestamp: ts };
      existing[modelId] = p.total_equity;
      map.set(ts, existing);
    }

    // Also ingest current snapshot from account-totals
    const rows = (totalsData?.accountTotals ?? []) as AccountTotalsRow[];
    for (const row of rows) {
      const id = row.model_id ?? row.id;
      if (!id || typeof row.timestamp !== "number") continue;
      const v = row.dollar_equity ?? row.account_value;
      if (typeof v !== "number") continue;
      const ts = roundToMinute(toMs(row.timestamp));
      const existing = map.get(ts) || { timestamp: ts };
      existing[id] = v;
      map.set(ts, existing);
    }

    return Array.from(map.values()).sort((a, b) => a.timestamp - b.timestamp);
  }, [historyData, totalsData, traderIds]);

  const idsSet = new Set<string>();
  for (const p of series)
    for (const k of Object.keys(p)) if (k !== "timestamp") idsSet.add(k);

  // Guarantee at least 2 points
  let out = series;
  if (out.length === 1) {
    const only = out[0];
    const synth: SeriesPoint = { timestamp: only.timestamp - 3_600_000 };
    for (const key of Object.keys(only)) {
      if (key === "timestamp") continue;
      const value = only[key];
      if (typeof value === "number") synth[key] = value;
    }
    out = [synth, only];
  } else if (out.length === 0 && idsSet.size > 0) {
    const now = Date.now();
    const stub: SeriesPoint = { timestamp: now - 3_600_000 };
    const stub2: SeriesPoint = { timestamp: now };
    for (const id of idsSet) { stub[id] = 0; stub2[id] = 0; }
    out = [stub, stub2];
  }

  return {
    series: out,
    modelIds: Array.from(idsSet),
    isLoading: totalsLoading && historyLoading,
    isError: totalsError || !!historyError,
  };
}
