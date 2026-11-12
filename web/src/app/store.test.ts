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
