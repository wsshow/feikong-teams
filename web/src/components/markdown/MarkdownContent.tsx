import type { MouseEvent } from "react";
import { cn } from "@/lib/cn";
import { renderMarkdown } from "@/lib/markdown";

export function MarkdownContent({ content, className }: { content?: string; className?: string }) {
  return (
    <div
      className={cn("prose message-prose w-full max-w-none", className)}
      onClick={(event) => void handleMarkdownClick(event)}
      dangerouslySetInnerHTML={{ __html: renderMarkdown(content) }}
    />
  );
}

async function handleMarkdownClick(event: MouseEvent<HTMLDivElement>) {
  const target = event.target instanceof Element ? event.target : null;
  const button = target?.closest<HTMLButtonElement>("[data-markdown-copy]");
  if (!button) return;

  const block = button.closest(".markdown-code-block");
  const code = block?.querySelector("pre code")?.textContent || "";
  if (!code) return;

  await navigator.clipboard?.writeText(code);
  const previous = button.textContent || "复制";
  button.textContent = "已复制";
  button.dataset.copied = "true";
  window.setTimeout(() => {
    button.textContent = previous;
    delete button.dataset.copied;
  }, 1200);
}
