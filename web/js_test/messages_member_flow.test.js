const assert = require("node:assert/strict");
const test = require("node:test");

global.FKTeamsChat = function () {};
require("../js/messages.js");

function newChatWithRecordedMigrations() {
  const chat = Object.create(FKTeamsChat.prototype);
  return chat;
}

test("message event hides thinking after visible render handler", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const calls = [];
  chat.sessionId = "session-1";
  chat.rememberStreamEvent = () => {};
  chat.handleCoreMessageDelta = () => {
    calls.push("render");
  };
  chat.hideThinkingIndicator = () => {
    calls.push("hide");
  };

  chat.handleServerEvent({
    type: "message_delta",
    session_id: "session-1",
    role: "assistant",
    content: "hello",
  });

  assert.deepEqual(calls, ["render", "hide"]);
});

test("sources card uses local favicon proxy", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  chat.escapeHtml = (value) => String(value || "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");

  const html = chat._buildSourcesCard("", [{
    url: "https://example.com/report",
    label: "Example",
  }]);

  assert.match(html, /\/api\/fkteams\/favicon\?domain=example\.com&amp;size=32/);
  assert.match(html, /\/api\/fkteams\/favicon\?domain=example\.com&amp;size=16/);
  assert.doesNotMatch(html, /google\.com\/s2\/favicons/);
});

test("member tool flow key uses only canonical ref", () => {
  const chat = newChatWithRecordedMigrations();
  const entry = {};

  const key = chat.resolveMemberToolFlowKey(
    entry,
    {
      tool_call_ref: "tool_call:member-tool-call",
      tool_call_id: "member-tool-call",
      tool_call_index: 0,
      tool_name: "member_echo",
    },
    {
      id: "member-tool-call",
      index: 0,
      name: "member_echo",
    },
    0,
  );

  assert.equal(key, "ref:tool_call:member-tool-call");
});

test("member tool flow key rejects index-only event", () => {
  const chat = newChatWithRecordedMigrations();
  const entry = {};

  const key = chat.resolveMemberToolFlowKey(
    entry,
    {
      tool_call_index: 0,
      tool_name: "member_echo",
    },
    null,
    0,
  );

  assert.equal(key, "");
});

test("tool call normalization does not spread top-level identity into array calls", () => {
  const chat = newChatWithRecordedMigrations();

  const calls = chat.normalizeToolCallsForEvent({
    tool_call_id: "call-1",
    tool_call_ref: "ref-1",
    tool_call_index: 2,
    tool_name: "member_echo",
    tool_display_name: "Echo",
    tool_kind: "tool",
    tool_calls: [{
      id: "call-1",
      index: 2,
      name: "member_echo",
      arguments: "{\"text\":\"hello\"}",
    }],
  });

  assert.equal(calls.length, 1);
  assert.deepEqual(calls[0], {
    id: "call-1",
    ref: "",
    index: 2,
    name: "member_echo",
    display_name: "Echo",
    kind: "tool",
    target: "",
    arguments: "{\"text\":\"hello\"}",
  });
});

test("tool key rejects non-canonical span identity", () => {
  const chat = newChatWithRecordedMigrations();

  const calls = chat.normalizeToolCallsForEvent({
    type: "tool_update",
    span_id: "span-tool-1",
    tool_name: "member_echo",
    delta_kind: "tool_result",
    delta: "chunk",
  });

  assert.equal(calls.length, 1);
  assert.equal(chat.toolCallKey(calls[0], 0), "");
});

test("member tool flow rejects id-only event even when name alias exists", () => {
  const chat = newChatWithRecordedMigrations();
  const entry = {};

  const key = chat.resolveMemberToolFlowKey(
    entry,
    {
      tool_call_id: "member-tool-call",
      tool_name: "member_echo",
    },
    {
      id: "member-tool-call",
      name: "member_echo",
    },
    0,
  );

  assert.equal(key, "");
});

test("role tool message delta is routed as tool result update", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  let routed = null;
  chat.handleCoreToolUpdate = (event) => {
    routed = event;
  };

  chat.handleCoreMessageDelta({
    role: "tool",
    tool_call_id: "call-1",
    tool_call_ref: "tool_call:call-1",
    tool_name: "member_echo",
    content: "stream chunk",
  });

  assert.equal(routed.tool_call_id, "call-1");
  assert.equal(routed.tool_call_ref, "tool_call:call-1");
  assert.equal(routed.delta_kind, "tool_result");
  assert.equal(routed.content, "stream chunk");
});

test("role tool message end is routed as tool result end", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  let routed = null;
  chat.handleCoreToolEnd = (event) => {
    routed = event;
  };

  chat.handleCoreMessageEnd({
    role: "tool",
    tool_call_id: "call-1",
    tool_call_ref: "tool_call:call-1",
    tool_name: "member_echo",
    content: "done",
  });

  assert.equal(routed.tool_call_id, "call-1");
  assert.equal(routed.tool_call_ref, "tool_call:call-1");
  assert.equal(routed.content, "done");
});

test("thinking indicator stays visible for message start", () => {
  const chat = Object.create(FKTeamsChat.prototype);

  assert.equal(chat.shouldHideThinkingIndicatorForEvent({
    type: "message_start",
  }), false);
});

test("thinking indicator hides for visible message content", () => {
  const chat = Object.create(FKTeamsChat.prototype);

  assert.equal(chat.shouldHideThinkingIndicatorForEvent({
    type: "message_delta",
    content: "hello",
  }), true);
  assert.equal(chat.shouldHideThinkingIndicatorForEvent({
    type: "message_delta",
  }), false);
});

test("queued executing user message renders from user_message event", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  let rendered = null;
  let turnID = "";

  chat.messagesContainer = { querySelectorAll: () => [] };
  chat.beginRealtimeTurn = (id) => {
    turnID = id;
  };
  chat.addQueuedFollowUpMessage = (content, queueID) => {
    rendered = { content, queueID };
  };

  chat.handleUserMessageEvent({
    type: "user_message",
    content: "later",
    queued_executing: true,
    queue_kind: "follow_up",
    queue_id: "queue-1",
    turn_id: "turn-1",
  });

  assert.equal(turnID, "turn-1");
  assert.deepEqual(rendered, { content: "later", queueID: "queue-1" });
});

test("queued executing user message forwards content parts", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  let rendered = null;
  const parts = [
    { type: "text", text: "look" },
    { type: "image_url", base64_data: "abc123", mime_type: "image/png" },
  ];

  chat.messagesContainer = { querySelectorAll: () => [] };
  chat.beginRealtimeTurn = () => {};
  chat.addQueuedFollowUpMessage = (content, queueID, attachments) => {
    rendered = { content, queueID, attachments };
  };

  chat.handleUserMessageEvent({
    type: "user_message",
    content: "look",
    content_parts: parts,
    queued_executing: true,
    queue_kind: "follow_up",
    queue_id: "queue-1",
  });

  assert.deepEqual(rendered, { content: "look", queueID: "queue-1", attachments: parts });
});

test("message attachments render saved image parts", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  chat.escapeHtml = (value) => String(value || "");

  const html = chat.renderMessageAttachments([
    { type: "text", text: "look" },
    { type: "image_url", base64_data: "abc123", mime_type: "image/png" },
  ]);

  assert.match(html, /message-attachments single-attachment/);
  assert.match(html, /data:image\/png;base64,abc123/);
});

test("error renderer shows friendly message and technical details", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  chat.escapeHtml = (value) => String(value || "");

  const html = chat.renderErrorContent({
    title: "当前模型不支持图片输入",
    message: "这次消息里包含图片，但当前模型不支持图片输入。",
    technicalError: "deepseek does not support image_url type",
    suggestions: ["切换到支持视觉输入的模型后重试。"],
    agentName: "coordinator",
  });

  assert.match(html, /当前模型不支持图片输入/);
  assert.match(html, /切换到支持视觉输入的模型后重试/);
  assert.match(html, /技术详情/);
  assert.match(html, /deepseek does not support image_url type/);
  assert.match(html, /\[coordinator\]/);
});

test("processing start does not render queued follow-up user card", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const calls = [];

  chat.updateStatus = () => {};
  chat.beginRealtimeTurn = (id) => {
    calls.push({ turnID: id });
  };
  chat.showThinkingIndicator = () => {
    calls.push("thinking");
  };
  chat.addQueuedFollowUpMessage = (content, queueID) => {
    calls.push({ content, queueID });
  };

  chat.handleProcessingStart({
    type: "processing_start",
    queued_executing: true,
    queue_kind: "follow_up",
    content: "later",
    queue_id: "queue-1",
    turn_id: "turn-1",
  });

  assert.deepEqual(calls, [{ turnID: "turn-1" }, "thinking"]);
});

test("realtime turn change resets render state once", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  let finalized = 0;
  let reset = 0;

  chat.finalizeParallelMemberResults = () => {
    finalized += 1;
  };
  chat.resetParallelState = () => {
    reset += 1;
  };

  chat.beginRealtimeTurn("turn-1");
  chat.beginRealtimeTurn("turn-1");

  assert.equal(finalized, 1);
  assert.equal(reset, 1);
  assert.equal(chat.currentRealtimeTurnID, "turn-1");
});

test("queued steering does not reset realtime turn state", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  let turnID = "";

  chat.updateStatus = () => {};
  chat.showThinkingIndicator = () => {};
  chat.addSteeringExecutionNotice = () => {};
  chat.beginRealtimeTurn = (id) => {
    turnID = id;
  };

  chat.handleProcessingStart({
    type: "processing_start",
    queued_executing: true,
    queue_kind: "steering",
    content: "adjust",
    queue_id: "queue-1",
    turn_id: "turn-1",
  });

  assert.equal(turnID, "turn-1");
});

test("user message is inserted before existing thinking indicator", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const oldDocument = global.document;
  const messageEl = {
    className: "",
    dataset: {},
    innerHTML: "",
    setAttribute() {},
  };
  const calls = [];
  const container = {
    insertBefore(child, before) { calls.push({ action: "insertBefore", child, before }); },
    appendChild(child) { calls.push({ action: "appendChild", child }); },
  };
  const thinking = { parentElement: container };

  global.document = {
    createElement() { return messageEl; },
    getElementById(id) { return id === "thinking-indicator" ? thinking : null; },
  };
  chat.messagesContainer = container;
  chat.getCurrentTime = () => "00:00";
  chat.escapeHtml = (value) => String(value || "");
  chat.addQuestionToNav = () => {};
  chat.scrollToBottom = () => {};

  try {
    chat.addUserMessage("later", null);
  } finally {
    global.document = oldDocument;
  }

  assert.deepEqual(calls, [{ action: "insertBefore", child: messageEl, before: thinking }]);
});

test("dispatch task handling does not assume the first tool call", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  chat.isMemberRunEvent = () => false;
  chat.messagesContainer = { querySelectorAll: () => [] };
  chat.scrollToBottom = () => {};

  chat.handleToolCalls({
    tool_calls: [
      { name: "other_tool", arguments: "{\"x\":1}" },
      { name: "dispatch_tasks", arguments: "{\"tasks\":[{\"title\":\"task\"}]}" },
    ],
  });

  assert.deepEqual(chat._pendingDispatchTasks, [{ title: "task" }]);
});

test("member final message marks the member card done", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const entry = {};
  let status = null;

  chat.memberEntryFromEvent = () => entry;
  chat.flushMemberInnerToolResults = () => {};
  chat.memberCallIDFromEvent = () => "call-1";
  chat.setMemberFinalOutput = () => {};
  chat.setMemberFinalReasoning = () => {};
  chat.updateMemberActivity = () => {};
  chat.finalizeMemberMarkdown = () => {};
  chat.updateMemberStatus = (_entry, state, text) => {
    status = { entry: _entry, state, text };
  };
  chat.scrollToBottom = () => {};

  chat.handleMemberMessage({
    member_call_id: "call-1",
    content: "done",
  });

  assert.deepEqual(status, { entry, state: "done", text: "完成" });
});

test("agent tool result uses event metadata when stored tool mapping was reset", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const entry = {};
  let ensured = null;
  let status = null;

  chat.isMemberRunEvent = () => false;
  chat.toolCallsByID = {};
  chat.parallelToolMemberByID = {};
  chat.parallelMemberResultChunks = {};
  chat.lastToolName = "";
  chat.ensureMemberCard = (key, label, agentName) => {
    ensured = { key, label, agentName };
    return entry;
  };
  chat.memberHasOutputContent = () => false;
  chat.setMemberFinalOutput = () => {};
  chat.updateMemberStatus = (_entry, state, text) => {
    status = { entry: _entry, state, text };
  };
  chat.scrollToBottom = () => {};

  chat.handleToolResult({
    type: "tool_result",
    tool_call_id: "call-1",
    tool_call_ref: "tool_call:call-1",
    tool_name: "ask_fkagent_researcher",
    tool_display_name: "指派给 researcher",
    tool_kind: "agent",
    tool_target: "researcher",
    content: "result",
  });

  assert.deepEqual(ensured, {
    key: "call:call-1",
    label: "researcher",
    agentName: "researcher",
  });
  assert.deepEqual(status, { entry, state: "done", text: "完成" });
});

test("agent tool result does not finish a started member task", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const entry = { memberStarted: true };
  let status = null;

  chat.isMemberRunEvent = () => false;
  chat.toolCallsByID = {};
  chat.parallelToolMemberByID = {};
  chat.parallelMemberResultChunks = {};
  chat.lastToolName = "";
  chat.ensureMemberCard = () => entry;
  chat.memberHasOutputContent = () => false;
  chat.setMemberFinalOutput = () => {};
  chat.updateMemberStatus = (_entry, state, text) => {
    status = { entry: _entry, state, text };
  };
  chat.scrollToBottom = () => {};

  chat.handleToolResult({
    type: "tool_result",
    tool_call_id: "call-1",
    tool_call_ref: "tool_call:call-1",
    tool_name: "ask_fkagent_researcher",
    tool_display_name: "指派给 researcher",
    tool_kind: "agent",
    tool_target: "researcher",
    content: "intermediate result",
  });

  assert.equal(status, null);
});

test("member ask tool result does not create a second tool card", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const flow = { el: { isConnected: true }, status: { textContent: "已回答" } };
  const entry = {
    toolFlows: { "ask:interrupt-1": flow },
    activeAskFlowKeys: ["ask:interrupt-1"],
    activeAskFlowKey: "ask:interrupt-1",
  };
  let ensured = false;

  chat.memberEntryFromEvent = () => entry;
  chat.normalizeToolCallForEvent = () => ({ name: "ask_questions" });
  chat.ensureMemberToolFlow = () => {
    ensured = true;
    return null;
  };
  chat.scrollToBottom = () => {};

  chat.handleToolResult({
    type: "tool_result",
    member_call_id: "member-call-1",
    tool_name: "ask_questions",
    content: "{\"selected\":[\"A\"]}",
  });

  assert.equal(ensured, false);
  assert.equal(flow.status.textContent, "已完成");
  assert.deepEqual(entry.activeAskFlowKeys, []);
});

test("member event reopens a prematurely completed member card", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const classNames = new Set(["parallel-member-done"]);
  const entry = {
    el: {
      classList: {
        contains(name) {
          return classNames.has(name);
        },
      },
    },
  };
  let status = null;

  chat.ensureMemberCard = () => entry;
  chat.updateMemberStatus = (_entry, state, text) => {
    status = { entry: _entry, state, text };
    classNames.delete("parallel-member-done");
  };

  const got = chat.memberEntryFromEvent({
    member_call_id: "call-1",
    member_name: "researcher",
  });

  assert.equal(got, entry);
  assert.equal(entry.memberStarted, true);
  assert.deepEqual(status, { entry, state: "running", text: "运行中" });
});

test("new top-level member tool batch starts a fresh panel", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const oldPanel = { isConnected: true };
  chat.parallelPanel = oldPanel;
  chat.parallelPanelBatchMode = false;
  chat.parallelMemberCards = {};
  chat.parallelEntriesForPanel = (panel) => panel === oldPanel ? [{}] : [];
  chat.getToolDisplay = () => ({ kind: "agent" });
  chat.memberKeyForToolCall = (toolCall) => "call:" + toolCall.id;

  chat.prepareMemberPanelForToolCalls([{ id: "call-2", name: "ask_fkagent_researcher" }]);

  assert.equal(chat.parallelPanel, null);
});

test("existing member tool batch keeps the current panel", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const oldPanel = { isConnected: true };
  chat.parallelPanel = oldPanel;
  chat.parallelPanelBatchMode = false;
  chat.parallelMemberCards = {
    "call:call-1": { el: { isConnected: true } },
  };
  chat.parallelEntriesForPanel = () => [{}];
  chat.getToolDisplay = () => ({ kind: "agent" });
  chat.memberKeyForToolCall = (toolCall) => "call:" + toolCall.id;

  chat.prepareMemberPanelForToolCalls([{ id: "call-1", name: "ask_fkagent_researcher" }]);

  assert.equal(chat.parallelPanel, oldPanel);
});

test("member ask questions are routed into member card", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  let memberEvent = null;
  let globalEvent = null;

  chat.isMemberRunEvent = (event) => !!event.member_call_id;
  chat.showMemberAskQuestions = (event) => {
    memberEvent = event;
  };
  chat.showInlineAskForm = (event) => {
    globalEvent = event;
  };

  chat.handleAskQuestions({
    type: "ask_questions",
    member_call_id: "call-1",
    question: "Need input?",
  });

  assert.equal(memberEvent.member_call_id, "call-1");
  assert.equal(globalEvent, null);
});

test("member ask questions create flow keyed by ask id", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const details = { open: false };
  const status = { textContent: "" };
  const argsWrap = { style: { display: "none" } };
  const args = { textContent: "" };
  const label = { textContent: "结果" };
  const resultWrap = {
    style: { display: "none" },
    querySelector(selector) {
      return selector === ".parallel-member-tool-label" ? label : null;
    },
  };
  const result = { style: { display: "" } };
  const flow = {
    el: { isConnected: true, querySelector: (selector) => (selector === "details" ? details : null) },
    status,
    argsWrap,
    args,
    resultWrap,
    result,
  };
  const entry = { toolFlows: {} };
  let ensuredKey = "";
  let formTarget = null;

  chat.memberEntryFromEvent = () => entry;
  chat.ensureMemberToolFlow = (_entry, key) => {
    ensuredKey = key;
    entry.toolFlows[key] = flow;
    return flow;
  };
  chat.showInlineAskForm = (_event, target) => {
    formTarget = target;
    return { isConnected: true };
  };
  chat.updateMemberStatus = () => {};
  chat.updateMemberActivity = () => {};
  chat.updateMemberDetailVisibility = () => {};
  chat.scrollToBottom = () => {};

  chat.showMemberAskQuestions({
    type: "ask_questions",
    ask_id: "interrupt-1",
    member_call_id: "call-1",
    question: "Need input?",
  });

  assert.equal(ensuredKey, "ask:interrupt-1");
  assert.equal(details.open, true);
  assert.equal(status.textContent, "待回复");
  assert.equal(argsWrap.style.display, "");
  assert.equal(args.textContent, "Need input?");
  assert.equal(label.textContent, "回答");
  assert.equal(result.style.display, "none");
  assert.equal(formTarget, resultWrap);
});

test("member pending ask derives card status from ask state", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const classes = new Set();
  const statusEl = { textContent: "" };
  const activityEl = { textContent: "", style: { display: "none" } };
  const entry = {
    el: {
      classList: {
        add(name) { classes.add(name); },
        remove(...names) { names.forEach((name) => classes.delete(name)); },
      },
      querySelector(selector) {
        if (selector === ".parallel-member-status") return statusEl;
        if (selector === ".parallel-member-activity") return activityEl;
        return null;
      },
    },
    panel: {},
    activity: activityEl,
    activeAskFlowKeys: ["ask:ask-1"],
    activeAskFlowKey: "ask:ask-1",
    toolFlows: {
      "ask:ask-1": {
        el: { isConnected: true },
        askPending: true,
        askCompleted: false,
        askToolCompleted: false,
      },
    },
  };
  chat.truncateRunes = (text) => text;
  chat.updateParallelMembersHeader = () => {};
  chat.finalizeMemberMarkdown = () => {};

  chat.updateMemberStatus(entry, "running", "运行中");

  assert.equal(statusEl.textContent, "待回复");
  assert.equal(activityEl.textContent, "等待用户回答");
  assert.equal(activityEl.style.display, "");
  assert.ok(classes.has("parallel-member-running"));
});

test("member ask submission releases pending status", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const statusEl = { textContent: "" };
  const activityEl = { textContent: "", style: { display: "none" } };
  const entry = {
    el: {
      classList: {
        add() {},
        remove() {},
      },
      querySelector(selector) {
        if (selector === ".parallel-member-status") return statusEl;
        if (selector === ".parallel-member-activity") return activityEl;
        return null;
      },
    },
    panel: {},
    activity: activityEl,
    activeAskFlowKeys: ["ask:ask-1"],
    toolFlows: {
      "ask:ask-1": {
        el: { isConnected: true },
        askPending: true,
        askCompleted: true,
        askToolCompleted: false,
      },
    },
  };
  chat.truncateRunes = (text) => text;
  chat.updateParallelMembersHeader = () => {};
  chat.finalizeMemberMarkdown = () => {};

  chat.updateMemberStatus(entry, "running", "处理中");

  assert.equal(statusEl.textContent, "处理中");
  assert.equal(activityEl.textContent, "处理中");
});

test("member ask id does not claim existing unbound ask tool flow", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const oldFlow = { el: { isConnected: true }, displayName: "ask_questions" };
  const newFlow = {
    el: { isConnected: true, querySelector: (selector) => (selector === "details" ? { open: false } : null) },
    status: { textContent: "" },
    argsWrap: { style: { display: "none" } },
    args: { textContent: "" },
    resultWrap: {
      style: { display: "none" },
      querySelector(selector) {
        return selector === ".parallel-member-tool-label" ? { textContent: "" } : null;
      },
    },
    result: { style: { display: "" } },
  };
  const entry = { toolFlows: { "ref:ask-tool": oldFlow } };
  let ensuredKey = "";

  chat.memberEntryFromEvent = () => entry;
  chat.ensureMemberToolFlow = (_entry, key) => {
    ensuredKey = key;
    entry.toolFlows[key] = newFlow;
    return newFlow;
  };
  chat.showInlineAskForm = () => ({ isConnected: true });
  chat.updateMemberStatus = () => {};
  chat.updateMemberActivity = () => {};
  chat.updateMemberDetailVisibility = () => {};
  chat.scrollToBottom = () => {};

  chat.showMemberAskQuestions({
    type: "ask_questions",
    ask_id: "interrupt-1",
    member_call_id: "call-1",
    question: "Need input?",
  });

  assert.equal(ensuredKey, "ask:interrupt-1");
  assert.equal(entry.toolFlows["ask:interrupt-1"], newFlow);
  assert.equal(entry.toolFlows["ref:ask-tool"], oldFlow);
  assert.equal(newFlow.askID, "interrupt-1");
  assert.equal(entry.activeAskFlowKeys[0], "ask:interrupt-1");
});

test("member ask flow ids keep repeated questions separate", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const entry = { toolFlows: {}, timeline: { querySelectorAll: () => [], appendChild() {} } };
  const created = [];

  chat.memberEntryFromEvent = () => entry;
  chat.insertMemberTimelineItem = (_entry, item, order) => {
    item.order = order;
    item.parentElement = entry.timeline;
  };
  chat.escapeHtml = (value) => String(value || "");
  chat.updateMemberActivity = () => {};
  chat.updateMemberDetailVisibility = () => {};
  chat.updateMemberStatus = () => {};
  chat.scrollToBottom = () => {};
  chat.showInlineAskForm = () => ({ isConnected: true });

  const makeFlowEl = (key) => ({
    isConnected: true,
    getAttribute(name) {
      return name === "data-event-order" ? String(this.order ?? "") : "";
    },
    setAttribute(name, value) {
      if (name === "data-tool-flow-key") this.key = value;
      if (name === "data-event-order") this.order = value;
    },
    querySelector(selector) {
      if (selector === "details") return { open: false };
      if (selector === ".parallel-member-tool-title") return { textContent: "ask_questions" };
      if (selector === ".parallel-member-tool-status") return { textContent: "" };
      if (selector === ".parallel-member-tool-args") return { style: { display: "none" } };
      if (selector === ".parallel-member-tool-args pre") return { textContent: "" };
      if (selector === ".parallel-member-tool-result") {
        return {
          style: { display: "none" },
          querySelector(labelSelector) {
            return labelSelector === ".parallel-member-tool-label" ? { textContent: "" } : null;
          },
        };
      }
      if (selector === ".parallel-member-tool-result pre") return { textContent: "", style: { display: "" } };
      return null;
    },
  });

  const oldDocument = global.document;
  global.document = {
    createElement() {
      const key = "created-" + created.length;
      const el = makeFlowEl(key);
      created.push(el);
      return el;
    },
  };
  try {
    chat.showMemberAskQuestions({ ask_id: "ask-1", member_call_id: "call-1", question: "First?" });
    chat.showMemberAskQuestions({ ask_id: "ask-2", member_call_id: "call-1", question: "Second?" });

    assert.equal(chat.resolveMemberToolFlowKey(entry, { tool_name: "ask_questions" }, { name: "ask_questions" }), "ask:ask-1");
    chat.updateMemberToolFlowResult(entry, "ask:ask-1", "ask_questions", "{\"selected\":[\"A\"]}", false);
    assert.equal(chat.resolveMemberToolFlowKey(entry, { tool_name: "ask_questions" }, { name: "ask_questions" }), "ask:ask-2");
    assert.deepEqual(entry.activeAskFlowKeys, ["ask:ask-2"]);
  } finally {
    global.document = oldDocument;
  }
});

test("member ask form uses event sequence for timeline order", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const entry = { toolFlows: {}, activeAskFlowKeys: [] };
  let requestedOrder;
  const flow = {
    el: { querySelector: (selector) => (selector === "details" ? { open: false } : null) },
    status: { textContent: "" },
    argsWrap: { style: { display: "none" } },
    args: { textContent: "" },
    resultWrap: {
      style: { display: "none" },
      querySelector(selector) {
        return selector === ".parallel-member-tool-label" ? { textContent: "" } : null;
      },
    },
    result: { style: { display: "" } },
  };

  chat.memberEntryFromEvent = () => entry;
  chat.ensureMemberToolFlow = (_entry, _key, _displayName, order) => {
    requestedOrder = order;
    return flow;
  };
  chat.showInlineAskForm = () => ({ isConnected: true });
  chat.updateMemberActivity = () => {};
  chat.updateMemberDetailVisibility = () => {};
  chat.updateMemberStatus = () => {};
  chat.scrollToBottom = () => {};

  chat.showMemberAskQuestions({
    ask_id: "ask-with-order",
    member_call_id: "call-1",
    question: "Need input?",
    sequence: 10,
  });

  assert.equal(requestedOrder, 10);
});

test("member ask tool call anchors an early ask form to tool sequence", () => {
  const chat = Object.create(FKTeamsChat.prototype);
  const entry = { toolFlows: {}, activeAskFlowKeys: [] };
  const flows = {};

  const makeFlow = () => ({
    el: {
      isConnected: true,
      getAttribute(name) {
        return name === "data-event-order" ? String(this.order ?? "") : "";
      },
      setAttribute(name, value) {
        if (name === "data-event-order") this.order = Number(value);
      },
      querySelector(selector) {
        return selector === "details" ? { open: false } : null;
      },
    },
    status: { textContent: "" },
    argsWrap: { style: { display: "none" } },
    args: { textContent: "" },
    resultWrap: {
      style: { display: "none" },
      querySelector(selector) {
        return selector === ".parallel-member-tool-label" ? { textContent: "" } : null;
      },
    },
    result: { style: { display: "" } },
  });

  chat.memberEntryFromEvent = () => entry;
  chat.ensureMemberToolFlow = (_entry, key, _displayName, order) => {
    if (!flows[key]) {
      flows[key] = makeFlow();
      entry.toolFlows[key] = flows[key];
    }
    if (order !== undefined) flows[key].order = order;
    return flows[key];
  };
  chat.insertMemberTimelineItem = (_entry, item, order) => {
    item.order = Number(order);
  };
  chat.getToolDisplay = (toolCall) => ({ displayName: toolCall.name, kind: "tool" });
  chat.showInlineAskForm = () => ({ isConnected: true });
  chat.updateMemberActivity = () => {};
  chat.updateMemberDetailVisibility = () => {};
  chat.updateMemberStatus = () => {};
  chat.scrollToBottom = () => {};

  chat.showMemberAskQuestions({
    ask_id: "ask-1",
    tool_call_id: "ask-call-1",
    tool_call_ref: "tool_call:ask-call-1",
    member_call_id: "member-call-1",
    question: "Need input?",
    sequence: 5,
  });

  const key = "ref:tool_call:ask-call-1";
  assert.equal(flows[key].order, 5);
  assert.equal(flows[key].provisionalOrder, true);

  chat.handleToolCalls({
    type: "message_end",
    member_call_id: "member-call-1",
    sequence: 20,
    tool_calls: [{
      id: "ask-call-1",
      ref: "tool_call:ask-call-1",
      name: "ask_questions",
    }],
  });

  assert.equal(flows[key].order, 20);
  assert.equal(flows[key].provisionalOrder, false);
  assert.deepEqual(Object.keys(entry.toolFlows), [key]);
  assert.deepEqual(entry.activeAskFlowKeys, [key]);
});
