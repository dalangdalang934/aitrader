import PriceTicker from "@/components/layout/PriceTicker";
import AccountValueChart from "@/components/chart/AccountValueChart";
import StrategyOverview from "@/components/overview/StrategyOverview";
import RightTabs from "@/components/tabs/RightTabs";
import TabButton from "@/components/tabs/TabButton";
import { TabProvider } from "@/components/tabs/TabContext";
import { Suspense } from "react";

export default function Home() {
  return (
    <div
      className="dashboard-shell terminal-scan fixed inset-x-0 bottom-0 flex flex-col"
      style={{ top: "var(--header-h)" }}
    >
      <PriceTicker />

      <div className="min-h-0 flex-1 overflow-hidden px-3 pb-3 pt-2 sm:px-4 sm:pb-4">
        <div className="mx-auto flex h-full max-w-[1680px] flex-col lg:flex-row gap-4">
          {/* ── 左栏 ── */}
          <div className="flex-[1.8] min-w-0 min-h-0 relative h-full">
            <LeftPanel />
          </div>
          {/* ── 右栏 ── */}
          <div className="flex-1 min-w-0 min-h-0 relative w-full h-full">
            <RightPanel />
          </div>
        </div>
      </div>
    </div>
  );
}

/**
 * 左栏：图表面板。
 * contain: layout paint → 右栏的 DOM 变化不会触发这里重排/重绘。
 * 图表用 absolute inset-0 锁定尺寸，只响应窗口 resize。
 */
function LeftPanel() {
  return (
    <div
      className="dashboard-panel overflow-hidden rounded-[20px] p-4 sm:p-5 h-full"
      style={{ display: "flex", flexDirection: "column", gap: "0.75rem" }}
    >
      <div className="flex items-center justify-between gap-3 shrink-0">
        <div>
          <div className="dashboard-kicker">Performance Arena</div>
          <h1 className="dashboard-title mt-1">AI 账户表现总览</h1>
        </div>
        <div className="dashboard-note">资金曲线 · 实时行情</div>
      </div>

      <div className="shrink-0">
        <StrategyOverview compact />
      </div>

      <div style={{ height: "calc(100vh - 320px)", minHeight: "350px" }}>
        <AccountValueChart />
      </div>
    </div>
  );
}

/**
 * 右栏：Command Deck。
 * contain: layout paint → 内容变化不外泄到左栏。
 * scrollbar-gutter: stable → 滚动条不引起宽度跳动。
 */
function RightPanel() {
  return (
    <TabProvider>
      <div
        className="dashboard-panel overflow-hidden rounded-[20px] p-4 sm:p-5 min-h-0 h-full"
        style={{ display: "flex", flexDirection: "column" }}
      >
        <div className="flex items-center justify-between gap-3 mb-1">
          <div>
            <div className="dashboard-kicker">Command Deck</div>
            <h2 className="dashboard-title mt-1">模型明细与执行记录</h2>
          </div>
        </div>

        <div
          className="flex items-center overflow-x-auto scrollbar-hide mb-3"
          style={{ borderBottom: "1px solid color-mix(in srgb, var(--panel-border) 55%, transparent)" }}
        >
          <TabButton name="持仓" tabKey="positions" />
          <TabButton name="成交" tabKey="trades" />
          <TabButton name="对话" tabKey="chat" />
          <TabButton name="AI 学习" tabKey="ai-learning" />
          <TabButton name="新闻" tabKey="news" />
        </div>

        <div
          className="min-h-0 flex-1 overflow-y-auto pr-1"
          style={{ scrollbarGutter: "stable" }}
        >
          <Suspense fallback={null}>
            <RightTabs />
          </Suspense>
        </div>
      </div>
    </TabProvider>
  );
}
