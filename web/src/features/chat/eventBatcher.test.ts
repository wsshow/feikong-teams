import { describe, expect, test } from "bun:test";
import { EventBatcher, type BatchScheduler } from "./eventBatcher";

describe("event batcher", () => {
  test("coalesces events scheduled in the same window", () => {
    const scheduler = new ManualScheduler();
    const batches: number[][] = [];
    const batcher = new EventBatcher<number>((items) => batches.push(items), scheduler);

    batcher.push([1]);
    batcher.push([2, 3]);

    expect(batches).toHaveLength(0);
    expect(scheduler.pending()).toBe(1);
    scheduler.runNext();
    expect(batches).toEqual([[1, 2, 3]]);
  });

  test("flushes terminal work immediately and can discard stale work", () => {
    const scheduler = new ManualScheduler();
    const batches: number[][] = [];
    const batcher = new EventBatcher<number>((items) => batches.push(items), scheduler);

    batcher.push([1]);
    batcher.flush();
    expect(batches).toEqual([[1]]);
    expect(scheduler.pending()).toBe(0);

    batcher.push([2]);
    batcher.discard();
    scheduler.runNext();
    expect(batches).toEqual([[1]]);
  });
});

class ManualScheduler implements BatchScheduler {
  private nextID = 0;
  private callbacks = new Map<number, () => void>();

  schedule(callback: () => void) {
    const id = this.nextID;
    this.nextID += 1;
    this.callbacks.set(id, callback);
    return id;
  }

  cancel(handle: unknown) {
    this.callbacks.delete(Number(handle));
  }

  pending() {
    return this.callbacks.size;
  }

  runNext() {
    const entry = this.callbacks.entries().next().value as [number, () => void] | undefined;
    if (!entry) return;
    this.callbacks.delete(entry[0]);
    entry[1]();
  }
}
