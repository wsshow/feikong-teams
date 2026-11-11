import { expect, test } from "bun:test";
import { decodeChatEvent } from "./events";

test("unknown and malformed envelopes stay outside application event types", () => {
  expect(decodeChatEvent({ type: "future_extension", content: "new" })).toBeUndefined();
  expect(decodeChatEvent({ type: "assistant_text_delta", content: 42 })).toBeUndefined();
  expect(decodeChatEvent(null)).toBeUndefined();
  expect(decodeChatEvent({ type: "member_completed", member_call_id: "member-1" })?.type).toBe("member_completed");
});
