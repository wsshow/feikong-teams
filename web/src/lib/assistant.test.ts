import { expect, test } from "bun:test";
import { assistantDisplayName, assistantSessionID } from "./assistant";
import { chatPath, chatSessionIDFromPath, panelFromPath } from "./navigation";

test("assistant routes reopen the same conversation and preserve ordinary chat routes", () => {
  expect(chatPath(assistantSessionID)).toBe("/assistant");
  expect(chatSessionIDFromPath("/assistant")).toBe(assistantSessionID);
  expect(chatSessionIDFromPath(`/chat/${assistantSessionID}`)).toBe(assistantSessionID);
  expect(panelFromPath("/assistant")).toBe("chat");
  expect(chatPath("normal")).toBe("/chat/normal");
  expect(chatSessionIDFromPath("/chat")).toBe("");
});

test("assistant name follows persisted session metadata", () => {
  expect(assistantDisplayName([])).toBe("助手");
  expect(assistantDisplayName([{ session_id: "other", title: "other" }, { session_id: assistantSessionID, title: "小空" }])).toBe("小空");
});
