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
