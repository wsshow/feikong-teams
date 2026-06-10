const assert = require("node:assert/strict");
const test = require("node:test");

global.FKTeamsChat = function () {};
require("../js/history.js");

function fakeAssistantMessage(body) {
  return {
    querySelector(selector) {
      if (selector === ".message-body") return body;
      return null;
    },
  };
}

function fakeMessageBody() {
  return {
    html: "",
    appendChild() {},
    prepend() {},
    querySelector() {
      return null;
    },
    setAttribute() {},
    set innerHTML(value) {
      this.html = value;
    },
    get innerHTML() {
      return this.html;
    },
  };
}

function fakeClassList(initial = []) {
  const classes = new Set(initial);
  return {
    add(name) { classes.add(name); },
    remove(name) { classes.delete(name); },
    contains(name) { return classes.has(name); },
  };
}

function fakeElement() {
  return {
    innerHTML: "",
    className: "",
    classList: fakeClassList(),
    setAttribute() {},
    addEventListener() {},
    querySelector() {
      return { addEventListener() {} };
    },
  };
}

test("history action splits assistant text timeline", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const bodies = [];
  const timeline = [];

  chat.createAssistantMessage = () => {
    const body = fakeMessageBody();
    bodies.push(body);
    timeline.push({ type: "message", body });
    return fakeAssistantMessage(body);
  };
  chat.renderSingleAction = (action) => {
    timeline.push({ type: "action", action });
  };
  chat.renderMarkdown = (content) => content;

  chat.renderHistoryAgentMessage({
    agent_name: "coordinator",
    events: [
      { type: "text", content: "before" },
      {
        type: "action",
        action: { action_type: "approval_required", content: "approval" },
      },
      { type: "text", content: "after" },
    ],
  });

  assert.equal(bodies.length, 2);
  assert.deepEqual(timeline.map((item) => item.type), [
    "message",
    "action",
    "message",
  ]);
  assert.equal(bodies[0].html, "before");
  assert.equal(bodies[1].html, "after");
});

test("sidebar history shows loading before debounced fetch", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  let debounceCalled = false;
  const classes = new Set();
  chat.sidebarSessionList = {
    innerHTML: '<div class="sidebar-session-empty">暂无会话记录</div>',
    classList: {
      add(name) { classes.add(name); },
      remove(name) { classes.delete(name); },
      contains(name) { return classes.has(name); },
    },
  };
  chat.debounce = () => {
    debounceCalled = true;
  };

  chat.loadSidebarHistory();

  assert.equal(debounceCalled, true);
  assert.equal(chat.sidebarSessionList.classList.contains("loading"), true);
  assert.match(chat.sidebarSessionList.innerHTML, /sidebar-session-loading/);
  assert.match(chat.sidebarSessionList.innerHTML, /加载中/);
});

test("sidebar session render clears loading layout", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  chat.sidebarSessionList = {
    innerHTML: "",
    appendChild() {},
    querySelectorAll() { return []; },
    classList: fakeClassList(["loading"]),
  };
  chat._sidebarMenuOutsideBound = true;
  chat.escapeHtml = (value) => String(value || "");
  chat.formatTime = () => "";

  chat.renderSidebarSessions([]);

  assert.equal(chat.sidebarSessionList.classList.contains("loading"), false);
});

test("sidebar session render shows labels for stored statuses", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const items = [];
  const oldDocument = global.document;
  global.document = {
    createElement() {
      return fakeElement();
    },
  };
  chat.sidebarSessionList = {
    innerHTML: "",
    appendChild(item) { items.push(item); },
    querySelectorAll() { return []; },
    classList: fakeClassList(),
  };
  chat._sidebarMenuOutsideBound = true;
  chat.escapeHtml = (value) => String(value || "");
  chat.formatTime = () => "刚刚";

  try {
    chat.renderSidebarSessions([
      { session_id: "completed", title: "done", status: "completed", mod_time: "2026-01-01T00:00:00Z" },
      { session_id: "error", title: "err", status: "error", mod_time: "2026-01-01T00:00:00Z" },
      { session_id: "cancelled", title: "cancel", status: "cancelled", mod_time: "2026-01-01T00:00:00Z" },
      { session_id: "idle", title: "idle", status: "idle", mod_time: "2026-01-01T00:00:00Z" },
      { session_id: "active", title: "active", status: "active", mod_time: "2026-01-01T00:00:00Z" },
    ]);

    const html = items.map((item) => item.innerHTML).join("\n");
    assert.match(html, /已完成/);
    assert.match(html, /失败/);
    assert.match(html, /已取消/);
    assert.match(html, /未开始/);
    assert.match(html, /已保存/);
  } finally {
    global.document = oldDocument;
  }
});
