import { afterEach, beforeAll, beforeEach, expect, test } from "bun:test";
import { JSDOM } from "jsdom";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { Provider } from "react-redux";
import { assistantUIExamples } from "./library";
import { decodeUIAction, encodeUIAction } from "./protocol";

const dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost/assistant" });
let state: typeof import("@/app/store");
let Content: typeof import("./AssistantUIContent").default;
let programProblem: typeof import("./AssistantUIContent").programProblem;
let root: Root;
let container: HTMLDivElement;
let originalFetch: typeof fetch;
let requests: { session_id: string; message: string; agent_name: string; mode: string }[];
const sourceID = "answer:text:message";
const content = `请确认后提交。\n\`\`\`openui-lang\n${assistantUIExamples[0]}\n\`\`\``;

beforeAll(async () => {
  Object.assign(globalThis, { window: dom.window, document: dom.window.document, location: dom.window.location, history: dom.window.history, localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true });
  state = await import("@/app/store");
  const module = await import("./AssistantUIContent");
  Content = module.default;
  programProblem = module.programProblem;
});

beforeEach(() => {
  Object.assign(globalThis, { window: dom.window, document: dom.window.document, location: dom.window.location, history: dom.window.history, localStorage: dom.window.localStorage, IS_REACT_ACT_ENVIRONMENT: true });
  state.store.dispatch(state.chatActions.finishRunningSession("workspace-assistant"));
  state.store.dispatch(state.chatActions.setSessionDetail({ session_id: "workspace-assistant", events: [] }));
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  requests = [];
  originalFetch = globalThis.fetch;
  globalThis.fetch = (async (_url: string | URL | Request, init?: RequestInit) => {
    requests.push(JSON.parse(String(init?.body)));
    return Response.json({ code: 0, data: { session_id: "workspace-assistant", status: "processing" } });
  }) as unknown as typeof fetch;
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  globalThis.fetch = originalFetch;
});

async function render(text = content, streaming = false, key = "same") {
  await act(async () => root.render(<Provider store={state.store}><Content key={key} content={text} sourceID={sourceID} streaming={streaming} /></Provider>));
}

function submitButton() { return container.querySelector<HTMLButtonElement>('button[type="submit"]')!; }
async function submit() {
  await act(async () => {
    container.querySelector("form")!.dispatchEvent(new dom.window.Event("submit", { bubbles: true, cancelable: true }));
  });
}

test("streams disabled controls, submits once through chat, and restores submitted values on replay", async () => {
  await render(content, true);
  expect(submitButton().disabled).toBe(true);
  await submit();
  expect(requests).toHaveLength(0);
  await render();
  expect(submitButton().disabled).toBe(false);
  const select = container.querySelector("select")!;
  await act(async () => { select.value = "周一"; select.dispatchEvent(new dom.window.Event("change", { bubbles: true })); });
  await submit();
  expect(requests).toHaveLength(1);
  expect(requests[0]).toMatchObject({ session_id: "workspace-assistant", agent_name: "workspace_assistant", mode: "team" });
  const action = decodeUIAction(requests[0].message)!;
  expect(action.formState?.weekly.day.value).toBe("周一");
  expect(action.formState?.weekly.name.value).toBe("每周工作总结");
  expect(action.source).toBe(`${sourceID}:0`);
  expect(submitButton().disabled).toBe(true);
  await submit();
  expect(requests).toHaveLength(1);
  await act(async () => state.store.dispatch(state.chatActions.setSessionDetail({ session_id: "workspace-assistant", events: [{ type: "user_message", content: requests[0].message }] })));
  await render(content, false, "reloaded");
  expect(container.querySelector("select")?.value).toBe("周一");
  expect(submitButton().disabled).toBe(true);
  expect(container.textContent).toContain("已提交给助手");
});

test("failed sends roll back the optimistic message and keep the form editable for retry", async () => {
  await render();
  let calls = 0;
  globalThis.fetch = (async () => { calls++; return new Response("temporary failure", { status: 503 }); }) as unknown as typeof fetch;
  await submit();
  expect(calls).toBe(1);
  expect(state.store.getState().chat.messages).toHaveLength(0);
  expect(submitButton().disabled).toBe(false);
  expect(container.querySelector('[role="alert"]')?.textContent).toContain("填写内容已保留");
  await submit();
  expect(calls).toBe(2);
});

test("running tasks accept UI input into the existing queue and deletion permits resubmission", async () => {
  state.store.dispatch(state.chatActions.activateRunningSession({ sessionID: "workspace-assistant" }));
  globalThis.fetch = (async (_url: string | URL | Request, init?: RequestInit) => {
    const request = JSON.parse(String(init?.body));
    requests.push(request);
    return Response.json({ code: 0, data: { session_id: "workspace-assistant", status: "queued", queue: [{ queue_id: "q", kind: "follow_up", text: request.message }] } });
  }) as unknown as typeof fetch;
  await render();
  await submit();
  expect(requests).toHaveLength(1);
  expect(state.store.getState().chat.messages).toHaveLength(0);
  expect(state.store.getState().chat.queue).toHaveLength(1);
  expect(submitButton().disabled).toBe(true);
  await act(async () => state.store.dispatch(state.chatActions.setQueue([])));
  expect(submitButton().disabled).toBe(false);
});

test("unsafe or malformed programs cannot execute, and ordinary text is escaped", async () => {
  for (const program of ['root = Query("delete", {})', 'root = Card("x", [Button("x", @ToAssistant("x"))])', "x".repeat(32001), "[".repeat(41)]) expect(programProblem(program)).toBe(true);
  expect(programProblem('root = Card("Email", [Text("a@b.com ${text}")])')).toBe(false);
  await render('```openui-lang\nroot = Unknown("bad")\n```');
  expect(container.textContent).toContain("交互内容暂时无法显示");
  expect(requests).toHaveLength(0);
  await render('```openui-lang\nroot = Card("安全", [Text("<img src=x onerror=alert(1)>")])\n```');
  expect(container.querySelector("img")).toBeNull();
  expect(container.textContent).toContain("<img src=x onerror=alert(1)>");
  await render('```openui-lang\nroot = Card("未闭合", [Text("内容")])');
  expect(container.textContent).toContain("交互内容暂时无法显示");
});

test("restored queue values are read-only and another session cannot submit this assistant UI", async () => {
  const message = encodeUIAction({ version: 1, source: `${sourceID}:0`, message: "创建", formState: { weekly: { name: { value: "已入队" }, day: { value: "周一" } } } });
  state.store.dispatch(state.chatActions.setQueue([{ queue_id: "q", kind: "follow_up", text: message }]));
  await render();
  expect(container.querySelector<HTMLInputElement>('input[name="name"]')?.value).toBe("已入队");
  expect(submitButton().disabled).toBe(true);
  await act(async () => {
    state.store.dispatch(state.chatActions.setQueue([]));
    state.store.dispatch(state.chatActions.setActiveSession("another"));
  });
  expect(submitButton().disabled).toBe(true);
  await submit();
  expect(requests).toHaveLength(0);
});

test("a pending submission cannot duplicate requests or steal navigation after switching sessions", async () => {
  let resolve!: (response: Response) => void;
  globalThis.fetch = (async () => {
    requests.push({ session_id: "workspace-assistant", message: "", agent_name: "", mode: "" });
    return await new Promise<Response>((done) => { resolve = done; });
  }) as unknown as typeof fetch;
  await render();
  await submit();
  await submit();
  expect(requests).toHaveLength(1);
  await act(async () => state.store.dispatch(state.chatActions.setActiveSession("another")));
  await act(async () => resolve(Response.json({ code: 0, data: { session_id: "workspace-assistant", status: "processing" } })));
  expect(state.store.getState().chat.activeSessionID).toBe("another");
  expect(state.store.getState().chat.runningTasks["workspace-assistant"]?.phase).toBe("processing");
});

test("result card buttons continue conversation without form fields", async () => {
  await render(`\`\`\`openui-lang\n${assistantUIExamples[1]}\n\`\`\``);
  await act(async () => container.querySelector<HTMLButtonElement>("button")!.click());
  expect(requests).toHaveLength(1);
  expect(decodeUIAction(requests[0].message)?.message).toContain("发布流程");
});

test("shared sender preserves ordinary chat targeting, attachments and new-session navigation", async () => {
  const { sendChatMessage } = await import("../sendMessage");
  let sent: Record<string, unknown> | undefined;
  globalThis.fetch = (async (_url: string | URL | Request, init?: RequestInit) => {
    sent = JSON.parse(String(init?.body));
    return Response.json({ code: 0, data: { session_id: "ordinary", status: "processing" } });
  }) as unknown as typeof fetch;
  const contents = [{ type: "image_url" as const, url: "https://example.com/image.png" }];
  await state.store.dispatch(sendChatMessage({ sessionID: "ordinary", message: "查看图片", contents, mode: "deep", agent: "coder", newSession: true }));
  expect(sent).toMatchObject({ session_id: "ordinary", message: "查看图片", mode: "deep", agent_name: "coder", contents });
  expect(state.store.getState().chat.messages.at(-1)?.contentParts).toEqual(contents);
  expect(state.store.getState().sessions.items.find((session) => session.session_id === "ordinary")?.title).toBe("查看图片");
  expect(location.pathname).toBe("/chat/ordinary");
  state.store.dispatch(state.chatActions.finishRunningSession("ordinary"));
  history.replaceState(null, "", "/assistant");
});
