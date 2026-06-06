/**
 * messages.js - 消息处理与渲染
 */

FKTeamsChat.prototype.getToolDisplay = function (toolCall) {
  const name = toolCall?.name || "";
  if (toolCall?.display_name) {
    return {
      name,
      displayName: toolCall.display_name,
      kind: toolCall.kind || "tool",
      target: toolCall.target || "",
    };
  }
  return { name, displayName: name, kind: "tool", target: "" };
};

FKTeamsChat.prototype.normalizeToolCallForEvent = function (event, toolCall, fallbackIndex) {
  const source = toolCall || {};
  const index = source.index !== undefined && source.index !== null
    ? source.index
    : event?.tool_call_index !== undefined && event?.tool_call_index !== null
      ? event.tool_call_index
      : event?.detail !== undefined && event?.detail !== null
        ? event.detail
        : fallbackIndex;
  return {
    id: source.id || event?.tool_call_id || "",
    ref: source.ref || event?.tool_call_ref || "",
    index,
    name: source.name || event?.tool_name || "",
    display_name: source.display_name || event?.tool_display_name || "",
    kind: source.kind || event?.tool_kind || "",
    target: source.target || event?.tool_target || "",
    arguments: source.arguments || event?.tool_args || (event?.delta_kind === "tool_args" ? event?.delta || event?.content || "" : ""),
  };
};

FKTeamsChat.prototype.normalizeToolCallsForEvent = function (event) {
  const rawCalls = [];
  if (Array.isArray(event?.tool_calls) && event.tool_calls.length > 0) {
    rawCalls.push(...event.tool_calls);
  } else if (event?.tool_call) {
    rawCalls.push(event.tool_call);
  }
  if (rawCalls.length === 0 && (event?.tool_name || event?.tool_call_id || event?.tool_call_ref || event?.tool_call_index !== undefined)) {
    rawCalls.push(null);
  }
  return rawCalls.map((toolCall, i) => this.normalizeToolCallForEvent(event, toolCall, i));
};

FKTeamsChat.prototype.getStreamKey = function (event) {
  return [event.agent_name || "", event.run_path || ""].join("|");
};

FKTeamsChat.prototype.resetParallelState = function () {
  this.currentMessageElement = null;
  this.currentMessageElements = {};
  this.pendingToolCalls = {};
  this.toolCallsByID = {};
  this.toolCallsByIndex = {};
  this.parallelPanel = null;
  this.parallelMemberCards = {};
  this.parallelMemberByAgent = {};
  this.parallelToolMemberByID = {};
  this.parallelMemberResultChunks = {};
  this.parallelMemberInnerResultChunks = {};
  this.parallelMemberToolFlows = {};
  this.parallelPanelBatchMode = false;
  this.lastToolName = "";
};

FKTeamsChat.prototype.toolCallKey = function (toolCall, fallbackIndex) {
  if (toolCall?.ref) return "ref:" + toolCall.ref;
  if (toolCall?.id) return "id:" + toolCall.id;
  if (toolCall?.index !== undefined && toolCall?.index !== null) return "idx:" + toolCall.index;
  return "fallback:" + fallbackIndex;
};

FKTeamsChat.prototype.findToolCallCard = function (key) {
  const cards = this.messagesContainer.querySelectorAll(".tool-call");
  for (const card of cards) {
    if (card.getAttribute("data-tool-key") === key) return card;
  }
  return null;
};

FKTeamsChat.prototype.findToolCallCardByIdentity = function (event, toolCall) {
  const keys = [];
  if (event?.tool_call_ref) keys.push("ref:" + event.tool_call_ref);
  if (toolCall?.ref) keys.push("ref:" + toolCall.ref);
  if (event?.tool_call_id) keys.push("id:" + event.tool_call_id);
  if (toolCall?.id) keys.push("id:" + toolCall.id);
  if (event?.tool_call_index !== undefined && event?.tool_call_index !== null) keys.push("idx:" + event.tool_call_index);
  if (toolCall?.index !== undefined && toolCall?.index !== null) keys.push("idx:" + toolCall.index);
  for (const key of keys) {
    const card = this.findToolCallCard(key);
    if (card) return card;
  }
  const cards = this.messagesContainer.querySelectorAll(".tool-call");
  for (const card of cards) {
    if (event?.tool_call_id && card.getAttribute("data-tool-call-id") === event.tool_call_id) return card;
    if (toolCall?.id && card.getAttribute("data-tool-call-id") === toolCall.id) return card;
    const eventIndex = event?.tool_call_index;
    const toolIndex = toolCall?.index;
    if (eventIndex !== undefined && eventIndex !== null && card.getAttribute("data-tool-index") === String(eventIndex)) return card;
    if (toolIndex !== undefined && toolIndex !== null && card.getAttribute("data-tool-index") === String(toolIndex)) return card;
  }
  return null;
};

FKTeamsChat.prototype.appendToolResultToCard = function (card, content, toolDisplay) {
  if (!card) return false;
  const detail = card.querySelector(".tool-call-detail") || card;
  let resultEl = card.querySelector(".tool-call-result");
  if (!resultEl) {
    resultEl = document.createElement("div");
    resultEl.className = "tool-call-result";
    resultEl.innerHTML = `
      <div class="tool-call-result-header">${this.escapeHtml(toolDisplay?.kind === "agent" ? "成员结果" : "执行结果")}</div>
      <pre class="tool-result-content"></pre>
    `;
    detail.appendChild(resultEl);
  }
  this.updateToolCallStatus(card, "已完成");
  const pre = resultEl.querySelector(".tool-result-content");
  if (!pre) return true;
  let formattedResult = content || "";
  try {
    const parsed = JSON.parse(formattedResult);
    formattedResult = JSON.stringify(parsed, null, 2);
  } catch {
    /* keep original text */
  }
  pre.textContent = formattedResult;
  return true;
};

FKTeamsChat.prototype.isAgentToolErrorResult = function (content) {
  const text = String(content || "").trim();
  if (!text) return false;
  return /^\[执行出错\]/.test(text) ||
    text.includes("执行任务时遇到错误") ||
    text.includes("[NodeRunError]") ||
    text.includes("[GraphRunError]");
};

FKTeamsChat.prototype.updateToolCallStatus = function (card, status) {
  const statusEl = card?.querySelector(".tool-call-status");
  if (statusEl) statusEl.textContent = status;
};

FKTeamsChat.prototype.bindToolCallToggle = function (card) {
  if (!card || card._toolToggleBound) return;
  card._toolToggleBound = true;
  card.addEventListener("click", (e) => {
    if (e.target.closest("a, button, pre, code")) return;
    const detail = card.querySelector(".tool-call-detail");
    if (!detail) return;
    card.classList.toggle("tool-call-expanded");
    detail.style.display = card.classList.contains("tool-call-expanded") ? "" : "none";
  });
};

FKTeamsChat.prototype.formatToolArgsForDisplay = function (argsText, emptyText) {
  if (!argsText) return emptyText || "无参数";
  try {
    return JSON.stringify(JSON.parse(argsText), null, 2);
  } catch {
    return argsText;
  }
};

FKTeamsChat.prototype.createToolCallCard = function (toolCall, key, toolDisplay, status, argsText) {
  const card = document.createElement("div");
  const isAgentTool = toolDisplay?.kind === "agent";
  card.className = "tool-call" + (isAgentTool ? " agent-tool-call" : "");
  card.setAttribute("data-tool-key", key);
  if (toolCall?.id) card.setAttribute("data-tool-call-id", toolCall.id);
  if (toolCall?.index !== undefined && toolCall.index !== null) card.setAttribute("data-tool-index", toolCall.index);
  card.innerHTML = `
      <div class="tool-call-header">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="3"/>
              <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>
          </svg>
          <span>${isAgentTool ? "成员指派:" : "工具调用:"}</span>
          <code class="tool-call-name">${this.escapeHtml(toolDisplay?.displayName || toolCall?.name || "tool")}</code>
          <span class="tool-call-status">${this.escapeHtml(status || (isAgentTool ? "指派中" : "参数准备中"))}</span>
      </div>
      <div class="tool-call-detail" style="display:none">
        <pre class="tool-call-args">${this.escapeHtml(argsText || (isAgentTool ? "等待成员开始执行..." : "参数准备中..."))}</pre>
      </div>
  `;
  this.bindToolCallToggle(card);
  return card;
};

FKTeamsChat.prototype.ensureToolCallCard = function (toolCall, key, toolDisplay, status, argsText) {
  let pending = this.pendingToolCalls[key];
  let card = pending?.el || this.findToolCallCard(key);
  if (!card) {
    card = this.createToolCallCard(toolCall, key, toolDisplay, status, argsText);
    this.pendingToolCalls[key] = { el: card, toolCall };
    this.messagesContainer.appendChild(card);
  }
  if (toolCall?.id) this.toolCallsByID[toolCall.id] = toolCall;
  if (toolCall?.index !== undefined && toolCall.index !== null) this.toolCallsByIndex[String(toolCall.index)] = toolCall;
  return card;
};

FKTeamsChat.prototype.migrateToolCallCard = function (fromKey, toKey, toolCall) {
  if (!fromKey || !toKey || fromKey === toKey) return this.pendingToolCalls[toKey]?.el || this.findToolCallCard(toKey);
  const existing = this.pendingToolCalls[toKey]?.el || this.findToolCallCard(toKey);
  const pending = this.pendingToolCalls[fromKey];
  const card = pending?.el || this.findToolCallCard(fromKey);
  if (!card) return existing;
  if (existing && existing !== card) {
    const existingArgs = existing.querySelector(".tool-call-args");
    const cardArgs = card.querySelector(".tool-call-args");
    if (existingArgs && cardArgs && (!existingArgs.textContent || existingArgs.textContent === "参数准备中..." || existingArgs.textContent === "等待成员开始执行...")) {
      existingArgs.textContent = cardArgs.textContent;
    }
    card.remove();
    delete this.pendingToolCalls[fromKey];
    return existing;
  }
  this.pendingToolCalls[toKey] = pending || { el: card, toolCall };
  delete this.pendingToolCalls[fromKey];
  card.setAttribute("data-tool-key", toKey);
  if (toolCall?.id) card.setAttribute("data-tool-call-id", toolCall.id);
  return card;
};

FKTeamsChat.prototype.memberKeyForToolCall = function (toolCall, fallbackIndex) {
  if (toolCall?.id) return "call:" + toolCall.id;
  if (toolCall?.index !== undefined && toolCall?.index !== null) return "pending:" + toolCall.index;
  return "pending:" + fallbackIndex;
};

FKTeamsChat.prototype.migrateMemberCard = function (fromKey, toKey) {
  if (!fromKey || !toKey || fromKey === toKey) return;
  const existing = this.parallelMemberCards[toKey];
  const pending = this.parallelMemberCards[fromKey];
  if (!pending) return;
  if (!existing && this.isStalePendingMemberEntry(fromKey, pending)) {
    if (this.parallelPanel === pending.panel) this.parallelPanel = null;
    delete this.parallelMemberCards[fromKey];
    return;
  }
  if (existing) {
    if (pending.timeline && existing.timeline) {
      while (pending.timeline.firstChild) existing.timeline.appendChild(pending.timeline.firstChild);
    }
    existing.toolFlows = { ...(existing.toolFlows || {}), ...(pending.toolFlows || {}) };
    if ((pending.taskRaw || "") && !(existing.taskRaw || "")) {
      this.updateMemberTaskContent(existing, pending.taskRaw, false);
    }
    if (pending.el?.classList.contains("parallel-member-error")) {
      this.updateMemberStatus(existing, "error", "失败");
    } else if (pending.el?.classList.contains("parallel-member-done")) {
      this.updateMemberStatus(existing, "done", "完成");
    }
    if (pending.el) pending.el.remove();
    delete this.parallelMemberCards[fromKey];
    this.updateMemberDetailVisibility(existing);
    this.updateParallelMembersHeader(existing.panel);
    return;
  }
  this.parallelMemberCards[toKey] = pending;
  delete this.parallelMemberCards[fromKey];
  if (pending.el) pending.el.setAttribute("data-member-key", toKey);
};

FKTeamsChat.prototype.hasActiveTextSelection = function () {
  const selection = window.getSelection ? window.getSelection() : null;
  return !!(selection && !selection.isCollapsed && String(selection).trim());
};

FKTeamsChat.prototype.getMessageElementForEvent = function (event) {
  const key = this.getStreamKey(event);
  if (this.hasToolCallAfterMessage) {
    this.currentMessageElements = {};
    this.hasToolCallAfterMessage = false;
  }
  if (!this.currentMessageElements[key]) {
    this.currentMessageElements[key] = this.createAssistantMessage(event.agent_name);
  }
  this.currentMessageElement = this.currentMessageElements[key];
  return this.currentMessageElement;
};

FKTeamsChat.prototype.normalizedAgentName = function (name) {
  return (name || "").toLowerCase().replace(/^ask_fkagent_/, "").replace(/[^a-z0-9_-]+/g, "_");
};

FKTeamsChat.prototype.agentNameFromTool = function (name) {
  return this.normalizedAgentName((name || "").startsWith("ask_fkagent_") ? name.slice(12) : name);
};

FKTeamsChat.prototype.memberCallIDFromEvent = function (event) {
  return event?.member_call_id || event?.parent_tool_call_id || "";
};

FKTeamsChat.prototype.isMemberRunEvent = function (event) {
  return !!(event && (event.is_member_event || this.memberCallIDFromEvent(event)));
};

FKTeamsChat.prototype.memberKeyFromEvent = function (event) {
  return "call:" + (this.memberCallIDFromEvent(event) || event.tool_call_id || event.tool_call_ref || event.message_id || "unknown");
};

FKTeamsChat.prototype.memberEntryFromEvent = function (event) {
  const key = this.memberKeyFromEvent(event);
  const label = event.member_name || event.agent_name || event.member_tool_name;
  return this.ensureMemberCard(key, label, event.member_name || event.agent_name || event.member_tool_name, event.member_order);
};

FKTeamsChat.prototype.memberInnerResultKey = function (event) {
  const memberCallID = this.memberCallIDFromEvent(event);
  if (event.tool_call_ref) return [memberCallID, event.tool_call_ref].join(":");
  return [memberCallID, event.tool_call_id || event.tool_call_index || ""].join(":");
};

FKTeamsChat.prototype.parallelEntriesForPanel = function (panel) {
  if (!panel) return [];
  return Object.values(this.parallelMemberCards || {}).filter((entry) => entry?.el?.isConnected && panel.contains(entry.el));
};

FKTeamsChat.prototype.parallelPanelCompleted = function (panel) {
  const entries = this.parallelEntriesForPanel(panel);
  if (entries.length === 0) return false;
  return entries.every((entry) => this.memberEntryTerminal(entry));
};

FKTeamsChat.prototype.memberEntryTerminal = function (entry) {
  return !!(
    entry?.el?.classList.contains("parallel-member-done") ||
    entry?.el?.classList.contains("parallel-member-error") ||
    entry?.el?.classList.contains("parallel-member-cancelled")
  );
};

FKTeamsChat.prototype.isStalePendingMemberEntry = function (key, entry) {
  if (!entry || !entry.el) return false;
  const isPendingKey = key.startsWith("pending:") || key.startsWith("fallback:");
  if (!isPendingKey) return false;
  if (!entry.panel) entry.panel = entry.el.closest(".parallel-members-panel");
  return this.parallelPanelCompleted(entry.panel);
};

FKTeamsChat.prototype.registerMemberToolFlowAlias = function (entry, toolName, key) {
  if (!entry || !toolName || !key) return;
  if (!entry.toolFlowKeyByName) entry.toolFlowKeyByName = {};
  entry.toolFlowKeyByName[toolName] = key;
};

FKTeamsChat.prototype.resolveMemberToolFlowKey = function (entry, event, toolCall, fallbackIndex) {
  const finalKey = this.memberToolFlowKey(event, toolCall, fallbackIndex);
  const aliases = [];
  const pushAlias = (key) => {
    if (key && key !== finalKey && !aliases.includes(key)) aliases.push(key);
  };

  if (toolCall?.id) pushAlias("id:" + toolCall.id);
  if (event?.tool_call_id) pushAlias("id:" + event.tool_call_id);
  if (toolCall?.index !== undefined && toolCall?.index !== null) pushAlias("idx:" + toolCall.index);
  if (event?.tool_call_index !== undefined && event?.tool_call_index !== null) pushAlias("idx:" + event.tool_call_index);
  if (fallbackIndex !== undefined && fallbackIndex !== null) pushAlias("fallback:" + fallbackIndex);
  if (event?.detail !== undefined && event?.detail !== null) pushAlias("idx:" + event.detail);
  const toolName = toolCall?.name || event?.tool_name || "";
  const aliasKey = toolName && entry?.toolFlowKeyByName ? entry.toolFlowKeyByName[toolName] : "";
  pushAlias(aliasKey);

  aliases.forEach((key) => this.migrateMemberToolFlow(entry, key, finalKey));
  return finalKey;
};

FKTeamsChat.prototype.truncateRunes = function (text, maxLen) {
  const chars = Array.from(String(text || ""));
  if (chars.length <= maxLen) return String(text || "");
  return chars.slice(0, maxLen).join("") + "...";
};

FKTeamsChat.prototype.ensureParallelPanel = function () {
  if (
    this.parallelPanel &&
    this.parallelPanel.isConnected &&
    (this.parallelPanelBatchMode || !this.parallelPanelCompleted(this.parallelPanel))
  ) {
    return this.parallelPanel;
  }
  const panel = document.createElement("div");
  panel.className = "parallel-members-panel";
  panel.innerHTML = `
    <div class="dispatch-header dispatch-status-partial parallel-members-header">
      <span class="parallel-members-title">成员任务处理中</span>
      <span class="dispatch-progress-counter parallel-members-count" data-total="0" data-done="0">0/0</span>
    </div>
    <div class="dispatch-cards parallel-members-list"></div>
  `;
  this.parallelPanel = panel;
  this.messagesContainer.appendChild(panel);
  return panel;
};

FKTeamsChat.prototype.insertMemberCardByOrder = function (list, card, order) {
  const cardOrder = Number.isFinite(order) ? order : Number.MAX_SAFE_INTEGER;
  card.setAttribute("data-member-order", String(cardOrder));
  const cards = Array.from(list.querySelectorAll(".parallel-member-card"));
  const before = cards.find((el) => {
    if (el === card) return false;
    const existing = Number(el.getAttribute("data-member-order") || Number.MAX_SAFE_INTEGER);
    return existing > cardOrder;
  });
  if (before) {
    if (card.nextElementSibling !== before) list.insertBefore(card, before);
    return;
  }
  if (card.parentElement !== list) {
    list.appendChild(card);
  }
};

FKTeamsChat.prototype.ensureMemberCard = function (key, label, agentName, order) {
  const existing = this.parallelMemberCards[key];
  if (existing && existing.el && existing.el.isConnected) {
    if (this.isStalePendingMemberEntry(key, existing)) {
      if (this.parallelPanel === existing.panel) this.parallelPanel = null;
      delete this.parallelMemberCards[key];
    } else {
      if (order !== undefined && order !== null) {
        const list = existing.el.parentElement;
        if (list) this.insertMemberCardByOrder(list, existing.el, order);
      }
      return existing;
    }
  }

  const panel = this.ensureParallelPanel();
  const list = panel.querySelector(".parallel-members-list");
  const card = document.createElement("div");
  card.className = "dispatch-card dispatch-card-running parallel-member-card parallel-member-running";
  card.setAttribute("data-member-key", key);
  card.innerHTML = `
    <div class="dispatch-card-head parallel-member-head">
      <span class="dispatch-card-status-dot parallel-member-dot"></span>
      <span class="dispatch-card-desc parallel-member-name">${this.escapeHtml(label || agentName || "Member")}</span>
      <span class="parallel-member-task-content" style="display:none">
        <span class="parallel-member-task-text"></span>
      </span>
      <span class="parallel-member-activity"></span>
      <span class="dispatch-card-ops-count parallel-member-status">运行中</span>
      <span class="dispatch-card-toggle parallel-member-toggle"></span>
    </div>
    <div class="dispatch-card-detail parallel-member-detail" style="display:none">
      <div class="parallel-member-task-detail" style="display:none"></div>
      <div class="parallel-member-timeline"></div>
    </div>
  `;
  card.addEventListener("click", (e) => {
    if (e.target.closest("a, button, summary, pre, code, details, .sources-card, .source-item, .footnote-cite, .parallel-member-tool-flow")) return;
    if (this.hasActiveTextSelection()) return;
    if (!e.target.closest(".parallel-member-head")) return;
    const detail = card.querySelector(".parallel-member-detail");
    const currentKey = card.getAttribute("data-member-key") || key;
    const currentEntry = this.parallelMemberCards[currentKey] || card._memberEntry;
    const hasDetail = this.memberHasDetail(currentEntry);
    if (!detail || !hasDetail) return;
    card.classList.toggle("dispatch-card-expanded");
    detail.style.display = card.classList.contains("dispatch-card-expanded") ? "" : "none";
  });
  this.insertMemberCardByOrder(list, card, order);
  const entry = {
    el: card,
    panel,
    task: card.querySelector(".parallel-member-task-content"),
    activity: card.querySelector(".parallel-member-activity"),
    taskDetail: card.querySelector(".parallel-member-task-detail"),
    timeline: card.querySelector(".parallel-member-timeline"),
    detail: card.querySelector(".parallel-member-detail"),
  };
  card._memberEntry = entry;
  this.parallelMemberCards[key] = entry;
  if (agentName) this.parallelMemberByAgent[this.normalizedAgentName(agentName)] = key;
  this.updateMemberActivity(entry, "等待开始");
  this.updateParallelMembersHeader(panel);
  return entry;
};

FKTeamsChat.prototype.updateMemberActivity = function (entry, text) {
  if (!entry || !entry.el) return;
  if (!entry.activity) entry.activity = entry.el.querySelector(".parallel-member-activity");
  if (!entry.activity) return;
  const value = String(text || "").trim();
  if (!value) {
    entry.activity.textContent = "";
    entry.activity.style.display = "none";
    return;
  }
  entry.activity.textContent = this.truncateRunes(value, 80);
  entry.activity.style.display = "";
};

FKTeamsChat.prototype.updateMemberStatus = function (entry, status, text) {
  if (!entry || !entry.el) return;
  entry.el.classList.remove(
    "parallel-member-running",
    "parallel-member-done",
    "parallel-member-error",
    "parallel-member-cancelled",
    "dispatch-card-running",
    "dispatch-card-done",
    "dispatch-card-fail",
    "dispatch-card-cancelled",
  );
  entry.el.classList.add("parallel-member-" + status);
  entry.el.classList.add(
    status === "error"
      ? "dispatch-card-fail"
      : status === "done"
        ? "dispatch-card-done"
        : status === "cancelled"
          ? "dispatch-card-cancelled"
          : "dispatch-card-running",
  );
  const statusEl = entry.el.querySelector(".parallel-member-status");
  if (statusEl) statusEl.textContent = text;
  if (status === "done" || status === "error" || status === "cancelled") {
    this.updateMemberActivity(entry, "");
    this.finalizeMemberMarkdown(entry);
  } else if (text) {
    this.updateMemberActivity(entry, text);
  }
  this.updateParallelMembersHeader(entry.panel);
};

FKTeamsChat.prototype.finalizeMemberMarkdown = function (entry) {
  if (!entry) return;
  entry.el?.querySelectorAll(".parallel-member-markdown[data-raw]").forEach((body) => {
    const raw = body.getAttribute("data-raw") || "";
    body._memberRenderStreaming = false;
    body._memberRenderDirty = false;
    body._memberRenderPending = false;
    body.removeAttribute("data-streaming");
    body.innerHTML = raw ? this.renderMarkdown(raw, false) : "";
  });
};

FKTeamsChat.prototype.memberHasDetail = function (entry) {
  if (!entry) return false;
  return !!((entry.taskText && entry.taskText.trim()) || (entry.timeline && entry.timeline.childElementCount > 0));
};

FKTeamsChat.prototype.updateMemberDetailVisibility = function (entry) {
  if (!entry || !entry.el || !entry.detail) return;
  const hasDetail = this.memberHasDetail(entry);
  const head = entry.el.querySelector(".parallel-member-head");
  entry.el.style.cursor = "";
  if (head) head.style.cursor = hasDetail ? "pointer" : "";
  if (!hasDetail) {
    entry.el.classList.remove("dispatch-card-expanded");
    entry.detail.style.display = "none";
  }
};

FKTeamsChat.prototype.updateParallelMembersHeader = function (panel) {
  const targetPanel = panel || this.parallelPanel;
  if (!targetPanel) return;
  const entries = this.parallelEntriesForPanel(targetPanel);
  const total = entries.length;
  const done = entries.filter((entry) => this.memberEntryTerminal(entry)).length;
  const cancelled = entries.filter((entry) => entry?.el?.classList.contains("parallel-member-cancelled")).length;
  const counter = targetPanel.querySelector(".parallel-members-count");
  if (counter) {
    counter.dataset.total = String(total);
    counter.dataset.done = String(done);
    counter.textContent = `${done}/${total}`;
  }
  const title = targetPanel.querySelector(".parallel-members-title");
  if (title) {
    const completed = total > 0 && done === total;
    const allCancelled = completed && cancelled === total;
    if (total > 1) {
      title.textContent = allCancelled ? "成员并行已取消" : completed ? "成员并行完成" : "成员并行处理中";
    } else {
      title.textContent = allCancelled ? "成员任务已取消" : completed ? "成员任务完成" : "成员任务处理中";
    }
  }
  const header = targetPanel.querySelector(".parallel-members-header");
  if (header) {
    header.classList.toggle("dispatch-status-ok", total > 0 && done === total);
    header.classList.toggle("dispatch-status-partial", !(total > 0 && done === total));
  }
};

FKTeamsChat.prototype.cancelActiveMemberCards = function () {
  const panels = new Set();
  Object.values(this.parallelMemberCards || {}).forEach((entry) => {
    if (!entry || !entry.el || !entry.el.isConnected || this.memberEntryTerminal(entry)) return;
    this.updateMemberStatus(entry, "cancelled", "已取消");
    panels.add(entry.panel || entry.el.closest(".parallel-members-panel"));
  });
  panels.forEach((panel) => this.updateParallelMembersHeader(panel));
};

FKTeamsChat.prototype.memberTimelineOrder = function (order) {
  const value = Number(order);
  return Number.isFinite(value) ? value : Number.MAX_SAFE_INTEGER;
};

FKTeamsChat.prototype.insertMemberTimelineItem = function (entry, item, order) {
  if (!entry || !entry.timeline || !item) return;
  const itemOrder = this.memberTimelineOrder(order);
  item.setAttribute("data-event-order", String(itemOrder));
  const items = Array.from(entry.timeline.querySelectorAll(".parallel-member-event"));
  const before = items.find((el) => {
    if (el === item) return false;
    const existing = this.memberTimelineOrder(el.getAttribute("data-event-order"));
    return existing > itemOrder;
  });
  if (before) {
    if (item.nextElementSibling !== before) entry.timeline.insertBefore(item, before);
    return;
  }
  if (item.parentElement !== entry.timeline) entry.timeline.appendChild(item);
};

FKTeamsChat.prototype.ensureMemberTimelineItem = function (entry, type, title, streaming, streamID, order) {
  if (!entry || !entry.timeline) return null;
  if (streamID) {
    if (!entry.streamItems) entry.streamItems = {};
    const existing = entry.streamItems[streamID];
    if (existing && existing.isConnected) {
      this.insertMemberTimelineItem(entry, existing, existing.getAttribute("data-event-order"));
      return existing;
    }
  }
  if (streaming && entry.lastTimelineType === type && entry.currentTimelineItem) {
    this.insertMemberTimelineItem(entry, entry.currentTimelineItem, entry.currentTimelineItem.getAttribute("data-event-order"));
    return entry.currentTimelineItem;
  }
  const item = document.createElement("div");
  item.className = "parallel-member-event parallel-member-event-" + type;
  if (streamID) item.setAttribute("data-stream-id", streamID);
  item.innerHTML = `
    <div class="parallel-member-event-title">${this.escapeHtml(title)}</div>
    <div class="parallel-member-event-body"></div>
  `;
  this.insertMemberTimelineItem(entry, item, order);
  if (streamID) {
    if (!entry.streamItems) entry.streamItems = {};
    entry.streamItems[streamID] = item;
  } else {
    entry.lastTimelineType = type;
    entry.currentTimelineItem = item;
  }
  this.updateMemberDetailVisibility(entry);
  return item;
};

FKTeamsChat.prototype.memberActivityText = function (type, title) {
  if (type === "reasoning") return "正在思考";
  if (type === "output") return "输出中";
  if (type === "tool") return title || "工具执行中";
  if (type === "task") return "任务已分配";
  if (type === "result") return title || "收到结果";
  if (type === "action") return title || "状态更新";
  return title || "";
};

FKTeamsChat.prototype.appendMemberMarkdownEvent = function (entry, type, title, content, streaming, streamID, chunkIndex, order) {
  if (!entry || !content) return;
  const item = this.ensureMemberTimelineItem(entry, type, title, streaming, streamID, order);
  if (!item) return;
  this.updateMemberActivity(entry, this.memberActivityText(type, title));
  if (streamID && chunkIndex !== undefined && chunkIndex !== null) {
    if (!entry.streamChunkIndexes) entry.streamChunkIndexes = {};
    const last = entry.streamChunkIndexes[streamID];
    if (last !== undefined && chunkIndex <= last) return;
    entry.streamChunkIndexes[streamID] = chunkIndex;
  }
  const body = item.querySelector(".parallel-member-event-body");
  if (!body) return;
  body.classList.add("parallel-member-markdown", "markdown-body", "markdown-body-compact");
  let raw = body.getAttribute("data-raw") || "";
  if (!streaming && raw && content && (raw === content || content.includes(raw))) {
    raw = content.length >= raw.length ? content : raw;
    body.setAttribute("data-raw", raw);
    this.scheduleMemberMarkdownRender(body, !!streaming);
    return;
  }
  raw += content;
  body.setAttribute("data-raw", raw);
  this.scheduleMemberMarkdownRender(body, !!streaming);
  this.updateMemberDetailVisibility(entry);
};

FKTeamsChat.prototype.scheduleMemberMarkdownRender = function (body, streaming) {
  if (!body) return;
  body._memberRenderStreaming = !!streaming;
  if (body._memberRenderPending) {
    body._memberRenderDirty = true;
    return;
  }
  body._memberRenderPending = true;
  const schedule = window.requestAnimationFrame || ((fn) => window.setTimeout(fn, 16));
  schedule(() => {
    if (!body.isConnected) return;
    body._memberRenderPending = false;
    const raw = body.getAttribute("data-raw") || "";
    if (body._memberRenderStreaming) {
      body.setAttribute("data-streaming", "1");
      body.textContent = raw;
    } else {
      body.removeAttribute("data-streaming");
      body.innerHTML = raw ? this.renderMarkdown(raw, false) : "";
    }
    if (body._memberRenderDirty) {
      body._memberRenderDirty = false;
      this.scheduleMemberMarkdownRender(body, body._memberRenderStreaming);
    }
  });
};

FKTeamsChat.prototype.appendMemberOutput = function (entry, content) {
  this.appendMemberMarkdownEvent(entry, "output", "输出", content, true);
};

FKTeamsChat.prototype.appendMemberReasoning = function (entry, content) {
  this.appendMemberMarkdownEvent(entry, "reasoning", "思考过程", content, true);
};

FKTeamsChat.prototype.appendMemberOutputFinal = function (entry, content, order) {
  this.appendMemberMarkdownEvent(entry, "output", "输出", content, false, "", null, order);
};

FKTeamsChat.prototype.appendMemberReasoningFinal = function (entry, content, order) {
  this.appendMemberMarkdownEvent(entry, "reasoning", "思考过程", content, false, "", null, order);
};

FKTeamsChat.prototype.setMemberFinalReasoning = function (entry, content, order) {
  if (!entry || !content) return;
  const bodies = Array.from(entry.el?.querySelectorAll(".parallel-member-event-reasoning .parallel-member-markdown") || []);
  const body = bodies.reduce((latest, current) => {
    if (!latest) return current;
    const latestItem = latest.closest(".parallel-member-event");
    const currentItem = current.closest(".parallel-member-event");
    const latestOrder = this.memberTimelineOrder(latestItem?.getAttribute("data-event-order"));
    const currentOrder = this.memberTimelineOrder(currentItem?.getAttribute("data-event-order"));
    return currentOrder >= latestOrder ? current : latest;
  }, null);
  if (!body) {
    this.appendMemberReasoningFinal(entry, content, order);
    return;
  }
  const existingRaw = body.getAttribute("data-raw") || "";
  let finalContent = content;
  if (existingRaw && content && !content.includes(existingRaw) && !existingRaw.includes(content)) {
    finalContent = existingRaw + content;
  } else if (existingRaw && existingRaw.length > content.length && existingRaw.includes(content)) {
    finalContent = existingRaw;
  }
  body.setAttribute("data-raw", finalContent);
  body._memberRenderStreaming = false;
  body._memberRenderDirty = false;
  body._memberRenderPending = false;
  body.removeAttribute("data-streaming");
  body.innerHTML = this.renderMarkdown(finalContent, false);
};

FKTeamsChat.prototype.appendMemberStreamEvent = function (entry, event, type, title) {
  const memberCallID = this.memberCallIDFromEvent(event);
  const streamID = event.stream_id || (event.message_id ? [memberCallID, event.message_id, type].filter(Boolean).join(":") : "");
  this.appendMemberMarkdownEvent(
    entry,
    type,
    title,
    event.content || "",
    true,
    streamID,
    event.chunk_index,
    event.sequence,
  );
};

FKTeamsChat.prototype.summarizeMemberToolResult = function (content) {
  if (!content) return "工具返回空结果";
  let text = content;
  try {
    const parsed = JSON.parse(content);
    if (parsed.error_message) return "工具执行失败: " + parsed.error_message;
    if (parsed.status_code) {
      const type = parsed.content_type ? `，${parsed.content_type}` : "";
      const size = parsed.content ? `，${String(parsed.content).length} 字符` : "";
      return `工具返回 HTTP ${parsed.status_code}${type}${size}`;
    }
    if (parsed.message) return String(parsed.message);
    text = JSON.stringify(parsed);
  } catch {
    /* keep original text */
  }
  text = String(text).replace(/\s+/g, " ").trim();
  if (text.length > 180) text = text.slice(0, 180) + "...";
  return text || "工具返回结果";
};

FKTeamsChat.prototype.appendMemberOp = function (entry, text) {
  this.appendMemberTextEvent(entry, "tool", "工具事件", text);
};

FKTeamsChat.prototype.appendMemberTextEvent = function (entry, type, title, text, order) {
  if (!entry || !entry.timeline || !text) return;
  const item = this.ensureMemberTimelineItem(entry, type, title, false, "", order);
  if (!item) return;
  const body = item.querySelector(".parallel-member-event-body");
  if (body) body.textContent = text;
  this.updateMemberActivity(entry, this.memberActivityText(type, title));
  entry.lastTimelineType = type + ":" + Date.now() + ":" + Math.random();
  entry.currentTimelineItem = null;
  this.updateMemberDetailVisibility(entry);
};

FKTeamsChat.prototype.appendMemberToolResult = function (entry, content) {
  this.appendMemberTextEvent(entry, "result", "工具结果", content);
};

FKTeamsChat.prototype.memberToolFlowKey = function (event, toolCall, fallbackIndex) {
  if (toolCall?.ref) return "ref:" + toolCall.ref;
  if (event?.tool_call_ref) return "ref:" + event.tool_call_ref;
  if (toolCall?.id) return "id:" + toolCall.id;
  if (event?.tool_call_id) return "id:" + event.tool_call_id;
  if (toolCall?.index !== undefined && toolCall?.index !== null) return "idx:" + toolCall.index;
  if (event?.tool_call_index !== undefined && event?.tool_call_index !== null) return "idx:" + event.tool_call_index;
  return "fallback:" + (fallbackIndex || 0);
};

FKTeamsChat.prototype.ensureMemberToolFlow = function (entry, key, displayName, order) {
  if (!entry || !entry.timeline || !key) return null;
  if (!entry.toolFlows) entry.toolFlows = {};
  const existing = entry.toolFlows[key];
  if (existing && existing.el && existing.el.isConnected) {
    const title = existing.el.querySelector(".parallel-member-tool-title");
    if (title && displayName) title.textContent = displayName;
    this.insertMemberTimelineItem(entry, existing.el, existing.el.getAttribute("data-event-order"));
    return existing;
  }
  entry.lastTimelineType = "toolflow:" + key;
  entry.currentTimelineItem = null;
  const item = document.createElement("div");
  item.className = "parallel-member-event parallel-member-event-tool parallel-member-tool-flow";
  item.setAttribute("data-tool-flow-key", key);
  item.innerHTML = `
    <details>
      <summary>
        <span class="parallel-member-tool-title">${this.escapeHtml(displayName || "工具调用")}</span>
        <span class="parallel-member-tool-status">准备参数</span>
      </summary>
      <div class="parallel-member-tool-section parallel-member-tool-args" style="display:none">
        <div class="parallel-member-tool-label">参数</div>
        <pre></pre>
      </div>
      <div class="parallel-member-tool-section parallel-member-tool-result" style="display:none">
        <div class="parallel-member-tool-label">结果</div>
        <pre></pre>
      </div>
    </details>
  `;
  this.insertMemberTimelineItem(entry, item, order);
  const flow = {
    el: item,
    status: item.querySelector(".parallel-member-tool-status"),
    argsWrap: item.querySelector(".parallel-member-tool-args"),
    args: item.querySelector(".parallel-member-tool-args pre"),
    resultWrap: item.querySelector(".parallel-member-tool-result"),
    result: item.querySelector(".parallel-member-tool-result pre"),
    argsRaw: "",
    resultRaw: "",
  };
  entry.toolFlows[key] = flow;
  this.updateMemberActivity(entry, `准备调用工具：${displayName || "工具调用"}`);
  this.updateMemberDetailVisibility(entry);
  return flow;
};

FKTeamsChat.prototype.migrateMemberToolFlow = function (entry, fromKey, toKey) {
  if (!entry || !fromKey || !toKey || fromKey === toKey) return;
  if (!entry.toolFlows) return;
  const from = entry.toolFlows[fromKey];
  if (!from) return;
  const to = entry.toolFlows[toKey];
  if (to && (!to.el || !to.el.isConnected)) {
    delete entry.toolFlows[toKey];
    entry.toolFlows[toKey] = from;
    delete entry.toolFlows[fromKey];
    if (from.el) from.el.setAttribute("data-tool-flow-key", toKey);
    this.rebindMemberToolFlowAliases(entry, fromKey, toKey);
    this.updateMemberDetailVisibility(entry);
    return;
  }
  if (!to) {
    entry.toolFlows[toKey] = from;
    delete entry.toolFlows[fromKey];
    if (from.el) from.el.setAttribute("data-tool-flow-key", toKey);
    this.rebindMemberToolFlowAliases(entry, fromKey, toKey);
    return;
  }
  if ((from.argsRaw || "") && !(to.argsRaw || "")) {
    to.argsRaw = from.argsRaw;
    if (to.argsWrap) to.argsWrap.style.display = "";
    if (to.args) to.args.textContent = this.truncateRunes(to.argsRaw, 600);
  }
  if ((from.resultRaw || "") && !(to.resultRaw || "")) {
    to.resultRaw = from.resultRaw;
    if (to.resultWrap) to.resultWrap.style.display = "";
    if (to.result) to.result.textContent = to.resultRaw;
  }
  if (to.status && from.status && from.status.textContent === "已完成") {
    to.status.textContent = "已完成";
  }
  if (from.el && from.el !== to.el) from.el.remove();
  delete entry.toolFlows[fromKey];
  this.rebindMemberToolFlowAliases(entry, fromKey, toKey);
  this.updateMemberDetailVisibility(entry);
};

FKTeamsChat.prototype.rebindMemberToolFlowAliases = function (entry, fromKey, toKey) {
  if (!entry?.toolFlowKeyByName || !fromKey || !toKey) return;
  Object.keys(entry.toolFlowKeyByName).forEach((name) => {
    if (entry.toolFlowKeyByName[name] === fromKey) {
      entry.toolFlowKeyByName[name] = toKey;
    }
  });
};

FKTeamsChat.prototype.updateMemberToolFlowArgs = function (entry, key, displayName, args, append, order) {
  const flow = this.ensureMemberToolFlow(entry, key, displayName, order);
  if (!flow) return;
  flow.argsRaw = append ? (flow.argsRaw || "") + (args || "") : (args || "");
  if (flow.argsRaw) {
    flow.argsWrap.style.display = "";
    flow.args.textContent = this.truncateRunes(flow.argsRaw, 600);
    if (flow.status) flow.status.textContent = "已调用";
  }
  this.updateMemberActivity(entry, `调用工具：${displayName || "工具调用"}`);
};

FKTeamsChat.prototype.setMemberFinalOutput = function (entry, content, order) {
  if (!entry || !content) return;
  const bodies = Array.from(entry.el?.querySelectorAll(".parallel-member-event-output .parallel-member-markdown") || []);
  const body = bodies.reduce((latest, current) => {
    if (!latest) return current;
    const latestItem = latest.closest(".parallel-member-event");
    const currentItem = current.closest(".parallel-member-event");
    const latestOrder = this.memberTimelineOrder(latestItem?.getAttribute("data-event-order"));
    const currentOrder = this.memberTimelineOrder(currentItem?.getAttribute("data-event-order"));
    return currentOrder >= latestOrder ? current : latest;
  }, null);
  if (!body) {
    this.appendMemberOutputFinal(entry, content, order);
    return;
  }
  const existingRaw = body.getAttribute("data-raw") || "";
  let finalContent = content;
  if (existingRaw && content && !content.includes(existingRaw) && !existingRaw.includes(content)) {
    finalContent = existingRaw + content;
  } else if (existingRaw && existingRaw.length > content.length && existingRaw.includes(content)) {
    finalContent = existingRaw;
  }
  body.setAttribute("data-raw", finalContent);
  body._memberRenderStreaming = false;
  body._memberRenderDirty = false;
  body._memberRenderPending = false;
  body.removeAttribute("data-streaming");
  body.innerHTML = this.renderMarkdown(finalContent, false);
};

FKTeamsChat.prototype.memberHasOutputContent = function (entry) {
  if (!entry || !entry.el) return false;
  const bodies = entry.el.querySelectorAll(".parallel-member-event-output .parallel-member-markdown");
  for (const body of bodies) {
    const raw = body.getAttribute("data-raw") || body.textContent || "";
    if (raw.trim()) return true;
  }
  return false;
};

FKTeamsChat.prototype.updateMemberToolFlowResult = function (entry, key, displayName, result, append, order) {
  const flow = this.ensureMemberToolFlow(entry, key, displayName, order);
  if (!flow) return;
  flow.resultRaw = append ? (flow.resultRaw || "") + (result || "") : (result || "");
  if (flow.resultRaw) {
    flow.resultWrap.style.display = "";
    flow.result.textContent = flow.resultRaw;
    if (flow.status) flow.status.textContent = "已完成";
  }
  this.updateMemberActivity(entry, `工具完成：${displayName || "工具调用"}`);
};

FKTeamsChat.prototype.extractMemberTaskContent = function (argsText) {
  const raw = String(argsText || "").trim();
  if (!raw) return "";
  try {
    const parsed = JSON.parse(raw);
    if (typeof parsed === "string") return parsed;
    if (parsed && typeof parsed === "object") {
      const keys = ["task", "request", "content", "instruction", "instructions", "prompt", "message", "input", "description"];
      for (const key of keys) {
        if (typeof parsed[key] === "string" && parsed[key].trim()) return parsed[key].trim();
      }
      return "";
    }
    return "";
  } catch {
    return raw;
  }
};

FKTeamsChat.prototype.updateMemberTaskContent = function (entry, deltaOrText, append) {
  if (!entry || !deltaOrText) return;
  entry.taskRaw = append ? (entry.taskRaw || "") + deltaOrText : deltaOrText;
  if (!entry.task) entry.task = entry.el?.querySelector(".parallel-member-task-content");
  if (!entry.task) return;
  const taskText = this.extractMemberTaskContent(entry.taskRaw);
  if (!taskText) return;
  entry.taskText = taskText;
  entry.task.style.display = "";
  const textEl = entry.task.querySelector(".parallel-member-task-text") || entry.task;
  textEl.textContent = this.truncateRunes(taskText, 520);
  if (!entry.taskDetail) entry.taskDetail = entry.el?.querySelector(".parallel-member-task-detail");
  if (entry.taskDetail) {
    entry.taskDetail.style.display = "";
    entry.taskDetail.textContent = taskText;
  }
  this.updateMemberActivity(entry, "任务已分配");
  this.updateMemberDetailVisibility(entry);
};

FKTeamsChat.prototype.updateMemberArgs = function (entry, deltaOrText, append) {
  this.updateMemberTaskContent(entry, deltaOrText, append);
};

FKTeamsChat.prototype.finalizeParallelMemberResults = function () {
  const chunks = this.parallelMemberResultChunks || {};
  for (const callID of Object.keys(chunks)) {
    const content = chunks[callID] || "";
    const toolCall = this.toolCallsByID[callID];
    const toolName = toolCall?.name || "";
    const display = this.getToolDisplay(toolCall || { name: toolName });
    const agentName = this.agentNameFromTool(toolName);
    const memberKey = this.parallelToolMemberByID[callID] || "call:" + callID;
    const entry = this.parallelMemberCards[memberKey];
    if (!entry) {
      delete chunks[callID];
      continue;
    }
    if (this.isAgentToolErrorResult(content)) {
      if (content) this.setMemberFinalOutput(entry, content);
      this.updateMemberStatus(entry, "error", "失败");
    } else {
      if (content && !this.memberHasOutputContent(entry)) {
        this.setMemberFinalOutput(entry, content);
      }
      this.updateMemberStatus(entry, "done", "完成");
    }
    if (display.target || agentName) entry.name = display.target || agentName;
    delete chunks[callID];
  }
};

FKTeamsChat.prototype.flushMemberInnerToolResults = function (entry, memberCallID) {
  if (!entry || !memberCallID) return;
  const chunks = this.parallelMemberInnerResultChunks || {};
  for (const key of Object.keys(chunks)) {
    if (!key.startsWith(memberCallID + ":")) continue;
    delete chunks[key];
  }
};

FKTeamsChat.prototype.flushAllMemberInnerToolResults = function () {
  const chunks = this.parallelMemberInnerResultChunks || {};
  for (const key of Object.keys(chunks)) {
    const memberCallID = key.split(":")[0];
    const entry = this.parallelMemberCards["call:" + memberCallID];
    if (!entry) continue;
    delete chunks[key];
  }
};

FKTeamsChat.prototype.handleMemberStreamChunk = function (event) {
  const entry = this.memberEntryFromEvent(event);
  this.flushMemberInnerToolResults(entry, this.memberCallIDFromEvent(event));
  this.updateMemberActivity(entry, "输出中");
  this.appendMemberStreamEvent(entry, event, "output", "输出");
  this.scrollToBottom();
};

FKTeamsChat.prototype.handleMemberReasoningChunk = function (event) {
  const entry = this.memberEntryFromEvent(event);
  this.updateMemberActivity(entry, "正在思考");
  this.appendMemberStreamEvent(entry, event, "reasoning", "思考过程");
  this.scrollToBottom();
};

FKTeamsChat.prototype.handleMemberMessage = function (event) {
  const entry = this.memberEntryFromEvent(event);
  this.flushMemberInnerToolResults(entry, this.memberCallIDFromEvent(event));
  if (event.reasoning_content) this.setMemberFinalReasoning(entry, event.reasoning_content, event.sequence);
  if (event.content) this.setMemberFinalOutput(entry, event.content, event.sequence);
  if (event.content) this.updateMemberActivity(entry, "输出完成");
  this.finalizeMemberMarkdown(entry);
  this.scrollToBottom();
};

FKTeamsChat.prototype.sendMessage = async function () {
  const message = this.messageInput.value.trim();
  const hasAttachments = this.attachments && this.attachments.length > 0;
  if ((!message && !hasAttachments) || this.isProcessing) return;

  // 页面刷新后首次发送消息时，先创建/复用会话并恢复状态
  if (!this._hasLoadedSession) {
    try {
      const body = { title: message };
      // 如果有持久化的会话 ID，尝试复用（跨刷新保留上下文）
      if (this.sessionId) {
        body.session_id = this.sessionId;
      }
      const resp = await this.fetchWithAuth("/api/fkteams/sessions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      const result = await resp.json();
      if (result.code !== 0 || !result.data || !result.data.session_id) {
        this.showNotification("创建会话失败", "error");
        return;
      }
      this.sessionId = result.data.session_id;
      this.sessionIdInput.value = this.sessionId;
      localStorage.setItem("fk_session_id", this.sessionId);
      this._hasLoadedSession = true;
      this.loadSidebarHistory();
      // 从服务端恢复当前 @智能体 状态（跨设备同步）
      if (result.data.current_agent) {
        const agent = this.agents.find((a) => a.name === result.data.current_agent);
        if (agent) {
          this.setCurrentAgent(agent, false); // 从服务端还原，仅更新 UI
        }
      }
    } catch (err) {
      console.error("Error creating session:", err);
      this.showNotification("创建会话失败", "error");
      return;
    }
  }

  const welcomeMsg = this.messagesContainer.querySelector(".welcome-message");
  if (welcomeMsg) welcomeMsg.remove();

  // 用户发送新消息时，重置滚动状态
  this.userScrolledUp = false;
  this.showScrollToBottomBtn(false);

  // 隐藏智能体建议
  this.hideAgentSuggestions();
  // 隐藏文件建议
  this.hideFileSuggestions();

  // 检查是否有@智能体提及
  const mention = this.extractAgentMention(message);

  // 提取文件路径
  const filePaths = this.extractFilePaths(message);

  if (mention) {
    // 查找智能体
    const agent = this.agents.find((a) => a.name === mention.agentName || (a.aliases || []).includes(mention.agentName));
    if (agent) {
      this.setCurrentAgent(agent, true); // 用户主动 @智能体，持久化

      // 显示切换通知
      this.showAgentSwitchNotification(agent.name, agent.description);
    } else {
      // 智能体不存在，显示错误
      this.showNotification(`未找到智能体: ${mention.agentName}`, "error");
      return;
    }
  }

  // 显示用户消息（包含附件预览）
  this.addUserMessage(message, this.attachments);

  // 构建发送 payload
  const payload = {
    type: "chat",
    session_id: this.sessionId,
    message: message,
    mode: this.mode,
  };

  if (this.currentAgent) {
    payload.agent_name = this.currentAgent.name;
  }

  if (filePaths.length > 0) {
    payload.file_paths = filePaths;
  }

  // 多模态内容
  if (hasAttachments) {
    const contents = [];
    if (message) {
      contents.push({ type: "text", text: message });
    }
    for (const att of this.attachments) {
      if (att.type === "image") {
        contents.push({
          type: "image_base64",
          base64_data: att.base64,
          mime_type: att.mimeType,
        });
      }
    }
    payload.contents = contents;
  }

  this.ws.send(JSON.stringify(payload));

  this.messageInput.value = "";
  this.clearAttachments();
  this.handleInputChange();
  this.isProcessing = true;
  this.updateSendButtonState();
  this.updateStatus("processing", "处理中...");
  this.showThinkingIndicator();
};

// 显示等待模型响应的思考指示器
FKTeamsChat.prototype.showThinkingIndicator = function () {
  this.hideThinkingIndicator();
  const el = document.createElement("div");
  el.className = "thinking-indicator";
  el.id = "thinking-indicator";
  el.innerHTML = `
    <div class="thinking-dots">
      <span></span><span></span><span></span>
    </div>
    <span class="thinking-text">思考中</span>
  `;
  this.messagesContainer.appendChild(el);
  this.scrollToBottom();
};

// 隐藏思考指示器
FKTeamsChat.prototype.hideThinkingIndicator = function () {
  const el = document.getElementById("thinking-indicator");
  if (el) el.remove();
};

// 显示智能体切换通知
FKTeamsChat.prototype.showAgentSwitchNotification = function (
  agentName,
  description,
) {
  const notificationEl = document.createElement("div");
  notificationEl.className = "action-event agent-switch";
  notificationEl.innerHTML = `
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
            <circle cx="9" cy="7" r="4" />
            <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
            <path d="M16 3.13a4 4 0 0 1 0 7.75" />
        </svg>
        <span>已切换到智能体: <strong>${this.escapeHtml(agentName)}</strong> - ${this.escapeHtml(description)}</span>
        <button class="reset-mode-btn" onclick="app.resetToTeamMode()" title="切换回团队模式">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
                <circle cx="9" cy="7" r="4" />
                <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
                <path d="M16 3.13a4 4 0 0 1 0 7.75" />
            </svg>
            团队模式
        </button>
    `;
  this.messagesContainer.appendChild(notificationEl);
  this.scrollToBottom();
};

// 重置回团队模式
FKTeamsChat.prototype.resetToTeamMode = function () {
  this.setCurrentAgent(null, true); // 用户主动切回团队模式，持久化
  const resetNotificationEl = document.createElement("div");
  resetNotificationEl.className = "action-event agent-switch";
  resetNotificationEl.innerHTML = `
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
            <circle cx="9" cy="7" r="4" />
            <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
            <path d="M16 3.13a4 4 0 0 1 0 7.75" />
        </svg>
        <span>已切换回 <strong>${this.mode === "team" || this.mode === "supervisor" ? "团队模式" : this.mode === "roundtable" ? "圆桌讨论模式" : "自定义会议模式"}</strong></span>
    `;
  this.messagesContainer.appendChild(resetNotificationEl);
  this.scrollToBottom();
  this.showNotification("已切换回团队模式", "success");
};

FKTeamsChat.prototype.handleServerEvent = function (event) {
  this.rememberStreamEvent(event);

  // 会话隔离：跟踪所有进行中的会话
  const eventSessionId = event.session_id;
  // 只有明确匹配当前 session_id 的事件才视为当前会话的事件
  // 没有 session_id 的系统消息（connected/pong）直接放行
  const isSystemEvent = ["connected", "pong"].includes(event.type);
  const isCurrentSession =
    isSystemEvent || (eventSessionId && eventSessionId === this.sessionId);

  // 维护进行中会话集合
  if (!this._processingSessions) {
    this._processingSessions = new Set();
  }
  if (event.type === "processing_start" && eventSessionId) {
    this._processingSessions.add(eventSessionId);
    this.loadSidebarHistory();
  }
  if (
    (event.type === "processing_end" ||
      event.type === "error" ||
      event.type === "cancelled") &&
    eventSessionId
  ) {
    this._processingSessions.delete(eventSessionId);
    this.loadSidebarHistory();
  }

  // 非当前会话的事件只更新状态，缓冲渲染事件供切换时回放
  if (!isCurrentSession) {
    if (eventSessionId && this._sessionDOMCache[eventSessionId]) {
      // 缓存中有该会话的 DOM，缓冲事件供恢复时回放
      if (!this._sessionEventBuffer) this._sessionEventBuffer = {};
      if (!this._sessionEventBuffer[eventSessionId])
        this._sessionEventBuffer[eventSessionId] = [];
      this._sessionEventBuffer[eventSessionId].push(event);
      // processing_end 时清除 DOM 缓存，下次切回从服务器加载完整历史
      if (event.type === "processing_end") {
        delete this._sessionDOMCache[eventSessionId];
        delete this._sessionEventBuffer[eventSessionId];
      }
    }
    return;
  }

  // 首个内容事件到来时移除思考指示器
  if (!["connected", "processing_start", "pong", "user_message"].includes(event.type)) {
    this.hideThinkingIndicator();
  }
  // resume 后收到内容事件则标记回放成功
  if (
    this._resumePending &&
    !["connected", "pong", "processing_start", "processing_end"].includes(
      event.type,
    )
  ) {
    this._resumeReplayed = true;
  }

  switch (event.type) {
    case "connected":
      break;
    case "pong":
      break;
    case "user_message":
      this.handleUserMessageEvent(event);
      break;
    case "agent_start":
    case "agent_end":
    case "turn_start":
    case "turn_end":
      break;
    case "processing_start":
      this._cancelledSessionId = null;
      this._resumePending = false;
      this.isProcessing = true;
      this.updateStatus("processing", "处理中...");
      break;
    case "processing_end":
      // resume 回放未收到任何内容事件，说明流已提前结束，从历史 API 重新加载
      if (this._resumePending && !this._resumeReplayed) {
        this._resumePending = false;
        this.isProcessing = false;
        this.updateStatus("connected", "已连接");
        this.updateSendButtonState();
        this.resetParallelState();
        this.loadSession(this.sessionId);
        break;
      }
      this._resumePending = false;
      this.isProcessing = false;
      this.updateStatus("connected", "已连接");
      this.updateSendButtonState();
      // 流式结束后，对所有含 data-raw 的消息做一次脚注最终渲染
      this.flushAllMemberInnerToolResults();
      this.finalizeParallelMemberResults();
      this._finalizeFootnotes();
      this.resetParallelState();
      break;
    case "cancelled":
      this.handleCancelled(event);
      break;
    case "message_start":
      if (this._cancelledSessionId === eventSessionId) break;
      this.handleCoreMessageStart(event);
      break;
    case "message_delta":
      if (this._cancelledSessionId === eventSessionId) break;
      this.handleCoreMessageDelta(event);
      break;
    case "message_end":
      if (this._cancelledSessionId === eventSessionId) break;
      this.handleCoreMessageEnd(event);
      break;
    case "tool_start":
      if (this._cancelledSessionId === eventSessionId) break;
      this.handleCoreToolStart(event);
      break;
    case "tool_update":
      if (this._cancelledSessionId === eventSessionId) break;
      this.handleCoreToolUpdate(event);
      break;
    case "tool_end":
      if (this._cancelledSessionId === eventSessionId) break;
      this.handleCoreToolEnd(event);
      break;
    case "action":
      if (this._cancelledSessionId === eventSessionId) break;
      this.handleAction(event);
      break;
    case "usage":
      break;
    case "dispatch_progress":
      this.handleDispatchProgress(event);
      break;
    case "approval_required":
      this.showApprovalRequest(event.message);
      this.showApprovalDialog(event.message);
      break;
    case "ask_questions":
      this.showInlineAskForm(event);
      break;
    case "error":
      this.handleError(event);
      break;
    default:
      console.log("Unknown event:", event);
  }
};

FKTeamsChat.prototype.handleUserMessageEvent = function (event) {
  const content = event.content || "";
  if (!content) return;

  const users = this.messagesContainer.querySelectorAll(".message.user .message-body");
  const last = users.length > 0 ? users[users.length - 1].textContent || "" : "";
  if (last === content) return;

  const welcomeMsg = this.messagesContainer.querySelector(".welcome-message");
  if (welcomeMsg) welcomeMsg.remove();
  this.addUserMessage(content, null);
};

FKTeamsChat.prototype.trimLeadingWhitespace = function (text) {
  if (!text) return "";
  return text.replace(/^[\s\n\r\u00A0\u2000-\u200B\uFEFF]+/, "");
};

FKTeamsChat.prototype.wrapMarkdownTables = function (html) {
  if (!html || !html.includes("<table")) return html;
  return html.replace(/<table([\s\S]*?)<\/table>/g, function (match) {
    if (match.includes('class="markdown-table-wrap"')) return match;
    return '<div class="markdown-table-wrap">' + match + "</div>";
  });
};

// 渲染 Markdown（streaming 为 true 时跳过脚注处理以提升流式性能）
FKTeamsChat.prototype.renderMarkdown = function (text, streaming) {
  if (!text) return "";
  try {
    if (typeof marked !== "undefined") {
      if (!this._markedInstance) {
        this._markedInstance = new marked.Marked({ breaks: true, gfm: true });
        // 链接在新标签中打开
        this._markedInstance.use({
          renderer: {
            link: function (token) {
              var href = token.href || "";
              var title = token.title ? ' title="' + token.title + '"' : "";
              var text = token.text || href;
              if (href.startsWith("#")) {
                return '<a href="' + href + '"' + title + ">" + text + "</a>";
              }
              return (
                '<a href="' +
                href +
                '"' +
                title +
                ' target="_blank" rel="noopener noreferrer">' +
                text +
                "</a>"
              );
            },
          },
        });
      }

      // 流式渲染时跳过脚注处理，仅在最终渲染时处理
      if (!streaming) {
        var footnotes = this._extractFootnotes(text);
        text = footnotes.text;
        var html = this._markedInstance.parse(text);
        // 在 marked 解析后替换占位符为真正的脚注链接
        if (footnotes.orderedNums && footnotes.orderedNums.length > 0) {
          html = this._replaceFootnotePlaceholders(
            html,
            footnotes.definitions,
            footnotes.orderedNums,
          );
        }
        if (footnotes.items.length > 0) {
          html = this._buildSourcesCard(html, footnotes.items);
        }
        return this.wrapMarkdownTables(html);
      }
      return this.wrapMarkdownTables(this._markedInstance.parse(text));
    }
  } catch (e) {
    console.error("Markdown parse error:", e);
  }
  return this.escapeHtml(text);
};

// 从 markdown 文本中提取脚注定义，替换行内引用为占位符，移除定义行
FKTeamsChat.prototype._extractFootnotes = function (text) {
  var definitions = {};
  var orderedNums = [];

  // 提取脚注定义: [^N]: 内容（可能是 URL + 描述，或纯文本，或 markdown 链接）
  text.replace(/^\[\^(\d+)\]:\s*(.+)$/gm, function (match, num, content) {
    content = content.trim();
    var url = "",
      label = "";

    // 尝试匹配 markdown 链接: [text](url)
    var mdLink = content.match(/^\[([^\]]*)\]\((https?:\/\/[^)]+)\)(.*)$/);
    if (mdLink) {
      url = mdLink[2];
      label = (mdLink[1] + " " + mdLink[3]).trim() || url;
    } else {
      // 尝试匹配裸 URL: https://... 可选描述
      var urlMatch = content.match(/^(https?:\/\/\S+)(?:\s+(.*))?$/);
      if (urlMatch) {
        url = urlMatch[1];
        label = urlMatch[2] || url;
      } else {
        label = content;
      }
    }

    definitions[num] = { url: url, label: label };
    if (orderedNums.indexOf(num) === -1) orderedNums.push(num);
    return match;
  });

  if (orderedNums.length === 0) {
    return { text: text, items: [] };
  }

  // 移除脚注定义行（包括前后可能的空行）
  text = text.replace(/\n*^\[\^(\d+)\]:\s*(.+)$/gm, "");

  // 将行内引用 [^N] 替换为占位符（marked 会当作普通文本保留）
  var items = [];
  orderedNums.forEach(function (num) {
    items.push(definitions[num]);
  });

  text = text.replace(/\[\^(\d+)\]/g, function (match, num) {
    var def = definitions[num];
    if (!def) return match;
    var idx = orderedNums.indexOf(num);
    return "<!--fnref:" + idx + ":" + num + "-->";
  });

  return {
    text: text,
    items: items,
    definitions: definitions,
    orderedNums: orderedNums,
  };
};

// 将占位符替换为真正的脚注链接（在 marked.parse 之后调用）
FKTeamsChat.prototype._replaceFootnotePlaceholders = function (
  html,
  definitions,
  orderedNums,
) {
  return html.replace(/<!--fnref:(\d+):(\d+)-->/g, function (match, idx, num) {
    var def = definitions[num];
    if (!def) return match;
    var displayNum = parseInt(idx, 10) + 1;
    if (def.url) {
      return (
        '<a class="footnote-cite" href="' +
        def.url +
        '" data-url="' +
        def.url +
        '" target="_blank" rel="noopener noreferrer">' +
        displayNum +
        "</a>"
      );
    }
    return '<span class="footnote-cite">' + displayNum + "</span>";
  });
};

// 根据提取的脚注项构建来源卡片，追加到 HTML 末尾
FKTeamsChat.prototype._buildSourcesCard = function (html, items) {
  // 收集可用 favicon 的域名
  var favicons = [];
  items.forEach(function (item) {
    if (item.url && /^https?:\/\//.test(item.url)) {
      try {
        var domain = new URL(item.url).hostname;
        if (favicons.indexOf(domain) === -1) favicons.push(domain);
      } catch (e) {
        /* ignore */
      }
    }
  });

  // 构建图标堆叠（最多显示5个）
  var iconsHtml = "";
  var showCount = Math.min(favicons.length, 5);
  if (showCount > 0) {
    for (var i = 0; i < showCount; i++) {
      iconsHtml +=
        '<img class="source-favicon" src="https://www.google.com/s2/favicons?domain=' +
        favicons[i] +
        '&sz=32" alt="" style="z-index:' +
        (showCount - i) +
        ";margin-left:" +
        (i === 0 ? "0" : "-6px") +
        ';">';
    }
  } else {
    iconsHtml =
      '<span class="source-icon-fallback"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg></span>';
  }

  // 构建来源列表
  var listHtml = "";
  items.forEach(function (item, idx) {
    var favicon = "";
    if (item.url && /^https?:\/\//.test(item.url)) {
      try {
        var d = new URL(item.url).hostname;
        favicon =
          '<img class="source-item-favicon" src="https://www.google.com/s2/favicons?domain=' +
          d +
          '&sz=16" alt="">';
      } catch (e) {
        /* ignore */
      }
    }
    if (!favicon) {
      favicon =
        '<span class="source-item-icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="14" height="14"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg></span>';
    }
    var linkAttr = item.url
      ? ' href="' + item.url + '" target="_blank" rel="noopener noreferrer"'
      : "";
    var tag = item.url ? "a" : "span";
    listHtml +=
      "<" +
      tag +
      ' class="source-item"' +
      linkAttr +
      ">" +
      favicon +
      '<span class="source-item-label">' +
      (idx + 1) +
      ". " +
      item.label +
      "</span></" +
      tag +
      ">";
  });

  var cardHtml =
    '<div class="sources-card">' +
    '<div class="sources-header" onclick="this.parentElement.classList.toggle(\'expanded\')">' +
    '<div class="sources-icons">' +
    iconsHtml +
    "</div>" +
    '<span class="sources-count">' +
    items.length +
    " 个来源</span>" +
    '<svg class="sources-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>' +
    "</div>" +
    '<div class="sources-list">' +
    listHtml +
    "</div>" +
    "</div>";

  return html + cardHtml;
};

// 流式结束后，对含脚注的消息体做一次最终渲染（仅处理尚未完成脚注渲染的消息）
FKTeamsChat.prototype._finalizeFootnotes = function () {
  var bodies = this.messagesContainer.querySelectorAll(
    ".message.assistant .message-body[data-raw]:not([data-fn-done])",
  );
  for (var i = 0; i < bodies.length; i++) {
    var body = bodies[i];
    var raw = body.getAttribute("data-raw");
    if (raw && /\[\^\d+\]/.test(raw)) {
      body.innerHTML = this.renderMarkdown(raw, false);
    }
    // 标记已完成脚注渲染，后续不再重复处理
    body.setAttribute("data-fn-done", "1");
  }
};

FKTeamsChat.prototype.coreToolCallFromEvent = function (event) {
  const toolCall = {
    id: event.tool_call_id || "",
    ref: event.tool_call_ref || "",
    index: event.tool_call_index,
    name: event.tool_name || "",
    display_name: event.tool_display_name || "",
    kind: event.tool_kind || "",
    target: event.tool_target || "",
    arguments: event.tool_args || event.delta || event.content || "",
  };
  if (toolCall.index === undefined || toolCall.index === null) {
    toolCall.index = event.detail !== undefined ? event.detail : 0;
  }
  return toolCall;
};

FKTeamsChat.prototype.coreToolEventKey = function (event) {
  if (event.tool_call_ref) return "ref:" + event.tool_call_ref;
  if (event.tool_call_id) return "id:" + event.tool_call_id;
  if (event.tool_call_index !== undefined && event.tool_call_index !== null) return "idx:" + event.tool_call_index;
  return "idx:" + (event.detail !== undefined ? event.detail : 0);
};

FKTeamsChat.prototype.migrateCoreToolEventKey = function (event, toolCall) {
  if (this.isMemberRunEvent(event) || !event.tool_call_ref) return;
  const refKey = "ref:" + event.tool_call_ref;
  if (event.tool_call_id) this.migrateToolCallCard("id:" + event.tool_call_id, refKey, toolCall);
  if (event.tool_call_index !== undefined && event.tool_call_index !== null) {
    this.migrateToolCallCard("idx:" + event.tool_call_index, refKey, toolCall);
  }
};

FKTeamsChat.prototype.handleCoreMessageStart = function (event) {
  if (event.role === "user") return;
  if (event.role === "tool") return;
  if (this.isMemberRunEvent(event)) {
    const entry = this.memberEntryFromEvent(event);
    this.updateMemberActivity(entry, event.delta_kind === "reasoning" ? "正在思考" : "准备输出");
    this.scrollToBottom();
    return;
  }
  this.finalizeParallelMemberResults();
  this.getMessageElementForEvent(event);
  this.scrollToBottom();
};

FKTeamsChat.prototype.handleCoreMessageDelta = function (event) {
  if (event.role === "user") return;
  if (event.role === "tool") return;
  const content = event.delta || event.content || "";
  if (!content) return;

  if (event.delta_kind === "tool_result") {
    this.handleCoreToolUpdate(event);
    return;
  }
  if (event.delta_kind === "tool_args") {
    this.handleCoreToolArgsDelta(event);
    return;
  }
  if (event.delta_kind === "reasoning") {
    this.handleReasoningChunk({ ...event, content });
    return;
  }
  this.handleStreamChunk({ ...event, content });
};

FKTeamsChat.prototype.handleCoreMessageEnd = function (event) {
  if (event.role === "user") return;
  if (event.role === "tool") return;

  if (event.reasoning_content || event.content) {
    this.handleMessage(event);
  }

  if (event.tool_calls && event.tool_calls.length > 0) {
    this.handleToolCallsPreparing(event);
    this.handleToolCalls(event);
  }
};

FKTeamsChat.prototype.handleCoreToolStart = function (event) {
  const toolCall = this.normalizeToolCallsForEvent(event)[0] || this.coreToolCallFromEvent(event);
  this.migrateCoreToolEventKey(event, toolCall);
  const nextEvent = { ...event, tool_calls: [toolCall] };
  this.handleToolCallsPreparing(nextEvent);
  if (toolCall.arguments) {
    this.handleToolCalls(nextEvent);
  }
};

FKTeamsChat.prototype.handleCoreToolArgsDelta = function (event) {
  const content = event.delta || event.content || "";
  if (!content) return;

  if (!this.isMemberRunEvent(event)) {
    const toolCall = this.normalizeToolCallForEvent({ ...event, content: "" }, null, 0);
    this.migrateCoreToolEventKey(event, toolCall);
    const key = this.coreToolEventKey(event);
    const toolDisplay = this.getToolDisplay(toolCall);
    this.ensureToolCallCard(toolCall, key, toolDisplay, "准备参数", "参数准备中...");
  }

  this.handleToolCallsArgsDelta({ ...event, content });
};

FKTeamsChat.prototype.handleCoreToolUpdate = function (event) {
  const content = event.delta || event.tool_result || event.content || "";
  if (event.delta_kind === "tool_args") {
    this.handleCoreToolArgsDelta(event);
    return;
  }
  if (!content) return;
  this.handleToolResult({ ...event, type: "tool_result_chunk", content });
};

FKTeamsChat.prototype.handleCoreToolEnd = function (event) {
  const content = event.tool_result || event.content || "";
  if (!content) return;
  this.handleToolResult({ ...event, type: "tool_result", content });
};

FKTeamsChat.prototype.handleStreamChunk = function (event) {
  if (this.isMemberRunEvent(event)) {
    this.handleMemberStreamChunk(event);
    return;
  }
  this.finalizeParallelMemberResults();
  this.getMessageElementForEvent(event);

  const bodyEl = this.currentMessageElement.querySelector(".message-body");
  if (bodyEl) {
    const indicator = bodyEl.querySelector(".streaming-indicator");
    if (indicator) indicator.remove();

    // 推理结束后开始正式内容：折叠推理块并更新标题
    const reasoningBlock = bodyEl.querySelector(".reasoning-block.expanded");
    if (reasoningBlock) {
      reasoningBlock.classList.remove("expanded");
      const title = reasoningBlock.querySelector(".reasoning-title");
      if (title) title.textContent = "思考过程";
    }

    // 获取原始文本
    let rawText = bodyEl.getAttribute("data-raw") || "";
    let newContent = event.content || "";

    if (rawText === "") {
      newContent = this.trimLeadingWhitespace(newContent);
    }

    rawText += newContent;
    bodyEl.setAttribute("data-raw", rawText);

    // 流式渲染 Markdown（跳过脚注处理，但剥离脚注定义行避免原文可见）
    // 1. 剥离完整定义行：[^N]: 内容
    var streamText = rawText.replace(/\n*^\[\^(\d+)\]:\s*(.+)$/gm, "");
    // 2. 剥离尾部不完整的定义行（如 [^、[^1、[^1]、[^1]: 等正在输入中的部分）
    streamText = streamText.replace(/\n\[\^[^\]]*\]?:?\s*$/, "");
    // 3. 规范化尾部空白，避免换行符差异导致无意义的 DOM 更新
    streamText = streamText.replace(/\s+$/, "");
    // 仅当可见内容变化时更新 DOM（避免脚注定义行到达时的无意义重绘）
    var lastStreamText = bodyEl.getAttribute("data-stream-text") || "";
    if (streamText !== lastStreamText) {
      bodyEl.setAttribute("data-stream-text", streamText);
      const existingReasoning = bodyEl.querySelector(".reasoning-block");
      if (existingReasoning) {
        let textContainer = bodyEl.querySelector(".message-text-content");
        if (!textContainer) {
          textContainer = document.createElement("div");
          textContainer.className = "message-text-content";
          bodyEl.appendChild(textContainer);
        }
        textContainer.innerHTML = this.renderMarkdown(streamText, true);
      } else {
        bodyEl.innerHTML = this.renderMarkdown(streamText, true);
      }
    }
  }
  this.scrollToBottom();
};

// 处理推理/思考内容的流式事件
FKTeamsChat.prototype.handleReasoningChunk = function (event) {
  if (this.isMemberRunEvent(event)) {
    this.handleMemberReasoningChunk(event);
    return;
  }
  this.finalizeParallelMemberResults();
  this.getMessageElementForEvent(event);

  const bodyEl = this.currentMessageElement.querySelector(".message-body");
  if (!bodyEl) return;

  const indicator = bodyEl.querySelector(".streaming-indicator");
  if (indicator) indicator.remove();

  // 查找或创建推理内容块
  let reasoningBlock = bodyEl.querySelector(".reasoning-block");
  if (!reasoningBlock) {
    reasoningBlock = document.createElement("div");
    reasoningBlock.className = "reasoning-block expanded";
    reasoningBlock.innerHTML = `
            <div class="reasoning-header" onclick="this.parentElement.classList.toggle('expanded')">
                <svg class="reasoning-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9.663 17h4.673M12 3v1M6.5 5.5l.7.7M3 12h1M20 12h1M16.8 6.2l.7-.7M17.5 12A5.5 5.5 0 1 0 7 14.5V17a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1v-2.5A5.5 5.5 0 0 0 17.5 12z"/></svg>
                <span class="reasoning-title">思考中...</span>
                <svg class="reasoning-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>
            </div>
            <div class="reasoning-content markdown-body markdown-body-compact"></div>
        `;
    bodyEl.prepend(reasoningBlock);
  }

  const contentEl = reasoningBlock.querySelector(".reasoning-content");
  if (contentEl) {
    let rawReasoning = contentEl.getAttribute("data-raw") || "";
    rawReasoning += event.content || "";
    contentEl.setAttribute("data-raw", rawReasoning);
    contentEl.innerHTML = this.renderMarkdown(rawReasoning, true);
  }

  this.scrollToBottom();
};

FKTeamsChat.prototype.handleMessage = function (event) {
  if (!event.content && !event.reasoning_content) return;
  if (this.isMemberRunEvent(event)) {
    this.handleMemberMessage(event);
    return;
  }
  this.finalizeParallelMemberResults();

  this.getMessageElementForEvent(event);

  const bodyEl = this.currentMessageElement.querySelector(".message-body");
  if (bodyEl) {
    const indicator = bodyEl.querySelector(".streaming-indicator");
    if (indicator) indicator.remove();

    // 处理推理/思考内容（非流式完整消息）
    if (event.reasoning_content) {
      let reasoningBlock = bodyEl.querySelector(".reasoning-block");
      if (!reasoningBlock) {
        reasoningBlock = document.createElement("div");
        reasoningBlock.className = "reasoning-block";
        reasoningBlock.innerHTML = `
                    <div class="reasoning-header" onclick="this.parentElement.classList.toggle('expanded')">
                        <svg class="reasoning-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9.663 17h4.673M12 3v1M6.5 5.5l.7.7M3 12h1M20 12h1M16.8 6.2l.7-.7M17.5 12A5.5 5.5 0 1 0 7 14.5V17a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1v-2.5A5.5 5.5 0 0 0 17.5 12z"/></svg>
                        <span class="reasoning-title">思考过程</span>
                        <svg class="reasoning-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>
                    </div>
                    <div class="reasoning-content markdown-body markdown-body-compact">${this.renderMarkdown(event.reasoning_content)}</div>
                `;
        bodyEl.prepend(reasoningBlock);
      }
    }

    if (event.content) {
      const existingRaw = bodyEl.getAttribute("data-raw") || "";
      let content = existingRaw ? event.content : this.trimLeadingWhitespace(event.content);
      if (existingRaw && content && !content.includes(existingRaw) && !existingRaw.includes(content)) {
        content = existingRaw + content;
      } else if (existingRaw && existingRaw.length > content.length && existingRaw.includes(content)) {
        content = existingRaw;
      }
      bodyEl.setAttribute("data-raw", content);
      bodyEl.setAttribute("data-fn-done", "1");
      // 保留已有的推理块
      const existingReasoning = bodyEl.querySelector(".reasoning-block");
      if (existingReasoning) {
        // 保留推理块，创建新的文本内容容器
        let textContainer = bodyEl.querySelector(".message-text-content");
        if (!textContainer) {
          textContainer = document.createElement("div");
          textContainer.className = "message-text-content";
          bodyEl.appendChild(textContainer);
        }
        textContainer.innerHTML = this.renderMarkdown(content);
      } else {
        bodyEl.innerHTML = this.renderMarkdown(content);
      }
    }
  }
  this.scrollToBottom();
};

FKTeamsChat.prototype.handleToolCallsPreparing = function (event) {
  const toolCalls = this.normalizeToolCallsForEvent(event);
  if (toolCalls.length === 0) return;

  if (this.isMemberRunEvent(event)) {
    const entry = this.memberEntryFromEvent(event);
    toolCalls.forEach((toolCall, i) => {
      const display = this.getToolDisplay(toolCall);
      if (toolCall.id) this.toolCallsByID[toolCall.id] = toolCall;
      if (toolCall.index !== undefined && toolCall.index !== null) this.toolCallsByIndex[String(toolCall.index)] = toolCall;
      const flowKey = this.resolveMemberToolFlowKey(entry, event, toolCall, i);
      this.registerMemberToolFlowAlias(entry, toolCall.name, flowKey);
      this.ensureMemberToolFlow(entry, flowKey, display.displayName, event.sequence);
    });
    this.scrollToBottom();
    return;
  }

  this.hasToolCallAfterMessage = true;
  toolCalls.forEach((toolCall, i) => {
    this.lastToolName = toolCall.name;
    const key = this.toolCallKey(toolCall, i);
    const toolDisplay = this.getToolDisplay(toolCall);
    if (toolDisplay.kind === "agent") {
      const agentName = this.agentNameFromTool(toolCall.name);
      const memberKey = this.memberKeyForToolCall(toolCall, i);
      const entry = this.ensureMemberCard(memberKey, toolDisplay.target || agentName, agentName, toolCall.index);
      this.parallelMemberByAgent[agentName] = memberKey;
      if (toolCall.id) {
        this.parallelToolMemberByID[toolCall.id] = memberKey;
        this.toolCallsByID[toolCall.id] = toolCall;
      }
      if (toolCall.index !== undefined && toolCall.index !== null) this.toolCallsByIndex[String(toolCall.index)] = toolCall;
      return;
    }
    this.ensureToolCallCard(toolCall, key, toolDisplay, "参数准备中", "参数准备中...");
  });
  this.scrollToBottom();
};

// 处理工具参数增量流式更新
FKTeamsChat.prototype.handleToolCallsArgsDelta = function (event) {
  if (!event.content) return;

  if (this.isMemberRunEvent(event)) {
    const entry = this.memberEntryFromEvent(event);
    const toolCall = this.normalizeToolCallForEvent(event, null, 0);
    const flowKey = this.resolveMemberToolFlowKey(entry, event, toolCall, 0);
    const display = this.getToolDisplay(toolCall);
    this.registerMemberToolFlowAlias(entry, toolCall.name, flowKey);
    this.updateMemberToolFlowArgs(entry, flowKey, display.displayName || "工具调用", event.content, true, event.sequence);
    this.scrollToBottom();
    return;
  }

  const callIndex = event.tool_call_index ?? event.detail ?? "0";
  const key = event.tool_call_ref ? "ref:" + event.tool_call_ref : event.tool_call_id ? "id:" + event.tool_call_id : "idx:" + callIndex;
  const pendingMemberKey = "pending:" + callIndex;
  const pendingMemberEntry = this.parallelMemberCards[pendingMemberKey];
  const memberKey = event.tool_call_id && this.parallelToolMemberByID[event.tool_call_id]
    ? this.parallelToolMemberByID[event.tool_call_id]
    : pendingMemberEntry && !this.isStalePendingMemberEntry(pendingMemberKey, pendingMemberEntry)
      ? pendingMemberKey
      : "";
  if (memberKey) {
    const entry = this.parallelMemberCards[memberKey];
    this.updateMemberArgs(entry, event.content, true);
    this.scrollToBottom();
    return;
  }
  const pending = this.pendingToolCalls[key];
  const card = pending?.el || this.findToolCallCard(key);
  if (!card) return;

  const argsEl = card.querySelector(".tool-call-args");
  if (!argsEl) return;
  this.updateToolCallStatus(card, "准备参数");

  // 首次增量到达时清除占位文本
  if (argsEl.textContent === "参数准备中..." || argsEl.textContent === "任务准备中...") {
    argsEl.textContent = "";
    argsEl.classList.add("streaming");
  }

  argsEl.textContent += event.content;
  // 保持参数区域滚动到最新内容
  argsEl.scrollTop = argsEl.scrollHeight;
  this.scrollToBottom();
};

FKTeamsChat.prototype.handleToolCalls = function (event) {
  const toolCalls = this.normalizeToolCallsForEvent(event);
  if (toolCalls.length === 0) return;

  if (this.isMemberRunEvent(event)) {
    const entry = this.memberEntryFromEvent(event);
    toolCalls.forEach((toolCall, i) => {
      const display = this.getToolDisplay(toolCall);
      if (toolCall.id) this.toolCallsByID[toolCall.id] = toolCall;
      if (toolCall.index !== undefined && toolCall.index !== null) this.toolCallsByIndex[String(toolCall.index)] = toolCall;
      const flowKey = this.resolveMemberToolFlowKey(entry, event, toolCall, i);
      this.registerMemberToolFlowAlias(entry, toolCall.name, flowKey);
      this.updateMemberToolFlowArgs(entry, flowKey, display.displayName, toolCall.arguments || "", false, event.sequence);
    });
    this.scrollToBottom();
    return;
  }

  // dispatch_tasks: 暂存任务列表，卡片在审批通过后由 dispatch_progress 触发创建
  const dispatchToolCall = toolCalls.find((toolCall) => toolCall.name === "dispatch_tasks" && toolCall.arguments);
  if (dispatchToolCall) {
    try {
      const args = JSON.parse(dispatchToolCall.arguments);
      if (args.tasks && args.tasks.length > 0) {
        this._pendingDispatchTasks = args.tasks;
        // 移除 tool-call 占位
        const toolCalls = this.messagesContainer.querySelectorAll(".tool-call");
        const lastToolCall = toolCalls[toolCalls.length - 1];
        if (lastToolCall) lastToolCall.remove();
        this.scrollToBottom();
        return;
      }
    } catch {
      /* fall through */
    }
  }

  toolCalls.forEach((toolCall, i) => {
    this.lastToolName = toolCall.name;
    const key = this.toolCallKey(toolCall, i);
    if (toolCall.id) this.toolCallsByID[toolCall.id] = toolCall;
    if (toolCall.index !== undefined && toolCall.index !== null) this.toolCallsByIndex[String(toolCall.index)] = toolCall;
    const display = this.getToolDisplay(toolCall);
    if (display.kind === "agent") {
      const agentName = this.agentNameFromTool(toolCall.name);
      const memberKey = this.memberKeyForToolCall(toolCall, i);
      if (toolCall.id && toolCall.index !== undefined && toolCall.index !== null) {
        this.migrateMemberCard("pending:" + toolCall.index, memberKey);
      }
      const entry = this.ensureMemberCard(memberKey, display.target || agentName, agentName, toolCall.index);
      this.parallelMemberByAgent[agentName] = memberKey;
      if (toolCall.id) this.parallelToolMemberByID[toolCall.id] = memberKey;
      if (toolCall.arguments) this.updateMemberTaskContent(entry, toolCall.arguments, false);
      return;
    }

    let pending = this.pendingToolCalls[key];
    if (!pending && toolCall.id && toolCall.index !== undefined && toolCall.index !== null) {
      const indexKey = "idx:" + toolCall.index;
      pending = this.pendingToolCalls[indexKey];
      if (pending) {
        this.pendingToolCalls[key] = pending;
        delete this.pendingToolCalls[indexKey];
        pending.el.setAttribute("data-tool-key", key);
        pending.el.setAttribute("data-tool-call-id", toolCall.id);
      }
    }
    if (!pending) {
      const fallbackKey = "fallback:" + i;
      pending = this.pendingToolCalls[fallbackKey];
      if (pending) {
        this.pendingToolCalls[key] = pending;
        delete this.pendingToolCalls[fallbackKey];
        pending.el.setAttribute("data-tool-key", key);
      }
    }
    const argsEl = pending?.el?.querySelector(".tool-call-args");
    if (argsEl && toolCall.arguments) {
      this.updateToolCallStatus(pending.el, "已调用");
      argsEl.classList.remove("streaming");
      try {
        var args = JSON.parse(toolCall.arguments);
        argsEl.textContent = JSON.stringify(args, null, 2);
      } catch (e) {
        argsEl.textContent = toolCall.arguments;
      }
    }
  });
  this.scrollToBottom();
};

FKTeamsChat.prototype.handleToolResult = function (event) {
  let content = event.content || "";
  if (this.isMemberRunEvent(event)) {
    const entry = this.memberEntryFromEvent(event);
    const toolCall = this.normalizeToolCallForEvent(event, null, 0);
    const display = this.getToolDisplay(toolCall);
    const flowKey = this.resolveMemberToolFlowKey(entry, event, toolCall, 0);
    this.registerMemberToolFlowAlias(entry, toolCall.name, flowKey);
    if (event.type === "tool_result_chunk") {
      const key = this.memberInnerResultKey(event);
      if (key) this.parallelMemberInnerResultChunks[key] = (this.parallelMemberInnerResultChunks[key] || "") + content;
      this.updateMemberToolFlowResult(entry, flowKey, display.displayName || "工具调用", content, true, event.sequence);
      return;
    }
    const key = this.memberInnerResultKey(event);
    const chunked = key ? this.parallelMemberInnerResultChunks[key] || "" : "";
    if (chunked) {
      content = content && !chunked.includes(content) ? chunked + content : chunked || content;
      delete this.parallelMemberInnerResultChunks[key];
    }
    if (content) this.updateMemberToolFlowResult(entry, flowKey, display.displayName || "工具调用", content, false, event.sequence);
    this.scrollToBottom();
    return;
  }
  const toolCall = event.tool_call_id ? this.toolCallsByID[event.tool_call_id] : null;
  const toolName = toolCall?.name || event.tool_name || this.lastToolName || "";
  const toolDisplay = this.getToolDisplay(toolCall || { name: toolName });

  if (toolDisplay.kind === "agent") {
    if (event.type === "tool_result_chunk") {
      if (event.tool_call_id) {
        this.parallelMemberResultChunks[event.tool_call_id] = (this.parallelMemberResultChunks[event.tool_call_id] || "") + content;
      }
      return;
    }
    const agentName = this.agentNameFromTool(toolName);
    const memberKey = this.parallelToolMemberByID[event.tool_call_id] || "call:" + event.tool_call_id;
    const entry = this.ensureMemberCard(memberKey, toolDisplay.target || agentName, agentName, event.member_order);
    const chunked = this.parallelMemberResultChunks[event.tool_call_id] || "";
    if (chunked) {
      content = content && !chunked.includes(content) ? chunked + content : chunked || content;
      delete this.parallelMemberResultChunks[event.tool_call_id];
    }
    if (this.isAgentToolErrorResult(content)) {
      if (content) this.setMemberFinalOutput(entry, content);
      this.updateMemberStatus(entry, "error", "失败");
    } else {
      if (content && !this.memberHasOutputContent(entry)) {
        this.setMemberFinalOutput(entry, content, event.sequence);
      }
      this.updateMemberStatus(entry, "done", "完成");
    }
    this.scrollToBottom();
    return;
  }

  // dispatch_tasks 专用渲染
  if (toolName === "dispatch_tasks") {
    // 移除实时进度容器
    const progress = document.getElementById("dispatch-progress");
    if (progress) progress.remove();
    const el = this.renderDispatchResult(content);
    if (el) {
      this.messagesContainer.appendChild(el);
      this.scrollToBottom();
      return;
    }
  }

  const resultCard = this.findToolCallCardByIdentity(event, toolCall);
  if (resultCard && this.appendToolResultToCard(resultCard, content, toolDisplay)) {
    this.scrollToBottom();
    return;
  }

  let formattedContent = content;

  try {
    const parsed = JSON.parse(content);
    formattedContent = JSON.stringify(parsed, null, 2);
    if (formattedContent.length > 2048) {
      formattedContent = formattedContent.substring(0, 2048) + "\n...";
    }
  } catch {
    if (content.length > 2048) {
      formattedContent = content.substring(0, 2048) + "\n...";
    }
  }

  const toolResultEl = document.createElement("div");
  toolResultEl.className = "tool-result";
  if (event.tool_call_id) toolResultEl.setAttribute("data-tool-call-id", event.tool_call_id);
  const resultTitle = toolDisplay.kind === "agent" ? "成员结果" : "执行结果";
  toolResultEl.innerHTML = `
        <div class="tool-result-header">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <polyline points="20 6 9 17 4 12"/>
            </svg>
            <span>${resultTitle}</span>
        </div>
        <pre class="tool-result-content">${this.escapeHtml(formattedContent)}</pre>
    `;
  this.messagesContainer.appendChild(toolResultEl);
  this.scrollToBottom();
};

// 初始化 dispatch 实时进度容器（从 tool_calls 解析得到全部任务）
FKTeamsChat.prototype._initDispatchProgress = function (tasks) {
  // 移除旧的进度容器（如有），确保第二批任务能正确创建新容器
  const old = document.getElementById("dispatch-progress");
  if (old) old.remove();

  const container = document.createElement("div");
  container.id = "dispatch-progress";
  container.className = "dispatch-result dispatch-progress-live";

  const header = document.createElement("div");
  header.className = "dispatch-header dispatch-status-partial";
  header.innerHTML = `<span class="dispatch-progress-title">并行分发 ${tasks.length} 个子任务</span>
        <span class="dispatch-progress-counter" data-total="${tasks.length}" data-done="0">0/${tasks.length}</span>`;
  container.appendChild(header);

  const cardsWrap = document.createElement("div");
  cardsWrap.className = "dispatch-cards";

  for (let i = 0; i < tasks.length; i++) {
    const t = tasks[i];
    const card = document.createElement("div");
    card.id = "dispatch-task-" + i;
    card.className = "dispatch-card dispatch-card-waiting";
    card.innerHTML = `
            <div class="dispatch-card-head">
                <span class="dispatch-card-status-dot"></span>
                <span class="dispatch-card-desc">${this.escapeHtml(t.description || "")}</span>
            </div>
            <div class="dispatch-card-detail" style="display:none">
                <div class="dispatch-card-ops-list"></div>
                <div class="dispatch-card-error" style="display:none"></div>
            </div>
        `;
    // 点击展开/收起详情
    card.addEventListener("click", function () {
      const detail = card.querySelector(".dispatch-card-detail");
      const hasCont =
        detail.querySelector(".dispatch-card-ops-list").childElementCount > 0 ||
        detail.querySelector(".dispatch-card-error").style.display !== "none";
      if (!hasCont) return;
      card.classList.toggle("dispatch-card-expanded");
      detail.style.display = card.classList.contains("dispatch-card-expanded")
        ? ""
        : "none";
    });
    cardsWrap.appendChild(card);
  }
  container.appendChild(cardsWrap);
  this.messagesContainer.appendChild(container);
};

// 渲染 dispatch_tasks 最终结果（替换实时进度）
FKTeamsChat.prototype.renderDispatchResult = function (content) {
  try {
    const data = JSON.parse(content);
    if (data.error) {
      const el = document.createElement("div");
      el.className = "dispatch-result";
      el.innerHTML = `<div class="dispatch-header dispatch-status-partial"><span>${this.escapeHtml(data.error)}</span></div>`;
      return el;
    }
    if (data.results) {
      return this._buildDispatchCards(data.results);
    }
  } catch {
    /* fallback */
  }
  return null;
};

FKTeamsChat.prototype._buildDispatchCards = function (results) {
  const total = results.length;
  const success = results.filter((r) => r.status === "success").length;
  const failed = total - success;

  const container = document.createElement("div");
  container.className = "dispatch-result";

  const statusClass =
    failed === 0 ? "dispatch-status-ok" : "dispatch-status-partial";
  const header = document.createElement("div");
  header.className = "dispatch-header " + statusClass;
  header.innerHTML = `<span>子任务分发完成: ${success}/${total} 成功${failed > 0 ? "，" + failed + " 失败" : ""}</span>`;
  container.appendChild(header);

  const cardsWrap = document.createElement("div");
  cardsWrap.className = "dispatch-cards";

  const self = this;
  for (const r of results) {
    const isOk = r.status === "success";
    const card = document.createElement("div");
    card.className =
      "dispatch-card " + (isOk ? "dispatch-card-done" : "dispatch-card-fail");

    // 操作摘要
    let opsText = "";
    if (r.operations && r.operations.length > 0) {
      opsText = `<span class="dispatch-card-ops-count">${r.operations.length} 项操作</span>`;
    }

    card.innerHTML = `
            <div class="dispatch-card-head">
                <span class="dispatch-card-status-dot"></span>
                <span class="dispatch-card-desc">${self.escapeHtml(r.description || "")}</span>
                ${opsText}
                <span class="dispatch-card-toggle"></span>
            </div>
            <div class="dispatch-card-detail" style="display:none"></div>
        `;

    // 构建详情面板
    const detailEl = card.querySelector(".dispatch-card-detail");
    let hasDetail = false;

    if (r.error) {
      const errDiv = document.createElement("div");
      errDiv.className = "dispatch-card-error";
      errDiv.textContent = r.error;
      detailEl.appendChild(errDiv);
      hasDetail = true;
    }

    if (r.operations && r.operations.length > 0) {
      const opsList = document.createElement("div");
      opsList.className = "dispatch-card-ops-list";
      for (const op of r.operations) {
        const line = document.createElement("div");
        line.className = "dispatch-card-op-item";
        line.textContent = op;
        opsList.appendChild(line);
      }
      detailEl.appendChild(opsList);
      hasDetail = true;
    }

    if (r.result) {
      const resDiv = document.createElement("div");
      resDiv.className = "dispatch-card-result markdown-body markdown-body-compact";
      resDiv.innerHTML = self.renderMarkdown(r.result);
      detailEl.appendChild(resDiv);
      hasDetail = true;
    }

    if (hasDetail) {
      card.style.cursor = "pointer";
      card.addEventListener("click", function () {
        card.classList.toggle("dispatch-card-expanded");
        detailEl.style.display = card.classList.contains(
          "dispatch-card-expanded",
        )
          ? ""
          : "none";
      });
    }

    cardsWrap.appendChild(card);
  }
  container.appendChild(cardsWrap);
  return container;
};

// 处理 dispatch 子任务实时进度事件
FKTeamsChat.prototype.handleDispatchProgress = function (event) {
  let detail;
  try {
    detail = JSON.parse(event.detail || "{}");
  } catch {
    return;
  }

  const idx = detail.task_index;
  const evtType = detail.event_type;
  const desc = detail.description || "";

  // 如果进度容器不存在，用暂存的任务列表创建（审批通过后首次收到进度事件时）
  let container = document.getElementById("dispatch-progress");
  if (!container) {
    const tasks = this._pendingDispatchTasks || [{ description: desc }];
    this._initDispatchProgress(tasks);
    this._pendingDispatchTasks = null;
    container = document.getElementById("dispatch-progress");
  }

  let card = document.getElementById("dispatch-task-" + idx);
  if (!card) {
    // 动态追加：任务列表不完整（回退场景）或新发现的任务
    const cardsWrap = container.querySelector(".dispatch-cards");
    const c = document.createElement("div");
    c.id = "dispatch-task-" + idx;
    c.className = "dispatch-card dispatch-card-waiting";
    c.innerHTML = `
            <div class="dispatch-card-head">
                <span class="dispatch-card-status-dot"></span>
                <span class="dispatch-card-desc">${this.escapeHtml(desc)}</span>
            </div>
            <div class="dispatch-card-detail" style="display:none">
                <div class="dispatch-card-ops-list"></div>
                <div class="dispatch-card-error" style="display:none"></div>
            </div>
        `;
    // 绑定展开/收起事件（与 _initDispatchProgress 中一致）
    c.addEventListener("click", function () {
      const detail = c.querySelector(".dispatch-card-detail");
      const hasCont =
        detail.querySelector(".dispatch-card-ops-list").childElementCount > 0 ||
        detail.querySelector(".dispatch-card-error").style.display !== "none";
      if (!hasCont) return;
      c.classList.toggle("dispatch-card-expanded");
      detail.style.display = c.classList.contains("dispatch-card-expanded")
        ? ""
        : "none";
    });
    cardsWrap.appendChild(c);
    card = c;
    // 更新 header 中的任务总数
    this._updateDispatchTotal(container);
  }

  const opsListEl = card.querySelector(".dispatch-card-ops-list");
  const errEl = card.querySelector(".dispatch-card-error");

  switch (evtType) {
    case "start":
      card.className = card.className.replace(
        /dispatch-card-waiting/,
        "dispatch-card-running",
      );
      break;
    case "op":
      if (opsListEl) {
        const line = document.createElement("div");
        line.className = "dispatch-card-op-item";
        line.textContent = detail.event_detail || "";
        opsListEl.appendChild(line);
      }
      break;
    case "content":
      // 内容会在最终 tool_result 中呈现，进度阶段不显示
      break;
    case "done": {
      card.className = card.className.replace(
        /dispatch-card-(waiting|running)/,
        "dispatch-card-done",
      );
      this._updateDispatchCounter(container, 1);
      break;
    }
    case "error":
      card.className = card.className.replace(
        /dispatch-card-(waiting|running)/,
        "dispatch-card-fail",
      );
      if (detail.event_detail && errEl) {
        errEl.style.display = "";
        errEl.textContent = detail.event_detail;
      }
      this._updateDispatchCounter(container, 1);
      break;
    case "timeout":
      card.className = card.className.replace(
        /dispatch-card-(waiting|running)/,
        "dispatch-card-fail",
      );
      if (errEl) {
        errEl.style.display = "";
        errEl.textContent = "任务超时";
      }
      this._updateDispatchCounter(container, 1);
      break;
  }

  this.scrollToBottom();
};

// 更新进度计数器
FKTeamsChat.prototype._updateDispatchCounter = function (container, increment) {
  const counter = container.querySelector(".dispatch-progress-counter");
  if (!counter) return;
  let done = parseInt(counter.getAttribute("data-done") || "0") + increment;
  const total = parseInt(counter.getAttribute("data-total") || "0");
  counter.setAttribute("data-done", done);
  counter.textContent = done + "/" + total;
  if (done >= total) {
    // 全部完成，移除闪烁
    container.classList.remove("dispatch-progress-live");
    const title = container.querySelector(".dispatch-progress-title");
    if (title) title.textContent = "子任务执行完成，等待汇总...";
  }
};

// 根据实际卡片数量更新 header 中的任务总数和标题
FKTeamsChat.prototype._updateDispatchTotal = function (container) {
  const cardsWrap = container.querySelector(".dispatch-cards");
  if (!cardsWrap) return;
  const actualTotal = cardsWrap.children.length;
  const counter = container.querySelector(".dispatch-progress-counter");
  if (counter) {
    counter.setAttribute("data-total", actualTotal);
    const done = parseInt(counter.getAttribute("data-done") || "0");
    counter.textContent = done + "/" + actualTotal;
  }
  const title = container.querySelector(".dispatch-progress-title");
  if (title && !title.textContent.includes("完成")) {
    title.textContent = "并行分发 " + actualTotal + " 个子任务";
  }
};

FKTeamsChat.prototype.handleAction = function (event) {
  if (!event.content && !event.action_type) {
    return;
  }

  let actionClass = "";
  let actionIcon = "";

  const compressIcon = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <polyline points="4 14 10 14 10 20"/><polyline points="20 10 14 10 14 4"/>
        <line x1="14" y1="10" x2="21" y2="3"/><line x1="3" y1="21" x2="10" y2="14"/>
    </svg>`;

  // 上下文压缩开始：创建带 ID 的临时卡片
  if (event.action_type === "context_compress_start") {
    const startEl = document.createElement("div");
    startEl.className = "action-event context-compress";
    startEl.id = "context-compress-pending";
    startEl.innerHTML = `${compressIcon}<span>[${this.escapeHtml(event.agent_name)}] ${this.escapeHtml(event.content || event.action_type)}</span>`;
    this.messagesContainer.appendChild(startEl);
    this.scrollToBottom();
    return;
  }

  // 上下文压缩完成：替换临时卡片为可展开的最终卡片
  if (event.action_type === "context_compress") {
    const pendingEl = document.getElementById("context-compress-pending");
    if (pendingEl) pendingEl.remove();

    const cardEl = document.createElement("div");
    cardEl.className = "action-event context-compress";
    if (event.detail) {
      cardEl.style.cursor = "pointer";
      cardEl.style.flexWrap = "wrap";
      const toggleIcon = `<svg class="toggle-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width:12px;height:12px;transition:transform 0.2s;margin-left:auto;">
                <polyline points="6 9 12 15 18 9"/>
            </svg>`;
      cardEl.innerHTML = `${compressIcon}<span>[${this.escapeHtml(event.agent_name)}] ${this.escapeHtml(event.content || event.action_type)}</span>${toggleIcon}
                <div class="compress-detail" style="display:none;width:100%;margin-top:8px;padding:10px;background:var(--bg-primary);border-radius:6px;font-size:12px;line-height:1.6;white-space:pre-wrap;word-break:break-word;color:var(--text-primary);max-height:300px;overflow-y:auto;">${this.escapeHtml(event.detail)}</div>`;
      cardEl.addEventListener("click", function () {
        const detail = cardEl.querySelector(".compress-detail");
        const toggle = cardEl.querySelector(".toggle-icon");
        if (detail.style.display === "none") {
          detail.style.display = "block";
          toggle.style.transform = "rotate(180deg)";
        } else {
          detail.style.display = "none";
          toggle.style.transform = "rotate(0deg)";
        }
      });
    } else {
      cardEl.innerHTML = `${compressIcon}<span>[${this.escapeHtml(event.agent_name)}] ${this.escapeHtml(event.content || event.action_type)}</span>`;
    }
    this.messagesContainer.appendChild(cardEl);
    this.scrollToBottom();
    return;
  }

  if (this.isMemberRunEvent(event)) {
    const entry = this.memberEntryFromEvent(event);
    if (event.action_type === "exit") {
      this.updateMemberStatus(entry, "done", "完成");
    } else if (event.content || event.action_type) {
      this.updateMemberActivity(entry, event.content || event.action_type);
      this.appendMemberTextEvent(entry, "action", "状态", event.content || event.action_type, event.sequence);
    }
    this.scrollToBottom();
    return;
  }

  switch (event.action_type) {
    case "transfer":
      actionClass = "transfer";
      actionIcon = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M17 1l4 4-4 4"/><path d="M3 11V9a4 4 0 0 1 4-4h14"/>
                <path d="M7 23l-4-4 4-4"/><path d="M21 13v2a4 4 0 0 1-4 4H3"/>
            </svg>`;
      break;
    case "exit":
      actionClass = "exit";
      actionIcon = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <polyline points="20 6 9 17 4 12"/>
            </svg>`;
      break;
    default:
      actionIcon = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/>
                <line x1="12" y1="16" x2="12.01" y2="16"/>
            </svg>`;
  }

  const actionEl = document.createElement("div");
  actionEl.className = `action-event ${actionClass}`;
  actionEl.innerHTML = `${actionIcon}<span>[${this.escapeHtml(event.agent_name)}] ${this.escapeHtml(event.content || event.action_type)}</span>`;
  this.messagesContainer.appendChild(actionEl);
  this.scrollToBottom();
};

FKTeamsChat.prototype.handleError = function (event) {
  this._resumePending = false;
  const errorMsg = event.error || "";
  if (errorMsg.includes("登录已过期")) {
    this.showAuthExpiredOverlay();
    return;
  }

  const errorEl = document.createElement("div");
  const isMaxIterations = errorMsg.includes("exceeds max iterations");

  if (isMaxIterations) {
    errorEl.className = "error-message error-continuable";
    errorEl.innerHTML = `
        <div class="error-continuable-content">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10"/>
                <line x1="12" y1="8" x2="12" y2="12"/>
                <line x1="12" y1="16" x2="12.01" y2="16"/>
            </svg>
            <span>${event.agent_name ? `[${this.escapeHtml(event.agent_name)}] ` : ""}执行步数已达上限，任务自动停止。</span>
        </div>
        <button class="continue-btn" onclick="app.continueAfterMaxIterations()">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <polygon points="5 3 19 12 5 21 5 3"/>
            </svg>
            继续
        </button>
    `;
  } else {
    errorEl.className = "error-message";
    errorEl.innerHTML = `
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10"/>
            <line x1="15" y1="9" x2="9" y2="15"/>
            <line x1="9" y1="9" x2="15" y2="15"/>
        </svg>
        <span>${event.agent_name ? `[${this.escapeHtml(event.agent_name)}] ` : ""}${this.escapeHtml(event.error)}</span>
    `;
  }

  this.messagesContainer.appendChild(errorEl);
  this.scrollToBottom();
  this.hideChatLoading();
  this.isProcessing = false;
  this.updateStatus("connected", "已连接");
  this.updateSendButtonState();
};

// 继续执行（达到最大步数后）
FKTeamsChat.prototype.continueAfterMaxIterations = function () {
  if (this.isProcessing) return;
  if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
    this.showNotification("连接已断开，请刷新页面", "error");
    return;
  }

  // 隐藏继续按钮，显示已继续状态
  const continuableErrors = this.messagesContainer.querySelectorAll(".error-continuable .continue-btn");
  continuableErrors.forEach(function (btn) {
    btn.disabled = true;
    btn.textContent = "继续中...";
    btn.style.opacity = "0.5";
    btn.style.cursor = "default";
  });

  const payload = {
    type: "chat",
    session_id: this.sessionId,
    message: "上一步因为执行步数达到上限被中断了。请回顾上文，找到中断点，从中断处继续完成任务。",
    mode: this.mode,
  };
  if (this.currentAgent) {
    payload.agent_name = this.currentAgent.name;
  }

  this.ws.send(JSON.stringify(payload));
  this.isProcessing = true;
  this.updateSendButtonState();
  this.updateStatus("processing", "处理中...");
  this.showThinkingIndicator();
};

FKTeamsChat.prototype.addUserMessage = function (content, attachments) {
  const messageEl = document.createElement("div");
  messageEl.className = "message user";
  messageEl.setAttribute("data-message-id", `msg-${Date.now()}`);

  let attachmentsHtml = "";
  if (attachments && attachments.length > 0) {
    const previews = attachments
      .map((att) => {
        if (att.type === "image") {
          return `<img class="attachment-preview-img" src="data:${att.mimeType};base64,${att.base64}" alt="uploaded image" />`;
        }
        return "";
      })
      .join("");
    if (previews) {
      attachmentsHtml = `<div class="message-attachments">${previews}</div>`;
    }
  }

  messageEl.innerHTML = `
        <div class="message-content">
            <div class="message-header">
                <span class="message-name">您</span>
                <span class="message-time">${this.getCurrentTime()}</span>
            </div>
            ${attachmentsHtml}
            <div class="message-body">${this.escapeHtml(content)}</div>
        </div>
    `;
  this.messagesContainer.appendChild(messageEl);

  // 添加到问题列表
  this.addQuestionToNav(content, messageEl);

  this.scrollToBottom();
};

FKTeamsChat.prototype.createAssistantMessage = function (
  agentName,
  timeInfo = null,
) {
  const messageEl = document.createElement("div");
  messageEl.className = "message assistant";
  messageEl.setAttribute("data-agent", agentName || "");

  // 如果提供了时间信息，使用历史时间；否则使用当前时间
  const timeDisplay = timeInfo
    ? this.formatHistoryTime(timeInfo)
    : this.getCurrentTime();

  messageEl.innerHTML = `
        <div class="message-content">
            <div class="message-header">
                <span class="message-name">${this.escapeHtml(agentName || "Assistant")}</span>
                <span class="agent-tag">${this.escapeHtml(agentName || "AI")}</span>
                <span class="message-time">${timeDisplay}</span>
            </div>
            <div class="message-body markdown-body"><span class="streaming-indicator"><span></span><span></span><span></span></span></div>
        </div>
    `;
  this.messagesContainer.appendChild(messageEl);
  this.scrollToBottom();
  return messageEl;
};

FKTeamsChat.prototype.cancelTask = function () {
  if (!this.isProcessing) return;
  if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
    this.showNotification("连接已断开，无法取消", "error");
    return;
  }

  // 发送取消消息（带 session_id 以支持多会话并发）
  this.ws.send(
    JSON.stringify({
      type: "cancel",
      session_id: this.sessionId,
    }),
  );

  this._cancelledSessionId = this.sessionId;
  this.showNotification("正在取消任务...", "info");
};

FKTeamsChat.prototype.handleCancelled = function (event) {
  this._resumePending = false;
  this._cancelledSessionId = event.session_id || this.sessionId;
  this.isProcessing = false;
  this.updateStatus("connected", "已连接");
  this.updateSendButtonState();
  this.cancelActiveMemberCards();
  this.resetParallelState();

  // 关闭审批弹窗
  var modal = document.getElementById("approval-modal");
  if (modal) modal.style.display = "none";

  // 禁用所有未提交的 ask 表单
  this._dismissPendingAskForms();

  this.renderCancelledNotice(event.message || "任务已取消");

  this.showNotification("任务已取消", "success");
};

FKTeamsChat.prototype.renderCancelledNotice = function (message) {
  const cancelEl = document.createElement("div");
  cancelEl.className = "action-event cancelled";
  cancelEl.innerHTML = `
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10"/>
            <line x1="15" y1="9" x2="9" y2="15"/>
            <line x1="9" y1="9" x2="15" y2="15"/>
        </svg>
        <span>${this.escapeHtml(message || "任务已取消")}</span>
    `;
  this.messagesContainer.appendChild(cancelEl);
  this.scrollToBottom();
};

// 关闭所有未提交的 ask 表单
FKTeamsChat.prototype._dismissPendingAskForms = function () {
  var forms = this.messagesContainer.querySelectorAll(
    ".inline-ask-form:not(.submitted)",
  );
  forms.forEach(function (form) {
    form.classList.add("submitted");
    form.querySelectorAll("input, textarea, button").forEach(function (el) {
      el.disabled = true;
    });
  });
};

// 在聊天区域显示审批请求卡片
FKTeamsChat.prototype.showApprovalRequest = function (message) {
  var el = document.createElement("div");
  el.className = "action-event approval-request";
  el.innerHTML = "<span>" + this.escapeHtml(message || "需要审批") + "</span>";
  this.messagesContainer.appendChild(el);
  this.scrollToBottom();
};

// 显示审批弹窗
FKTeamsChat.prototype.showApprovalDialog = function (message) {
  var modal = document.getElementById("approval-modal");
  var msgEl = document.getElementById("approval-message");
  msgEl.textContent = message || "需要审批";
  modal.style.display = "flex";

  // 更新状态提示
  this.updateStatus("processing", "等待审批...");
};

// 发送审批决定
FKTeamsChat.prototype.sendApprovalDecision = function (decision) {
  var modal = document.getElementById("approval-modal");
  modal.style.display = "none";

  if (this.ws && this.ws.readyState === WebSocket.OPEN) {
    this.ws.send(
      JSON.stringify({
        type: "approval",
        session_id: this.sessionId,
        decision: decision,
      }),
    );
  }

  // 在聊天中显示审批结果
  var labels = {
    0: "已拒绝",
    1: "已允许（一次）",
    2: "已允许（该项）",
    3: "已全部允许",
  };
  var classes = { 0: "rejected", 1: "approved", 2: "approved", 3: "approved" };

  var el = document.createElement("div");
  el.className = "action-event approval-result " + (classes[decision] || "");
  el.innerHTML =
    "<span>" + this.escapeHtml(labels[decision] || "审批完成") + "</span>";
  this.messagesContainer.appendChild(el);
  this.scrollToBottom();

  this.updateStatus("processing", "处理中...");
};

// 在聊天流中内联显示提问表单
FKTeamsChat.prototype.showInlineAskForm = function (event) {
  var self = this;

  // 自动关闭之前未提交的 ask 表单（防止多个表单同时显示）
  this._dismissPendingAskForms();

  var container = document.createElement("div");
  container.className = "inline-ask-form";

  // 问题文本
  var questionP = document.createElement("div");
  questionP.className = "inline-ask-question";
  questionP.textContent = event.question || "请回答以下问题";
  container.appendChild(questionP);

  var multiSelect = event.multi_select || false;
  var inputType = multiSelect ? "checkbox" : "radio";

  // 选项区域
  var optionsDiv = document.createElement("div");
  optionsDiv.className = "inline-ask-options";
  container.appendChild(optionsDiv);

  // 自由输入框
  var freeTextArea = document.createElement("textarea");
  freeTextArea.className = "inline-ask-free-text";
  freeTextArea.rows = 2;
  freeTextArea.placeholder = "输入自定义回答...";
  container.appendChild(freeTextArea);

  if (event.options && event.options.length > 0) {
    event.options.forEach(function (opt) {
      var label = document.createElement("label");
      label.className = "inline-ask-option-label";
      var input = document.createElement("input");
      input.type = inputType;
      input.name = "inline-ask-opt-" + Date.now();
      input.value = opt;
      input.className = "inline-ask-option-input";
      label.appendChild(input);
      label.appendChild(document.createTextNode(" " + opt));
      optionsDiv.appendChild(label);
    });

    // 自行输入选项
    var customLabel = document.createElement("label");
    customLabel.className = "inline-ask-option-label inline-ask-option-custom";
    var customInput = document.createElement("input");
    customInput.type = inputType;
    customInput.name = "inline-ask-opt-" + Date.now();
    customInput.value = "__custom__";
    customInput.className = "inline-ask-option-input";
    customLabel.appendChild(customInput);
    customLabel.appendChild(document.createTextNode(" 自行输入"));
    optionsDiv.appendChild(customLabel);

    freeTextArea.style.display = "none";
    optionsDiv.addEventListener("change", function () {
      var show = false;
      optionsDiv.querySelectorAll("input").forEach(function (inp) {
        if (inp.value === "__custom__" && inp.checked) show = true;
      });
      freeTextArea.style.display = show ? "block" : "none";
      if (show) freeTextArea.focus();
    });
  } else {
    freeTextArea.style.display = "block";
  }

  // 提交按钮
  var footer = document.createElement("div");
  footer.className = "inline-ask-footer";
  var submitBtn = document.createElement("button");
  submitBtn.className = "inline-ask-submit-btn";
  submitBtn.textContent = "提交回答";
  footer.appendChild(submitBtn);
  container.appendChild(footer);

  submitBtn.addEventListener("click", function () {
    var selected = [];
    var freeText = "";
    var checkedInputs = optionsDiv.querySelectorAll("input:checked");
    checkedInputs.forEach(function (inp) {
      if (inp.value !== "__custom__") selected.push(inp.value);
    });
    var customChecked = false;
    checkedInputs.forEach(function (inp) {
      if (inp.value === "__custom__") customChecked = true;
    });
    if (customChecked || optionsDiv.children.length === 0) {
      freeText = (freeTextArea.value || "").trim();
    }

    // 禁用表单
    submitBtn.disabled = true;
    submitBtn.textContent = "已提交";
    container.classList.add("submitted");
    optionsDiv.querySelectorAll("input").forEach(function (inp) {
      inp.disabled = true;
    });
    freeTextArea.disabled = true;

    if (self.ws && self.ws.readyState === WebSocket.OPEN) {
      self.ws.send(
        JSON.stringify({
          type: "ask_response",
          session_id: self.sessionId,
          ask_selected: selected,
          ask_free_text: freeText,
        }),
      );
    }

    // 显示回答摘要
    var parts = [];
    if (selected.length > 0) parts.push("选择: " + selected.join(", "));
    if (freeText) parts.push("输入: " + freeText);
    var summary = parts.length > 0 ? parts.join(" | ") : "已回答";
    var resultEl = document.createElement("div");
    resultEl.className = "inline-ask-result";
    resultEl.textContent = summary;
    container.appendChild(resultEl);

    self.updateStatus("processing", "处理中...");
    self.scrollToBottom();
  });

  this.messagesContainer.appendChild(container);
  this.updateStatus("processing", "等待回答...");
  this.scrollToBottom();
};

// 在聊天区域显示提问请求卡片（保留兼容，历史渲染用）
FKTeamsChat.prototype.showAskRequest = function (event) {
  var el = document.createElement("div");
  el.className = "action-event ask-request";
  el.innerHTML =
    "<span>[提问] " +
    this.escapeHtml(event.question || "模型有问题要问你") +
    "</span>";
  this.messagesContainer.appendChild(el);
  this.scrollToBottom();
};

// 显示提问弹窗
FKTeamsChat.prototype.showAskDialog = function (event) {
  var modal = document.getElementById("ask-modal");
  var questionEl = document.getElementById("ask-question");
  var optionsEl = document.getElementById("ask-options");
  var freeTextEl = document.getElementById("ask-free-text");
  var submitBtn = document.getElementById("ask-submit-btn");

  questionEl.textContent = event.question || "请回答以下问题";
  optionsEl.innerHTML = "";
  freeTextEl.value = "";

  var multiSelect = event.multi_select || false;
  var inputType = multiSelect ? "checkbox" : "radio";

  if (event.options && event.options.length > 0) {
    event.options.forEach(function (opt, idx) {
      var label = document.createElement("label");
      label.className = "ask-option-label";
      var input = document.createElement("input");
      input.type = inputType;
      input.name = "ask-option";
      input.value = opt;
      input.className = "ask-option-input";
      label.appendChild(input);
      label.appendChild(document.createTextNode(" " + opt));
      optionsEl.appendChild(label);
    });

    // 添加"自行输入"选项
    var customLabel = document.createElement("label");
    customLabel.className = "ask-option-label ask-option-custom";
    var customInput = document.createElement("input");
    customInput.type = inputType;
    customInput.name = "ask-option";
    customInput.value = "__custom__";
    customInput.className = "ask-option-input";
    customLabel.appendChild(customInput);
    customLabel.appendChild(document.createTextNode(" 自行输入"));
    optionsEl.appendChild(customLabel);

    // 自行输入选中时显示文本框
    freeTextEl.style.display = "none";
    optionsEl.addEventListener("change", function () {
      var inputs = optionsEl.querySelectorAll("input");
      var showFreeText = false;
      inputs.forEach(function (inp) {
        if (inp.value === "__custom__" && inp.checked) showFreeText = true;
      });
      freeTextEl.style.display = showFreeText ? "block" : "none";
      if (showFreeText) freeTextEl.focus();
    });
  } else {
    // 无选项，只有自由输入
    freeTextEl.style.display = "block";
    freeTextEl.placeholder = "请输入您的回答...";
  }

  // 重新绑定提交按钮
  var self = this;
  var newSubmitBtn = submitBtn.cloneNode(true);
  submitBtn.parentNode.replaceChild(newSubmitBtn, submitBtn);
  newSubmitBtn.addEventListener("click", function () {
    self.submitAskResponse(modal, optionsEl, freeTextEl);
  });

  modal.style.display = "flex";
  this.updateStatus("processing", "等待回答...");
};

// 提交提问回答
FKTeamsChat.prototype.submitAskResponse = function (
  modal,
  optionsEl,
  freeTextEl,
) {
  var selected = [];
  var freeText = "";

  var inputs = optionsEl.querySelectorAll("input:checked");
  inputs.forEach(function (inp) {
    if (inp.value !== "__custom__") {
      selected.push(inp.value);
    }
  });

  // 检查是否选了自行输入
  var customChecked = false;
  inputs.forEach(function (inp) {
    if (inp.value === "__custom__") customChecked = true;
  });
  if (customChecked || optionsEl.children.length === 0) {
    freeText = (freeTextEl.value || "").trim();
  }

  modal.style.display = "none";

  if (this.ws && this.ws.readyState === WebSocket.OPEN) {
    this.ws.send(
      JSON.stringify({
        type: "ask_response",
        session_id: this.sessionId,
        ask_selected: selected,
        ask_free_text: freeText,
      }),
    );
  }

  // 在聊天中显示回答结果
  var parts = [];
  if (selected.length > 0) parts.push("选择: " + selected.join(", "));
  if (freeText) parts.push("输入: " + freeText);
  var summary = parts.length > 0 ? parts.join(" | ") : "已回答";

  var el = document.createElement("div");
  el.className = "action-event ask-response approved";
  el.innerHTML = "<span>" + this.escapeHtml(summary) + "</span>";
  this.messagesContainer.appendChild(el);
  this.scrollToBottom();

  this.updateStatus("processing", "处理中...");
};

// 附件管理
FKTeamsChat.prototype.clearAttachments = function () {
  this.attachments = [];
  const preview = document.getElementById("attachments-preview");
  if (preview) {
    preview.innerHTML = "";
    preview.style.display = "none";
  }
  this.updateSendButtonState();
};

FKTeamsChat.prototype.removeAttachment = function (index) {
  this.attachments.splice(index, 1);
  this.renderAttachmentPreviews();
  this.updateSendButtonState();
};

FKTeamsChat.prototype.renderAttachmentPreviews = function () {
  const preview = document.getElementById("attachments-preview");
  if (!preview) return;

  if (this.attachments.length === 0) {
    preview.innerHTML = "";
    preview.style.display = "none";
    return;
  }

  preview.style.display = "flex";
  preview.innerHTML = this.attachments
    .map((att, i) => {
      if (att.type === "image") {
        return `<div class="attachment-item">
                <img src="data:${att.mimeType};base64,${att.base64}" alt="preview" />
                <button class="attachment-remove" onclick="app.removeAttachment(${i})">&times;</button>
            </div>`;
      }
      return "";
    })
    .join("");
};

FKTeamsChat.prototype.initFileUpload = function () {
  const fileInput = document.getElementById("file-upload");
  const uploadBtn = document.getElementById("upload-btn");
  if (!fileInput || !uploadBtn) return;

  uploadBtn.addEventListener("click", () => fileInput.click());

  fileInput.addEventListener("change", (e) => {
    const files = Array.from(e.target.files);
    files.forEach((file) => {
      if (!file.type.startsWith("image/")) return;
      const reader = new FileReader();
      reader.onload = (ev) => {
        const base64 = ev.target.result.split(",")[1];
        this.attachments.push({
          type: "image",
          mimeType: file.type,
          base64: base64,
          name: file.name,
        });
        this.renderAttachmentPreviews();
        this.updateSendButtonState();
      };
      reader.readAsDataURL(file);
    });
    fileInput.value = "";
  });
};

FKTeamsChat.prototype.clearChat = async function () {
  // 通过 REST API 删除会话历史
  if (this.sessionId) {
    try {
      await this.fetchWithAuth(
        `/api/fkteams/sessions/${encodeURIComponent(this.sessionId)}`,
        {
          method: "DELETE",
        },
      );
    } catch (error) {
      console.error("Error clearing history:", error);
    }
  }

  this.clearChatUI();
  // 刷新侧边栏历史
  this.loadSidebarHistory();
};

FKTeamsChat.prototype.clearChatUI = function () {
  // 只清空界面，不发送删除历史的消息到后端
  this.messagesContainer.innerHTML = `
        <div class="welcome-message">
            <div class="welcome-icon">
                <img src="/static/assets/fkteams-logo.svg" alt="" />
            </div>
            <h2>非空小队</h2>
            <p>多智能体协作系统，开始您的对话</p>
        </div>
    `;
  this.resetParallelState();

  // 隐藏回到底部按钮（切换到空页面时需要重置）
  this.showScrollToBottomBtn(false);
  this.userScrolledUp = false;

  // 清空问题导航
  this.clearQuickNav();
};

FKTeamsChat.prototype.exportToHTML = function () {
  const messagesContainer = document.getElementById("messages");
  if (!messagesContainer) return;

  // 获取当前会话ID用于文件名
  const sessionId = this.sessionId || "default";
  const timestamp = new Date().toISOString().slice(0, 19).replace(/[:.]/g, "-");
  const filename = `fkteams_chat_${sessionId}_${timestamp}.html`;

  // 创建HTML模板
  const htmlTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>非空小队对话记录 - ${sessionId}</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Noto Sans SC', sans-serif;
            line-height: 1.6;
            max-width: 900px;
            margin: 0 auto;
            padding: 20px;
            background: #fafafa;
            color: #333;
        }
        .header {
            text-align: center;
            margin-bottom: 30px;
            padding-bottom: 20px;
            border-bottom: 2px solid #e5e5e5;
        }
        .header h1 {
            color: #5c6bc0;
            margin-bottom: 10px;
        }
        .header .info {
            color: #666;
            font-size: 14px;
        }
        svg {
            width: 16px;
            height: 16px;
            flex-shrink: 0;
        }
        .tool-call-header svg,
        .tool-result-header svg,
        .action-event svg {
            width: 14px;
            height: 14px;
        }
        .message {
            margin-bottom: 20px;
            animation: fadeIn 0.3s ease;
        }
        .message-header {
            display: flex;
            align-items: center;
            gap: 8px;
            margin-bottom: 8px;
        }
        .message-name {
            font-weight: 600;
            color: #333;
        }
        .agent-tag {
            background: #e8eaf6;
            color: #5c6bc0;
            padding: 2px 6px;
            border-radius: 3px;
            font-size: 11px;
            font-weight: 500;
        }
        .message-time {
            color: #999;
            font-size: 11px;
        }
        .message-body {
            padding: 12px 16px;
            border-radius: 8px;
            background: #fff;
            border: 1px solid #e5e5e5;
            word-break: break-word;
        }
        .message.user .message-body {
            background: #5c6bc0;
            color: white;
            margin-left: 60px;
        }
        .tool-call, .tool-result {
            margin: 8px 0;
            padding: 10px 12px;
            border-radius: 6px;
            font-size: 13px;
        }
        .tool-call {
            background: #e3f2fd;
            border: 1px solid #42a5f5;
        }
        .tool-result {
            background: #f5f5f5;
            border: 1px solid #e5e5e5;
        }
        .action-event {
            padding: 8px 12px;
            background: #fff3e0;
            border-radius: 6px;
            color: #ffa726;
            margin: 8px 0;
        }
        @keyframes fadeIn {
            from { opacity: 0; transform: translateY(10px); }
            to { opacity: 1; transform: translateY(0); }
        }
        pre {
            background: #f6f8fa;
            padding: 12px;
            border-radius: 6px;
            overflow-x: auto;
        }
        code {
            background: rgba(0,0,0,0.06);
            padding: 2px 6px;
            border-radius: 3px;
            font-family: 'SF Mono', Monaco, Consolas, monospace;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>非空小队对话记录</h1>
        <div class="info">
            <div>会话ID: ${sessionId}</div>
            <div>导出时间: ${new Date().toLocaleString("zh-CN")}</div>
        </div>
    </div>
    <div class="messages">
        ${messagesContainer.innerHTML}
    </div>
</body>
</html>`;

  // 创建并下载文件
  const blob = new Blob([htmlTemplate], { type: "text/html;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);

  // 显示成功提示
  this.showNotification(`对话记录已导出为 ${filename}`, "success");
};
