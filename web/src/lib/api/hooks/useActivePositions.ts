"use client";
import useSWR from "swr";
import { activityAwareRefresh } from "./activityAware";
import { fetcher } from "../api";

export interface ActivePosition {
  id: string;
  model_id?: string;
  model_name?: string;
  symbol: string;
  side: string;
  open_time: string;
  open_price: number;
  open_quantity: number;
  open_order_id: string;
  leverage: number;
  mark_price: number;
  status: string;
  remaining_qty: number;
  realized_pnl: number;
  total_commission: number;
  unrealized_pnl: number;
  unrealized_pnl_pct: number;
  created_at: string;
  updated_at: string;
}

export function useActivePositions(model?: string) {
  const endpoint =
    model && model !== "ALL"
      ? `/api/backend/positions/active?trader_id=${encodeURIComponent(model)}`
      : "/api/backend/positions/active";
  const { data, error, isLoading } = useSWR<ActivePosition[]>(
    endpoint,
    fetcher,
    {
      ...activityAwareRefresh(5_000),
    },
  );

  return {
    positions: data ?? [],
    isLoading,
    isError: !!error,
  };
}
