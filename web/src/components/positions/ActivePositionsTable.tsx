"use client";
import { useActivePositions } from "@/lib/api/hooks/useActivePositions";
import { fmtUSD, fmtNumber, pnlClass, withSign } from "@/lib/utils/formatters";

function SkeletonRows({ rows = 3 }: { rows?: number }) {
  return (
    <>
      {Array.from({ length: rows }).map((_, i) => (
        <tr key={i}>
          {Array.from({ length: 9 }).map((_, j) => (
            <td key={j} className="py-3 px-3">
              <div className="h-3 rounded animate-pulse" style={{ background: "var(--skeleton-bg, rgba(255,255,255,0.08))" }} />
            </td>
          ))}
        </tr>
      ))}
    </>
  );
}

export function ActivePositionsTable() {
  const { positions, isLoading } = useActivePositions();

  if (isLoading) {
    return (
      <div className="tab-surface overflow-hidden">
        <div className="tab-filterbar">
          <div>
            <div className="tab-toolbar-label">Active Ledger</div>
            <div className="tab-toolbar-title">活跃仓位</div>
          </div>
          <div className="tab-toolbar-note">实时查看当前未平仓头寸</div>
        </div>
        <div className="tab-table-wrap">
          <table className="tab-table text-sm">
          <thead>
            <tr>
              <th className="text-left py-2 px-3">币种</th>
              <th className="text-center py-2 px-3">方向</th>
              <th className="text-right py-2 px-3">开仓价</th>
              <th className="text-right py-2 px-3">最新价</th>
              <th className="text-right py-2 px-3">数量</th>
              <th className="text-right py-2 px-3">杠杆</th>
              <th className="text-right py-2 px-3">未实现盈亏</th>
              <th className="text-center py-2 px-3">状态</th>
              <th className="text-center py-2 px-3">操作</th>
            </tr>
          </thead>
          <tbody>
            <SkeletonRows />
          </tbody>
        </table>
        </div>
      </div>
    );
  }

  if (positions.length === 0) {
    return (
      <div className="tab-surface tab-empty text-center">
        当前没有活跃仓位
      </div>
    );
  }

  return (
    <div className="tab-surface overflow-hidden">
      <div className="tab-filterbar">
        <div>
          <div className="tab-toolbar-label">Active Ledger</div>
          <div className="tab-toolbar-title">活跃仓位</div>
        </div>
        <div className="tab-toolbar-note">点击任意行查看仓位详情</div>
      </div>
      <div className="tab-table-wrap">
        <table className="tab-table text-sm">
        <thead>
          <tr>
            <th className="text-left py-2 px-3">币种</th>
            <th className="text-center py-2 px-3">方向</th>
            <th className="text-right py-2 px-3">开仓价</th>
            <th className="text-right py-2 px-3">最新价</th>
            <th className="text-right py-2 px-3">数量</th>
            <th className="text-right py-2 px-3">杠杆</th>
            <th className="text-right py-2 px-3">未实现盈亏</th>
            <th className="text-center py-2 px-3">状态</th>
            <th className="text-center py-2 px-3">操作</th>
          </tr>
        </thead>
        <tbody>
          {positions.map((pos) => {
            const sideLower = pos.side?.toLowerCase();
            const href = `/position/${pos.id}`;
            return (
              <tr
                key={pos.id}
                className="cursor-pointer transition-colors"
                onClick={() => window.open(href, "_blank")}
              >
                <td className="py-2 px-3 font-semibold whitespace-nowrap">{pos.symbol}</td>
                <td className="text-center py-2 px-3">
                  <span className={`px-2 py-1 rounded text-xs ${
                    sideLower === "long" ? "bg-green-500/20 text-green-500" : "bg-red-500/20 text-red-500"
                  }`}>
                    {sideLower === "long" ? "做多" : "做空"}
                  </span>
                </td>
                <td className="text-right py-2 px-3 tabular-nums whitespace-nowrap">{fmtUSD(pos.open_price)}</td>
                <td className="text-right py-2 px-3 tabular-nums whitespace-nowrap">{fmtUSD(pos.mark_price)}</td>
                <td className="text-right py-2 px-3 tabular-nums">{fmtNumber(pos.remaining_qty, 4)}</td>
                <td className="text-right py-2 px-3 tabular-nums">{pos.leverage}x</td>
                <td className={`text-right py-2 px-3 whitespace-nowrap ${pnlClass(pos.unrealized_pnl)}`}>
                  <div className="tabular-nums">{fmtUSD(pos.unrealized_pnl)}</div>
                  <div className="text-xs tabular-nums" style={{ color: "var(--muted-text)" }}>{withSign(pos.unrealized_pnl_pct, 2)}%</div>
                </td>
                <td className="text-center py-2 px-3">
                  <span className={`px-2 py-1 rounded text-xs ${
                    pos.status === "open" ? "bg-green-500/20 text-green-500" :
                    pos.status === "partial_closed" ? "bg-yellow-500/20 text-yellow-500" :
                    "bg-gray-500/20 text-gray-500"
                  }`}>
                    {pos.status === "open" ? "持仓中" :
                     pos.status === "partial_closed" ? "部分平仓" : "已平仓"}
                  </span>
                </td>
                <td className="text-center py-2 px-3">
                  <a
                    href={href}
                    target="_blank"
                    rel="noreferrer"
                    onClick={(e) => e.stopPropagation()}
                    className="text-blue-500 hover:text-blue-400 text-xs transition-colors"
                  >
                    详情
                  </a>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
      </div>
    </div>
  );
}
