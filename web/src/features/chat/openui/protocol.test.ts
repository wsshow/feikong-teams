import { describe, expect, test } from "bun:test";
import { actionSummary, decodeUIAction, encodeUIAction, splitUIMessage } from "./protocol";
import { createParser } from "@openuidev/react-lang";
import { assistantUIExamples, assistantUILibrary, assistantUIPromptOptions } from "./library";

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
      expect(parsed.root?.typeName).toBe("Card");
      expect(parsed.meta.errors).toEqual([]);
      expect(parsed.meta.incomplete).toBe(false);
    }
    const embedded = await Bun.file(new URL("../../../../../internal/app/agent/catalog/workspaceassistant/openui_prompt.txt", import.meta.url)).text();
    expect(embedded).toBe(`${assistantUILibrary.prompt(assistantUIPromptOptions)}\n`);
  });
});
