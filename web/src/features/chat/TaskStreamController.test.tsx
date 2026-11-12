import { afterEach, beforeEach, expect, test } from "bun:test";
import { JSDOM } from "jsdom";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { Provider } from "react-redux";
import type { PresentationSnapshot } from "@/types/presentation";

let dom: JSDOM;
let root: Root;
let state: typeof import("@/app/store");
let Controller: typeof import("./TaskStreamController").TaskStreamController;
let savedGlobals: Record<string, unknown>;
let savedFetch: typeof fetch;
const sessionID = "reconnect-test";

function view(content: string): PresentationSnapshot {
  return {
    items: [
      {
        id: "answer",
        index: 0,
        order: 1,
        kind: "message",
        node: {
          message: {
            id: "answer",
            role: "assistant",
            content,
            createdAt: "2026-11-12T00:00:00Z",
          },
          parts: [{ type: "text", key: "answer:text", content }],
          showAgentLabel: true,
          showCopyAction: true,
          order: 1,
        },
      },
    ],
  };
}

beforeEach(async () => {
  savedFetch = globalThis.fetch;
  savedGlobals = Object.fromEntries(
    ["window", "document", "location", "localStorage"].map((key) => [
      key,
      Reflect.get(globalThis, key),
    ]),
  );
  dom = new JSDOM(
    "<!doctype html><html><body><div id='root'></div></body></html>",
    { url: "http://localhost/chat" },
  );
  Object.assign(globalThis, {
    window: dom.window,
    document: dom.window.document,
    location: dom.window.location,
    localStorage: dom.window.localStorage,
    IS_REACT_ACT_ENVIRONMENT: true,
  });
  state = await import("@/app/store");
  Controller = (await import("./TaskStreamController")).TaskStreamController;
  state.store.dispatch(
    state.chatActions.setSessionDetail({
      session_id: sessionID,
      view: { items: [] },
    }),
  );
  root = createRoot(document.getElementById("root")!);
});

afterEach(async () => {
  await act(async () => root.unmount());
  state.store.dispatch(state.chatActions.finishRunningSession(sessionID));
  globalThis.fetch = savedFetch;
  Object.assign(globalThis, savedGlobals);
  dom.window.close();
});

async function run(initialOffset?: number) {
  state.store.dispatch(
    state.chatActions.activateRunningSession({ sessionID, initialOffset }),
  );
  await act(async () =>
    root.render(
      <Provider store={state.store}>
        <Controller />
      </Provider>,
    ),
  );
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 650));
  });
}

test("reconnect replaces an unfinished draft with committed history even after task cache expiry", async () => {
  const requests: string[] = [];
  globalThis.fetch = (async (input: string | URL | Request) => {
    const url = String(input);
    requests.push(url);
    if (url.includes("/subscribe/")) {
      const event = {
        type: "assistant_completed",
        message_id: "answer",
        content: "draft",
        view_update: { items: view("draft").items },
      };
      return new Response(`id: 0\ndata: ${JSON.stringify(event)}\n\n`, {
        headers: { "Content-Type": "text/event-stream" },
      });
    }
    if (url.includes("/snapshot/"))
      return Response.json({
        code: 0,
        data: {
          session_id: sessionID,
          status: "completed",
          has_task: false,
          next_offset: 0,
          view: view("committed answer"),
        },
      });
    throw new Error(`unexpected request: ${url}`);
  }) as unknown as typeof fetch;
  await run(0);
  expect(requests).toHaveLength(2);
  expect(requests[1]).toContain("/snapshot/");
  expect(state.store.getState().chat.presentation).toEqual(
    view("committed answer"),
  );
  expect(state.store.getState().chat.runningTasks[sessionID]).toBeUndefined();
});

test("an initial snapshot failure retries before declaring the task finished", async () => {
  let attempts = 0;
  globalThis.fetch = (async () => {
    attempts += 1;
    if (attempts === 1)
      return new Response("temporarily unavailable", { status: 503 });
    return Response.json({
      code: 0,
      data: {
        session_id: sessionID,
        status: "completed",
        has_task: false,
        next_offset: 0,
        view: view("restored answer"),
      },
    });
  }) as unknown as typeof fetch;
  await run();
  expect(attempts).toBe(2);
  expect(state.store.getState().chat.presentation).toEqual(
    view("restored answer"),
  );
  expect(state.store.getState().chat.runningTasks[sessionID]).toBeUndefined();
});
