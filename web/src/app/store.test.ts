import { beforeAll, beforeEach, describe, expect, test } from "bun:test";
import { JSDOM } from "jsdom";
import type { AppDispatch, RootState } from "./store";

const testDOM = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost/" });

let dispatch: AppDispatch;
let getState: () => RootState;
let chatActions: typeof import("./store").chatActions;
let sessionsActions: typeof import("./store").sessionsActions;

beforeAll(async () => {
  Object.assign(globalThis, {
    document: testDOM.window.document,
    history: testDOM.window.history,
    localStorage: testDOM.window.localStorage,
    location: testDOM.window.location,
    window: testDOM.window,
  });
  const module = await import("./store");
  dispatch = module.store.dispatch;
  getState = module.store.getState;
  chatActions = module.chatActions;
  sessionsActions = module.sessionsActions;
});

beforeEach(() => {
  localStorage.clear();
  dispatch(chatActions.clearMessages());
  dispatch(chatActions.setActiveSession(""));
  dispatch(sessionsActions.setSessionsLoading(false));
});

describe("chat state", () => {
  test("leaving the persistent assistant restores ordinary chat targeting", () => {
    dispatch(chatActions.setActiveSession("workspace-assistant"));
    dispatch(chatActions.setCurrentAgent("workspace_assistant"));
    dispatch(chatActions.setMode("deep"));
    dispatch(chatActions.setActiveSession(""));
    expect(getState().chat.currentAgent).toBe("");
    expect(getState().chat.mode).toBe("team");
  });
  test("does not persist active sessions in browser storage", () => {
    dispatch(chatActions.setActiveSession("session-a"));
    dispatch(chatActions.beginRunningSession({ sessionID: "session-b", startedAt: 1 }));

    expect(localStorage.getItem("fk_session_id")).toBeNull();
  });

  test("rolls back a failed first submission without leaving a phantom session", () => {
    dispatch(chatActions.appendUserMessage({ id: "local-message", content: "hello", sessionID: "new-session" }));
    dispatch(chatActions.beginRunningSession({ sessionID: "new-session", startedAt: 1 }));
    dispatch(chatActions.finishRunningSession("new-session"));
    dispatch(chatActions.rollbackUserMessage({
      id: "local-message",
      sessionID: "new-session",
      resetSession: true,
    }));

    expect(getState().chat.activeSessionID).toBe("");
    expect(getState().chat.viewSessionID).toBe("");
    expect(getState().chat.messages).toHaveLength(0);
    expect(getState().chat.presentation.items).toHaveLength(0);
  });

  test("restores the execution target when opening a session", () => {
    dispatch(chatActions.setActiveSession("configured-session"));
    dispatch(chatActions.setSessionDetail({
      session_id: "configured-session",
      view: { items: [] },
      mode: "deep",
      current_agent: "coder",
    }));

    expect(getState().chat.mode).toBe("deep");
    expect(getState().chat.currentAgent).toBe("coder");
  });
});

describe("session list state", () => {
  test("ignores responses from an obsolete request", () => {
    dispatch(sessionsActions.beginSessionsRequest(100));
    dispatch(sessionsActions.beginSessionsRequest(200));
    dispatch(sessionsActions.setSessions({
      items: [{ session_id: "stale", title: "stale" }],
      requestStartedAt: 100,
    }));

    expect(getState().sessions.loading).toBe(true);
    expect(getState().sessions.items.some((item) => item.session_id === "stale")).toBe(false);

    dispatch(sessionsActions.setSessions({
      items: [{ session_id: "current", title: "current" }],
      requestStartedAt: 200,
    }));
    expect(getState().sessions.items.map((item) => item.session_id)).toContain("current");
  });

  test("preserves local changes made while a request is pending", () => {
    dispatch(sessionsActions.beginSessionsRequest(300));
    dispatch(sessionsActions.setSessions({
      items: [{ session_id: "patched", title: "before" }],
      requestStartedAt: 300,
    }));
    dispatch(sessionsActions.beginSessionsRequest(400));
    dispatch(sessionsActions.renameSessionLocal({ sessionID: "patched", title: "after" }));
    dispatch(sessionsActions.setSessions({
      items: [{ session_id: "patched", title: "before" }],
      requestStartedAt: 400,
    }));

    expect(getState().sessions.items.find((item) => item.session_id === "patched")?.title).toBe("after");
  });
});

describe("conversation ownership", () => {
  function view(content: string): import("@/types/presentation").PresentationSnapshot {
    return { items: [{ id: "message", index: 0, order: 1, kind: "message", node: {
      message: { id: "message", role: "user", content, createdAt: "2026-11-12T00:00:00Z" },
      parts: [], showAgentLabel: false, showCopyAction: false, order: 1,
    } }] };
  }

  test("selection clears the complete view atomically while preserving background execution", () => {
    dispatch(chatActions.setActiveSession("background"));
    dispatch(chatActions.setSessionDetail({ session_id: "background", view: view("原会话"), queue: [{ queue_id: "q", kind: "follow_up" }] }));
    dispatch(chatActions.activateRunningSession({ sessionID: "background" }));
    dispatch(chatActions.setError("原会话错误"));
    dispatch(chatActions.receiveEvent({ type: "system_notice", session_id: "background", content: "原会话状态" }));
    dispatch(chatActions.setActiveSession("selected"));
    const state = getState().chat;
    expect(state.activeSessionID).toBe("selected");
    expect(state.viewSessionID).toBe("");
    expect(state.presentation.items).toEqual([]);
    expect(state.messages).toEqual([]);
    expect(state.queue).toEqual([]);
    expect(state.error).toBeUndefined();
    expect(state.statusText).toBeUndefined();
    expect(state.runningTasks.background?.phase).toBe("processing");
    dispatch(chatActions.finishRunningSession("background"));
  });

  test("late details, errors and stream payloads cannot change another session's view", () => {
    dispatch(chatActions.setActiveSession("selected"));
    dispatch(chatActions.setSessionDetail({ session_id: "selected", view: view("当前内容") }));
    dispatch(chatActions.setSessionDetail({ session_id: "previous", view: view("过期内容") }));
    dispatch(chatActions.sessionLoadFailed({ sessionID: "previous", message: "过期错误" }));
    dispatch(chatActions.setPresentationSnapshot({ sessionID: "previous", view: view("过期快照"), nextOffset: 100 }));
    dispatch(chatActions.receiveEvent({ type: "error", session_id: "previous", error: "过期流错误", view_update: { reset: view("过期流内容") } }));
    expect(getState().chat.activeSessionID).toBe("selected");
    expect(getState().chat.viewSessionID).toBe("selected");
    expect(getState().chat.presentation).toEqual(view("当前内容"));
    expect(getState().chat.error).toBeUndefined();
  });

  test("late background frames cannot populate the new conversation page", () => {
    dispatch(chatActions.setActiveSession("previous"));
    dispatch(chatActions.setSessionDetail({ session_id: "previous", view: view("原会话") }));
    dispatch(chatActions.setActiveSession(""));
    dispatch(chatActions.receiveEvent({ type: "assistant_completed", session_id: "previous", message_id: "late", view_update: { reset: view("过期内容") } }));
    expect(getState().chat.presentation.items).toEqual([]);
    expect(getState().chat.messages).toEqual([]);
  });
});
