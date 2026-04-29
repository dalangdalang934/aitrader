"use client";
import { useState } from "react";
import { useTab } from "./TabContext";
import { PositionsPanel } from "@/components/tabs/PositionsPanel";
import TradesTable from "@/components/trades/TradesTable";
import AiLearningPanel from "@/components/analytics/AiLearningPanel";
import MacroNewsPanel from "@/components/tabs/MacroNewsPanel";
import ModelChatPanel from "@/components/chat/ModelChatPanel";
import { ActivePositionsTable } from "@/components/positions/ActivePositionsTable";

function CombinedPositionsTab() {
  const [sub, setSub] = useState<"overview" | "active">("overview");
  return (
    <div className="flex flex-col gap-3">
      <SegmentControl
        options={[{ key: "overview", label: "持仓概览" }, { key: "active", label: "活跃仓位" }]}
        value={sub}
        onChange={(k) => setSub(k as "overview" | "active")}
      />
      {sub === "overview" ? <PositionsPanel /> : <ActivePositionsTable />}
    </div>
  );
}

function CombinedTradesTab() {
  return <TradesTable />;
}

function SegmentControl({ options, value, onChange }: {
  options: { key: string; label: string }[];
  value: string;
  onChange: (k: string) => void;
}) {
  return (
    <div className="flex gap-1 rounded-xl border p-1 w-fit"
      style={{ borderColor: "color-mix(in srgb, var(--panel-border) 70%, transparent)", background: "color-mix(in srgb, var(--logo-chip-bg) 50%, transparent)" }}>
      {options.map((o) => (
        <button
          key={o.key}
          onClick={() => onChange(o.key)}
          className="rounded-lg px-3 py-1 text-[11px] font-medium tracking-wide transition-all"
          style={o.key === value
            ? { background: "color-mix(in srgb, var(--logo-chip-bg) 140%, transparent)", color: "var(--foreground)", boxShadow: "0 1px 4px rgba(0,0,0,0.15)" }
            : { color: "var(--muted-text)" }}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

export default function RightTabs() {
  const { tab } = useTab();
  if (tab === "chat") return <ModelChatPanel />;
  if (tab === "trades") return <CombinedTradesTab />;
  if (tab === "ai-learning") return <AiLearningPanel />;
  if (tab === "news") return <MacroNewsPanel />;
  return <CombinedPositionsTab />;
}
