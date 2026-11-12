import { afterEach, beforeEach, expect, test } from "bun:test";
import { JSDOM } from "jsdom";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { chatMessageElementID } from "./dom";
import { useActiveQuestion } from "./useActiveQuestion";

let dom: JSDOM;
let root: Root;
let container: HTMLDivElement;
let scroll: HTMLDivElement;
let scrollRef: { current: HTMLDivElement | null };
let content: HTMLDivElement;
let viewportHeight: number;
let contentHeight: number;
let offsets: Map<string, number>;
let frames: Map<number, FrameRequestCallback>;
let observers: TestResizeObserver[];
let originalGlobals: Record<string, unknown>;

class TestResizeObserver {
  observed: Element[] = [];
  disconnected = false;
  constructor(readonly callback: ResizeObserverCallback) { observers.push(this); }
  observe(element: Element) { this.observed.push(element); }
  unobserve() {}
  disconnect() { this.disconnected = true; }
  notify() { this.callback([], this as unknown as ResizeObserver); }
}

beforeEach(() => {
  originalGlobals = Object.fromEntries(["window", "document", "ResizeObserver", "IS_REACT_ACT_ENVIRONMENT"].map((key) => [key, Reflect.get(globalThis, key)]));
  dom = new JSDOM("<!doctype html><html><body></body></html>");
  Object.assign(globalThis, { window: dom.window, document: dom.window.document, ResizeObserver: TestResizeObserver, IS_REACT_ACT_ENVIRONMENT: true });
  frames = new Map();
  observers = [];
  offsets = new Map();
  let nextFrame = 0;
  dom.window.requestAnimationFrame = (callback) => { frames.set(++nextFrame, callback); return nextFrame; };
  dom.window.cancelAnimationFrame = (id) => { frames.delete(id); };
  viewportHeight = 500;
  contentHeight = 5000;
  scroll = document.createElement("div");
  content = document.createElement("div");
  scroll.appendChild(content);
  document.body.appendChild(scroll);
  scrollRef = { current: scroll };
  Object.defineProperties(scroll, {
    clientHeight: { get: () => viewportHeight },
    scrollHeight: { get: () => contentHeight },
  });
  scroll.getBoundingClientRect = () => new dom.window.DOMRect(0, 100, 600, viewportHeight);
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  Object.assign(globalThis, originalGlobals);
  dom.window.close();
});

function Harness({ questions }: { questions: { id: string }[] }) {
  const active = useActiveQuestion(scrollRef, questions);
  return <output>{active}</output>;
}

function addQuestion(id: string, offset: number) {
  offsets.set(id, offset);
  const element = document.createElement("article");
  element.id = chatMessageElementID(id);
  element.getBoundingClientRect = () => new dom.window.DOMRect(0, 100 + offsets.get(id)! - scroll.scrollTop, 600, 80);
  content.appendChild(element);
  return { id };
}

async function flushFrames() {
  await act(async () => {
    const pending = [...frames.values()];
    frames.clear();
    pending.forEach((callback) => callback(0));
  });
}

async function render(questions: { id: string }[]) {
  await act(async () => root.render(<Harness questions={questions} />));
  await flushFrames();
}

async function scrollTo(top: number) {
  scroll.scrollTop = top;
  scroll.dispatchEvent(new dom.window.Event("scroll"));
  await flushFrames();
}

test("tracks manual scrolling in both directions and retains the question throughout a long answer", async () => {
  await render([addQuestion("first", 0), addQuestion("second", 2000), addQuestion("third", 3500)]);
  expect(container.textContent).toBe("first");
  await scrollTo(1000);
  expect(container.textContent).toBe("first");
  await scrollTo(1800);
  expect(container.textContent).toBe("second");
  await scrollTo(3300);
  expect(container.textContent).toBe("third");
  await scrollTo(2100);
  expect(container.textContent).toBe("second");
  await scrollTo(0);
  expect(container.textContent).toBe("first");
});

test("initial selection uses the scroll position and bottom selects the final question below the midpoint", async () => {
  scroll.scrollTop = 1800;
  await render([addQuestion("first", 0), addQuestion("second", 2000), addQuestion("last", 4900)]);
  expect(container.textContent).toBe("second");
  await scrollTo(4500);
  expect(container.textContent).toBe("last");
  await scrollTo(4470);
  expect(container.textContent).toBe("last");
  await scrollTo(4440);
  expect(container.textContent).toBe("second");
});

test("new questions do not steal the highlight while reading earlier history", async () => {
  const questions = [addQuestion("first", 0), addQuestion("second", 2000)];
  await render(questions);
  await scrollTo(800);
  await render([...questions, addQuestion("new", 4000)]);
  expect(container.textContent).toBe("first");
  await scrollTo(3800);
  expect(container.textContent).toBe("new");
});

test("content layout and viewport changes update the selection without a scroll event", async () => {
  await render([addQuestion("first", 0), addQuestion("second", 2000)]);
  await scrollTo(1800);
  expect(container.textContent).toBe("second");
  expect(observers[0]!.observed).toEqual([scroll, content]);
  offsets.set("second", 2200);
  observers[0]!.notify();
  await flushFrames();
  expect(container.textContent).toBe("first");
  viewportHeight = 900;
  observers[0]!.notify();
  await flushFrames();
  expect(container.textContent).toBe("second");
});

test("ignores missing anchors and anchors outside the message scroll container", async () => {
  const outside = document.createElement("article");
  outside.id = chatMessageElementID("outside");
  document.body.appendChild(outside);
  await render([addQuestion("first", 0), { id: "missing" }, { id: "outside" }]);
  await scrollTo(4500);
  expect(container.textContent).toBe("first");
  await render([]);
  expect(container.textContent).toBe("");
});

test("switching conversations replaces the anchors and disconnects the old observer", async () => {
  await render([addQuestion("old", 0)]);
  const oldObserver = observers[0]!;
  content.replaceChildren();
  await render([addQuestion("new-first", 0), addQuestion("new-second", 2000)]);
  expect(oldObserver.disconnected).toBe(true);
  expect(container.textContent).toBe("new-first");
  await scrollTo(2100);
  expect(container.textContent).toBe("new-second");
});

test("coalesces scroll events into one frame and cancels pending work on unmount", async () => {
  await render([addQuestion("first", 0), addQuestion("second", 2000)]);
  scroll.scrollTop = 2100;
  for (let index = 0; index < 5; index++) scroll.dispatchEvent(new dom.window.Event("scroll"));
  expect(frames.size).toBe(1);
  await flushFrames();
  expect(container.textContent).toBe("second");
  scroll.dispatchEvent(new dom.window.Event("scroll"));
  await act(async () => root.unmount());
  expect(frames.size).toBe(0);
  expect(observers[0]!.disconnected).toBe(true);
  scroll.dispatchEvent(new dom.window.Event("scroll"));
  expect(frames.size).toBe(0);
});
