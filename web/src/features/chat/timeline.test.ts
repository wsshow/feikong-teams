import { describe, expect, test } from "bun:test";
import { buildTimelineModel, eventsForDisplay, messagePartKey, timelineItemSpacingClass } from "./timeline";
import type { ChatEvent } from "@/types/events";
import type { ChatViewMessage } from "@/types/chat";

describe("timeline projection", () => {
  test("UI source identity survives replacing live deltas with persisted completion-only history", () => {
    const content = '```openui-lang\nroot = Card("操作", [Button("继续", "继续处理")])\n```';
    const delta: ChatEvent = { type: "assistant_text_delta", message_id: "answer", content };
    const completed: ChatEvent = { type: "assistant_completed", message_id: "answer", content };
    function source(events: ChatEvent[]) {
      const message: ChatViewMessage = { id: "answer", role: "assistant", content, events };
      const model = buildTimelineModel([message], events, new Set(), new Set(), false);
      const item = model.items.find((item) => item.kind === "message")!;
      if (item.kind !== "message") throw new Error("missing message");
      return messagePartKey(item.node.parts[0], message.id);
    }
    expect(source([delta, completed])).toBe(source([completed]));
  });

  test("replay deduplicates within a run without losing the same sequence from another run", () => {
    const events: ChatEvent[] = [
      { type: "assistant_text_delta", message_id: "message", run_id: "a", sequence: 1, content: "one" },
      { type: "assistant_text_delta", message_id: "message", run_id: "a", sequence: 1, content: "one" },
      { type: "assistant_text_delta", message_id: "message", run_id: "b", sequence: 1, content: "two" },
    ];
    expect(eventsForDisplay(events).map((event) => event.content)).toEqual(["one", "two"]);
    expect(events).toHaveLength(3);
  });

  test("projection preserves input messages and gives only the final answer a copy action", () => {
    const messages: ChatViewMessage[] = [
      { id: "user", role: "user", content: "question", events: [] },
      { id: "first", role: "assistant", content: "working", events: [] },
      { id: "last", role: "assistant", content: "answer", events: [] },
    ];
    const before = structuredClone(messages);
    const timeline = buildTimelineModel(messages, [], new Set(), new Set(), false);
    const copyIDs = timeline.items.flatMap((item) => item.kind === "message" && item.node.showCopyAction ? [item.node.message.id] : []);
    expect(copyIDs).toEqual(["last"]);
    expect(messages).toEqual(before);
  });

  test("consecutive tool-only assistant messages share compact spacing after projection", () => {
    const events: ChatEvent[] = ["first", "second"].map((id, index) => ({
      type: "assistant_completed", message_id: id, agent_name: "assistant", sequence: index + 1,
      tool_calls: [{ name: "execute", ref: id, arguments: '{"command":"bun test"}' }],
    }));
    const messages: ChatViewMessage[] = events.map((event) => ({ id: event.message_id!, role: "assistant", agent: "assistant", content: "", events: [event] }));
    const { items } = buildTimelineModel(messages, events, new Set(), new Set(), false);
    expect(items.map((item) => item.kind)).toEqual(["tool", "tool"]);
    expect(timelineItemSpacingClass(items, 1)).toBe("mt-0");
  });

  test("standalone tools and adjacent message tools share the same spacing", () => {
    const text: ChatEvent = { type: "assistant_text_delta", message_id: "first", content: "先检查文件", sequence: 1 };
    const first: ChatEvent = { type: "assistant_completed", message_id: "first", tool_calls: [{ name: "execute", ref: "first-tool" }], sequence: 2 };
    const events: ChatEvent[] = [text, first,
      { type: "tool_call_started", tool_name: "execute", tool_call_ref: "second-tool", sequence: 3 },
      { type: "tool_call_started", tool_name: "execute", tool_call_ref: "third-tool", sequence: 4 },
    ];
    const messages: ChatViewMessage[] = [{ id: "first", role: "assistant", content: "先检查文件", events: [text, first] }];
    const { items } = buildTimelineModel(messages, events, new Set(), new Set(), true);
    expect(items.map((item) => item.kind)).toEqual(["message", "tool", "tool"]);
    expect(timelineItemSpacingClass(items, 1)).toBe("mt-0");
    expect(timelineItemSpacingClass(items, 2)).toBe("mt-0");
  });

  test("new agents and narrative messages keep their separation from tools", () => {
    const events: ChatEvent[] = ["first", "second"].map((id, index) => ({
      type: "assistant_completed", message_id: id, sequence: index + 1,
      tool_calls: [{ name: "execute", ref: id }],
    }));
    const messages: ChatViewMessage[] = events.map((event, index) => ({ id: event.message_id!, role: "assistant", agent: `agent-${index}`, content: index === 1 ? "继续处理" : "", events: [event] }));
    messages.push({ id: "answer", role: "assistant", agent: "agent-1", content: "处理完成", events: [] });
    const { items } = buildTimelineModel(messages, events, new Set(), new Set(), false);
    expect(items.map((item) => item.kind)).toEqual(["tool", "message", "message"]);
    expect(timelineItemSpacingClass(items, 1)).not.toBe("mt-0");
    expect(timelineItemSpacingClass(items, 2)).toBe("mt-6");
  });

  test("tool states follow execution events rather than model message completion", () => {
    const declared: ChatEvent = {
      type: "assistant_completed", message_id: "calls", tool_calls: [
        { name: "execute", id: "first", ref: "tool_call:first", arguments: '{"command":"bun test"}' },
        { name: "execute", id: "second", ref: "tool_call:second", arguments: '{"command":"bun run build"}' },
      ],
    };
    const events: ChatEvent[] = [declared];
    const statuses = () => buildTimelineModel([], events, new Set(), new Set(), true).items.flatMap((item) => item.kind === "tool" ? [item.part.tool.status] : []);
    expect(statuses()).toEqual(["pending", "pending"]);
    events.push({ type: "tool_call_started", tool_name: "execute", tool_call_ref: "tool_call:first" });
    expect(statuses()).toEqual(["running", "pending"]);
    events.push({ type: "tool_call_result_delta", tool_name: "execute", tool_call_ref: "tool_call:first", content: "test output" });
    expect(statuses()).toEqual(["running", "pending"]);
    events.push({ type: "tool_call_completed", tool_name: "execute", tool_call_ref: "tool_call:first", content: "done" });
    expect(statuses()).toEqual(["completed", "pending"]);
    events.push({ type: "tool_call_started", tool_name: "execute", tool_call_ref: "tool_call:second" });
    expect(statuses()).toEqual(["completed", "running"]);
    events.push({ type: "tool_call_failed", tool_name: "execute", tool_call_ref: "tool_call:second", error: "command failed" });
    expect(statuses()).toEqual(["completed", "error"]);
  });

  test("cancellation preserves finished calls and marks unfinished calls as cancelled", () => {
    const events: ChatEvent[] = [
      { type: "tool_call_completed", tool_name: "read_file", tool_call_ref: "finished", content: "file contents" },
      { type: "tool_call_started", tool_name: "execute", tool_call_ref: "unfinished" },
      { type: "cancelled" },
    ];
    const { items } = buildTimelineModel([], events, new Set(), new Set(), false);
    expect(items.flatMap((item) => item.kind === "tool" ? [item.part.tool.status] : [])).toEqual(["completed", "cancelled"]);
  });

  test("previous runs cannot overwrite the status of a new running call", () => {
    for (const [type, status] of [["processing_end", "completed"], ["cancelled", "cancelled"], ["error", "error"]] as const) {
      const terminal: ChatEvent = type === "error" ? { type, run_id: "old", error: "execution failed" } : { type, run_id: "old" };
      const events: ChatEvent[] = [
        { type: "tool_call_started", run_id: "old", tool_name: "execute", tool_call_ref: "old-call" },
        terminal,
        { type: "tool_call_started", run_id: "new", tool_name: "execute", tool_call_ref: "new-call" },
      ];
      const { items } = buildTimelineModel([], events, new Set(), new Set(), true);
      expect(items.flatMap((item) => item.kind === "tool" ? [item.part.tool.status] : [])).toEqual([status, "running"]);
    }
  });
});
