"use client";
import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import { createPortal } from "react-dom";
import clsx from "clsx";

type Placement = "top" | "bottom" | "left" | "right";
type Point = { x: number; y: number };

function subscribeCoarsePointer(onChange: () => void) {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
    return () => {};
  }
  const media = window.matchMedia("(pointer: coarse)");
  const handler = () => onChange();
  media.addEventListener("change", handler);
  return () => media.removeEventListener("change", handler);
}

function getCoarsePointerSnapshot() {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
    return false;
  }
  try {
    return window.matchMedia("(pointer: coarse)").matches;
  } catch {
    return false;
  }
}

export default function Tooltip({
  content,
  children,
  placement = "top",
  closeable = true,
  closeDelay = 120,
  trackPointer = true,
}: {
  content: React.ReactNode;
  children: React.ReactNode;
  placement?: Placement;
  closeable?: boolean;
  closeDelay?: number;
  trackPointer?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const longPress = useRef<ReturnType<typeof setTimeout> | null>(null);
  const rootRef = useRef<HTMLSpanElement>(null);
  const tipRef = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null);
  const pointer = useRef<Point | null>(null);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const raf = useRef<number | null>(null);
  const coarse = useSyncExternalStore(
    subscribeCoarsePointer,
    getCoarsePointerSnapshot,
    () => false,
  );
  const EDGE = 12; // 视口边距
  const MAXW = 420; // 桌面最大宽度

  useEffect(() => {
    if (!closeable) return;
    const captureOptions = { capture: true } as const;
    const onDoc = (e: MouseEvent | TouchEvent | PointerEvent) => {
      if (!rootRef.current) return;
      const t = e.target as Node;
      if (
        !rootRef.current.contains(t) &&
        (!tipRef.current || !tipRef.current.contains(t as Node))
      )
        setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    // 捕获阶段尽早关闭，避免被其它组件阻止冒泡
    document.addEventListener("pointerdown", onDoc, captureOptions);
    document.addEventListener("click", onDoc, captureOptions);
    document.addEventListener("touchstart", onDoc, captureOptions);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onDoc, captureOptions);
      document.removeEventListener("click", onDoc, captureOptions);
      document.removeEventListener("touchstart", onDoc, captureOptions);
      document.removeEventListener("keydown", onKey);
    };
  }, [closeable]);

  useLayoutEffect(() => {
    if (!open) return;
    const update = () => {
      if (trackPointer && pointer.current && !coarse) {
        const { x, y } = pointer.current;
        const offset = 18;
        const top = Math.max(EDGE, y - offset);
        const w = tipRef.current?.offsetWidth || MAXW;
        const half = w / 2;
        const left = Math.min(
          Math.max(EDGE + half, x),
          window.innerWidth - (EDGE + half),
        );
        setPos({ top, left });
      } else {
        const el = rootRef.current;
        if (!el) return;
        const r = el.getBoundingClientRect();
        let top = 0,
          left = 0;
        switch (placement) {
          case "bottom":
            top = r.bottom + 8;
            left = r.left + r.width / 2;
            break;
          case "left":
            top = r.top + r.height / 2;
            left = r.left - 8;
            break;
          case "right":
            top = r.top + r.height / 2;
            left = r.right + 8;
            break;
          case "top":
          default:
            top = Math.max(EDGE, r.top - 8);
            left = r.left + r.width / 2;
        }
        setPos({ top, left });
      }
    };
    update();
    const onScroll = () => update();
    window.addEventListener("scroll", onScroll, true);
    window.addEventListener("resize", onScroll);
    return () => {
      window.removeEventListener("scroll", onScroll, true);
      window.removeEventListener("resize", onScroll);
    };
  }, [open, placement, coarse, trackPointer]);

  useEffect(() => {
    return () => {
      if (closeTimer.current) clearTimeout(closeTimer.current);
      if (longPress.current) clearTimeout(longPress.current);
      if (raf.current != null) cancelAnimationFrame(raf.current);
    };
  }, []);

  const transform =
    placement === "left" || placement === "right"
      ? "-translate-y-1/2"
      : "-translate-x-1/2";

  return (
    <span
      ref={rootRef}
      className="relative inline-flex items-center"
      onMouseEnter={(e) => {
        if (trackPointer)
          pointer.current = { x: e.clientX, y: e.clientY };
        setOpen(true);
      }}
      onMouseLeave={() => {
        if (!closeable) return;
        if (closeTimer.current) clearTimeout(closeTimer.current);
        closeTimer.current = setTimeout(() => setOpen(false), closeDelay);
      }}
      onMouseMove={(e) => {
        if (trackPointer && !coarse) {
          pointer.current = { x: e.clientX, y: e.clientY };
          if (open) {
            if (raf.current) cancelAnimationFrame(raf.current);
            raf.current = requestAnimationFrame(() => {
              const { x, y } = pointer.current!;
              const offset = 18;
              const top = Math.max(EDGE, y - offset);
              const w = tipRef.current?.offsetWidth || MAXW;
              const half = w / 2;
              const left = Math.min(
                Math.max(EDGE + half, x),
                window.innerWidth - (EDGE + half),
              );
              setPos({ top, left });
            });
          }
        }
      }}
      onFocus={() => setOpen(true)}
      onBlur={() => {
        if (closeable) setOpen(false);
      }}
      onClick={() => {
        // 触屏点击用于开/关；桌面点击不改变状态
        if (coarse && closeable) setOpen((v) => !v);
      }}
      onTouchStart={() => {
        if (longPress.current) clearTimeout(longPress.current);
        longPress.current = setTimeout(() => setOpen(true), 350);
      }}
      onTouchEnd={() => {
        if (longPress.current) clearTimeout(longPress.current);
      }}
    >
      {children}
      {open &&
        pos &&
        createPortal(
          <>
            {/* 在桌面悬浮模式不渲染遮罩，避免阻挡鼠标；触屏设备保留遮罩用于关闭 */}
            {coarse && (
              <span
                className="fixed inset-0 z-[9998]"
                onClick={() => {
                  if (closeable) setOpen(false);
                }}
                onTouchStart={() => {
                  if (closeable) setOpen(false);
                }}
              />
            )}
            <div
              ref={tipRef}
              className={clsx(
                // 桌面悬浮下禁用指针事件，避免拦截鼠标
                coarse ? "pointer-events-auto" : "pointer-events-none",
                "fixed z-[9999] rounded-md border p-2 text-[11px] shadow-lg",
                transform,
              )}
              style={{
                top: pos.top,
                left: pos.left,
                background: "var(--tooltip-bg)",
                borderColor: "var(--tooltip-border)",
                color: "var(--tooltip-fg)",
                maxWidth: `min(${MAXW}px, calc(100vw - ${EDGE * 2}px))`,
                width: "max-content",
                whiteSpace: "normal",
                wordBreak: "break-word",
                overflowWrap: "anywhere",
                lineHeight: 1.4,
              }}
              role="tooltip"
              onMouseLeave={() => {
                if (!closeable) return;
                if (closeTimer.current) clearTimeout(closeTimer.current);
                closeTimer.current = setTimeout(
                  () => setOpen(false),
                  closeDelay,
                );
              }}
              onMouseEnter={() => {
                if (closeTimer.current) clearTimeout(closeTimer.current);
              }}
            >
              {content}
            </div>
          </>,
          document.body,
        )}
    </span>
  );
}
