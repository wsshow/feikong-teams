import { expect, spyOn, test } from "bun:test";
import { getSession } from "./sessions";
import { accessPublicShare } from "./shares";
import { streamSnapshot, subscribeStream, updateQueueItem } from "./stream";
import type { ChatEvent } from "@/types/events";

const valid: ChatEvent = { type: "assistant_completed", message_id: "message", content: "answer" };
const invalid = { type: "tool_call_completed", tool_calls: 42, sequence: "invalid" };

test("SSE skips malformed events and validates transport IDs before delivery", async () => {
  const fetch = mockFetch(async () => new Response([
    `id: -1\ndata: ${JSON.stringify(invalid)}\n\n`,
    "data: {broken\n\n",
    `id: -2\ndata: ${JSON.stringify(valid)}\n\n`,
    `id: 12\r\ndata: ${JSON.stringify(valid)}\r\n\r\n`,
    "data: [DONE]\n\n",
  ].join(""), { headers: { "Content-Type": "text/event-stream" } }));
  try {
    const received: ChatEvent[] = [];
    expect(await subscribeStream("session", 0, (events) => received.push(...events))).toBe("done");
    expect(received).toHaveLength(2);
    expect(received[0].stream_event_id).toBeUndefined();
    expect(received[1].stream_event_id).toBe(12);
  } finally { fetch.mockRestore(); }
});

test("snapshot, history and shared history use the same validated payloads", async () => {
  const fetch = mockFetch(async () => Response.json({ code: 0, data: {
    session_id: "session", events: [invalid, valid], queue: [{ queue_id: "q", kind: "follow_up" }, { queue_id: 1 }],
  } }));
  try {
    for (const response of [await streamSnapshot("session"), await getSession("session"), await accessPublicShare("share")]) {
      expect(response.events).toEqual([valid]);
      expect(response.queue).toEqual([{ queue_id: "q", kind: "follow_up" }]);
    }
  } finally { fetch.mockRestore(); }
});

test("queue edits send the message field expected by the Go handler", async () => {
  let payload: unknown;
  const fetch = mockFetch(async (_url, init) => {
    payload = JSON.parse(String(init?.body));
    return Response.json({ code: 0, data: { queue: [] } });
  });
  try {
    await updateQueueItem("session", "item", "updated");
    expect(payload).toEqual({ message: "updated" });
  } finally { fetch.mockRestore(); }
});

function mockFetch(handler: (...args: Parameters<typeof globalThis.fetch>) => ReturnType<typeof globalThis.fetch>) {
  return spyOn(globalThis, "fetch").mockImplementation(Object.assign(handler, { preconnect: globalThis.fetch.preconnect }));
}

test("a terminal SSE frame releases the response stream", async () => {
  let cancelled = false;
  const body = new ReadableStream<Uint8Array>({
    start(controller) { controller.enqueue(new TextEncoder().encode("data: [DONE]\n\n")); },
    cancel() { cancelled = true; },
  });
  const fetch = mockFetch(async () => new Response(body));
  try {
    expect(await subscribeStream("session", 0, () => undefined)).toBe("done");
    expect(cancelled).toBe(true);
    expect(body.locked).toBe(false);
  } finally { fetch.mockRestore(); }
});
