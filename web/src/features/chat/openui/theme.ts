import { createTheme, defaultLightTheme } from "@openuidev/react-ui";

const font = '"Kaiti SC", "STKaiti", "KaiTi", "LXGW WenKai", ui-sans-serif, system-ui, sans-serif';
export const assistantUITheme = createTheme({
  // 上游复合字体 token 内嵌 Inter，仅设置 fontBody 不会改变实际字体。
  ...Object.fromEntries(Object.entries(defaultLightTheme).filter(([key, value]) => key.startsWith("text") && typeof value === "string" && value.includes('"Inter"')).map(([key, value]) => [key, String(value).replace(/"Inter", sans-serif/g, font)])),
  background: "hsl(var(--background))",
  foreground: "hsl(var(--card))",
  popoverBackground: "hsl(var(--popover))",
  sunk: "hsl(var(--muted))",
  sunkLight: "hsl(var(--muted) / 0.5)",
  textNeutralPrimary: "hsl(var(--foreground))",
  textNeutralSecondary: "hsl(var(--muted-foreground))",
  textNeutralTertiary: "hsl(var(--muted-foreground))",
  textNeutralLink: "hsl(var(--primary))",
  textAccentPrimary: "hsl(var(--primary-foreground))",
  interactiveAccentDefault: "hsl(var(--primary))",
  interactiveAccentHover: "hsl(var(--primary) / 0.9)",
  interactiveAccentPressed: "hsl(var(--primary))",
  interactiveAccentDisabled: "hsl(var(--primary) / 0.45)",
  borderDefault: "hsl(var(--border))",
  borderInteractive: "hsl(var(--input))",
  borderInteractiveSelected: "hsl(var(--ring))",
  fontBody: font,
  fontHeading: font,
  fontLabel: font,
  fontCode: '"SFMono-Regular", Consolas, monospace',
});
