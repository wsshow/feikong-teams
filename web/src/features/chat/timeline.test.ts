import { describe, expect, test } from "bun:test";
import { buildTimelineModel, eventsForDisplay } from "./timeline";
import type { ChatEvent } from "@/types/events";
import type { ChatViewMessage } from "@/types/chat";

describe("timeline projection", () => {
  test("replay deduplicates within a run without losing the same sequence from another run", () => {
    const events: ChatEvent[] = [
      { type: "assistant_text_delta", run_id: "a", sequence: 1, content: "one" },
      { type: "assistant_text_delta", run_id: "a", sequence: 1, content: "one" },
      { type: "assistant_text_delta", run_id: "b", sequence: 1, content: "two" },
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
});
