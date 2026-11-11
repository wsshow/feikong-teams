import { describe, expect, test } from "bun:test";
import { mergeDeepConfig, normalizeConfig, normalizeConfigForSave } from "./configModel";
import type { AppConfig } from "@/types/config";

describe("configuration model", () => {
  test("saving serializes built-in overrides and preserves complete custom agent definitions", () => {
    const config: AppConfig = { agents: { items: [
      { id: "coordinator", builtin: true, enabled: true },
      { id: "coder", builtin: true, enabled: false, prompt: "server-owned" },
      { id: "reviewer", name: "Reviewer", enabled: true, prompt: "review", tools: ["file"] },
    ] } };
    const before = structuredClone(config);
    expect(normalizeConfigForSave(config).agents?.items).toEqual([
      { id: "coder", enabled: false },
      { id: "reviewer", name: "Reviewer", enabled: true, prompt: "review", tools: ["file"] },
    ]);
    expect(config).toEqual(before);
  });

  test("explicit false values survive defaults and partial deep updates", () => {
    const config = normalizeConfig({ deep: { shell: { enabled: false, timeout: "90s" } }, tools: { approval: { auto_approve: ["file", "all", "file", "invalid"] } } });
    expect(config.deep?.shell?.enabled).toBe(false);
    expect(config.tools?.approval?.auto_approve).toEqual(["all"]);
    expect(mergeDeepConfig(config.deep || {}, { shell: { streaming: true } }).shell).toEqual({ enabled: false, timeout: "90s", streaming: true });
  });
});
