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
