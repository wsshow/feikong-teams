import { describe, expect, test } from "bun:test";
import {
  maxFallbackEventKeys,
  rememberChatEvent,
  stableChatEventIdentity,
  type ChatEventDedupState,
} from "./eventBuffer";

describe("chat event buffer", () => {
  test("uses stable protocol identities for replay deduplication", () => {
    expect(stableChatEventIdentity({ type: "system_notice", content: "notice", event_id: "event-1" })).toBe("event:event-1");
    expect(stableChatEventIdentity({ type: "system_notice", content: "notice", run_id: "run-1", sequence: 2 })).toBe("run:run-1:2");
    expect(stableChatEventIdentity({ type: "system_notice", content: "notice", sequence: 2 })).toBeUndefined();
  });

  test("deduplicates stream events with a constant-size watermark", () => {
    const state = dedupState();
    for (let index = 0; index < 10_000; index += 1) {
      expect(rememberChatEvent(state, {
        type: "assistant_text_delta", content: "text", message_id: "message",
        event_id: `event-${index}`,
        stream_event_id: index,
      })).toBe(true);
    }
    expect(state.seenStreamEventID).toBe(9_999);
    expect(Object.keys(state.seenEventKeys)).toHaveLength(0);
    expect(rememberChatEvent(state, { type: "assistant_text_delta", content: "text", message_id: "message", stream_event_id: 9_999 })).toBe(false);
  });

  test("bounds fallback identities and matches history against stream replay", () => {
    const state = dedupState();
    expect(rememberChatEvent(state, { type: "assistant_completed", message_id: "message", event_id: "shared" })).toBe(true);
    expect(rememberChatEvent(state, {
      type: "assistant_completed", message_id: "message",
      event_id: "shared",
      stream_event_id: 1,
    })).toBe(false);

    for (let index = 0; index < maxFallbackEventKeys + 600; index += 1) {
      rememberChatEvent(state, { type: "system_notice", content: "notice", event_id: `fallback-${index}` });
    }
    expect(state.seenEventKeyOrder.length).toBeLessThanOrEqual(maxFallbackEventKeys);
    expect(Object.keys(state.seenEventKeys).length).toBe(state.seenEventKeyOrder.length);
  });
});

function dedupState(): ChatEventDedupState {
  return { seenEventKeys: {}, seenEventKeyOrder: [] };
}
