import { describe, expect, test } from "bun:test";
import {
  applyPresentationUpdate,
  decodePresentationSnapshot,
  isPresentationUpdate,
  type PresentationSnapshot,
  type TimelineItem,
} from "@/types/presentation";

function message(id: string, index: number, content: string): TimelineItem {
  return {
    id,
    index,
    order: index,
    kind: "message",
    node: {
      message: {
        id,
        role: "assistant",
        content,
        createdAt: "2026-11-12T00:00:00Z",
      },
      parts: [{ type: "text", key: `${id}:text`, content }],
      order: index,
      showAgentLabel: true,
      showCopyAction: true,
    },
  };
}
describe("server conversation presentation", () => {
  test("snapshot plus ordered updates equals the final authoritative snapshot", () => {
    const state: PresentationSnapshot = { items: [] };
    applyPresentationUpdate(state, { items: [message("first", 0, "draft")] });
    applyPresentationUpdate(state, {
      appends: [{ item_id: "first", key: "first:text", content: " text" }],
    });
    applyPresentationUpdate(state, { items: [message("second", 1, "next")] });
    const final = {
      items: [message("first", 0, "corrected"), message("second", 1, "next")],
    };
    applyPresentationUpdate(state, {
      reset: decodePresentationSnapshot(final),
    });
    expect(state).toEqual(decodePresentationSnapshot(final));
  });
  test("uses explicit insertion positions and keeps input payloads immutable", () => {
    const state = { items: [message("later", 0, "later")] };
    const update = { items: [message("earlier", 0, "earlier")] };
    applyPresentationUpdate(state, update);
    expect(state.items.map((item) => item.id)).toEqual(["earlier", "later"]);
    const node = state.items[0];
    if (node.kind !== "message") throw new Error("missing node");
    node.node.parts = [];
    expect(
      update.items[0].kind === "message" && update.items[0].node.parts.length,
    ).toBe(1);
  });
  test("rejects malformed or missing view identities at the API boundary", () => {
    expect(() => decodePresentationSnapshot({ events: [] })).toThrow();
    const bad = message("m", 0, "text");
    delete (bad as Partial<TimelineItem>).index;
    expect(() => decodePresentationSnapshot({ items: [bad] })).toThrow();
    expect(
      isPresentationUpdate({
        appends: [{ item_id: "m", key: "text", content: 1 }],
      }),
    ).toBe(false);
    expect(() =>
      applyPresentationUpdate(
        { items: [] },
        { appends: [{ item_id: "missing", key: "text", content: "x" }] },
      ),
    ).toThrow();
  });
});

test("every Go projection frame produces exactly the same browser snapshot", async () => {
  const fixture = await import("@/types/testdata/chat-view.json");
  const state: PresentationSnapshot = { items: [] };
  for (const frame of fixture.frames) {
    expect(isPresentationUpdate(frame.update)).toBe(true);
    applyPresentationUpdate(
      state,
      frame.update as import("@/types/presentation").PresentationUpdate,
    );
    expect(state).toEqual(decodePresentationSnapshot(frame.snapshot));
  }
});
