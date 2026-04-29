"use client";
import { useState, useMemo } from "react";
import { useMacroNews } from "@/lib/api/hooks/useMacroNews";
import { useMacroOutlook } from "@/lib/api/hooks/useMacroOutlook";
import { OutlookContent } from "@/components/news/MacroOutlookCard";
import ErrorBanner from "@/components/ui/ErrorBanner";
import { fmtTs } from "@/lib/utils/formatters";

export default function MacroNewsPanel() {
  const { items, isLoading, isError } = useMacroNews();
  const { outlook } = useMacroOutlook();
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [showOutlook, setShowOutlook] = useState(false);

  const digestIdSet = useMemo(
    () => new Set(outlook?.digest_ids ?? []),
    [outlook],
  );

  return (
    <div className="flex flex-col gap-3">
      <ErrorBanner message={isError ? "宏观新闻源暂不可用，请稍后重试。" : undefined} />

      {/* 宏观研判可折叠块 */}
      {outlook && (
        <div
          className="rounded-xl border overflow-hidden"
          style={{ borderColor: "color-mix(in srgb, var(--panel-border) 80%, transparent)", background: "color-mix(in srgb, var(--logo-chip-bg) 50%, transparent)" }}
        >
          <button
            className="w-full flex items-center justify-between px-4 py-2.5 text-left"
            onClick={() => setShowOutlook((v) => !v)}
          >
            <span className="text-[11px] font-bold tracking-[0.18em] uppercase" style={{ color: "var(--muted-text)" }}>
              宏观基本面研判
            </span>
            <span className="text-[11px]" style={{ color: "var(--muted-text)" }}>{showOutlook ? "▲ 收起" : "▼ 展开"}</span>
          </button>
          {showOutlook && (
            <div className="border-t px-4 pb-4 pt-3" style={{ borderColor: "color-mix(in srgb, var(--panel-border) 50%, transparent)", color: "var(--foreground)" }}>
              <OutlookContent outlook={outlook} />
            </div>
          )}
        </div>
      )}

      {/* 新闻滚动列表 */}
      <div
        className="rounded-xl border overflow-hidden"
        style={{ borderColor: "color-mix(in srgb, var(--panel-border) 80%, transparent)" }}
      >
        {/* 列表头 */}
        <div
          className="flex items-center justify-between px-4 py-2.5 border-b"
          style={{ borderColor: "color-mix(in srgb, var(--panel-border) 55%, transparent)", background: "color-mix(in srgb, var(--logo-chip-bg) 60%, transparent)" }}
        >
          <span className="text-[11px] font-bold tracking-[0.18em] uppercase" style={{ color: "var(--muted-text)" }}>
            Macro Wire
          </span>
          <span className="text-[11px]" style={{ color: "var(--muted-text)" }}>
            {items.length} 条
          </span>
        </div>

        {isLoading && !items.length && (
          <div className="px-4 py-6 text-xs" style={{ color: "var(--muted-text)" }}>正在拉取宏观新闻…</div>
        )}
        {!isLoading && !items.length && (
          <div className="px-4 py-6 text-xs" style={{ color: "var(--muted-text)" }}>暂无新闻</div>
        )}

        {/* 新闻条目列表 */}
        <div className="divide-y" style={{ "--tw-divide-opacity": "1" } as React.CSSProperties}>
          {items.map((item) => {
            const inOutlook = digestIdSet.has(item.id);
            const isExpanded = expandedId === item.id;
            const impactColor =
              item.impact === "利好" ? "text-green-400" :
              item.impact === "利空" ? "text-red-400" : "";

            return (
              <div
                key={item.id}
                className="border-b last:border-b-0"
                style={{ borderColor: "color-mix(in srgb, var(--panel-border) 40%, transparent)" }}
              >
                {/* 折叠行：点击展开/收起 */}
                <button
                  className="w-full flex items-start gap-3 px-4 py-3 text-left transition-colors hover:bg-white/[0.03]"
                  onClick={() => setExpandedId(isExpanded ? null : item.id)}
                >
                  {/* 影响标签 */}
                  <span
                    className={`shrink-0 mt-0.5 rounded px-1.5 py-0.5 text-[10px] font-bold ${impactColor}`}
                    style={{ background: "color-mix(in srgb, var(--logo-chip-bg) 80%, transparent)" }}
                  >
                    {item.impact ?? "—"}
                  </span>

                  {/* 标题 */}
                  <span className="flex-1 text-sm leading-snug font-medium line-clamp-2" style={{ color: "var(--foreground)" }}>
                    {item.headline || "—"}
                  </span>

                  {/* 右侧：已纳入研判 + 时间 + 展开箭头 */}
                  <div className="shrink-0 flex flex-col items-end gap-1">
                    {inOutlook && (
                      <span className="rounded px-1.5 py-0.5 text-[9px] font-medium" style={{ background: "var(--brand-accent)", color: "var(--background)" }}>
                        已纳入
                      </span>
                    )}
                    <time className="text-[11px] tabular-nums" style={{ color: "var(--muted-text)" }}>
                      {fmtTs(item.timestamp ?? undefined)}
                    </time>
                    <span className="text-[10px]" style={{ color: "var(--muted-text)" }}>{isExpanded ? "▲" : "▼"}</span>
                  </div>
                </button>

                {/* 展开详情 */}
                {isExpanded && (
                  <div
                    className="px-4 pb-4 pt-1 border-t space-y-3"
                    style={{ borderColor: "color-mix(in srgb, var(--panel-border) 40%, transparent)", background: "color-mix(in srgb, var(--logo-chip-bg) 30%, transparent)" }}
                  >
                    {/* 来源 */}
                    {(item.source || item.url) && (
                      <div className="flex flex-wrap items-center gap-3 text-xs" style={{ color: "var(--muted-text)" }}>
                        {item.source && <span>来源：{item.source}</span>}
                        {item.url && (
                          <a href={item.url} target="_blank" rel="noreferrer"
                            className="hover:underline" style={{ color: "var(--brand-accent)" }}
                            onClick={(e) => e.stopPropagation()}>
                            查看原文
                          </a>
                        )}
                      </div>
                    )}

                    {/* 正文摘要 */}
                    {item.summary && (
                      <p className="text-sm leading-relaxed" style={{ color: "var(--foreground)" }}>
                        {item.summary}
                      </p>
                    )}

                    {/* AI 观点 */}
                    {item.reasoning && (
                      <div className="rounded-lg px-3 py-2.5 text-xs leading-relaxed"
                        style={{ background: "color-mix(in srgb, var(--logo-chip-bg) 70%, transparent)", color: "var(--muted-text)" }}>
                        <span className="font-semibold" style={{ color: "var(--foreground)" }}>AI 观点：</span>
                        {item.reasoning}
                      </div>
                    )}

                    {/* 底部标签行 */}
                    <div className="flex flex-wrap items-center gap-2">
                      {item.sentiment && (
                        <span className={`rounded px-2 py-0.5 text-xs font-medium ${
                          item.sentiment === "positive" ? "bg-green-500/20 text-green-400" :
                          item.sentiment === "negative" ? "bg-orange-500/20 text-orange-400" : ""
                        }`} style={item.sentiment === "neutral" ? { background: "var(--logo-chip-bg)", color: "var(--muted-text)" } : undefined}>
                          {item.sentiment === "positive" ? "看多" : item.sentiment === "negative" ? "看空" : "中性"}
                        </span>
                      )}
                      {item.confidence != null && (
                        <span className="rounded px-2 py-0.5 text-xs" style={{ background: "var(--logo-chip-bg)", color: "var(--muted-text)" }}>
                          置信度 {item.confidence}%
                        </span>
                      )}
                      {item.source_type && (
                        <span className="rounded px-2 py-0.5 text-xs" style={{ background: "var(--logo-chip-bg)", color: "var(--muted-text)" }}>
                          {sourceTypeLabel(item.source_type)}
                        </span>
                      )}
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}

function sourceTypeLabel(sourceType?: string) {
  switch (sourceType) {
    case "rss": return "RSS";
    case "websocket": return "WebSocket";
    case "web_search": return "搜索补源";
    default: return "新闻源";
  }
}
