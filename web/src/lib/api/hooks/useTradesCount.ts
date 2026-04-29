"use client";
import useSWR from "swr";
import { activityAwareRefresh } from "./activityAware";
import { endpoints, fetcher } from "../api";

type Trade = { model_id: string };
type Resp = { trades: Trade[] };

export function useTradesCountMap() {
  const { data, error, isLoading } = useSWR<Resp>(
    endpoints.trades?.() ?? "/api/backend/trades",
    fetcher,
    {
      ...activityAwareRefresh(15_000),
    },
  );
  const map: Record<string, number> = {};
  for (const t of data?.trades ?? []) {
    const id = t.model_id;
    if (!id) continue;
    map[id] = (map[id] || 0) + 1;
  }
  return { map, isLoading, isError: !!error };
}
