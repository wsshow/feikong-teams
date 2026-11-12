import { tokenize } from "@openuidev/lang-core";
import { maxUIProgramLength } from "./protocol";

// 使用与渲染器相同的 lexer，避免字符串、注释、转义影响动作检测。
const kind = (sample: string) => tokenize(sample)[0].t;
const call = kind("Stack");
const builtin = kind("@Set");
const open = new Set([kind("("), kind("["), kind("{")]);
const close = new Set([kind(")"), kind("]"), kind("}")]);
const allowedFunctions = new Set(["ToAssistant", "OpenUrl", "Set", "Reset", "Count", "First", "Last", "Sum", "Avg", "Min", "Max", "Sort", "Filter", "Round", "Abs", "Floor", "Ceil", "Each"]);

export function programProblem(program: string) {
  if (program.length > maxUIProgramLength) return true;
  let depth = 0;
  for (const token of tokenize(program)) {
    if (token.t === call && (token.v === "Query" || token.v === "Mutation")) return true;
    if (token.t === builtin && !allowedFunctions.has(String(token.v))) return true;
    if (open.has(token.t)) depth++;
    if (close.has(token.t)) depth--;
    if (depth > 40) return true;
  }
  return false;
}

export function safeExternalURL(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined;
  try {
    const url = new URL(value);
    return ["https:", "http:"].includes(url.protocol) && !url.username && !url.password ? url.href : undefined;
  } catch { return undefined; }
}
