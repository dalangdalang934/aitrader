"use client";
import useSWR from "swr";
import { activityAwareRefresh } from "./activityAware";

export interface MacroFactor {
  category: string;
  title: string;
  impact: string;
  importance: number;
  description: string;
}

export interface MacroRecommendation {
  preferred_direction: string;
  position_size_adj: number;
  max_leverage_adj: number;
  avoid_symbols: string[];
  focus_symbols: string[];
  reasoning: string;
}

export interface MacroOutlook {
  generated_at: string;
  valid_until: string;
  overall_bias: string;
  bias_score: number;
  risk_level: string;
  summary: string;
  key_factors: MacroFactor[];
  recommendations: MacroRecommendation;
  digest_ids: string[];
}

interface OutlookResponse {
  outlook: MacroOutlook | null;
  available: boolean;
  updated_at?: string;
}

async function fetcher(url: string): Promise<OutlookResponse> {
  const res = await fetch(url, { cache: "no-store" });
  if (!res.ok) {
    throw new Error(`Failed to load outlook: ${res.status}`);
  }
  try {
    return await res.json();
  } catch {
    return { outlook: null, available: false };
  }
}

export function useMacroOutlook() {
  const { data, error, isLoading } = useSWR<OutlookResponse>(
    "/api/news/outlook",
    fetcher,
    {
      ...activityAwareRefresh(120_000, { hiddenInterval: 600_000 }),
    },
  );

  return {
    outlook: data?.outlook ?? null,
    available: data?.available ?? false,
    isLoading,
    isError: !!error,
  };
}
