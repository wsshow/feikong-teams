import { expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import postcss from "postcss";

const tokens = new Map<string, string>();
postcss.parse(readFileSync(new URL("./globals.css", import.meta.url), "utf8")).walkRules(":root", (rule) => {
  rule.walkDecls(/^--/, ({ prop, value }) => { tokens.set(prop, value); });
});

function luminance(name: string): number {
  let value = tokens.get(`--${name}`)!;
  const visited = new Set<string>();
  while (value.startsWith("var(")) {
    if (visited.has(value)) throw new Error(`cyclic color token: ${name}`);
    visited.add(value);
    value = tokens.get(value.slice(4, -1))!;
  }
  const [hue, saturation, lightness] = value.split(/\s+/).map((part) => parseFloat(part));
  const h = hue / 30;
  const s = saturation / 100;
  const l = lightness / 100;
  const amplitude = s * Math.min(l, 1 - l);
  const rgb = [0, 8, 4].map((offset) => {
    const k = (offset + h) % 12;
    const channel = l - amplitude * Math.max(-1, Math.min(k - 3, 9 - k, 1));
    return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
  });
  return rgb[0] * 0.2126 + rgb[1] * 0.7152 + rgb[2] * 0.0722;
}

// 只读和禁用控件单独表达状态；可读正文、说明与可操作按钮保持普通字号 AA 对比度。
const readablePairs = [
  ["foreground", "background"],
  ["card-foreground", "card"],
  ["popover-foreground", "popover"],
  ["sidebar-foreground", "sidebar"],
  ["muted-foreground", "background"],
  ["muted-foreground", "card"],
  ["muted-foreground", "muted"],
  ["secondary-foreground", "secondary"],
  ["accent-foreground", "accent"],
  ["primary", "accent"],
  ["primary-foreground", "primary"],
  ["primary-foreground", "primary-hover"],
  ["primary-foreground", "primary-pressed"],
  ["destructive-foreground", "destructive"],
  ["destructive-foreground", "destructive-hover"],
  ["success", "success-muted"],
  ["warning", "warning-muted"],
];

test.each(readablePairs)("theme keeps %s readable on %s", (foreground, background) => {
  const values = [luminance(foreground), luminance(background)].sort((left, right) => left - right);
  expect((values[1] + 0.05) / (values[0] + 0.05)).toBeGreaterThanOrEqual(4.5);
});
