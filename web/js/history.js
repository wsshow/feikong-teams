/**
 * history.js - 历史记录管理
 */

// ===== 新增会话 =====

FKTeamsChat.prototype.showHomePage = function (options) {
  const clearStoredSession = options?.clearStoredSession !== false;
  if (clearStoredSession) {
    localStorage.removeItem("fk_session_id");
  }
  this._startupSessionId = "";
  if (!options?.skipSaveCurrentDOM) {
    this._saveSessionDOM();
  }
  this.sessionId = "";
  this._hasLoadedSession = false;
  this.isProcessing = false;
  if (this.sessionIdInput) {
    this.sessionIdInput.value = "";
  }
  if (typeof this.handleQueueUpdated === "function") {
    this.handleQueueUpdated({ queue: [] });
  }
  if (typeof this.updateStatus === "function" && this.ws && this.ws.readyState === 1) {
    this.updateStatus("connected", "已连接");
  }
  if (typeof this.updateSendButtonState === "function") {
    this.updateSendButtonState();
  }
  this.hideChatLoading();
  this.clearChatUI();
  this.updateSidebarSessionActive();
};

FKTeamsChat.prototype.createNewSession = async function (silent, title) {
  // 保存当前会话的 DOM 状态
  this._saveSessionDOM();

  // 从后端创建会话并获取 UUID
  let newSessionId;
  try {
    const resp = await this.fetchWithAuth("/api/fkteams/sessions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ title: title || "" }),
    });
    const result = await resp.json();
    if (result.code !== 0 || !result.data || !result.data.session_id) {
      this.showNotification("创建会话失败", "error");
      return;
    }
    newSessionId = result.data.session_id;
  } catch (err) {
    console.error("Error creating session:", err);
    this.showNotification("创建会话失败", "error");
    return;
  }

  this.sessionId = newSessionId;
  this.sessionIdInput.value = newSessionId;
  localStorage.setItem("fk_session_id", newSessionId);
  this._hasLoadedSession = true;
  this.setCurrentAgent(null, true); // 新建会话，重置智能体并持久化

  // 同步处理状态
  this.isProcessing = false;
  this.updateStatus("connected", "已连接");
  this.updateSendButtonState();

  this.clearChatUI();
  this.hideChatLoading();
  this.loadSidebarHistory();
};

// ===== 侧边栏历史会话 =====

FKTeamsChat.prototype.loadSidebarHistory = function () {
  this.showSidebarHistoryLoading();
  // 防抖：短时间多次调用只执行最后一次，避免频繁请求
  this.debounce(
    "sidebarHistory",
    () => {
      this._doLoadSidebarHistory();
    },
    300,
  );
};

FKTeamsChat.prototype.showSidebarHistoryLoading = function () {
  if (!this.sidebarSessionList) return;
  this.sidebarSessionList.classList.add("loading");
  this.sidebarSessionList.innerHTML =
    '<div class="sidebar-session-loading">' +
    '<span class="sidebar-session-loading-spinner"></span>' +
    '<span>加载中...</span>' +
    "</div>";
};

FKTeamsChat.prototype._doLoadSidebarHistory = async function () {
  if (!this.sidebarSessionList) return;

  try {
    const response = await this.fetchWithAuth("/api/fkteams/sessions");
    if (!response.ok) {
      this.sidebarSessionList.classList.remove("loading");
      this.sidebarSessionList.innerHTML =
        '<div class="sidebar-session-empty">加载失败</div>';
      return;
    }

    const result = await response.json();
    if (result.code !== 0 || !result.data || !result.data.sessions) {
      this.sidebarSessionList.classList.remove("loading");
      this.sidebarSessionList.innerHTML =
        '<div class="sidebar-session-empty">暂无会话记录</div>';
      return;
    }

    this.renderSidebarSessions(result.data.sessions);
  } catch (error) {
    console.error("Error loading sidebar history:", error);
    this.sidebarSessionList.classList.remove("loading");
    this.sidebarSessionList.innerHTML =
      '<div class="sidebar-session-empty">加载失败</div>';
  }
};

FKTeamsChat.prototype.renderSidebarSessions = function (files) {
  if (!this.sidebarSessionList) return;
  this.sidebarSessionList.classList.remove("loading");
  if (!this._sidebarMenuOutsideBound) {
    this._sidebarMenuOutsideBound = true;
    document.addEventListener("click", () => this.closeSidebarSessionMenus());
  }

  if (!files || files.length === 0) {
    this.sidebarSessionList.innerHTML =
      '<div class="sidebar-session-empty">暂无会话记录</div>';
    return;
  }

  // 按修改时间排序（最新的在前）
  files.sort((a, b) => new Date(b.mod_time) - new Date(a.mod_time));

  this.sidebarSessionList.innerHTML = "";

  files.forEach((file) => {
    const item = document.createElement("div");
    item.className = "sidebar-session-item";
    item.setAttribute("data-session-id", file.session_id);

    // 判断是否是当前活动会话
    if (this._hasLoadedSession && file.session_id === this.sessionId) {
      item.classList.add("active");
    }

    // 判断是否正在处理中（优先用实时状态，其次用后端 status）
    const isProcessing =
      (this._processingSessions &&
        this._processingSessions.has(file.session_id)) ||
      file.status === "processing";
    if (isProcessing) {
      if (!this._processingSessions) this._processingSessions = new Set();
      this._processingSessions.add(file.session_id);
    }
    if (isProcessing) {
      item.classList.add("processing");
    }

    // 状态文本
    let statusHtml = "";
    if (isProcessing) {
      statusHtml =
        '<span class="session-status-label processing">处理中</span>';
    } else {
      const statusInfo = this.getSidebarSessionStatusInfo(file.status);
      if (statusInfo) {
        statusHtml = `<span class="session-status-label ${statusInfo.className}">${statusInfo.label}</span>`;
      }
    }

    item.innerHTML = `
            <svg class="session-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>
            </svg>
            <div class="sidebar-session-info">
                <div class="sidebar-session-name">${this.escapeHtml(file.title)}</div>
                <div class="sidebar-session-time">${this.formatTime(file.mod_time)}${statusHtml ? " · " + statusHtml : ""}</div>
            </div>
            <div class="sidebar-session-actions">
                <button class="sidebar-session-menu-trigger" data-tooltip="更多操作" aria-label="更多操作">
                    <svg viewBox="0 0 24 24" fill="currentColor">
                        <circle cx="12" cy="5" r="1.8"/>
                        <circle cx="12" cy="12" r="1.8"/>
                        <circle cx="12" cy="19" r="1.8"/>
                    </svg>
                </button>
                <div class="sidebar-session-menu">
                    <button class="sidebar-session-menu-item share-action">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <circle cx="18" cy="5" r="3"/>
                            <circle cx="6" cy="12" r="3"/>
                            <circle cx="18" cy="19" r="3"/>
                            <line x1="8.59" y1="13.51" x2="15.42" y2="17.49"/>
                            <line x1="15.41" y1="6.51" x2="8.59" y2="10.49"/>
                        </svg>
                        <span>分享</span>
                    </button>
                    <button class="sidebar-session-menu-item rename-action">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <path d="M12 20h9M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"/>
                        </svg>
                        <span>重命名</span>
                    </button>
                    <button class="sidebar-session-menu-item delete-action">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <polyline points="3 6 5 6 21 6"/>
                            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
                        </svg>
                        <span>删除</span>
                    </button>
                </div>
            </div>
        `;

    // 点击加载会话
    item.addEventListener("click", (e) => {
      // 如果点击的是操作按钮，不加载会话
      if (e.target.closest(".sidebar-session-actions")) return;
      this.loadSidebarSession(file.session_id);
    });

    const menuTrigger = item.querySelector(".sidebar-session-menu-trigger");
    menuTrigger.addEventListener("click", (e) => {
      e.stopPropagation();
      const isOpen = item.classList.contains("menu-open");
      this.closeSidebarSessionMenus();
      if (!isOpen) {
        item.classList.add("menu-open");
      }
    });

    // 分享按钮
    item.querySelector(".share-action").addEventListener("click", (e) => {
      e.stopPropagation();
      this.closeSidebarSessionMenus();
      this.showSessionShareModal(file.session_id, file.title);
    });

    // 重命名按钮
    item.querySelector(".rename-action").addEventListener("click", (e) => {
      e.stopPropagation();
      this.closeSidebarSessionMenus();
      this.renameSession(file.session_id, file.title);
    });

    // 删除按钮
    item.querySelector(".delete-action").addEventListener("click", (e) => {
      e.stopPropagation();
      this.closeSidebarSessionMenus();
      this.deleteSession(file.session_id);
    });

    this.sidebarSessionList.appendChild(item);
  });
};

FKTeamsChat.prototype.getSidebarSessionStatusInfo = function (status) {
  const map = {
    completed: { label: "已完成", className: "completed" },
    error: { label: "失败", className: "error" },
    cancelled: { label: "已取消", className: "cancelled" },
    idle: { label: "未开始", className: "idle" },
    active: { label: "已保存", className: "active" },
  };
  return map[status] || null;
};

FKTeamsChat.prototype.closeSidebarSessionMenus = function () {
  if (!this.sidebarSessionList) return;
  this.sidebarSessionList.querySelectorAll(".sidebar-session-item.menu-open").forEach((item) => {
    item.classList.remove("menu-open");
  });
};

FKTeamsChat.prototype.loadSidebarSession = function (sessionId) {
  // 如果已经是当前会话，不需要切换
  if (this.sessionId === sessionId && this._hasLoadedSession) {
    return;
  }

  // 移动端切换会话时自动折叠侧边栏，避免 loading 遮挡
  if (window.innerWidth <= 768 && !this.sidebar.classList.contains("collapsed")) {
    this.toggleSidebar();
  }

  // 保存当前会话的 DOM 状态
  this._saveSessionDOM();

  this.sessionId = sessionId;
  localStorage.setItem("fk_session_id", sessionId);
  this._hasLoadedSession = true;
  this.sessionIdInput.value = sessionId;

  // 同步处理状态
  const isSessionProcessing =
    this._processingSessions && this._processingSessions.has(sessionId);
  this.isProcessing = isSessionProcessing;
  if (isSessionProcessing) {
    this.updateStatus("processing", "处理中...");
  } else {
    this.updateStatus("connected", "已连接");
  }
  this.updateSendButtonState();

  // 尝试从 DOM 缓存恢复
  if (this._restoreSessionDOM(sessionId)) {
    this.updateSidebarSessionActive();
    return;
  }

  // 缓存中没有，从服务器加载历史
  this.loadSession(sessionId);

  // 更新侧边栏高亮
  this.updateSidebarSessionActive();
};

// 保存当前会话的 DOM 到缓存（使用 DOM 节点保留引用）
FKTeamsChat.prototype._saveSessionDOM = function () {
  if (!this.sessionId || !this._hasLoadedSession) return;
  // 不缓存欢迎页面（空会话）
  if (this.messagesContainer.querySelector(".welcome-message")) return;

  // 将所有子节点移到 DocumentFragment 保留 DOM 引用
  const fragment = document.createDocumentFragment();
  while (this.messagesContainer.firstChild) {
    fragment.appendChild(this.messagesContainer.firstChild);
  }

  this._sessionDOMCache[this.sessionId] = {
    fragment: fragment,
    scrollTop: this.mainContent ? this.mainContent.scrollTop : 0,
    currentMessageElement: this.currentMessageElement,
    currentMessageElements: this.currentMessageElements,
    pendingToolCalls: this.pendingToolCalls,
    toolCallsByID: this.toolCallsByID,
    parallelPanel: this.parallelPanel,
    parallelMemberCards: this.parallelMemberCards,
    parallelMemberByAgent: this.parallelMemberByAgent,
    parallelToolMemberByID: this.parallelToolMemberByID,
    parallelPanelBatchMode: this.parallelPanelBatchMode,
    hasToolCallAfterMessage: this.hasToolCallAfterMessage,
    userQuestions: [...this.userQuestions],
    currentAgent: this.currentAgent,
  };
};

// 从缓存恢复会话 DOM，成功返回 true
FKTeamsChat.prototype._restoreSessionDOM = function (sessionId) {
  const cached = this._sessionDOMCache[sessionId];
  if (!cached || !cached.fragment) return false;

  // 清空当前内容，恢复缓存的 DOM 节点
  this.messagesContainer.innerHTML = "";
  this.messagesContainer.appendChild(cached.fragment);
  this.currentMessageElement = cached.currentMessageElement;
  this.currentMessageElements = cached.currentMessageElements || {};
  this.pendingToolCalls = cached.pendingToolCalls || {};
  this.toolCallsByID = cached.toolCallsByID || {};
  this.parallelPanel = cached.parallelPanel || null;
  this.parallelMemberCards = cached.parallelMemberCards || {};
  this.parallelMemberByAgent = cached.parallelMemberByAgent || {};
  this.parallelToolMemberByID = cached.parallelToolMemberByID || {};
  this.parallelPanelBatchMode = cached.parallelPanelBatchMode || false;
  this.hasToolCallAfterMessage = cached.hasToolCallAfterMessage;
  this.userQuestions = cached.userQuestions || [];
  this.setCurrentAgent(cached.currentAgent || null, false); // 从缓存还原，仅更新 UI
  this.updateQuickNav();
  this.hideChatLoading();

  // 删除已恢复的缓存
  delete this._sessionDOMCache[sessionId];

  // 回放缓冲的事件（切换期间未渲染的流式内容等）
  if (this._sessionEventBuffer && this._sessionEventBuffer[sessionId]) {
    const events = this._sessionEventBuffer[sessionId];
    delete this._sessionEventBuffer[sessionId];
    for (const event of events) {
      this.handleServerEvent(event);
    }
  }

  // 恢复滚动位置
  if (this.mainContent && cached.scrollTop) {
    requestAnimationFrame(() => {
      this.mainContent.scrollTop = cached.scrollTop;
    });
  }
  return true;
};

FKTeamsChat.prototype.updateSidebarSessionActive = function () {
  if (!this.sidebarSessionList) return;
  if (!this._hasLoadedSession) return;

  const items = this.sidebarSessionList.querySelectorAll(
    ".sidebar-session-item",
  );
  items.forEach((item) => {
    const itemSessionId = item.getAttribute("data-session-id");
    if (itemSessionId === this.sessionId) {
      item.classList.add("active");
    } else {
      item.classList.remove("active");
    }
  });
};

FKTeamsChat.prototype.removeSidebarSessionItem = function (sessionId) {
  if (!this.sidebarSessionList) return;
  if (this._processingSessions) this._processingSessions.delete(sessionId);

  const items = Array.from(
    this.sidebarSessionList.querySelectorAll?.(".sidebar-session-item") || [],
  );
  const item = items.find((el) => {
    if (el.dataset?.sessionId) return el.dataset.sessionId === sessionId;
    if (typeof el.getAttribute === "function") {
      return el.getAttribute("data-session-id") === sessionId;
    }
    return false;
  });
  if (item && typeof item.remove === "function") {
    item.remove();
  }

  const remainingItems = Array.from(
    this.sidebarSessionList.querySelectorAll?.(".sidebar-session-item") || [],
  );
  if (remainingItems.length === 0) {
    this.sidebarSessionList.classList?.remove("loading");
    this.sidebarSessionList.innerHTML =
      '<div class="sidebar-session-empty">暂无会话记录</div>';
  }
};

// ===== 历史记录弹窗管理 =====

FKTeamsChat.prototype.showHistoryModal = async function () {
  this.historyModal.style.display = "flex";
  // 清空搜索框
  if (this.historySearchInput) {
    this.historySearchInput.value = "";
    // 绑定搜索事件（防止重复绑定）
    if (!this._historySearchBound) {
      this._historySearchBound = true;
      this.historySearchInput.addEventListener("input", () => {
        this.filterSessionList();
      });
    }
  }
  await this.loadSessions();
};

FKTeamsChat.prototype.hideHistoryModal = function () {
  this.historyModal.style.display = "none";
};

FKTeamsChat.prototype.loadSessions = async function () {
  this.historyList.innerHTML = '<div class="history-loading">加载中...</div>';

  try {
    const response = await this.fetchWithAuth("/api/fkteams/sessions");
    if (!response.ok) {
      throw new Error("加载失败");
    }

    const result = await response.json();
    if (result.code !== 0) {
      throw new Error(result.message || "加载失败");
    }
    // 缓存文件列表用于搜索过滤
    this._sessionList = result.data.sessions || [];
    this.renderSessionList(this._sessionList);
  } catch (error) {
    console.error("Error loading history files:", error);
    this.historyList.innerHTML =
      '<div class="history-error">加载历史文件失败</div>';
  }
};

FKTeamsChat.prototype.renderSessionList = function (files) {
  if (!files || files.length === 0) {
    this.historyList.innerHTML =
      '<div class="history-empty">暂无历史记录</div>';
    return;
  }

  // 按修改时间排序（最新的在前）
  files.sort((a, b) => new Date(b.mod_time) - new Date(a.mod_time));

  const listHTML = files
    .map(
      (file) => `
        <div class="history-item" data-session-id="${this.escapeHtml(file.session_id)}">
            <div class="history-item-info">
                <div class="history-item-name">${this.escapeHtml(file.title)}</div>
                <div class="history-item-meta">
                    <span class="history-item-time">${this.formatTime(file.mod_time)}</span>
                    <span class="history-item-size">${this.formatSize(file.size)}</span>
                </div>
            </div>
            <div class="history-item-actions">
                <button class="history-action-btn load-btn" data-tooltip="加载并切换到该会话">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M5 3l14 9-14 9V3z"/>
                    </svg>
                </button>
                <button class="history-action-btn export-btn" data-tooltip="导出为 HTML 文件">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                        <polyline points="7 10 12 15 17 10"/>
                        <line x1="12" y1="15" x2="12" y2="3"/>
                    </svg>
                </button>
                <button class="history-action-btn share-btn" data-tooltip="分享该会话">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <circle cx="18" cy="5" r="3"/>
                        <circle cx="6" cy="12" r="3"/>
                        <circle cx="18" cy="19" r="3"/>
                        <line x1="8.59" y1="13.51" x2="15.42" y2="17.49"/>
                        <line x1="15.41" y1="6.51" x2="8.59" y2="10.49"/>
                    </svg>
                </button>
                <button class="history-action-btn rename-btn" data-tooltip="重命名该会话">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M12 20h9M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"/>
                    </svg>
                </button>
                <button class="history-action-btn delete-btn" data-tooltip="删除该会话">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <polyline points="3 6 5 6 21 6"/>
                        <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
                    </svg>
                </button>
            </div>
        </div>
    `,
    )
    .join("");

  this.historyList.innerHTML = listHTML;

  // 绑定事件
  this.historyList.querySelectorAll(".load-btn").forEach((btn) => {
    btn.addEventListener("click", (e) => {
      const item = e.target.closest(".history-item");
      const sessionId = item.dataset.sessionId;
      this.loadSession(sessionId);
    });
  });

  this.historyList.querySelectorAll(".export-btn").forEach((btn) => {
    btn.addEventListener("click", (e) => {
      const item = e.target.closest(".history-item");
      const sessionId = item.dataset.sessionId;
      this.exportSession(sessionId);
    });
  });

  this.historyList.querySelectorAll(".share-btn").forEach((btn) => {
    btn.addEventListener("click", (e) => {
      const item = e.target.closest(".history-item");
      const sessionId = item.dataset.sessionId;
      const title =
        item.querySelector(".history-item-name")?.textContent || sessionId;
      this.showSessionShareModal(sessionId, title);
    });
  });

  this.historyList.querySelectorAll(".rename-btn").forEach((btn) => {
    btn.addEventListener("click", (e) => {
      const item = e.target.closest(".history-item");
      const sessionId = item.dataset.sessionId;
      // 从 DOM 获取当前显示名用于编辑提示
      const title =
        item.querySelector(".history-item-name")?.textContent || sessionId;
      this.renameSession(sessionId, title);
    });
  });

  this.historyList.querySelectorAll(".delete-btn").forEach((btn) => {
    btn.addEventListener("click", (e) => {
      const item = e.target.closest(".history-item");
      const sessionId = item.dataset.sessionId;
      this.deleteSession(sessionId);
    });
  });
};

// ===== 历史记录搜索过滤 =====
FKTeamsChat.prototype.filterSessionList = function () {
  const query = (this.historySearchInput?.value || "").trim().toLowerCase();
  if (!this._sessionList) return;

  if (!query) {
    this.renderSessionList(this._sessionList);
    return;
  }

  const filtered = this._sessionList.filter((file) => {
    const name = (file.title || file.session_id || "").toLowerCase();
    return name.includes(query);
  });

  this.renderSessionList(filtered);
};

// ===== 导出单个历史会话 =====
FKTeamsChat.prototype.exportSession = async function (sessionId) {
  try {
    const response = await this.fetchWithAuth(
      `/api/fkteams/sessions/${encodeURIComponent(sessionId)}`,
    );
    if (!response.ok) {
      throw new Error("无法获取历史文件");
    }

    const result = await response.json();
    if (result.code !== 0) {
      throw new Error(result.message || "获取历史文件失败");
    }

    const messages = result.data?.messages || [];
    this.generateExportHTML(sessionId, messages);
  } catch (error) {
    console.error("Error exporting history file:", error);
    this.showNotification("导出失败: " + error.message, "error");
  }
};

FKTeamsChat.prototype.generateExportHTML = function (sessionId, agentMessages) {
  const timestamp = new Date().toISOString().slice(0, 19).replace(/[:.]/g, "-");
  const exportFilename = `fkteams_chat_${sessionId}_${timestamp}.html`;

  // 按事件顺序渲染每条 AgentMessage
  let messagesHTML = "";
  if (Array.isArray(agentMessages)) {
    agentMessages.forEach((msg) => {
      const agentName = msg.agent_name || "unknown";
      const isUser = agentName === "用户";
      const startTime = msg.start_time
        ? new Date(msg.start_time).toLocaleString("zh-CN")
        : "";

      if (!msg.events || msg.events.length === 0) return;

      // 用户消息：直接提取文本
      if (isUser) {
        let userContent = "";
        msg.events.forEach((evt) => {
          if (evt.type === "text" && evt.content) userContent += evt.content;
        });
        if (!userContent) return;
        messagesHTML += `
                    <div class="message user">
                        <div class="message-header">
                            <span class="message-name">您</span>
                            ${startTime ? `<span class="message-time">${startTime}</span>` : ""}
                        </div>
                        <div class="message-body user-body">${this.escapeHtml(userContent)}</div>
                    </div>`;
        return;
      }

      // Agent 消息：按事件顺序逐个渲染，保持 text / tool_call / action 的交错时间线
      let currentTextBlock = "";
      const flushText = () => {
        if (!currentTextBlock) return "";
        // 直接用 renderMarkdown 预渲染，避免 data 属性的引号转义问题
        const rendered = this.renderMarkdown(currentTextBlock);
        const html = `
                    <div class="message">
                        <div class="message-header">
                            <span class="message-name">${this.escapeHtml(agentName)}</span>
                            ${msg.run_path ? `<span class="agent-tag">${this.escapeHtml(msg.run_path)}</span>` : ""}
                            ${startTime ? `<span class="message-time">${startTime}</span>` : ""}
                        </div>
                        <div class="message-body markdown-body">${rendered}</div>
                    </div>`;
        currentTextBlock = "";
        return html;
      };

      msg.events.forEach((evt) => {
        switch (evt.type) {
          case "reasoning":
            // 在导出HTML中渲染推理内容
            messagesHTML += flushText();
            if (evt.content) {
              const rendered = this.renderMarkdown(evt.content);
              messagesHTML += `
                            <div class="message assistant" data-agent="${this.escapeHtml(msg.agent_name || "")}">
                                <div class="message-content">
                                    <div class="message-header"><span class="message-name">${this.escapeHtml(msg.agent_name || "Assistant")}</span></div>
                                    <div class="message-body"><details><summary>思考过程</summary>${rendered}</details></div>
                                </div>
                            </div>`;
            }
            break;
          case "text":
            currentTextBlock += evt.content || "";
            break;
          case "tool_call":
            // 先输出之前累积的文本
            messagesHTML += flushText();
            // 渲染工具调用
            if (evt.tool_call) {
              const tc = evt.tool_call;
              let argsDisplay = tc.arguments || "";
              try {
                argsDisplay = JSON.stringify(JSON.parse(tc.arguments), null, 2);
              } catch {
                /* 保持原样 */
              }

              let resultHTML = "";
              if (tc.result) {
                let formattedResult = tc.result;
                try {
                  const parsed = JSON.parse(tc.result);
                  formattedResult = JSON.stringify(parsed, null, 2);
                } catch {
                  /* 保持原样 */
                }
                if (formattedResult.length > 2048) {
                  formattedResult =
                    formattedResult.substring(0, 2048) + "\n...";
                }
                resultHTML = `
                                    <div class="tool-result">
                                        <div class="tool-result-header">执行结果</div>
                                        <pre class="tool-result-content">${this.escapeHtml(formattedResult)}</pre>
                                    </div>`;
              }

              messagesHTML += `
                                <div class="tool-call">
                                    <div class="tool-call-header">工具调用: <code>${this.escapeHtml(tc.name || "tool")}</code></div>
                                    ${argsDisplay ? `<pre class="tool-call-args">${this.escapeHtml(argsDisplay)}</pre>` : ""}
                                    ${resultHTML}
                                </div>`;
            }
            break;
          case "action":
            messagesHTML += flushText();
            if (evt.action) {
              if (!evt.action.content && !evt.action.action_type) break;
              const actionLabel =
                evt.action.content || evt.action.action_type || "action";
              messagesHTML += `
                                <div class="action-event">
                                    <span>[${this.escapeHtml(agentName)}] ${this.escapeHtml(actionLabel)}</span>
                                </div>`;
            }
            break;
        }
      });
      // 输出尾部未 flush 的文本
      messagesHTML += flushText();
    });
  }

  const htmlTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>非空小队对话记录 - ${this.escapeHtml(sessionId)}</title>
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
        .header h1 { color: #5c6bc0; margin-bottom: 10px; }
        .header .info { color: #666; font-size: 14px; }
        .message { margin-bottom: 20px; }
        .message-header {
            display: flex; align-items: center; gap: 8px; margin-bottom: 8px;
        }
        .message-name { font-weight: 600; color: #333; }
        .agent-tag {
            background: #e8eaf6; color: #5c6bc0;
            padding: 2px 6px; border-radius: 3px; font-size: 11px; font-weight: 500;
        }
        .message-time { color: #999; font-size: 11px; }
        .message-body {
            padding: 12px 16px; border-radius: 8px;
            background: #fff; border: 1px solid #e5e5e5; word-break: break-word;
        }
        .message.user .message-body {
            background: #5c6bc0; color: white; margin-left: 60px;
        }
        /* Markdown 渲染样式 */
        .markdown-body pre {
            background: #f6f8fa; padding: 12px; border-radius: 6px; overflow-x: auto;
        }
        .markdown-body code {
            background: rgba(0,0,0,0.06); padding: 2px 6px; border-radius: 3px;
            font-family: 'SF Mono', Monaco, Consolas, monospace; font-size: 0.9em;
        }
        .markdown-body pre code {
            background: none; padding: 0;
        }
        .markdown-table-wrap {
            max-width: 100%; overflow-x: auto; margin: 8px 0;
        }
        .markdown-body table {
            border-collapse: collapse; width: 100%; margin: 0;
        }
        .markdown-table-wrap table {
            min-width: max-content;
        }
        .markdown-body th, .markdown-body td {
            border: 1px solid #ddd; padding: 6px 10px; text-align: left;
        }
        .markdown-body th { background: #f0f0f0; }
        .markdown-body blockquote {
            border-left: 3px solid #5c6bc0; padding-left: 12px; color: #666; margin: 8px 0;
        }
        .markdown-body ul, .markdown-body ol { padding-left: 20px; }
        .markdown-body img { max-width: 100%; }
        .tool-call {
            margin: 8px 0; padding: 10px 12px; border-radius: 6px;
            background: #e3f2fd; border: 1px solid #90caf9; font-size: 13px;
        }
        .tool-call-header { font-weight: 600; margin-bottom: 4px; }
        .tool-call-header code {
            background: rgba(0,0,0,0.06); padding: 2px 6px; border-radius: 3px;
            font-family: 'SF Mono', Monaco, Consolas, monospace;
        }
        .tool-call-args {
            background: #f5f5f5; padding: 8px; border-radius: 4px; font-size: 12px;
            overflow-x: auto; white-space: pre-wrap; word-break: break-all; margin: 4px 0 0;
        }
        .tool-result {
            margin: 4px 0 8px; padding: 10px 12px; border-radius: 6px;
            background: #e8f5e9; border: 1px solid #81c784; font-size: 13px;
        }
        .tool-result-header { font-weight: 600; margin-bottom: 4px; }
        .tool-result-content {
            background: #f5f5f5; padding: 8px; border-radius: 4px; font-size: 12px;
            overflow-x: auto; white-space: pre-wrap; word-break: break-all; margin: 4px 0 0;
        }
        .action-event {
            margin: 6px 0; padding: 6px 10px; border-radius: 4px;
            background: #fff3e0; border: 1px solid #ffb74d; font-size: 12px; color: #e65100;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>非空小队对话记录</h1>
        <div class="info">
            <div>会话ID: ${this.escapeHtml(sessionId)}</div>
            <div>导出时间: ${new Date().toLocaleString("zh-CN")}</div>
        </div>
    </div>
    <div class="messages">
        ${messagesHTML}
    </div>
</body>
</html>`;

  // 创建并下载文件
  const blob = new Blob([htmlTemplate], { type: "text/html;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = exportFilename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);

  this.showNotification(`对话记录已导出为 ${exportFilename}`, "success");
};

FKTeamsChat.prototype.loadSession = function (sessionId) {
  if (!sessionId) return Promise.resolve();
  if (!this._sessionLoadPromises) this._sessionLoadPromises = {};
  if (this._sessionLoadPromises[sessionId]) {
    return this._sessionLoadPromises[sessionId];
  }

  const promise = this._loadSession(sessionId).finally(() => {
    if (this._sessionLoadPromises[sessionId] === promise) {
      delete this._sessionLoadPromises[sessionId];
    }
  });
  this._sessionLoadPromises[sessionId] = promise;
  return promise;
};

FKTeamsChat.prototype._loadSession = async function (sessionId) {
  // 切换会话
  if (this.sessionId !== sessionId) {
    this._saveSessionDOM();
    this.sessionId = sessionId;
    localStorage.setItem("fk_session_id", sessionId);
    this._hasLoadedSession = true;
  }
  // 确保隐藏域始终反映当前会话 ID
  this.sessionIdInput.value = sessionId;

  this.showChatLoading();
  this.messagesContainer.innerHTML = "";
  this.hideHistoryModal();

  try {
    const response = await this.fetchWithAuth(
      `/api/fkteams/sessions/${encodeURIComponent(sessionId)}`,
      { cache: "no-cache" },
    );
    if (!response.ok) {
      // 请求期间用户可能已切换到其他会话，放弃过期结果
      if (this.sessionId !== sessionId) return;
      this.hideChatLoading();
      if (response.status === 404) {
        this.showHomePage({ skipSaveCurrentDOM: true });
      } else {
        this.showNotification("加载会话失败", "error");
        this.clearChatUI();
      }
      return;
    }
    const result = await response.json();
    // 请求期间用户可能已切换到其他会话，放弃过期结果
    if (this.sessionId !== sessionId) return;
    if (result.code !== 0 || !result.data) {
      this.hideChatLoading();
      this.showNotification("加载会话失败", "error");
      this.clearChatUI();
      return;
    }
    // 复用已有的 handleHistoryLoaded 渲染逻辑
    this.handleHistoryLoaded({
      session_id: sessionId,
      current_agent: result.data.current_agent || "",
      messages: result.data.messages || [],
    });

    if (result.data.active_task) {
      const offset = 0;
      this.setStreamOffset(sessionId, offset);
      if (!this._processingSessions) this._processingSessions = new Set();
      this._processingSessions.add(sessionId);
      if (this.sessionId === sessionId) {
        this.isProcessing = true;
        this.updateStatus("processing", "处理中...");
        this.updateSendButtonState();
        this._resumePending = true;
        this._resumeReplayed = false;
        this.resumeSessionStream(sessionId, offset);
      }
    }
  } catch (error) {
    console.error("Error loading session:", error);
    this.hideChatLoading();
    this.showNotification("加载会话失败", "error");
    this.clearChatUI();
  }
};

FKTeamsChat.prototype.showChatLoading = function () {
  if (this.chatLoading) {
    this.chatLoading.style.display = "flex";
  }
};

FKTeamsChat.prototype.hideChatLoading = function () {
  if (this.chatLoading) {
    this.chatLoading.style.display = "none";
  }
};

FKTeamsChat.prototype.checkAndLoadSessionHistory = async function (sessionId) {
  try {
    // 检查会话是否存在
    const response = await this.fetchWithAuth("/api/fkteams/sessions");
    if (!response.ok) {
      this.clearChatUI();
      return;
    }

    const result = await response.json();
    if (result.code !== 0 || !result.data || !result.data.sessions) {
      this.clearChatUI();
      return;
    }

    // 查找是否存在该会话
    const fileExists = result.data.sessions.some(
      (file) => file.session_id === sessionId,
    );

    if (fileExists) {
      this.loadSession(sessionId);
    } else {
      this.showHomePage();
    }
  } catch (error) {
    console.error("Error checking session history:", error);
    this.clearChatUI();
  }
};

FKTeamsChat.prototype.isLegacyHistoryMemberMessage = function (msg) {
  return false;
};

FKTeamsChat.prototype.isHistoryMemberMessage = function (msg) {
  return !!(
    msg &&
    msg.member_call_id
  );
};

FKTeamsChat.prototype.historyMemberLabel = function (msg) {
  if (msg.member_name) return msg.member_name;
  const raw = this.isLegacyHistoryMemberMessage(msg)
    ? (msg.agent_name || "").slice(4)
    : msg.agent_name || "Member";
  return raw
    .split(/[_-]+/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ") || "Member";
};

FKTeamsChat.prototype.historyToolDisplay = function (tc) {
  const display = this.getToolDisplay(tc);
  if (display.kind === "agent" || !tc?.name || !/^ask_fkagent_[A-Za-z0-9_-]+$/.test(tc.name)) {
    return display;
  }
  const target = this.historyMemberLabel({ agent_name: tc.name });
  return {
    name: tc.name,
    displayName: target ? `指派给 ${target}` : tc.name,
    kind: "agent",
    target,
  };
};

FKTeamsChat.prototype.queueHistoryMemberTask = function (tc) {
  const display = this.historyToolDisplay(tc);
  if (display.kind !== "agent") return;
  if (!this._historyPendingMemberTasks) this._historyPendingMemberTasks = [];
  this._historyPendingMemberTasks.push({
    id: tc.id || "",
    ref: tc.ref || "",
    index: tc.index,
    name: tc.name || "",
    target: display.target || "",
    arguments: tc.arguments || "",
  });
};

FKTeamsChat.prototype.takeHistoryMemberTask = function (msg) {
  if (msg?.__historyMemberTask) return msg.__historyMemberTask;
  const tasks = this._historyPendingMemberTasks || [];
  if (tasks.length === 0) return null;

  const callID = msg.member_call_id || "";
  let idx = callID ? tasks.findIndex((task) => task.id && task.id === callID) : -1;
  if (idx < 0) return null;

  const task = tasks[idx];
  tasks.splice(idx, 1);
  return task;
};

FKTeamsChat.prototype.prepareHistoryMemberTasks = function (messages) {
  const assignTurn = (turn) => {
    if (!turn || turn.length === 0) return;
    const tasks = [];
    const members = [];

    turn.forEach((msg) => {
      if (this.isHistoryMemberMessage(msg)) {
        members.push(msg);
        return;
      }
      (msg.events || []).forEach((evt) => {
        if (evt.type !== "tool_call" || !evt.tool_call) return;
        const display = this.historyToolDisplay(evt.tool_call);
        if (display.kind !== "agent") return;
        tasks.push({
          id: evt.tool_call.id || "",
          ref: evt.tool_call.ref || "",
          index: evt.tool_call.index,
          name: evt.tool_call.name || "",
          target: display.target || "",
          arguments: evt.tool_call.arguments || "",
          ownerMessage: msg,
        });
      });
    });

    members.forEach((msg) => {
      if (msg.__historyMemberTask) return;
      const callID = msg.member_call_id || "";
      let idx = callID ? tasks.findIndex((task) => task.id && task.id === callID) : -1;
      if (idx < 0) return;
      msg.__historyMemberTask = tasks[idx];
      tasks.splice(idx, 1);
    });
  };

  let turn = [];
  (messages || []).forEach((msg) => {
    if (msg.agent_name === "用户") {
      assignTurn(turn);
      turn = [];
      return;
    }
    turn.push(msg);
  });
  assignTurn(turn);
};

FKTeamsChat.prototype.renderHistoryMemberGroup = function (messages) {
  if (!messages || messages.length === 0) return;

  const saved = {
    currentMessageElement: this.currentMessageElement,
    currentMessageElements: this.currentMessageElements,
    pendingToolCalls: this.pendingToolCalls,
    toolCallsByID: this.toolCallsByID,
    parallelPanel: this.parallelPanel,
    parallelMemberCards: this.parallelMemberCards,
    parallelMemberByAgent: this.parallelMemberByAgent,
    parallelToolMemberByID: this.parallelToolMemberByID,
    parallelMemberResultChunks: this.parallelMemberResultChunks,
    parallelMemberInnerResultChunks: this.parallelMemberInnerResultChunks,
    parallelPanelBatchMode: this.parallelPanelBatchMode,
    lastToolName: this.lastToolName,
  };

  this.currentMessageElement = null;
  this.currentMessageElements = {};
  this.pendingToolCalls = {};
  this.toolCallsByID = {};
  this.parallelPanel = null;
  this.parallelMemberCards = {};
  this.parallelMemberByAgent = {};
  this.parallelToolMemberByID = {};
  this.parallelMemberResultChunks = {};
  this.parallelMemberInnerResultChunks = {};
  this.parallelPanelBatchMode = true;
  this.lastToolName = "";

  messages.forEach((msg, index) => {
    const key = "history:" + (msg.member_call_id || msg.start_time || index);
    const label = this.historyMemberLabel(msg);
    const task = this.takeHistoryMemberTask(msg, index);
    const entry = this.ensureMemberCard(key, label, label, task?.index);
    if (task?.arguments) {
      this.updateMemberTaskContent(entry, task.arguments, false);
    }
    let hasError = false;
    let hasCancelled = false;
    let lastAskFlowKey = "";

    (msg.events || []).forEach((evt, evtIndex) => {
      if (evt.type === "cancelled") {
        hasCancelled = true;
        return;
      }
      if (evt.type === "reasoning" && evt.content) {
        this.appendMemberReasoningFinal(entry, evt.content, evt.sequence);
        return;
      }
      if (evt.type === "text" && evt.content) {
        this.appendMemberOutputFinal(entry, evt.content, evt.sequence);
        return;
      }
      if (evt.type === "error" && evt.content) {
        hasError = true;
        this.appendMemberOutputFinal(entry, "\n\n" + evt.content, evt.sequence);
        return;
      }
      if (evt.type === "tool_call" && evt.tool_call) {
        if (evt.tool_call.name === "ask_questions") return;
        const display = this.historyToolDisplay(evt.tool_call);
        const flowKey = evt.tool_call.ref ? "ref:" + evt.tool_call.ref : "";
        if (!flowKey) return;
        this.ensureMemberToolFlow(entry, flowKey, display.displayName, evt.sequence);
        this.updateMemberToolFlowArgs(entry, flowKey, display.displayName, evt.tool_call.arguments || "", false, evt.sequence);
        if (evt.tool_call.result) {
          this.updateMemberToolFlowResult(entry, flowKey, display.displayName, evt.tool_call.result, false);
        }
        return;
      }
      if (evt.type === "action" && evt.action?.content) {
        if (evt.action.action_type === "ask_questions") {
          const reusable = this.findReusableMemberAskFlow ? this.findReusableMemberAskFlow(entry, evt) : null;
          lastAskFlowKey = reusable?.key || (this.memberAskFlowKey ? this.memberAskFlowKey(evt) : "") || "ask:history:" + index + ":" + evtIndex;
          const flow = reusable?.flow || this.ensureMemberToolFlow(entry, lastAskFlowKey, "ask_questions", evt.sequence);
          if (flow?.status) flow.status.textContent = "待回复";
          if (flow?.argsWrap) flow.argsWrap.style.display = "";
          if (flow?.args) flow.args.textContent = evt.action.content;
          return;
        }
        if (evt.action.action_type === "ask_response" && (lastAskFlowKey || evt.action.detail)) {
          const responseFlowKey = evt.action.detail && this.memberAskFlowKey ? this.memberAskFlowKey(evt) : lastAskFlowKey;
          this.updateMemberToolFlowResult(entry, responseFlowKey, "ask_questions", evt.action.content, false);
          const flow = entry.toolFlows?.[responseFlowKey];
          if (flow?.status) flow.status.textContent = "已回答";
          if (flow) flow.askCompleted = true;
          return;
        }
        this.appendMemberTextEvent(entry, "tool", "工具事件", evt.action.content, evt.sequence);
      }
    });

    this.updateMemberStatus(
      entry,
      hasCancelled ? "cancelled" : hasError ? "error" : "done",
      hasCancelled ? "已取消" : hasError ? "失败" : "完成",
    );
    this.finalizeMemberMarkdown(entry);
    this.updateMemberDetailVisibility(entry);
  });

  this.updateParallelMembersHeader();
  const panel = this.parallelPanel;
  if (panel) {
    panel.classList.add("parallel-members-history");
    const title = panel.querySelector(".parallel-members-title");
    if (title) title.textContent = messages.length > 1 ? "成员并行任务" : "成员任务";
  }

  this.currentMessageElement = saved.currentMessageElement;
  this.currentMessageElements = saved.currentMessageElements;
  this.pendingToolCalls = saved.pendingToolCalls;
  this.toolCallsByID = saved.toolCallsByID;
  this.parallelPanel = saved.parallelPanel;
  this.parallelMemberCards = saved.parallelMemberCards;
  this.parallelMemberByAgent = saved.parallelMemberByAgent;
  this.parallelToolMemberByID = saved.parallelToolMemberByID;
  this.parallelMemberResultChunks = saved.parallelMemberResultChunks;
  this.parallelMemberInnerResultChunks = saved.parallelMemberInnerResultChunks;
  this.parallelPanelBatchMode = saved.parallelPanelBatchMode;
  this.lastToolName = saved.lastToolName;
};

FKTeamsChat.prototype.historyAgentToolCallIDs = function (msg) {
  const ids = new Set();
  (msg?.events || []).forEach((evt) => {
    if (evt.type !== "tool_call" || !evt.tool_call) return;
    const display = this.historyToolDisplay(evt.tool_call);
    if (display.kind !== "agent") return;
    if (evt.tool_call.id) ids.add(evt.tool_call.id);
  });
  return ids;
};

FKTeamsChat.prototype.historyAgentHasMemberToolCall = function (msg) {
  return (msg?.events || []).some((evt) => {
    if (evt.type !== "tool_call" || !evt.tool_call) return false;
    return this.historyToolDisplay(evt.tool_call).kind === "agent";
  });
};

FKTeamsChat.prototype.historyMemberOwnedByAgentMessage = function (msg, agentMsg) {
  return !!(msg?.__historyMemberTask?.ownerMessage && msg.__historyMemberTask.ownerMessage === agentMsg);
};

FKTeamsChat.prototype.historyMemberMatchesCallIDs = function (msg, callIDs) {
  if (!msg || !callIDs || callIDs.size === 0) return false;
  if (msg.member_call_id && callIDs.has(msg.member_call_id)) return true;
  const taskID = msg.__historyMemberTask?.id || "";
  return !!(taskID && callIDs.has(taskID));
};

FKTeamsChat.prototype.renderHistoryAgentMessagePart = function (msg, events) {
  if (!events || events.length === 0) return;
  this.renderHistoryAgentMessage({
    ...msg,
    events,
  });
};

FKTeamsChat.prototype.renderHistoryAgentWithMemberInsert = function (msg, messages, renderedMemberIndexes) {
  const events = msg?.events || [];
  const callIDs = this.historyAgentToolCallIDs(msg);
  if (callIDs.size === 0 && !this.historyAgentHasMemberToolCall(msg)) return false;

  const members = [];
  (messages || []).forEach((candidate, index) => {
    if (renderedMemberIndexes.has(index)) return;
    if (!this.isHistoryMemberMessage(candidate)) return;
    if (
      !this.historyMemberOwnedByAgentMessage(candidate, msg) &&
      !this.historyMemberMatchesCallIDs(candidate, callIDs)
    ) return;
    members.push({ msg: candidate, index });
  });
  if (members.length === 0) return false;

  let lastAgentToolIndex = -1;
  events.forEach((evt, index) => {
    if (evt.type !== "tool_call" || !evt.tool_call) return;
    const display = this.historyToolDisplay(evt.tool_call);
    if (display.kind === "agent") {
      lastAgentToolIndex = index;
    }
  });
  if (lastAgentToolIndex < 0) return false;

  this.renderHistoryAgentMessagePart(msg, events.slice(0, lastAgentToolIndex + 1));
  this.renderHistoryMemberGroup(members.map((item) => item.msg));
  members.forEach((item) => renderedMemberIndexes.add(item.index));
  this.renderHistoryAgentMessagePart(msg, events.slice(lastAgentToolIndex + 1));
  return true;
};

FKTeamsChat.prototype.handleHistoryLoaded = function (event) {
  // 隐藏loading
  this.hideChatLoading();

  // 清空当前消息
  this.messagesContainer.innerHTML = "";
  this.resetParallelState();
  this._historyPendingMemberTasks = [];

  // 清空快速导航（将重新构建）
  this.clearQuickNav();

  // 更新 session ID
  if (event.session_id) {
    this.sessionId = event.session_id;
    this.sessionIdInput.value = event.session_id;
    this._hasLoadedSession = true;
    this.updateSidebarSessionActive();
  }

  // 渲染历史消息
  if (!event.messages || event.messages.length === 0) {
    this.clearChatUI();
  } else {
    this.prepareHistoryMemberTasks(event.messages);
    const renderedMemberIndexes = new Set();
    for (let index = 0; index < event.messages.length; index++) {
      const msg = event.messages[index];
      // 检查是否是用户消息
      if (msg.agent_name === "用户") {
        // 渲染用户消息
        this.renderHistoryUserMessage(msg);
        continue;
      }

      if (this.isHistoryMemberMessage(msg)) {
        if (renderedMemberIndexes.has(index)) continue;
        const group = [];
        while (
          index < event.messages.length &&
          this.isHistoryMemberMessage(event.messages[index]) &&
          !renderedMemberIndexes.has(index)
        ) {
          group.push(event.messages[index]);
          renderedMemberIndexes.add(index);
          index++;
        }
        index--;
        this.renderHistoryMemberGroup(group);
      } else if (this.renderHistoryAgentWithMemberInsert(msg, event.messages, renderedMemberIndexes)) {
        continue;
      } else {
        this.renderHistoryAgentMessage(msg);
      }
    }
  }

  this.scrollToBottom();

  // 从服务端恢复当前 @智能体 状态（仅更新 UI，不写回）
  if (event.current_agent) {
    const agent = this.agents.find((a) => a.name === event.current_agent);
    this.setCurrentAgent(agent || null, false);
  } else {
    this.setCurrentAgent(null, false);
  }
};

FKTeamsChat.prototype.renderHistoryAgentMessage = function (msg) {
  const timeInfo = {
    startTime: msg.start_time,
    endTime: msg.end_time,
  };

  if (!msg.events || msg.events.length === 0) return;

  let currentMessageEl = null;
  let currentContent = "";

  msg.events.forEach((evt) => {
    switch (evt.type) {
      case "reasoning":
        if (!currentMessageEl) {
          currentMessageEl = this.createAssistantMessage(
            msg.agent_name,
            timeInfo,
          );
        }
        const reasoningBodyEl =
          currentMessageEl.querySelector(".message-body");
        if (reasoningBodyEl && evt.content) {
          const indicator = reasoningBodyEl.querySelector(
            ".streaming-indicator",
          );
          if (indicator) indicator.remove();
          let reasoningBlock =
            reasoningBodyEl.querySelector(".reasoning-block");
          if (!reasoningBlock) {
            reasoningBlock = document.createElement("div");
            reasoningBlock.className = "reasoning-block";
            reasoningBlock.innerHTML = `
                              <div class="reasoning-header" onclick="this.parentElement.classList.toggle('expanded')">
                                  <svg class="reasoning-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9.663 17h4.673M12 3v1M6.5 5.5l.7.7M3 12h1M20 12h1M16.8 6.2l.7-.7M17.5 12A5.5 5.5 0 1 0 7 14.5V17a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1v-2.5A5.5 5.5 0 0 0 17.5 12z"/></svg>
                                  <span class="reasoning-title">思考过程</span>
                                  <svg class="reasoning-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>
                              </div>
                              <div class="reasoning-content markdown-body markdown-body-compact">${this.renderMarkdown(evt.content)}</div>
                          `;
            reasoningBodyEl.prepend(reasoningBlock);
          }
        }
        break;

      case "text":
        currentContent += evt.content;
        if (!currentMessageEl) {
          currentMessageEl = this.createAssistantMessage(
            msg.agent_name,
            timeInfo,
          );
        }
        const bodyEl = currentMessageEl.querySelector(".message-body");
        if (bodyEl) {
          const indicator = bodyEl.querySelector(".streaming-indicator");
          if (indicator) indicator.remove();
          bodyEl.setAttribute("data-raw", currentContent);
          bodyEl.setAttribute("data-fn-done", "1");
          const existingReasoning =
            bodyEl.querySelector(".reasoning-block");
          if (existingReasoning) {
            let textContainer = bodyEl.querySelector(
              ".message-text-content",
            );
            if (!textContainer) {
              textContainer = document.createElement("div");
              textContainer.className = "message-text-content";
              bodyEl.appendChild(textContainer);
            }
            textContainer.innerHTML = this.renderMarkdown(currentContent);
          } else {
            bodyEl.innerHTML = this.renderMarkdown(currentContent);
          }
        }
        break;

      case "tool_call":
        if (evt.tool_call) {
          this.renderSingleToolCall(evt.tool_call);
        }
        currentMessageEl = null;
        currentContent = "";
        break;

      case "cancelled":
        if (msg.agent_name === "系统") {
          this.renderCancelledNotice(evt.content || "任务已取消");
        }
        currentMessageEl = null;
        currentContent = "";
        break;

      case "error":
        this.renderHistoryErrorMessage(evt, msg.agent_name);
        currentMessageEl = null;
        currentContent = "";
        break;

      case "action":
        if (evt.action) {
          this.renderSingleAction(evt.action, msg.agent_name);
        }
        currentMessageEl = null;
        currentContent = "";
        break;
    }
  });
};

FKTeamsChat.prototype.renderHistoryUserMessage = function (msg) {
  // 从events中提取用户输入的文本
  let userContent = "";
  let contentParts = [];
  if (msg.events && msg.events.length > 0) {
    msg.events.forEach((evt) => {
      if (evt.type === "text" && evt.content) {
        userContent += evt.content;
      }
      if (evt.type === "text" && Array.isArray(evt.content_parts)) {
        contentParts = contentParts.concat(evt.content_parts);
      }
    });
  }

  const attachmentsHtml = typeof this.renderMessageAttachments === "function"
    ? this.renderMessageAttachments(contentParts)
    : "";

  if (!userContent && !attachmentsHtml) return;

  // 创建用户消息元素
  const messageEl = document.createElement("div");
  messageEl.className = "message user";
  const messageId = `msg-${msg.start_time || Date.now()}`;
  messageEl.setAttribute("data-message-id", messageId);

  // 格式化时间
  const timeDisplay = msg.start_time
    ? this.formatHistoryTime({ startTime: msg.start_time })
    : this.getCurrentTime();

  const bodyHtml = userContent
    ? `<div class="message-body">${this.escapeHtml(userContent)}</div>`
    : "";

  messageEl.innerHTML = `
        <div class="message-content">
            <div class="message-header">
                <span class="message-name">您</span>
                <span class="message-time">${timeDisplay}</span>
            </div>
            ${attachmentsHtml}
            ${bodyHtml}
        </div>
    `;
  this.messagesContainer.appendChild(messageEl);

  // 添加到快速导航
  const question = {
    id: messageId,
    content: userContent,
    time: timeDisplay,
    element: messageEl,
  };
  this.userQuestions.push(question);
  this.updateQuickNav();
};

FKTeamsChat.prototype.renderHistoryErrorMessage = function (evt, agentName) {
  const errorRecord = typeof evt === "object" && evt ? evt.error || {} : {};
  const content = typeof evt === "string" ? evt : evt?.content || errorRecord.message || "";
  if (!content && !errorRecord.technical_detail) return;
  const escape = typeof this.escapeHtml === "function"
    ? this.escapeHtml.bind(this)
    : (value) => String(value || "");
  const errorEl = document.createElement("div");
  errorEl.className = "error-message";
  if (typeof this.renderErrorContent === "function") {
    errorEl.innerHTML = this.renderErrorContent({
      title: errorRecord.title || "任务执行失败",
      message: errorRecord.message || content,
      technicalError: errorRecord.technical_detail || content,
      suggestions: errorRecord.suggestions || [],
      agentName,
    });
  } else {
    errorEl.innerHTML = `
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10"/>
            <line x1="15" y1="9" x2="9" y2="15"/>
            <line x1="9" y1="9" x2="15" y2="15"/>
        </svg>
        <span>${agentName ? `[${escape(agentName)}] ` : ""}${escape(content)}</span>
    `;
  }
  this.messagesContainer.appendChild(errorEl);
};

FKTeamsChat.prototype.renderSingleToolCall = function (tc) {
  // dispatch_tasks 专用卡片渲染
  if (tc.name === "dispatch_tasks" && tc.result) {
    const el = this.renderDispatchResult(tc.result);
    if (el) {
      this.messagesContainer.appendChild(el);
      return;
    }
  }

  // 渲染工具调用
  const toolCallEl = document.createElement("div");
  const toolDisplay = this.historyToolDisplay(tc);
  if (toolDisplay.kind === "agent") return;
  toolCallEl.className = "tool-call" + (toolDisplay.kind === "agent" ? " agent-tool-call" : "");
  if (tc.ref) toolCallEl.setAttribute("data-tool-key", "ref:" + tc.ref);
  if (tc.id) toolCallEl.setAttribute("data-tool-call-id", tc.id);
  if (tc.index !== undefined && tc.index !== null) toolCallEl.setAttribute("data-tool-index", tc.index);

  let argsDisplay = tc.arguments || "无参数";
  try {
    const args = JSON.parse(tc.arguments);
    argsDisplay = JSON.stringify(args, null, 2);
  } catch {
    // 保持原样
  }

  toolCallEl.innerHTML = `
        <div class="tool-call-header">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="3"/>
                <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>
            </svg>
            <span>${toolDisplay.kind === "agent" ? "成员指派:" : "工具调用:"}</span>
            <code class="tool-call-name">${this.escapeHtml(toolDisplay.displayName)}</code>
            <span class="tool-call-status">${tc.result ? "已完成" : "已调用"}</span>
        </div>
        <div class="tool-call-detail" style="display:none">
          <pre class="tool-call-args">${this.escapeHtml(argsDisplay)}</pre>
        </div>
    `;
  this.bindToolCallToggle(toolCallEl);
  this.messagesContainer.appendChild(toolCallEl);

  // 渲染工具结果（如果有）
  if (tc.result) {
    this.appendToolResultToCard(toolCallEl, tc.result, toolDisplay);
  }
};

FKTeamsChat.prototype.renderSingleAction = function (action, agentName) {
  if (!action.content && !action.action_type) {
    return;
  }

  const compressIcon = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width:14px;height:14px;flex-shrink:0;">
        <polyline points="4 14 10 14 10 20"/><polyline points="20 10 14 10 14 4"/>
        <line x1="14" y1="10" x2="21" y2="3"/><line x1="3" y1="21" x2="10" y2="14"/>
    </svg>`;

  // 上下文压缩开始
  if (action.action_type === "context_compress_start") {
    const el = document.createElement("div");
    el.className = "action-event context-compress";
    el.innerHTML = `${compressIcon}<span>[${this.escapeHtml(agentName)}] ${this.escapeHtml(action.content || action.action_type)}</span>`;
    this.messagesContainer.appendChild(el);
    return;
  }

  // 上下文压缩完成：可展开的摘要卡片
  if (action.action_type === "context_compress") {
    const cardEl = document.createElement("div");
    cardEl.className = "action-event context-compress";
    if (action.detail) {
      cardEl.style.cursor = "pointer";
      cardEl.style.flexWrap = "wrap";
      const toggleIcon = `<svg class="toggle-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width:12px;height:12px;transition:transform 0.2s;margin-left:auto;">
                <polyline points="6 9 12 15 18 9"/>
            </svg>`;
      cardEl.innerHTML = `${compressIcon}<span>[${this.escapeHtml(agentName)}] ${this.escapeHtml(action.content || action.action_type)}</span>${toggleIcon}
                <div class="compress-detail" style="display:none;width:100%;margin-top:8px;padding:10px;background:var(--bg-primary);border-radius:6px;font-size:12px;line-height:1.6;white-space:pre-wrap;word-break:break-word;color:var(--text-primary);max-height:300px;overflow-y:auto;">${this.escapeHtml(action.detail)}</div>`;
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
      cardEl.innerHTML = `${compressIcon}<span>[${this.escapeHtml(agentName)}] ${this.escapeHtml(action.content || action.action_type)}</span>`;
    }
    this.messagesContainer.appendChild(cardEl);
    return;
  }

  // 审批请求
  if (action.action_type === "approval_required") {
    const el = document.createElement("div");
    el.className = "action-event approval-request";
    el.innerHTML = `<span>${this.escapeHtml(action.content || "需要审批")}</span>`;
    this.messagesContainer.appendChild(el);
    return;
  }

  // 审批决定
  if (action.action_type === "approval_decision") {
    const isApproved = action.content && !action.content.includes("拒绝");
    const el = document.createElement("div");
    el.className =
      "action-event approval-result " + (isApproved ? "approved" : "rejected");
    el.innerHTML = `<span>${this.escapeHtml(action.content || "审批完成")}</span>`;
    this.messagesContainer.appendChild(el);
    return;
  }

  // 提问请求
  if (action.action_type === "ask_questions") {
    const el = document.createElement("div");
    el.className = "action-event ask-request";
    el.innerHTML = `<span>[提问] ${this.escapeHtml(action.content || "模型提问")}</span>`;
    this.messagesContainer.appendChild(el);
    return;
  }

  // 提问回答
  if (action.action_type === "ask_response") {
    const el = document.createElement("div");
    el.className = "action-event ask-response approved";
    el.innerHTML = `<span>${this.escapeHtml(action.content || "已回答")}</span>`;
    this.messagesContainer.appendChild(el);
    return;
  }

  let actionClass = "";
  let actionIcon = "";

  switch (action.action_type) {
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
    case "interrupted":
      actionClass = "interrupted";
      actionIcon = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10"/>
                <line x1="15" y1="9" x2="9" y2="15"/>
                <line x1="9" y1="9" x2="15" y2="15"/>
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
  actionEl.innerHTML = `${actionIcon}<span>[${this.escapeHtml(agentName)}] ${this.escapeHtml(action.content || action.action_type)}</span>`;
  this.messagesContainer.appendChild(actionEl);
};

FKTeamsChat.prototype.deleteSession = function (sessionId) {
  this.currentDeleteSessionId = sessionId;
  this.deleteFilenameSpan.textContent = sessionId;
  this.showDeleteModal();
};

FKTeamsChat.prototype.showDeleteModal = function () {
  this.deleteModal.style.display = "flex";
  setTimeout(() => {
    this.deleteConfirmBtn.focus();
  }, 100);
};

FKTeamsChat.prototype.hideDeleteModal = function () {
  this.deleteModal.style.display = "none";
  this.currentDeleteSessionId = null;
};

FKTeamsChat.prototype.confirmDelete = async function () {
  const sessionId = this.currentDeleteSessionId;
  this.hideDeleteModal();

  if (!sessionId) return;

  try {
    const response = await this.fetchWithAuth(
      `/api/fkteams/sessions/${encodeURIComponent(sessionId)}`,
      {
        method: "DELETE",
      },
    );

    if (!response.ok) {
      throw new Error("删除失败");
    }

    const result = await response.json();
    if (result.code !== 0) {
      throw new Error(result.message || "删除失败");
    }

    this.showNotification("删除成功", "success");

    // 清除 DOM 缓存和事件缓冲
    delete this._sessionDOMCache[sessionId];
    if (this._sessionEventBuffer) delete this._sessionEventBuffer[sessionId];

    // 如果删除的是当前活动会话，切回欢迎页面
    if (this._hasLoadedSession && sessionId === this.sessionId) {
      localStorage.removeItem("fk_session_id");
      this.sessionId = "";
      this.sessionIdInput.value = "";
      this._hasLoadedSession = false;
      this.clearChatUI();
    }

    // 刷新历史弹窗列表（如果弹窗已打开）
    if (this.historyModal && this.historyModal.style.display !== "none") {
      await this.loadSessions();
    }
    this.removeSidebarSessionItem(sessionId);
  } catch (error) {
    console.error("Error deleting session:", error);
    this.showNotification(error.message || "删除失败", "error");
  }
};

FKTeamsChat.prototype.renameSession = async function (sessionId, currentTitle) {
  this.currentRenameSessionId = sessionId;
  this.renameInput.value = currentTitle || sessionId;
  this.showRenameModal();
};

FKTeamsChat.prototype.showRenameModal = function () {
  this.renameModal.style.display = "flex";
  setTimeout(() => {
    this.renameInput.focus();
    this.renameInput.select();
  }, 100);
};

FKTeamsChat.prototype.hideRenameModal = function () {
  this.renameModal.style.display = "none";
  this.currentRenameSessionId = null;
};

FKTeamsChat.prototype.confirmRename = async function () {
  const newTitle = this.renameInput.value.trim();
  const sessionId = this.currentRenameSessionId;

  if (!newTitle || !sessionId) {
    this.hideRenameModal();
    return;
  }

  try {
    const response = await this.fetchWithAuth("/api/fkteams/sessions/rename", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        session_id: sessionId,
        title: newTitle,
      }),
    });

    if (!response.ok) {
      const result = await response.json();
      throw new Error(result.message || "重命名失败");
    }

    const result = await response.json();
    if (result.code !== 0) {
      throw new Error(result.message || "重命名失败");
    }

    this.showNotification("重命名成功", "success");
    this.hideRenameModal();
    await this.loadSessions();
    await this.loadSidebarHistory();
  } catch (error) {
    console.error("Error renaming session:", error);
    this.showNotification(error.message || "重命名失败", "error");
  }
};

FKTeamsChat.prototype.formatTime = function (timeString) {
  if (!timeString) return "";
  const date = new Date(timeString);
  if (isNaN(date.getTime()) || date.getFullYear() < 2000) return "";
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const day = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  const days = Math.round((today - day) / (1000 * 60 * 60 * 24));
  const time = date.toLocaleTimeString("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
  });

  if (days === 0) {
    return "今天 " + time;
  } else if (days === 1) {
    return "昨天 " + time;
  } else if (days < 7) {
    return date.toLocaleDateString("zh-CN", {
      month: "2-digit",
      day: "2-digit",
    }) + " " + time;
  }
  return date.toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
};

FKTeamsChat.prototype.formatSize = function (bytes) {
  if (bytes < 1024) return bytes + " B";
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
  return (bytes / (1024 * 1024)).toFixed(1) + " MB";
};

FKTeamsChat.prototype.formatHistoryTime = function (timeInfo) {
  if (!timeInfo || !timeInfo.startTime) {
    return this.getCurrentTime();
  }

  const startDate = new Date(timeInfo.startTime);
  const endDate = timeInfo.endTime ? new Date(timeInfo.endTime) : null;

  // 格式化开始时间
  const timeStr = startDate.toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });

  // 如果有结束时间，计算持续时长
  if (endDate) {
    const duration = endDate - startDate;
    if (duration > 0) {
      const seconds = Math.floor(duration / 1000);
      const minutes = Math.floor(seconds / 60);
      const remainingSeconds = seconds % 60;

      if (minutes > 0) {
        return `${timeStr} (${minutes}分${remainingSeconds}秒)`;
      } else if (seconds > 0) {
        return `${timeStr} (${seconds}秒)`;
      }
    }
  }

  return timeStr;
};
