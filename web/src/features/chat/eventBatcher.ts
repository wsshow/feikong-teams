export interface BatchScheduler {
  schedule(callback: () => void): unknown;
  cancel(handle: unknown): void;
}

const defaultScheduler: BatchScheduler = {
  schedule: (callback) => globalThis.setTimeout(callback, 16),
  cancel: (handle) => globalThis.clearTimeout(handle as ReturnType<typeof setTimeout>),
};

export class EventBatcher<T> {
  private readonly flushBatch: (items: T[]) => void;
  private readonly scheduler: BatchScheduler;
  private items: T[] = [];
  private scheduled: unknown;

  constructor(flushBatch: (items: T[]) => void, scheduler: BatchScheduler = defaultScheduler) {
    this.flushBatch = flushBatch;
    this.scheduler = scheduler;
  }

  push(items: T[]) {
    if (items.length === 0) return;
    this.items.push(...items);
    if (this.scheduled !== undefined) return;
    this.scheduled = this.scheduler.schedule(() => {
      this.scheduled = undefined;
      this.flush();
    });
  }

  flush() {
    if (this.scheduled !== undefined) {
      this.scheduler.cancel(this.scheduled);
      this.scheduled = undefined;
    }
    if (this.items.length === 0) return;
    const items = this.items;
    this.items = [];
    this.flushBatch(items);
  }

  discard() {
    if (this.scheduled !== undefined) this.scheduler.cancel(this.scheduled);
    this.scheduled = undefined;
    this.items = [];
  }
}
