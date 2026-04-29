"use client";

import { useTheme } from "@/store/useTheme";

export function Header() {
  const theme = useTheme((s) => s.theme);
  const setTheme = useTheme((s) => s.setTheme);
  const barCls = "sticky top-0 z-50 w-full border-b backdrop-blur-xl";

  return (
    <header
      className={barCls}
      style={{
        background: "var(--header-bg)",
        borderColor: "var(--header-border)",
      }}
    >
      <div
        className="ui-sans mx-auto flex h-[var(--header-h)] w-full max-w-[1680px] items-center justify-between gap-4 px-4 text-xs sm:px-5"
        style={{ color: "var(--foreground)" }}
      >
        <div className="flex min-w-0 items-center gap-3" />

        <div className="flex items-center gap-2 text-[11px]">
          <div
            className="flex overflow-hidden rounded-full border p-1"
            style={{
              borderColor: "color-mix(in srgb, var(--chip-border) 90%, transparent)",
              background: "color-mix(in srgb, var(--logo-chip-bg) 78%, transparent)",
            }}
          >
            {(["dark", "light", "system"] as const).map((t) => (
              <button
                key={t}
                title={t}
                className="rounded-full px-3 py-1.5 capitalize chip-btn"
                style={
                  theme === t
                    ? {
                        background: "var(--btn-active-bg)",
                        color: "var(--btn-active-fg)",
                        boxShadow:
                          "inset 0 0 0 1px color-mix(in srgb, var(--panel-border) 82%, transparent)",
                      }
                    : { color: "var(--btn-inactive-fg)" }
                }
                onClick={() => setTheme(t)}
              >
                {t}
              </button>
            ))}
          </div>
        </div>
      </div>
    </header>
  );
}

export default Header;
