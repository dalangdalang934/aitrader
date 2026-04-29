"use client";
import Link from "next/link";
import { useParams, useSearchParams } from "next/navigation";
import useSWR from "swr";
import { fmtUSD, fmtNumber } from "@/lib/utils/formatters";

interface Position {
  id: string;
  symbol: string;
  side: string;
  open_time: string;
  open_price: number;
  open_quantity: number;
  open_order_id: string;
  leverage: number;
  closes: Array<{
    close_time: string;
    close_price: number;
    close_qty: number;
    close_order_id: string;
    pnl: number;
    commission: number;
    reason: string;
  }>;
  status: string;
  remaining_qty: number;
  realized_pnl: number;
  total_commission: number;
  created_at: string;
  updated_at: string;
}

export default function PositionDetailPage() {
  const params = useParams();
  const searchParams = useSearchParams();
  const positionId = params.id as string;
  const traderId = searchParams.get("trader_id") ?? "";
  const homeHref = traderId ? `/?model=${encodeURIComponent(traderId)}` : "/";
  const qs = traderId ? `?trader_id=${encodeURIComponent(traderId)}` : "";
  const key = positionId ? `/api/backend/positions/${positionId}${qs}` : null;
  const { data: position, error, isLoading } = useSWR<Position>(
    key,
    async (url: string) => {
      const res = await fetch(url, { cache: "no-store" });
      if (!res.ok) {
        throw new Error("Failed to fetch position");
      }
      return (await res.json()) as Position;
    },
  );

  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center p-6">
        <div className="rounded-lg border p-8 text-center" style={{ background: "var(--panel-bg)", borderColor: "var(--panel-border)" }}>
          <div className="animate-pulse text-sm" style={{ color: "var(--muted-text)" }}>加载仓位数据中...</div>
        </div>
      </div>
    );
  }

  if (error || !position) {
    return (
      <div className="min-h-screen flex flex-col items-center justify-center p-6">
        <div className="rounded-lg border p-8 max-w-md text-center" style={{ background: "var(--panel-bg)", borderColor: "var(--panel-border)" }}>
          <div className="text-red-500 text-sm mb-4">
            {error instanceof Error ? error.message : "未找到仓位信息"}
          </div>
          <Link
            href={homeHref}
            className="inline-block rounded px-4 py-2 text-xs font-medium transition-colors hover:opacity-80"
            style={{ background: "var(--brand-accent, #3b82f6)", color: "#fff" }}
          >
            返回首页
          </Link>
        </div>
      </div>
    );
  }

  const closes = position.closes ?? [];
  const totalCloseQty = closes.reduce((sum, c) => sum + (c?.close_qty ?? 0), 0);
  const avgClosePrice = closes.length > 0
    ? closes.reduce((sum, c, _, arr) => sum + (c?.close_price ?? 0) / arr.length, 0)
    : 0;

  return (
    <div className="min-h-screen p-6">
      <div className="max-w-6xl mx-auto">
        <div className="mb-6">
          <Link
            href={homeHref}
            className="inline-flex items-center gap-1 text-xs mb-3 transition-opacity hover:opacity-70"
            style={{ color: "var(--muted-text)" }}
          >
            ← 返回首页
          </Link>
          <h1 className="text-2xl font-bold mb-2">仓位详情</h1>
          <div className="text-sm text-muted">
            仓位ID: <span className="font-mono text-xs">{position.id}</span>
          </div>
        </div>

        {/* 基本信息卡片 */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6 mb-6">
          <div className="rounded-lg border p-6" style={{ background: "var(--panel-bg)", borderColor: "var(--panel-border)" }}>
            <h2 className="text-lg font-semibold mb-4">开仓信息</h2>
            <div className="space-y-2">
              <div className="flex justify-between">
                <span className="text-muted">币种</span>
                <span className="font-semibold">{position.symbol}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted">方向</span>
                <span className={position.side?.toLowerCase() === "long" ? "text-green-500" : "text-red-500"}>
                  {position.side?.toLowerCase() === "long" ? "做多" : "做空"}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted">开仓价格</span>
                <span>{fmtNumber(position.open_price, 4)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted">开仓数量</span>
                <span>{fmtNumber(position.open_quantity, 4)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted">杠杆倍数</span>
                <span>{position.leverage}x</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted">开仓时间</span>
                <span className="text-xs">{(() => { const d = new Date(position.open_time); return Number.isNaN(d.getTime()) ? "—" : d.toLocaleString(); })()}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted">订单ID</span>
                <span className="font-mono text-xs">{position.open_order_id}</span>
              </div>
            </div>
          </div>

          <div className="rounded-lg border p-6" style={{ background: "var(--panel-bg)", borderColor: "var(--panel-border)" }}>
            <h2 className="text-lg font-semibold mb-4">仓位状态</h2>
            <div className="space-y-2">
              <div className="flex justify-between">
                <span className="text-muted">状态</span>
                <span className={`rounded px-2 py-1 text-xs ${
                  position.status === "open" ? "bg-green-500/20 text-green-500" :
                  position.status === "partial_closed" ? "bg-yellow-500/20 text-yellow-500" :
                  "bg-gray-500/20 text-gray-500"
                }`}>
                  {position.status === "open" ? "持仓中" :
                   position.status === "partial_closed" ? "部分平仓" : "已平仓"}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted">剩余数量</span>
                <span>{fmtNumber(position.remaining_qty, 4)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted">已平仓数量</span>
                <span>{fmtNumber(totalCloseQty, 4)}</span>
              </div>
              {closes.length > 0 && (
                <div className="flex justify-between">
                  <span className="text-muted">平均平仓价</span>
                  <span>{fmtNumber(avgClosePrice, 4)}</span>
                </div>
              )}
              <div className="flex justify-between">
                <span className="text-muted">已实现盈亏</span>
                <span className={position.realized_pnl >= 0 ? "text-green-500" : "text-red-500"}>
                  {fmtUSD(position.realized_pnl)}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted">总手续费</span>
                <span className="text-yellow-500">{fmtUSD(position.total_commission)}</span>
              </div>
            </div>
          </div>
        </div>

        {/* 平仓记录 */}
        {closes.length > 0 && (
          <div className="rounded-lg border p-6" style={{ background: "var(--panel-bg)", borderColor: "var(--panel-border)" }}>
            <h2 className="text-lg font-semibold mb-4">平仓记录</h2>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b" style={{ borderColor: "var(--panel-border)" }}>
                    <th className="text-left py-2">时间</th>
                    <th className="text-right py-2">平仓价</th>
                    <th className="text-right py-2">数量</th>
                    <th className="text-right py-2">盈亏</th>
                    <th className="text-right py-2">手续费</th>
                    <th className="text-left py-2">原因</th>
                    <th className="text-left py-2">订单ID</th>
                  </tr>
                </thead>
                <tbody>
                  {closes.map((close, idx) => (
                    <tr key={close.close_order_id || `close-${idx}`} className="border-b" style={{ borderColor: "var(--panel-border)" }}>
                      <td className="py-2 text-xs">{(() => { const d = new Date(close.close_time); return Number.isNaN(d.getTime()) ? "—" : d.toLocaleString(); })()}</td>
                      <td className="text-right py-2">{fmtNumber(close.close_price, 4)}</td>
                      <td className="text-right py-2">{fmtNumber(close.close_qty, 4)}</td>
                      <td className={`text-right py-2 ${close.pnl >= 0 ? "text-green-500" : "text-red-500"}`}>
                        {fmtUSD(close.pnl)}
                      </td>
                      <td className="text-right py-2 text-yellow-500">{fmtUSD(close.commission)}</td>
                      <td className="py-2 text-xs">{close.reason || "-"}</td>
                      <td className="py-2 font-mono text-xs">{close.close_order_id}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
