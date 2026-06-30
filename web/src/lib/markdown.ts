import { Marked } from "marked";
import markedKatex from "marked-katex-extension";

const marked = new Marked({
  breaks: true,
  gfm: true,
});

marked.use(markedKatex({
  nonStandard: true,
  throwOnError: false,
}));

export function renderMarkdown(value?: string) {
  if (!value) return "";
  try {
    return withCodeCopyButtons(marked.parse(normalizeMathDelimiters(value)) as string);
  } catch {
    return escapeHTML(value).replace(/\n/g, "<br />");
  }
}

function withCodeCopyButtons(html: string) {
  return html.replace(/<pre><code([^>]*)>([\s\S]*?)<\/code><\/pre>/g, (_match, attrs: string, code: string) => (
    `<div class="markdown-code-block"><button class="markdown-code-copy" type="button" data-markdown-copy title="复制代码" aria-label="复制代码">复制</button><pre><code${attrs}>${code}</code></pre></div>`
  ));
}

function normalizeMathDelimiters(value: string) {
  return value
    .split(/(```[\s\S]*?```|~~~[\s\S]*?~~~|`[^`\n]*`)/g)
    .map((part) => {
      if (part.startsWith("```") || part.startsWith("~~~") || part.startsWith("`")) return part;
      return part
        .replace(/\\\[([\s\S]*?)\\\]/g, (_, formula: string) => `\n$$\n${formula.trim()}\n$$\n`)
        .replace(/\\\(([\s\S]*?)\\\)/g, (_, formula: string) => `$${formula.trim()}$`);
    })
    .join("");
}

export function escapeHTML(value: string) {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}
