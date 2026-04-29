"use client";
import { useMemo } from "react";

const SIDES = ["ALL", "LONG", "SHORT"] as const;

export default function PositionsFilter({
  models,
  symbols,
  model,
  symbol,
  side,
  onChange,
}: {
  models: string[];
  symbols: string[];
  model: string;
  symbol: string;
  side: string;
  onChange: (next: Record<string, string>) => void;
}) {
  const modelOptions = useMemo(() => ["ALL", ...models], [models]);
  const symbolOptions = useMemo(() => ["ALL", ...symbols], [symbols]);

  return (
    <div
      className="flex flex-wrap items-center gap-2 text-[11px]"
      style={{ color: "var(--muted-text)" }}
    >
      <Select label="模型" value={model} options={modelOptions} onChange={(v) => onChange({ model: v })} />
      <Select label="币种" value={symbol} options={symbolOptions} onChange={(v) => onChange({ symbol: v })} />
      <Select label="方向" value={side} options={SIDES as unknown as string[]} onChange={(v) => onChange({ side: v })} />
    </div>
  );
}

function Select({ label, value, options, onChange }: {
  label: string; value: string; options: string[]; onChange: (v: string) => void;
}) {
  return (
    <label className="tab-filter-chip">
      <span className="tab-filter-label">{label}</span>
      <select className="tab-select" value={value} onChange={(e) => onChange(e.target.value)}>
        {options.map((opt) => (
          <option key={opt} value={opt}>{opt}</option>
        ))}
      </select>
    </label>
  );
}
