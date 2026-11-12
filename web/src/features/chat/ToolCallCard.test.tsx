import { afterEach, beforeEach, expect, test } from "bun:test";
import { JSDOM } from "jsdom";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import type { ToolCallDTO } from "@/types/events";
import { ToolCallCard } from "./ToolCallCard";

let dom: JSDOM;
let root: Root;
let container: HTMLDivElement;
let savedGlobals: Record<string, unknown>;
let disclosureID = 0;

beforeEach(() => {
  savedGlobals = Object.fromEntries(["window", "document", "IS_REACT_ACT_ENVIRONMENT"].map((key) => [key, Reflect.get(globalThis, key)]));
  dom = new JSDOM("<!doctype html><html><body></body></html>");
  Object.assign(globalThis, { window: dom.window, document: dom.window.document, IS_REACT_ACT_ENVIRONMENT: true });
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  disclosureID++;
});

afterEach(async () => {
  await act(async () => root.unmount());
  Object.assign(globalThis, savedGlobals);
  dom.window.close();
});

async function render(tool: Partial<ToolCallDTO>, children?: React.ReactNode) {
  await act(async () => root.render(<ToolCallCard tool={{ name: "execute", ...tool }} disclosureID={`tool-test:${disclosureID}`}>{children}</ToolCallCard>));
}

function header() { return container.querySelector<HTMLButtonElement>("button")!; }
function summary() { return header().querySelector<HTMLElement>(".font-mono[title]"); }

test("collapsed calls show a single-line command and keep full arguments available on expansion", async () => {
  const args = JSON.stringify({ command: "bun test\n  src", reason: "检查前端", timeout: 60 });
  await render({ arguments: args, result: "test output", status: "completed" });
  expect(summary()?.textContent).toBe("bun test src");
  expect(summary()?.title).toBe("bun test src");
  expect(header().textContent).not.toContain("检查前端");
  expect(header().getAttribute("aria-expanded")).toBe("false");
  expect(container.querySelector("pre")).toBeNull();
  await act(async () => header().click());
  expect(header().getAttribute("aria-expanded")).toBe("true");
  expect(document.getElementById(header().getAttribute("aria-controls")!)?.textContent).toContain("检查前端");
  expect(container.querySelectorAll("pre")[1].textContent).toBe("test output");
  await act(async () => header().click());
  expect(container.querySelector("pre")).toBeNull();
});

test("summaries use meaningful file, search, attachment and agent arguments", async () => {
  for (const [args, expected] of [
    [{ filepath: "docs/architecture.md", content: "document body" }, "docs/architecture.md"],
    [{ pattern: "ToolCallCard", path: "web/src" }, "ToolCallCard"],
    [{ attachment_id: "history:000000:00:01" }, "history:000000:00:01"],
    [{ task: "检查工具调用的展示" }, "检查工具调用的展示"],
  ] as const) {
    await render({ arguments: JSON.stringify(args) });
    expect(summary()?.textContent).toBe(expected);
  }
});

test("partial JSON, non-object values and unknown argument fields do not leak raw parameters", async () => {
  for (const args of ['{"command":"bun', "null", "[]", '"text"', '{"content":"long document body","timeout":60}', '{"command":42}']) {
    await render({ arguments: args });
    expect(summary()).toBeNull();
  }
  await render({ target: "reviewer", arguments: '{"task":' });
  expect(summary()?.textContent).toBe("reviewer");
});

test("status distinguishes pending, running, completed, failed and unknown calls", async () => {
  for (const [status, label] of [
    ["pending", "等待中"], ["running", "运行中"], ["completed", "已完成"],
    ["error", "失败"], ["cancelled", "已取消"], [undefined, "状态未知"],
  ] as const) {
    await render({ status });
    expect(header().textContent).toContain(label);
    expect(header().querySelectorAll("svg")).toHaveLength(2);
    expect(header().textContent).not.toContain(status === "running" ? "等待中" : "运行中");
  }
});

test("completed calls with explicit business failures do not appear successful", async () => {
  for (const result of [{ success: false }, { error_message: "file not found" }, { exit_code: 1 }]) {
    await render({ status: "completed", result: JSON.stringify(result) });
    expect(header().textContent).toContain("失败");
    expect(header().textContent).not.toContain("已完成");
  }
  for (const result of ["plain text", "[]", '{"success":true,"exit_code":0}', '{"error_message":""}']) {
    await render({ status: "completed", result });
    expect(header().textContent).toContain("已完成");
  }
});

test("streaming argument and status updates retain the expanded details", async () => {
  await render({ status: "pending", arguments: '{"command":' });
  await act(async () => header().click());
  await render({ status: "running", arguments: '{"command":"bun test src"}' });
  expect(summary()?.textContent).toBe("bun test src");
  expect(header().textContent).toContain("运行中");
  expect(header().getAttribute("aria-expanded")).toBe("true");
  await render({ status: "completed", arguments: '{"command":"bun test src"}', result: "done" });
  expect(header().textContent).toContain("已完成");
  expect(container.querySelectorAll("pre")[1].textContent).toBe("done");
});

test("agent details keep nested content without duplicating the dispatch result", async () => {
  await render({ kind: "agent", target: "reviewer", result: "duplicate result", arguments: '{"task":"检查改动"}' }, <div>成员执行详情</div>);
  await act(async () => header().click());
  expect(container.textContent).toContain("成员执行详情");
  expect(container.textContent).not.toContain("duplicate result");
});
