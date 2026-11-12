import { afterEach, beforeEach, expect, test } from "bun:test";
import { JSDOM } from "jsdom";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { Provider } from "react-redux";
import { installDialogStub } from "@/test/dialog";
import type { AppConfig } from "@/types/config";

let dom: JSDOM;
let root: Root;
let container: HTMLDivElement;
let state: typeof import("@/app/store");
let ConfigPanel: typeof import("./ConfigPanel").ConfigPanel;
let originalGlobals: Record<string, unknown>;
let originalFetch: typeof fetch;
let config: AppConfig;
let writes: AppConfig[];
let respondToSave: (next: AppConfig) => Response | Promise<Response>;

const success = (data: unknown) => Response.json({ code: 0, data });

beforeEach(async () => {
  originalFetch = globalThis.fetch;
  originalGlobals = Object.fromEntries(["window", "document", "location", "history", "localStorage", "Element", "CustomEvent"].map((key) => [key, Reflect.get(globalThis, key)]));
  dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost/config" });
  installDialogStub(dom);
  Object.assign(dom.window.HTMLElement.prototype, { attachEvent() {}, detachEvent() {} });
  Object.assign(globalThis, { window: dom.window, document: dom.window.document, location: dom.window.location,
    history: dom.window.history, localStorage: dom.window.localStorage, Element: dom.window.Element, CustomEvent: dom.window.CustomEvent,
    IS_REACT_ACT_ENVIRONMENT: true });
  config = {
    models: [{ id: "main", name: "示例模型", provider: "openai", model: "example-model", use_for: ["chat"], has_api_key: true, original_id: "main" }],
    server: { host: "127.0.0.1", port: 23456, auth: { enabled: false, username: "demo" } },
    agents: { items: [{ id: "reviewer", name: "审阅助手", enabled: true, prompt: "检查代码", tools: ["file"] }] },
    roundtable: { members: [{ id: "member", name: "讨论成员", model_id: "main" }] },
    tools: { mcp_servers: [{ id: "example_mcp", name: "示例 MCP", transport: "http", enabled: false }] },
    javascript: { tools: [{ id: "example_tool", name: "示例工具", source: "function execute(input) { return input; }" }], hooks: [{ id: "example_hook", name: "示例规则", source: 'function handle() { return { action: "continue" }; }' }] },
    extra_settings: { keep: true },
  };
  writes = [];
  respondToSave = (next) => { config = structuredClone(next); return success({ auth_changed: false }); };
  globalThis.fetch = (async (path: unknown, init?: RequestInit) => {
    if (String(path).endsWith("/tool-catalog")) return success([]);
    if (init?.method === "PUT") {
      const next = JSON.parse(init.body as string) as AppConfig;
      writes.push(next);
      return respondToSave(next);
    }
    return success(config);
  }) as typeof fetch;
  state = await import("@/app/store");
  state.store.dispatch(state.configActions.setConfig(undefined));
  state.store.dispatch(state.configActions.setTools([]));
  ConfigPanel = (await import("./ConfigPanel")).ConfigPanel;
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  await act(async () => root.render(<Provider store={state.store}><ConfigPanel /></Provider>));
});

afterEach(async () => {
  await act(async () => root.unmount());
  globalThis.fetch = originalFetch;
  Object.assign(globalThis, originalGlobals);
  dom.window.close();
});

function button(text: string, parent: ParentNode = container) {
  const found = [...parent.querySelectorAll<HTMLButtonElement>("button")].find((item) => item.textContent?.trim() === text || item.getAttribute("aria-label") === text);
  if (!found) throw new Error(`button not found: ${text}`);
  return found;
}
async function click(text: string, parent: ParentNode = container) {
  await act(async () => button(text, parent).click());
}
function dialog() { return container.querySelector<HTMLDialogElement>("dialog[open]")!; }
async function change(label: string, value: string) {
  const input = dialog().querySelector<HTMLInputElement | HTMLTextAreaElement>(`[aria-label="${label}"]`)!;
  const prototype = input.tagName === "TEXTAREA" ? dom.window.HTMLTextAreaElement.prototype : dom.window.HTMLInputElement.prototype;
  await act(async () => {
    input.focus();
    Object.getOwnPropertyDescriptor(prototype, "value")!.set!.call(input, value);
    input.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
    input.dispatchEvent(new dom.window.KeyboardEvent("keyup", { key: "a", bubbles: true }));
  });
}

test("model cards open isolated edits and cancellation restores renamed identifiers without saving", async () => {
  expect(container.querySelector("input")).toBeNull();
  expect(button("编辑示例模型").getAttribute("aria-haspopup")).toBe("dialog");
  await click("编辑示例模型");
  await change("ID", "renamed");
  await change("显示名称", "修改名称");
  expect(dialog()).not.toBeNull();
  expect(dialog().querySelector<HTMLInputElement>('[aria-label="ID"]')?.value).toBe("renamed");
  await click("取消", dialog());
  expect(dialog()).toBeNull();
  expect(button("编辑示例模型")).not.toBeNull();
  expect(writes).toHaveLength(0);
  await click("编辑示例模型");
  expect(dialog().querySelector<HTMLInputElement>('[aria-label="ID"]')?.value).toBe("main");
});

test("model vision setting saves explicit values and can return to provider behavior", async () => {
  for (const [label, expected] of [["支持", true], ["不支持", false], ["未设置（沿用供应商行为）", undefined]] as const) {
    await click("编辑示例模型");
    await click("视觉输入", dialog());
    await click(label, dialog());
    await click("确认并保存", dialog());
    expect(writes.at(-1)?.models?.[0].supports_vision).toBe(expected);
    await click("编辑示例模型");
    expect(button("视觉输入", dialog()).textContent).toContain(label);
    await click("取消", dialog());
  }
});

test("save conflicts preserve input, block duplicate saves while pending, and close only after a successful retry", async () => {
  let finish!: (response: Response) => void;
  respondToSave = () => new Promise((resolve) => { finish = resolve; });
  await click("编辑示例模型");
  await change("显示名称", "保存后的模型");
  await click("确认并保存", dialog());
  expect(writes).toHaveLength(1);
  expect(button("正在保存…", dialog()).disabled).toBe(true);
  expect(dialog().querySelector("fieldset")?.disabled).toBe(true);
  expect(button("取消", dialog()).disabled).toBe(true);
  await act(async () => finish(Response.json({ message: "任务运行中，请稍后重试" }, { status: 409 })));
  expect(dialog().textContent).toContain("任务运行中，请稍后重试");
  expect(dialog().querySelector<HTMLInputElement>('[aria-label="显示名称"]')?.value).toBe("保存后的模型");
  expect(state.store.getState().config.value?.models?.[0].name).toBe("示例模型");
  respondToSave = (next) => { config = structuredClone(next); return success({ auth_changed: false }); };
  await click("确认并保存", dialog());
  expect(dialog()).toBeNull();
  expect(writes).toHaveLength(2);
  expect(writes[1].models?.[0].name).toBe("保存后的模型");
  expect(writes[1].models?.[0].api_key).toBeUndefined();
  expect(writes[1].extra_settings).toEqual({ keep: true });
  expect(button("编辑保存后的模型")).not.toBeNull();
});

test.each([
  ["模型", "添加模型"], ["智能体", "添加智能体"], ["圆桌", "添加成员"], ["工具", "添加 MCP 服务"],
])("new items in %s open an editor and disappear on cancel", async (tab, add) => {
  await click(tab);
  const count = container.querySelectorAll('[aria-haspopup="dialog"]').length;
  await click(add);
  expect(dialog()).not.toBeNull();
  await click("取消", dialog());
  expect(container.querySelectorAll('[aria-haspopup="dialog"]')).toHaveLength(count);
  expect(writes).toHaveLength(0);
});

test.each([
  ["服务", "服务配置"], ["服务", "Web 认证"], ["服务", "OpenAI 兼容 API"], ["智能体", "审阅助手"],
  ["圆桌", "讨论成员"], ["圆桌", "讨论设置"], ["深度", "深度智能体"], ["深度", "能力配置"],
  ["记忆", "长期记忆"], ["通道", "QQ"], ["通道", "Discord"], ["通道", "微信"],
  ["工具", "示例 MCP"], ["扩展", "示例工具"], ["扩展", "示例规则"],
])("%s / %s uses a compact card and saves directly from its dialog", async (tab, title) => {
  await click(tab);
  expect(container.querySelector("input, textarea")).toBeNull();
  await click(`编辑${title}`);
  expect(dialog()).not.toBeNull();
  expect(writes).toHaveLength(0);
  await click("确认并保存", dialog());
  expect(writes).toHaveLength(1);
  expect(dialog()).toBeNull();
  expect(container.querySelector("input, textarea")).toBeNull();
});

test("permission edits retain the confirmation boundary and cancel without writing", async () => {
  await click("权限");
  await click("编辑权限审批");
  const editor = dialog();
  await click("弹出审批框", editor);
  expect(container.querySelectorAll("dialog[open]")).toHaveLength(2);
  const confirmation = [...container.querySelectorAll<HTMLDialogElement>("dialog[open]")].at(-1)!;
  await click("确认关闭", confirmation);
  expect(writes).toHaveLength(0);
  await click("取消", editor);
  await click("编辑权限审批");
  expect(button("弹出审批框", dialog()).getAttribute("aria-checked")).toBe("true");
  await click("弹出审批框", dialog());
  await click("确认关闭", [...container.querySelectorAll<HTMLDialogElement>("dialog[open]")].at(-1)!);
  await click("确认并保存", dialog());
  expect(writes).toHaveLength(1);
  expect(writes[0].tools?.approval?.auto_approve).toEqual(["all"]);
});

test("invalid extension schema blocks saving until repaired", async () => {
  await click("扩展");
  await click("编辑示例工具");
  await change("参数 JSON Schema", "{");
  await click("确认并保存", dialog());
  expect(writes).toHaveLength(0);
  expect(dialog()).not.toBeNull();
  await change("参数 JSON Schema", '{"type":"object","properties":{}}');
  await click("确认并保存", dialog());
  expect(writes).toHaveLength(1);
  expect(dialog()).toBeNull();
});

test("deletion saves only after confirmation and preserves the card when persistence fails", async () => {
  respondToSave = () => Response.json({ message: "存储失败" }, { status: 500 });
  await click("删除示例模型");
  expect(writes).toHaveLength(0);
  await click("确认删除", dialog());
  expect(writes).toHaveLength(1);
  expect(dialog().textContent).toContain("存储失败");
  expect(button("编辑示例模型")).not.toBeNull();
  await click("取消", dialog());
  expect(state.store.getState().config.value?.models).toHaveLength(1);
});

test("refreshing after reauthentication preserves the open edit", async () => {
  await click("编辑示例模型");
  await change("显示名称", "待保存名称");
  await act(async () => window.dispatchEvent(new dom.window.CustomEvent("fkteams:auth-restored")));
  expect(dialog().querySelector<HTMLInputElement>('[aria-label="显示名称"]')?.value).toBe("待保存名称");
  expect(writes).toHaveLength(0);
});
