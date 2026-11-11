import { ChevronDown } from "lucide-react";
import { useEffect, useId, useRef, useState, type AriaAttributes, type KeyboardEvent } from "react";
import { cn } from "@/lib/cn";

interface SelectProps extends Pick<AriaAttributes, "aria-label" | "aria-labelledby" | "aria-describedby"> {
  id?: string;
  className?: string;
  value?: string;
  options: string[];
  optionLabels?: Record<string, string>;
  onChange: (value: string) => void;
  disabled?: boolean;
}

export function Select({ value, options, optionLabels, onChange, disabled, className, ...props }: SelectProps) {
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);
  const containerRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef({ text: "", time: 0 });
  const listID = useId();
  const selected = value ?? options[0] ?? "";
  const selectedIndex = options.indexOf(selected);
  const expanded = open && !disabled && options.length > 0;
  const labelFor = (option: string) => optionLabels?.[option] ?? (option || "未选择");

  useEffect(() => {
    if (!expanded) return;
    function dismiss(event: PointerEvent) {
      if (!containerRef.current?.contains(event.target as Node)) setOpen(false);
    }
    document.addEventListener("pointerdown", dismiss);
    return () => document.removeEventListener("pointerdown", dismiss);
  }, [expanded]);

  useEffect(() => {
    if (disabled) setOpen(false);
  }, [disabled]);

  useEffect(() => {
    if (expanded) (listRef.current?.children[activeIndex] as HTMLElement | undefined)?.scrollIntoView?.({ block: "nearest" });
  }, [expanded, activeIndex]);

  function show(index = Math.max(0, selectedIndex)) {
    searchRef.current = { text: "", time: 0 };
    setActiveIndex(index);
    setOpen(true);
  }

  function choose(index: number) {
    if (options[index] === undefined) return;
    onChange(options[index]);
    setOpen(false);
    triggerRef.current?.focus();
  }

  function handleKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    if (disabled || !options.length) return;
    switch (event.key) {
      case "ArrowDown":
      case "ArrowUp": {
        event.preventDefault();
        const step = event.key === "ArrowDown" ? 1 : -1;
        if (!expanded) show(selectedIndex >= 0 ? selectedIndex : step > 0 ? 0 : options.length - 1);
        else setActiveIndex((current) => Math.max(0, Math.min(options.length - 1, current + step)));
        return;
      }
      case "Home":
      case "End":
        event.preventDefault();
        show(event.key === "Home" ? 0 : options.length - 1);
        return;
      case "Enter":
      case " ":
        event.preventDefault();
        if (expanded) choose(activeIndex);
        else show();
        return;
      case "Escape":
        if (expanded) {
          event.preventDefault();
          event.stopPropagation();
          setOpen(false);
        }
        return;
      case "Tab":
        setOpen(false);
        return;
    }
    if (event.key.length !== 1 || event.ctrlKey || event.metaKey || event.altKey) return;
    event.preventDefault();
    const now = Date.now();
    const text = (expanded && now - searchRef.current.time < 500 ? searchRef.current.text : "") + event.key.toLocaleLowerCase();
    const index = options.findIndex((option) => labelFor(option).toLocaleLowerCase().startsWith(text));
    if (!expanded) show();
    searchRef.current = { text, time: now };
    if (index >= 0) setActiveIndex(index);
  }

  return (
    <div
      ref={containerRef}
      className={cn("relative", className)}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false);
      }}
    >
      <button
        {...props}
        ref={triggerRef}
        type="button"
        role="combobox"
        aria-haspopup="listbox"
        aria-expanded={expanded}
        aria-controls={expanded ? listID : undefined}
        aria-activedescendant={expanded ? `${listID}-${activeIndex}` : undefined}
        disabled={disabled}
        className={cn(
          "sketch-inset flex h-9 w-full items-center justify-between gap-2 rounded-md px-3 py-1 text-left text-sm transition-colors hover:border-primary/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50",
          expanded && "ring-2 ring-ring",
        )}
        onClick={() => expanded ? setOpen(false) : show()}
        onKeyDown={handleKeyDown}
      >
        <span className={cn("min-w-0 truncate", !selected && "text-muted-foreground")}>{labelFor(selected)}</span>
        <ChevronDown aria-hidden="true" className={cn("h-4 w-4 shrink-0 text-muted-foreground transition-transform motion-reduce:transition-none", expanded && "rotate-180")} />
      </button>
      {expanded && (
        <div
          ref={listRef}
          id={listID}
          role="listbox"
          aria-label={props["aria-label"]}
          aria-labelledby={props["aria-labelledby"]}
          className="sketch-surface absolute left-0 right-0 top-[calc(100%+0.4rem)] z-40 max-h-56 space-y-1 overflow-y-auto rounded-xl bg-card p-2 text-sm shadow-[0_14px_32px_hsl(218_30%_25%/0.16)]"
        >
          {options.map((option, index) => (
            <button
              key={option}
              id={`${listID}-${index}`}
              type="button"
              role="option"
              tabIndex={-1}
              aria-selected={selected === option}
              className={cn(
                "flex w-full items-center rounded-lg px-3 py-2 text-left transition-colors",
                index === activeIndex && "bg-accent/70",
                selected === option && "bg-primary/10 text-primary",
              )}
              onPointerMove={() => setActiveIndex(index)}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => choose(index)}
            >
              <span className={cn("min-w-0 truncate", !option && "text-muted-foreground")}>{labelFor(option)}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
