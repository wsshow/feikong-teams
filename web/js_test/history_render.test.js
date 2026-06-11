const assert = require("node:assert/strict");
const test = require("node:test");

global.FKTeamsChat = function () {};
require("../js/history.js");
require("../js/messages.js");

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

function withFakeLocalStorage(fn) {
  const oldLocalStorage = global.localStorage;
  const store = new Map();
  global.localStorage = {
    getItem(key) {
      return store.has(key) ? store.get(key) : null;
    },
    setItem(key, value) {
      store.set(key, String(value));
    },
    removeItem(key) {
      store.delete(key);
    },
  };
  try {
    const result = fn(store);
    if (result && typeof result.finally === "function") {
      return result.finally(() => {
        global.localStorage = oldLocalStorage;
      });
    }
    global.localStorage = oldLocalStorage;
    return result;
  } catch (err) {
    global.localStorage = oldLocalStorage;
    throw err;
  }
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

test("history user message renders saved image content parts", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const items = [];
  const oldDocument = global.document;
  global.document = {
    createElement() {
      return fakeElement();
    },
  };
  chat.messagesContainer = {
    appendChild(item) { items.push(item); },
  };
  chat.escapeHtml = (value) => String(value || "");
  chat.formatHistoryTime = () => "09:05";
  chat.getCurrentTime = () => "09:05";
  chat.userQuestions = [];
  chat.updateQuickNav = () => {};

  try {
    chat.renderHistoryUserMessage({
      start_time: "2026-06-11T01:05:00Z",
      events: [{
        type: "text",
        content: "图中有什么",
        content_parts: [
          { type: "text", text: "图中有什么" },
          { type: "image_url", base64_data: "abc123", mime_type: "image/png" },
        ],
      }],
    });
  } finally {
    global.document = oldDocument;
  }

  assert.equal(items.length, 1);
  assert.match(items[0].innerHTML, /message-attachments/);
  assert.match(items[0].innerHTML, /data:image\/png;base64,abc123/);
  assert.match(items[0].innerHTML, /图中有什么/);
});

test("history agent message renders stored error event", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const items = [];
  const oldDocument = global.document;
  global.document = {
    createElement() {
      return fakeElement();
    },
  };
  chat.messagesContainer = {
    appendChild(item) { items.push(item); },
  };
  chat.escapeHtml = (value) => String(value || "");

  try {
    chat.renderHistoryAgentMessage({
      agent_name: "coordinator",
      events: [{
        type: "error",
        content: "这次消息里包含图片，但当前模型不支持图片输入。",
        error: {
          code: "model_unsupported_image_input",
          title: "当前模型不支持图片输入",
          message: "这次消息里包含图片，但当前模型不支持图片输入。",
          suggestions: ["切换到支持视觉输入的模型后重试。"],
          technical_detail: "deepseek does not support image_url type",
        },
      }],
    });
  } finally {
    global.document = oldDocument;
  }

  assert.equal(items.length, 1);
  assert.equal(items[0].className, "error-message");
  assert.match(items[0].innerHTML, /\[coordinator\]/);
  assert.match(items[0].innerHTML, /当前模型不支持图片输入/);
  assert.match(items[0].innerHTML, /切换到支持视觉输入的模型后重试/);
  assert.match(items[0].innerHTML, /技术详情/);
  assert.match(items[0].innerHTML, /does not support image_url/);
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

test("show home page clears selected session state", () => withFakeLocalStorage((store) => {
  const chat = Object.create(FKTeamsChat.prototype);
  let cleared = false;
  let queueCleared = false;
  let activeUpdated = false;
  store.set("fk_session_id", "session-1");
  chat._startupSessionId = "session-1";
  chat.sessionId = "session-1";
  chat._hasLoadedSession = true;
  chat.isProcessing = true;
  chat.sessionIdInput = { value: "session-1" };
  chat._saveSessionDOM = () => {};
  chat.hideChatLoading = () => {};
  chat.clearChatUI = () => { cleared = true; };
  chat.handleQueueUpdated = (event) => { queueCleared = Array.isArray(event.queue) && event.queue.length === 0; };
  chat.updateStatus = () => {};
  chat.updateSendButtonState = () => {};
  chat.updateSidebarSessionActive = () => { activeUpdated = true; };

  chat.showHomePage();

  assert.equal(store.has("fk_session_id"), false);
  assert.equal(chat._startupSessionId, "");
  assert.equal(chat.sessionId, "");
  assert.equal(chat._hasLoadedSession, false);
  assert.equal(chat.isProcessing, false);
  assert.equal(chat.sessionIdInput.value, "");
  assert.equal(cleared, true);
  assert.equal(queueCleared, true);
  assert.equal(activeUpdated, true);
}));

test("loaded session updates sidebar active state without reloading list", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  let reloaded = false;
  let activeUpdated = false;
  chat.sessionIdInput = { value: "" };
  chat.messagesContainer = { innerHTML: "" };
  chat.hideChatLoading = () => {};
  chat.clearChatUI = () => {};
  chat.clearQuickNav = () => {};
  chat.resetParallelState = () => {};
  chat.scrollToBottom = () => {};
  chat.agents = [];
  chat.setCurrentAgent = () => {};
  chat.loadSidebarHistory = () => { reloaded = true; };
  chat.updateSidebarSessionActive = () => { activeUpdated = true; };

  chat.handleHistoryLoaded({ session_id: "session-1", messages: [] });

  assert.equal(chat.sessionId, "session-1");
  assert.equal(chat.sessionIdInput.value, "session-1");
  assert.equal(chat._hasLoadedSession, true);
  assert.equal(activeUpdated, true);
  assert.equal(reloaded, false);
});

test("confirmed delete removes sidebar item without reloading list", async () => {
  const chat = Object.create(FKTeamsChat.prototype);
  let reloaded = false;
  let modalReloaded = false;
  let notified = false;
  const sidebarItems = [
    {
      dataset: { sessionId: "delete-me" },
      remove() {
        sidebarItems.splice(sidebarItems.indexOf(this), 1);
      },
    },
    {
      dataset: { sessionId: "keep-me" },
      remove() {
        sidebarItems.splice(sidebarItems.indexOf(this), 1);
      },
    },
  ];
  chat.sidebarSessionList = {
    innerHTML: "",
    classList: fakeClassList(),
    querySelectorAll(selector) {
      if (selector === ".sidebar-session-item") return sidebarItems;
      return [];
    },
  };
  chat.currentDeleteSessionId = "delete-me";
  chat._sessionDOMCache = {};
  chat._processingSessions = new Set(["delete-me"]);
  chat.historyModal = { style: { display: "none" } };
  chat.hideDeleteModal = () => { chat.currentDeleteSessionId = null; };
  chat.fetchWithAuth = async () => ({
    ok: true,
    json: async () => ({ code: 0 }),
  });
  chat.showNotification = (message, type) => {
    notified = message === "删除成功" && type === "success";
  };
  chat.loadSessions = async () => { modalReloaded = true; };
  chat.loadSidebarHistory = async () => { reloaded = true; };

  await chat.confirmDelete();

  assert.equal(notified, true);
  assert.deepEqual(sidebarItems.map((item) => item.dataset.sessionId), ["keep-me"]);
  assert.equal(chat._processingSessions.has("delete-me"), false);
  assert.equal(modalReloaded, false);
  assert.equal(reloaded, false);
});

test("missing loaded session falls back to home page", async () => withFakeLocalStorage(async (store) => {
  const chat = Object.create(FKTeamsChat.prototype);
  let saveCount = 0;
  let homeOptions = null;
  store.set("fk_session_id", "missing-session");
  chat.sessionId = "";
  chat._hasLoadedSession = false;
  chat.sessionIdInput = { value: "" };
  chat.messagesContainer = { innerHTML: "" };
  chat._saveSessionDOM = () => {
    if (chat.sessionId) saveCount += 1;
  };
  chat.showChatLoading = () => {};
  chat.hideChatLoading = () => {};
  chat.hideHistoryModal = () => {};
  chat.fetchWithAuth = async () => ({ ok: false, status: 404 });
  chat.showHomePage = (options) => {
    homeOptions = options;
    store.delete("fk_session_id");
    chat.sessionId = "";
    chat._hasLoadedSession = false;
    chat.sessionIdInput.value = "";
  };

  await chat._loadSession("missing-session");

  assert.equal(saveCount, 0);
  assert.deepEqual(homeOptions, { skipSaveCurrentDOM: true });
  assert.equal(store.has("fk_session_id"), false);
  assert.equal(chat.sessionId, "");
  assert.equal(chat._hasLoadedSession, false);
  assert.equal(chat.sessionIdInput.value, "");
}));
