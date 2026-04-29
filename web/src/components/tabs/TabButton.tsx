"use client";
import { useTab } from "./TabContext";

export default function TabButton({
  name,
  tabKey,
  disabled = false,
}: {
  name: string;
  tabKey?: string;
  disabled?: boolean;
}) {
  const { tab, setTab } = useTab();
  const active = tabKey ? tab === tabKey : false;

  return (
    <button
      onClick={() => {
        if (disabled || !tabKey) return;
        setTab(tabKey);
      }}
      aria-disabled={disabled}
      type="button"
      className={`relative inline-flex items-center px-3 py-2 text-[12px] font-medium tracking-wide transition-colors whitespace-nowrap
        ${disabled ? "cursor-not-allowed opacity-40" : "cursor-pointer"}
        ${active ? "" : "hover:text-foreground"}
      `}
      style={{
        color: active ? "var(--foreground)" : "var(--muted-text)",
      }}
    >
      {name}
      {active && (
        <span
          className="absolute bottom-0 left-0 right-0 h-[2px] rounded-full"
          style={{ background: "linear-gradient(90deg, rgba(16,163,127,0.9), rgba(77,107,254,0.9))" }}
        />
      )}
    </button>
  );
}
