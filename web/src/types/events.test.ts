import { describe, expect, test } from "bun:test";
import { chatEventTypes, decodeChatEvent, decodeChatEvents, decodeQueueItems, type ChatEvent } from "./events";
import fixture from "./testdata/chat-events.json";

const tool = { type: "tool_call_completed", tool_name: "read_file", tool_call_ref: "tool_call:call", tool_call_id: "call" };

describe("event protocol boundary", () => {
  test("accepts every event kind emitted by the Go adapter and its history projection", () => {
    expect(new Set(fixture.live.map((event) => event.type))).toEqual(new Set(chatEventTypes));
    for (const event of [...fixture.live, ...fixture.history]) {
      const decoded = decodeChatEvent(event);
      expect(decoded, `${event.type} ${event.event_id}`).toBeDefined();
      expect(decoded?.type).toBe(event.type as ChatEvent["type"]);
    }
  });

  test("unknown and malformed envelopes stay outside application event types", () => {
    for (const value of [null, [], 42, { type: "future_extension" }, { type: "assistant_text_delta", content: 42 }]) {
      expect(decodeChatEvent(value)).toBeUndefined();
    }
    expect(decodeChatEvent({ type: "member_completed", member_call_id: "member-1" })?.type).toBe("member_completed");
  });

  test("rejects malformed nested tools, counts, questions, queues and attachments", () => {
    const malformed = [
      { ...tool, tool_calls: 42, sequence: "invalid" },
      { ...tool, tool_calls: [{ name: 1 }] },
      { ...tool, tool_call: { name: "read_file", arguments: {} } },
      { ...tool, tool_calls: [{ name: "read_file", index: -1 }] },
      { ...tool, tool_calls: null },
      { ...tool, sequence: 1.5 },
      { ...tool, sequence: Number.NaN },
      { ...tool, stream_event_id: Number.MAX_SAFE_INTEGER + 1 },
      { ...tool, created_at: {} },
      { ...tool, content_parts: [{ type: "image_url", url: 42 }] },
      { ...tool, content_parts: [{ type: "future_part" }] },
      { type: "queue_updated", queue: [{ queue_id: "q", kind: "unknown" }] },
      { type: "queue_updated", queue: [{ queue_id: "q", kind: "steering", parts: [false] }] },
      { type: "queue_updated", queue: "invalid" },
      { type: "ask_requested", ask_id: "a", question: "choose", options: [42] },
      { type: "ask_answered", ask_id: "a", ask_selected: [42] },
      { type: "approval_requested", detail: "a", approval: { decision: 1 } },
      { type: "usage_reported", usage: { total_tokens: "5" } },
      { type: "error", error: "failed", error_suggestions: {} },
    ];
    for (const value of malformed) expect(decodeChatEvent(value), JSON.stringify(value)).toBeUndefined();
  });

  test("requires the fields that distinguish each event payload", () => {
    for (const value of [
      { type: "assistant_text_delta", message_id: "m" },
      { type: "assistant_completed", content: "answer" },
      { type: "tool_call_completed", tool_name: "read_file" },
      { type: "ask_requested", ask_id: "ask" },
      { type: "approval_requested", content: "continue?" },
      { type: "queue_updated" }, { type: "usage_reported" }, { type: "error" },
    ]) expect(decodeChatEvent(value)).toBeUndefined();
  });

  test("normalizes supported legacy fields once and removes unrelated extensions", () => {
    const input = { type: "ask_answered", detail: "ask", ask_selected: ["A"], ask_free_text: "answer", future: { value: 42 } };
    expect(decodeChatEvent(input)).toEqual({ type: "ask_answered", detail: "ask", ask_id: "ask", selected: ["A"], free_text: "answer" });
    expect(input).toHaveProperty("ask_selected");
    expect(decodeChatEvent({ type: "tool_call_completed", tool_call: { id: "call", name: "read_file" } })).toMatchObject({ tool_name: "read_file", tool_call_ref: "tool_call:call" });
    expect(decodeChatEvent({ type: "assistant_reasoning_delta", message_id: "m", reasoning_content: "reason" })).toMatchObject({ content: "reason" });
    expect(decodeChatEvent({ type: "usage_reported", usage: { prompt_tokens: 2, completion_tokens: 3 } })).toMatchObject({ total_tokens: 5 });
  });

  test("a malformed event cannot poison the following valid event", () => {
    expect(decodeChatEvents([{ ...tool, tool_calls: 42 }, { ...tool, tool_calls: [{ name: "read_file" }] }])).toHaveLength(1);
    expect(decodeQueueItems([{ queue_id: "q", kind: "follow_up" }, { queue_id: "bad", kind: "unknown" }])).toEqual([{ queue_id: "q", kind: "follow_up" }]);
  });
});

// 类型回归：缺少关键字段的对象不能绕过 decoder 进入应用。
function checkEventTypes(event: ChatEvent) {
  if (event.type === "queue_updated") {
    const queue: import("./events").QueueItem[] = event.queue;
    return queue.length;
  }
  if (event.type === "tool_call_completed") return event.tool_call_ref.length;
  return 0;
}
void checkEventTypes;
// @ts-expect-error 工具事件必须具有关联引用。
const invalidToolEvent: ChatEvent = { type: "tool_call_completed", tool_name: "read_file" };
void invalidToolEvent;
// @ts-expect-error 核心事件没有任意扩展字段。
const invalidExtension: ChatEvent = { type: "pong", arbitrary: 1 };
void invalidExtension;
