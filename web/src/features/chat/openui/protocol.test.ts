import { describe, expect, test } from "bun:test";
import { actionSummary, decodeUIAction, encodeUIAction, restoreUIState, splitUIMessage } from "./protocol";
import { createParser } from "@openuidev/react-lang";
import { assistantUIExamples, assistantUILibrary, assistantUIPromptOptions } from "./library";
import { openuiLibrary } from "@openuidev/react-ui";
import { programProblem, safeExternalURL } from "./program";

describe("assistant UI content", () => {
  test("keeps Markdown, ordinary code, multiple UI blocks and partial streams separate", () => {
    const code = 'root = Card("说明", [Text("内容")])';
    const parts = splitUIMessage(`说明\n\n\`\`\`openui-lang\n${code}\n\`\`\`\n后文\n~~~openui-lang\nroot = Card("未完成`);
    expect(parts.map((part) => part.kind)).toEqual(["markdown", "openui", "markdown", "openui"]);
    expect(parts[1]).toEqual({ kind: "openui", content: `${code}\n`, complete: true, index: 0 });
    expect(parts[3]).toMatchObject({ complete: false, index: 1 });
    const example = `\`\`\`\`markdown\n\`\`\`openui-lang\n${code}\n\`\`\`\n\`\`\`\`\n`;
    expect(splitUIMessage(example)).toEqual([{ kind: "markdown", content: example }]);
    expect(splitUIMessage("普通 Markdown")).toEqual([{ kind: "markdown", content: "普通 Markdown" }]);
  });

  test("action values survive a persisted user message and malformed actions remain plain text", () => {
    const raw = encodeUIAction({ version: 1, source: "message:block:0", message: "创建任务", formState: { weekly: { name: { value: "工作总结", componentType: "Input" } } } });
    const restored = decodeUIAction(raw)!;
    expect(restored.formState?.weekly.name).toEqual({ value: "工作总结" });
    expect(actionSummary(restored)).toBe("创建任务\nname：工作总结");
    expect(decodeUIAction("【助手界面操作】\n{} ")).toBeUndefined();
    expect(decodeUIAction("【助手界面操作】\n<script>alert(1)</script>")).toBeUndefined();
    expect(() => encodeUIAction({ version: 1, source: "x", message: "", formState: {} })).toThrow();
    expect(() => encodeUIAction({ version: 1, source: "x", message: "提交", formState: { form: { value: { value: "x".repeat(4001) } } } })).toThrow();
  });

  test("generated examples match the actual component library and embedded prompt", async () => {
    const parser = createParser(assistantUILibrary.toJSONSchema(), assistantUILibrary.root);
    for (const example of assistantUIExamples) {
      const parsed = parser.parse(example);
      expect(parsed.root?.typeName).toBe("Stack");
      expect(parsed.meta.errors).toEqual([]);
      expect(parsed.meta.incomplete).toBe(false);
    }
    const embedded = await Bun.file(new URL("../../../../../internal/app/agent/catalog/workspaceassistant/openui_prompt.txt", import.meta.url)).text();
    expect(embedded).toBe(`${assistantUILibrary.prompt(assistantUIPromptOptions)}\n`);
    expect(Object.keys(assistantUILibrary.components).sort()).toEqual(Object.keys(openuiLibrary.components).sort());
  });

  test("preserves rich field values, dates and action context across persistence", () => {
    const raw = encodeUIAction({ version: 2, source: "m:0", message: "提交", context: { id: 42 }, formState: { rich: {
      multi: { value: ["tasks", "sessions"] }, enabled: { value: { notify: true } }, slider: { value: [42] },
      date: { value: new Date("2026-11-12T00:00:00Z") },
      range: { value: { from: new Date("2026-11-12T00:00:00Z"), to: new Date("2026-11-13T00:00:00Z") } },
      table: { value: [{ id: "1", values: ["条目", 10, false] }] },
    } } });
    const action = decodeUIAction(raw)!;
    expect(action.context).toEqual({ id: 42 });
    expect(action.formState?.rich.enabled.value).toEqual({ notify: true });
    expect(restoreUIState(action.formState)).toMatchObject({ rich: {
      date: { value: new Date("2026-11-12T00:00:00Z") },
      range: { value: { from: new Date("2026-11-12T00:00:00Z"), to: new Date("2026-11-13T00:00:00Z") } },
    } });
    expect(actionSummary(action)).toContain("multi：tasks、sessions");
    expect(actionSummary(action)).not.toContain("[object Object]");
    for (const value of [Infinity, { constructor: "bad" }, "x".repeat(4001), Array(501).fill(0)]) {
      expect(() => encodeUIAction({ version: 2, source: "m", message: "提交", context: value })).toThrow();
    }
  });

  test("allows official local interactions but blocks browser tool execution and unsafe links", () => {
    for (const example of assistantUIExamples) expect(programProblem(example)).toBe(false);
    for (const action of ['Query("delete", {})', 'Mutation("delete", {})', '@Run(task)', '@Unknown("x")']) expect(programProblem(`root = Stack([${action}])`)).toBe(true);
    expect(programProblem('root = Stack([TextContent("Query @Run \\\" text")]) // @Run')).toBe(false);
    expect(safeExternalURL("https://example.com/docs")).toBe("https://example.com/docs");
    for (const url of ["javascript:alert(1)", "data:text/html,bad", "file:///tmp/x", "//example.com", "https://user:pass@example.com"]) expect(safeExternalURL(url)).toBeUndefined();
  });
});
