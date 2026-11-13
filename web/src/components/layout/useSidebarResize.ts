import { useEffect, useRef, useState } from "react";
import type { KeyboardEvent, PointerEvent } from "react";

const storageKey = "fk_sidebar_width";
const defaultWidth = 292;
const minWidth = 240;
const preferredMaxWidth = 480;

function readWidth() {
  try {
    const saved = Number(localStorage.getItem(storageKey));
    if (Number.isFinite(saved) && saved >= minWidth && saved <= preferredMaxWidth) return saved;
  } catch {
    // 浏览器禁用本地存储时仍允许调整当前页面。
  }
  return defaultWidth;
}

export function useSidebarResize(enabled: boolean) {
  const [preferredWidth, setPreferredWidth] = useState(readWidth);
  const [viewportWidth, setViewportWidth] = useState(() => window.innerWidth);
  const [isResizing, setIsResizing] = useState(false);
  const drag = useRef<{ pointerID: number; startX: number; startWidth: number } | null>(null);
  // 至少保留一半视口给内容区，缩小窗口时保留用户原先选择的宽度。
  const maxWidth = Math.max(minWidth, Math.min(preferredMaxWidth, Math.floor(viewportWidth / 2)));
  const width = Math.min(preferredWidth, maxWidth);
  const clampWidth = (value: number) => Math.round(Math.max(minWidth, Math.min(value, maxWidth)));

  useEffect(() => {
    try {
      localStorage.setItem(storageKey, String(preferredWidth));
    } catch {
      // 存储失败不影响拖拽交互。
    }
  }, [preferredWidth]);

  useEffect(() => {
    function stopResizing() {
      drag.current = null;
      setIsResizing(false);
    }
    function updateViewport() {
      setViewportWidth(window.innerWidth);
      stopResizing();
    }
    window.addEventListener("resize", updateViewport);
    window.addEventListener("blur", stopResizing);
    return () => {
      window.removeEventListener("resize", updateViewport);
      window.removeEventListener("blur", stopResizing);
    };
  }, []);

  useEffect(() => {
    if (!enabled) {
      drag.current = null;
      setIsResizing(false);
    }
  }, [enabled]);

  useEffect(() => {
    if (!isResizing) return;
    const { style } = document.body;
    const { cursor, userSelect } = style;
    style.cursor = "col-resize";
    style.userSelect = "none";
    return () => {
      style.cursor = cursor;
      style.userSelect = userSelect;
    };
  }, [isResizing]);

  function onPointerDown(event: PointerEvent<HTMLDivElement>) {
    if (!enabled || window.innerWidth < 768 || event.button !== 0 || !event.isPrimary) return;
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    drag.current = { pointerID: event.pointerId, startX: event.clientX, startWidth: width };
    setIsResizing(true);
  }

  function onPointerMove(event: PointerEvent<HTMLDivElement>) {
    if (!drag.current || drag.current.pointerID !== event.pointerId) return;
    setPreferredWidth(clampWidth(drag.current.startWidth + event.clientX - drag.current.startX));
  }

  function stopPointer(event: PointerEvent<HTMLDivElement>) {
    if (drag.current?.pointerID !== event.pointerId) return;
    drag.current = null;
    setIsResizing(false);
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (!enabled || window.innerWidth < 768) return;
    const step = event.shiftKey ? 40 : 10;
    const next = { ArrowLeft: width - step, ArrowRight: width + step, Home: minWidth, End: maxWidth }[event.key];
    if (next === undefined) return;
    event.preventDefault();
    setPreferredWidth(clampWidth(next));
  }

  return {
    width, minWidth, maxWidth, isResizing,
    separatorProps: {
      onPointerDown,
      onPointerMove,
      onPointerUp: stopPointer,
      onPointerCancel: stopPointer,
      onLostPointerCapture: stopPointer,
      onKeyDown,
      onDoubleClick: () => setPreferredWidth(defaultWidth),
    },
  };
}
