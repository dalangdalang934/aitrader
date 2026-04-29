"use client";
import useSWR from "swr";
import { activityAwareRefresh } from "./activityAware";
import { endpoints, fetcher } from "../api";

export interface LeaderboardRow {
  id: string; // model_id
  equity: number;
  return_pct?: number;
  num_trades?: number;
  num_wins?: number;
  num_losses?: number;
  sharpe?: number;
  [k: string]: unknown;
}

interface LeaderboardResponse {
  leaderboard: LeaderboardRow[];
}

export function useLeaderboard() {
  const { data, error, isLoading } = useSWR<LeaderboardResponse>(
    endpoints.leaderboard?.() ?? "/api/backend/leaderboard",
    fetcher,
    { ...activityAwareRefresh(60_000, { hiddenInterval: 120_000 }) },
  );
  return { rows: data?.leaderboard ?? [], isLoading, isError: !!error };
}
