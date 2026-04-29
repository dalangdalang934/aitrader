"use client";
import useSWR from "swr";
import { activityAwareRefresh } from "./activityAware";
import { endpoints, fetcher } from "../api";

export interface ConversationDecision {
  signal?: string;
  leverage?: number;
  profit_target?: number;
  stop_loss?: number;
  risk_usd?: number;
  invalidation_condition?: string;
  confidence?: number;
  quantity?: number;
  price?: number;
}

export type ConversationDecisionMap = Record<string, ConversationDecision>;

export interface ConversationHistoryItem {
  timestamp?: number | string;
  user_prompt?: string;
  cot_trace?: unknown;
  llm_response?: ConversationDecisionMap;
}

export interface ExecutedOrder {
  action?: string;
  symbol?: string;
  price?: number;
  quantity?: number;
  leverage?: number;
  success?: boolean;
  error?: string;
}

export interface ConversationItem extends ConversationHistoryItem {
  model_id: string;
  inserted_at?: number | string;
  summary?: string;
  cot_trace_summary?: string;
  history?: ConversationHistoryItem[];
  decisions?: ExecutedOrder[];
  execution_log?: string[];
}

export interface ConversationsResponse {
  conversations?: unknown;
  items?: unknown;
  logs?: unknown;
  [k: string]: unknown;
}

export function useConversations() {
  const { data, error, isLoading } = useSWR<ConversationsResponse>(
    endpoints.conversations(),
    fetcher,
    {
      ...activityAwareRefresh(30_000, { hiddenInterval: 90_000 }),
    },
  );
  const items: ConversationItem[] = normalize(data);
  return { items, raw: data, isLoading, isError: !!error };
}

function normalize(data?: ConversationsResponse): ConversationItem[] {
  if (!data) return [];
  const primary = normalizeList(data.conversations);
  if (primary.length) return primary;
  const items = normalizeList(data.items);
  if (items.length) return items;
  return normalizeList(data.logs);
}

function normalizeList(value: unknown): ConversationItem[] {
  if (!Array.isArray(value)) return [];
  return value
    .map(normalizeItem)
    .filter((item): item is ConversationItem => item !== null);
}

function normalizeItem(value: unknown): ConversationItem | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null;
  const row = value as Record<string, unknown>;
  const modelId = toText(row.model_id)?.trim();
  if (!modelId) return null;
  const history = Array.isArray(row.history)
    ? row.history
        .map(normalizeHistoryItem)
        .filter((item): item is ConversationHistoryItem => item !== null)
    : undefined;
  return {
    model_id: modelId,
    timestamp: toTimestamp(row.timestamp),
    inserted_at: toTimestamp(row.inserted_at),
    summary: toText(row.summary),
    cot_trace_summary: toText(row.cot_trace_summary),
    user_prompt: toText(row.user_prompt),
    cot_trace: row.cot_trace,
    llm_response: normalizeDecisionMap(row.llm_response),
    decisions: Array.isArray(row.decisions) ? row.decisions as ExecutedOrder[] : undefined,
    execution_log: Array.isArray(row.execution_log) ? row.execution_log as string[] : undefined,
    history,
  };
}

function normalizeHistoryItem(value: unknown): ConversationHistoryItem | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null;
  const row = value as Record<string, unknown>;
  return {
    timestamp: toTimestamp(row.timestamp),
    user_prompt: toText(row.user_prompt),
    cot_trace: row.cot_trace,
    llm_response: normalizeDecisionMap(row.llm_response),
  };
}

function normalizeDecisionMap(value: unknown): ConversationDecisionMap | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const out: ConversationDecisionMap = {};
  for (const [coin, decision] of Object.entries(value)) {
    const normalized = normalizeDecision(decision);
    if (Object.keys(normalized).length) out[coin] = normalized;
  }
  return Object.keys(out).length ? out : undefined;
}

function normalizeDecision(value: unknown): ConversationDecision {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  const row = value as Record<string, unknown>;
  return {
    signal: toText(row.signal),
    leverage: toNumber(row.leverage),
    profit_target: toNumber(row.profit_target),
    stop_loss: toNumber(row.stop_loss),
    risk_usd: toNumber(row.risk_usd),
    invalidation_condition: toText(row.invalidation_condition),
    confidence: toNumber(row.confidence),
    quantity: toNumber(row.quantity),
    price: toNumber(row.price),
  };
}

function toText(value: unknown): string | undefined {
  if (value == null) return undefined;
  return typeof value === "string" ? value : String(value);
}

function toTimestamp(value: unknown): number | string | undefined {
  if (typeof value === "number" || typeof value === "string") return value;
  return undefined;
}

function toNumber(value: unknown): number | undefined {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return undefined;
}
