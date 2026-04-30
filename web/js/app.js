/**
 * 非空小队 - AI 对话前端应用
 */

class FKTeamsChat {
  constructor() {
    this.ws = null;
    this.sessionId = "default";
    this._hasLoadedSession = false;
    this.mode = "supervisor";
    this.isProcessing = false;
    this.currentMessageElement = null;
    this.hasToolCallAfterMessage = false;
    this.reconnectAttempts = 0;
    this.maxReconnectAttempts = 5;
    this.userScrolledUp = false; // 用户是否向上滚动了
    this.currentRenameFilename = null; // 当前正在重命名的文件名
    this.userQuestions = []; // 存储用户问题列表
    this.agents = []; // 存储智能体列表
    this.agentSuggestions = null; // 智能体建议弹窗
    this.selectedAgentIndex = -1; // 当前选中的智能体索引
    this.currentAgent = null; // 当前使用的智能体
    this.activeNotifications = []; // 活动的通知列表
    this.notificationStyleAdded = false; // 标记样式是否已添加
    this.files = []; // 存储文件列表
    this._sessionDOMCache = {}; // 会话DOM缓存，用于切换时保存/恢复UI状态
    this._sessionEventBuffer = {}; // 非当前会话的事件缓冲，切回时回放
    this.fileSuggestions = null; // 文件建议弹窗
    this.selectedFileIndex = -1; // 当前选中的文件索引
    this.currentPath = ""; // 当前浏览的路径
    this.attachments = []; // 多模态附件列表
    this._debounceTimers = {}; // 防抖定时器

    this.init();
  }

  // 通用防抖方法
  debounce(key, fn, delay) {
    clearTimeout(this._debounceTimers[key]);
    this._debounceTimers[key] = setTimeout(fn, delay);
  }

  // 获取 auth token
  getToken() {
    return localStorage.getItem("fk_token") || "";
  }

  // 带认证的 fetch 封装
  async fetchWithAuth(url, options = {}) {
    const token = this.getToken();
    if (token) {
      options.headers = {
        ...options.headers,
        Authorization: `Bearer ${token}`,
      };
    }
    const resp = await fetch(url, options);
    if (resp.status === 401) {
      localStorage.removeItem("fk_token");
      document.cookie = "fk_token=; path=/; max-age=0";
      window.location.href = "/login";
      throw new Error("unauthorized");
    }
    return resp;
  }

  init() {
    this.bindElements();
    this.bindEvents();
    this.restoreSidebarState();
    this.initTooltips();
    this.initMobileToolbar();
    this.initSchedule();
    this.initFileUpload();
    this.initFileManager();
    this.initConfig();
    this.loadAgents();
    this.loadVersion();
    this.connect();
  }

  bindElements() {
    this.messagesContainer = document.getElementById("messages");
    this.messagesWrapper = document.getElementById("messages-wrapper");
    this.messageInput = document.getElementById("message-input");
    this.sendBtn = document.getElementById("send-btn");
    this.cancelBtn = document.getElementById("cancel-btn");
    this.sessionIdInput = document.getElementById("session-id");
    this.statusIndicator = document.getElementById("status-indicator");
    this.historyManageBtn = document.getElementById("history-manage-btn");
    this.historySearchInput = document.getElementById("history-search-input");
    this.historyModal = document.getElementById("history-modal");
    this.historyModalClose = document.getElementById("history-modal-close");
    this.historyList = document.getElementById("history-list");
    this.renameModal = document.getElementById("rename-modal");
    this.renameModalClose = document.getElementById("rename-modal-close");
    this.renameInput = document.getElementById("rename-input");
    this.renameCancelBtn = document.getElementById("rename-cancel-btn");
    this.renameConfirmBtn = document.getElementById("rename-confirm-btn");
    this.deleteModal = document.getElementById("delete-modal");
    this.deleteModalClose = document.getElementById("delete-modal-close");
    this.deleteCancelBtn = document.getElementById("delete-cancel-btn");
    this.deleteConfirmBtn = document.getElementById("delete-confirm-btn");
    this.deleteFilenameSpan = document.getElementById("delete-filename");
    this.modeButtons = document.querySelectorAll(".mode-btn");
    this.sidebar = document.getElementById("sidebar");
    this.sidebarToggle = document.getElementById("sidebar-toggle");
    this.mainContent = document.getElementById("main-content");
    this.scrollToBottomBtn = document.getElementById("scroll-to-bottom");
    this.chatLoading = document.getElementById("chat-loading");
    this.quickNavWrapper = document.getElementById("quick-nav-wrapper");
    this.quickNavList = document.getElementById("quick-nav-list");
    this.newSessionBtn = document.getElementById("new-session-btn");
    this.sidebarSessionList = document.getElementById("sidebar-session-list");

    // 当前工作上下文指示器（显示在输入框正上方）
    this._contextIndicator = document.createElement("div");
    this._contextIndicator.className = "current-context-indicator";
    const inputWrapper = document.querySelector(".input-wrapper");
    if (inputWrapper && inputWrapper.parentNode) {
      inputWrapper.parentNode.insertBefore(this._contextIndicator, inputWrapper);
    }
  }

  bindEvents() {
    this.sendBtn.addEventListener("click", () => this.sendMessage());
    this.cancelBtn.addEventListener("click", () => this.cancelTask());
    this.messageInput.addEventListener("input", () => {
      this.handleInputChange();
      this.handleInputForMention();
    });
    this.messageInput.addEventListener("keydown", (e) => this.handleKeyDown(e));
    this.sessionIdInput.addEventListener("change", () => {
      const newSessionId = this.sessionIdInput.value || "default";
      if (newSessionId !== this.sessionId) {
        this.sessionId = newSessionId;
        this.checkAndLoadSessionHistory(newSessionId);
        this.updateSidebarSessionActive();
      }
    });
    this.historyManageBtn.addEventListener("click", () =>
      this.showHistoryModal(),
    );
    this.historyModalClose.addEventListener("click", () =>
      this.hideHistoryModal(),
    );

    // 重命名弹窗事件
    this.renameModalClose.addEventListener("click", () =>
      this.hideRenameModal(),
    );
    this.renameCancelBtn.addEventListener("click", () =>
      this.hideRenameModal(),
    );
    this.renameConfirmBtn.addEventListener("click", () => this.confirmRename());
    this.renameModal.addEventListener("click", (e) => {
      if (e.target === this.renameModal) {
        this.hideRenameModal();
      }
    });
    this.renameInput.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        this.confirmRename();
      } else if (e.key === "Escape") {
        this.hideRenameModal();
      }
    });
    // 删除弹窗事件
    this.deleteModalClose.addEventListener("click", () =>
      this.hideDeleteModal(),
    );
    this.deleteCancelBtn.addEventListener("click", () =>
      this.hideDeleteModal(),
    );
    this.deleteConfirmBtn.addEventListener("click", () => this.confirmDelete());
    this.deleteModal.addEventListener("click", (e) => {
      if (e.target === this.deleteModal) {
        this.hideDeleteModal();
      }
    });
    // 审批弹窗事件
    const approvalModal = document.getElementById("approval-modal");
    approvalModal.querySelectorAll(".approval-btn").forEach((btn) => {
      btn.addEventListener("click", () => {
        this.sendApprovalDecision(parseInt(btn.dataset.decision, 10));
      });
    });
    this.modeButtons.forEach((btn) => {
      btn.addEventListener("click", () => this.setMode(btn.dataset.mode));
    });
    if (this.sidebarToggle) {
      this.sidebarToggle.addEventListener("click", () => this.toggleSidebar());
    }
    // 新增会话按钮
    if (this.newSessionBtn) {
      this.newSessionBtn.addEventListener("click", () =>
        this.createNewSession(),
      );
    }
    // 监听滚动事件（使用 rAF 节流避免高频触发）
    if (this.mainContent) {
      this._scrollRAF = null;
      this.mainContent.addEventListener("scroll", () => {
        if (!this._scrollRAF) {
          this._scrollRAF = requestAnimationFrame(() => {
            this.handleScroll();
            this._scrollRAF = null;
          });
        }
      });
    }
    // 回到底部按钮
    if (this.scrollToBottomBtn) {
      this.scrollToBottomBtn.addEventListener("click", () =>
        this.scrollToBottomAndResume(),
      );
    }
  }

  handleScroll() {
    const { scrollTop, scrollHeight, clientHeight } = this.mainContent;
    const distanceFromBottom = scrollHeight - scrollTop - clientHeight;

    // 如果距离底部超过 100px，认为用户向上滚动了
    if (distanceFromBottom > 100) {
      // 仅当有实际消息时才显示回到底部按钮（欢迎页不显示）
      var hasMessages =
        this.messagesContainer &&
        this.messagesContainer.querySelector(".message");
      this.userScrolledUp = !!hasMessages;
      this.showScrollToBottomBtn(!!hasMessages);
    } else {
      // 用户回到了底部附近
      this.userScrolledUp = false;
      this.showScrollToBottomBtn(false);
    }
  }

  showScrollToBottomBtn(show) {
    if (!this.scrollToBottomBtn || this._scrollBtnVisible === show) return;
    this._scrollBtnVisible = show;
    if (show) {
      this.scrollToBottomBtn.style.display = "flex";
      // 触发重排以启动动画
      this.scrollToBottomBtn.offsetHeight;
      this.scrollToBottomBtn.style.opacity = "1";
      this.scrollToBottomBtn.style.transform =
        "translateX(calc(-50% + var(--sidebar-width) / 2)) translateY(0)";
    } else {
      this.scrollToBottomBtn.style.opacity = "0";
      this.scrollToBottomBtn.style.transform =
        "translateX(calc(-50% + var(--sidebar-width) / 2)) translateY(20px)";
      setTimeout(() => {
        if (this.scrollToBottomBtn.style.opacity === "0") {
          this.scrollToBottomBtn.style.display = "none";
        }
      }, 200);
    }
  }

  scrollToBottomAndResume() {
    this.userScrolledUp = false;
    this.showScrollToBottomBtn(false);
    this.forceScrollToBottom();
  }

  forceScrollToBottom() {
    if (this.mainContent) {
      this.mainContent.scrollTop = this.mainContent.scrollHeight;
    }
  }

  toggleSidebar() {
    const isCollapsed = this.sidebar.classList.toggle("collapsed");
    this.sidebarToggle.classList.toggle("collapsed", isCollapsed);
    // 调整回到底部按钮的位置
    if (this.scrollToBottomBtn) {
      this.scrollToBottomBtn.classList.toggle("sidebar-collapsed", isCollapsed);
    }
    // 移动端遮罩层
    this.updateSidebarOverlay();
    // 调整快速导航按钮和菜单的位置
    localStorage.setItem("sidebarCollapsed", isCollapsed);
  }

  updateSidebarOverlay() {
    if (!this._sidebarOverlay) return;
    const isMobile = window.innerWidth <= 768;
    const isOpen = !this.sidebar.classList.contains("collapsed");
    if (isMobile && isOpen) {
      this._sidebarOverlay.classList.add("active");
    } else {
      this._sidebarOverlay.classList.remove("active");
    }
  }

  restoreSidebarState() {
    const isMobile = window.innerWidth <= 768;
    const savedPref = localStorage.getItem("sidebarCollapsed");
    // 移动端首次加载默认隐藏侧边栏
    const isCollapsed = savedPref !== null ? savedPref === "true" : isMobile;
    if (isCollapsed) {
      this.sidebar.classList.add("collapsed");
      this.sidebarToggle.classList.add("collapsed");
      // 同步调整回到底部按钮位置
      if (this.scrollToBottomBtn) {
        this.scrollToBottomBtn.classList.add("sidebar-collapsed");
      }
    }
    // 创建移动端侧边栏遮罩层（延迟到 DOM 就绪）
    if (!this._sidebarOverlay) {
      this._sidebarOverlay = document.createElement("div");
      this._sidebarOverlay.className = "sidebar-overlay";
      this._sidebarOverlay.addEventListener("click", () => this.toggleSidebar());
      document.body.appendChild(this._sidebarOverlay);
    }
    this.updateSidebarOverlay();
  }

  async loadVersion() {
    try {
      const resp = await this.fetchWithAuth("/api/fkteams/version");
      const result = await resp.json();
      if (result.code === 0 && result.data?.version) {
        const el = document.getElementById("version-tag");
        if (el) {
          el.textContent = "v" + result.data.version;
          if (result.data.buildDate) {
            el.setAttribute(
              "data-tooltip",
              `v${result.data.version} (${result.data.buildDate})`,
            );
          }
        }
      }
    } catch (_) {}
  }

  connect() {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const token = this.getToken();
    const tokenParam = token ? `?token=${encodeURIComponent(token)}` : "";
    const wsUrl = `${protocol}//${window.location.host}/ws${tokenParam}`;

    const ws = new WebSocket(wsUrl);
    this.ws = ws;

    ws.onopen = () => {
      if (this.ws !== ws) return; // 旧连接的延迟事件，忽略
      this.updateStatus("connected", "已连接");
      this.reconnectAttempts = 0;
      this._startHeartbeat();
      // 如果有正在处理的任务，发送 resume 请求恢复输出流
      if (this.isProcessing && this.sessionId) {
        ws.send(
          JSON.stringify({
            type: "resume",
            session_id: this.sessionId,
          }),
        );
        this.updateStatus("processing", "处理中...");
      }
      // 加载侧边栏历史会话列表
      this.loadSidebarHistory();
    };

    ws.onclose = () => {
      if (this.ws !== ws) return; // 旧连接的延迟事件，忽略
      this._stopHeartbeat();
      this.updateStatus("disconnected", "服务未连接");
      // 连接断开时不立即重置 isProcessing，后端任务可能仍在运行
      // 重连后会通过 resume 恢复输出流，或由 processing_end 重置状态
      this.tryReconnect();
    };

    ws.onerror = (error) => {
      if (this.ws !== ws) return; // 旧连接的延迟事件，忽略
      console.error("WebSocket error:", error);
      this._stopHeartbeat();
      this.updateStatus("disconnected", "连接异常");
      // 连接错误时不立即重置 isProcessing，等待重连 resume
    };

    ws.onmessage = (event) => {
      if (this.ws !== ws) return; // 旧连接的延迟事件，忽略
      try {
        const data = JSON.parse(event.data);
        this.handleServerEvent(data);
      } catch (e) {
        console.error("Failed to parse message:", e);
      }
    };
  }

  tryReconnect() {
    if (this.reconnectAttempts < this.maxReconnectAttempts) {
      this.reconnectAttempts++;
      this.updateStatus(
        "disconnected",
        `重连中 (${this.reconnectAttempts}/${this.maxReconnectAttempts})...`,
      );
      setTimeout(() => this.connect(), 2000 * this.reconnectAttempts);
    } else {
      // 重连次数耗尽，重置处理状态避免 UI 卡死
      if (this.isProcessing) {
        this.isProcessing = false;
        this.updateSendButtonState();
      }
    }
  }

  _startHeartbeat() {
    this._stopHeartbeat();
    this._heartbeatTimer = setInterval(() => {
      if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        this.ws.send(JSON.stringify({ type: "ping" }));
      }
    }, 30000);
  }

  _stopHeartbeat() {
    if (this._heartbeatTimer) {
      clearInterval(this._heartbeatTimer);
      this._heartbeatTimer = null;
    }
  }

  updateStatus(status, text) {
    const dot = this.statusIndicator.querySelector(".status-dot");
    const textEl = this.statusIndicator.querySelector(".status-text");
    dot.className = "status-dot " + status;
    textEl.textContent = text;
  }

  handleInputChange() {
    const hasContent = this.messageInput.value.trim().length > 0;
    const hasAttachments = this.attachments && this.attachments.length > 0;
    this.sendBtn.disabled =
      (!hasContent && !hasAttachments) || this.isProcessing;
    this.messageInput.style.height = "auto";
    this.messageInput.style.height =
      Math.min(this.messageInput.scrollHeight, 120) + "px";
    this.updateSendButtonState();
  }

  handleKeyDown(e) {
    // 先处理文件建议的键盘导航
    if (this.handleFileSuggestionKeyDown(e)) {
      return;
    }

    // 处理智能体建议的键盘导航
    if (this.handleSuggestionKeyDown(e)) {
      return;
    }

    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      if (!this.sendBtn.disabled) {
        this.sendMessage();
      }
    }
  }

  setCurrentAgent(agent) {
    this.currentAgent = agent;
    // 持久化到 localStorage，页面刷新后恢复
    if (agent) {
      localStorage.setItem("fk_current_agent_" + this.sessionId, agent.name);
    } else {
      localStorage.removeItem("fk_current_agent_" + this.sessionId);
    }
    this.updateCurrentContextIndicator();
  }

  updateCurrentContextIndicator() {
    // 桌面端指示器
    if (this._contextIndicator) {
      if (this.currentAgent) {
        this._contextIndicator.className = "current-context-indicator agent-active active";
        this._contextIndicator.innerHTML = `
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/>
            <circle cx="9" cy="7" r="4"/>
            <path d="M23 21v-2a4 4 0 0 0-3-3.87"/>
            <path d="M16 3.13a4 4 0 0 1 0 7.75"/>
          </svg>
          <span class="context-label">@${this.escapeHtml(this.currentAgent.name)}</span>
          <button class="context-dismiss" title="切换回团队模式">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="18" y1="6" x2="6" y2="18"/>
              <line x1="6" y1="6" x2="18" y2="18"/>
            </svg>
          </button>
        `;
        this._contextIndicator.querySelector(".context-dismiss").addEventListener("click", (e) => {
          e.stopPropagation();
          this.resetToTeamMode();
        });
      } else {
        this._contextIndicator.className = "current-context-indicator";
        this._contextIndicator.innerHTML = "";
      }
    }

    // 移动端工具栏内嵌标签
    const mobileTag = document.getElementById("mobile-agent-tag");
    if (mobileTag) {
      if (this.currentAgent) {
        mobileTag.style.display = "flex";
        mobileTag.querySelector(".mobile-agent-tag-name").textContent = "@" + this.currentAgent.name;
      } else {
        mobileTag.style.display = "none";
      }
    }
  }

  setMode(mode) {
    this.mode = mode;
    this.modeButtons.forEach((btn) => {
      btn.classList.toggle("active", btn.dataset.mode === mode);
    });

    // 切换模式时，清除当前智能体
    if (this.currentAgent) {
      this.setCurrentAgent(null);
      this.showNotification("已切换回团队模式", "success");
    }

    // 更新状态显示
    let modeText = "未知模式";
    switch (mode) {
      case "supervisor":
        modeText = "团队模式";
        break;
      case "roundtable":
        modeText = "圆桌讨论模式";
        break;
      case "custom":
        modeText = "自定义会议模式";
        break;
      case "deep":
        modeText = "深度模式";
        break;
    }
    console.log(`已切换到: ${modeText}`);
  }

  updateSendButtonState() {
    if (this.isProcessing) {
      this.sendBtn.style.display = "none";
      this.cancelBtn.style.display = "flex";
      this.messageInput.disabled = true;
    } else {
      this.sendBtn.style.display = "flex";
      this.cancelBtn.style.display = "none";
      this.messageInput.disabled = false;
      const hasContent = this.messageInput.value.trim().length > 0;
      const hasAttachments = this.attachments && this.attachments.length > 0;
      this.sendBtn.disabled = !hasContent && !hasAttachments;
    }
  }

  scrollToBottom() {
    // 如果用户向上滚动了，不自动滚动
    if (this.userScrolledUp) {
      return;
    }
    requestAnimationFrame(() => {
      if (this.mainContent) {
        this.mainContent.scrollTop = this.mainContent.scrollHeight;
      }
    });
  }

  getCurrentTime() {
    return new Date().toLocaleTimeString("zh-CN", {
      hour: "2-digit",
      minute: "2-digit",
    });
  }

  escapeHtml(text) {
    if (!text) return "";
    if (!this._escapeDiv) {
      this._escapeDiv = document.createElement("div");
    }
    this._escapeDiv.textContent = text;
    return this._escapeDiv.innerHTML;
  }

  showNotification(message, type = "info") {
    // 添加动画样式（只添加一次）
    if (!this.notificationStyleAdded) {
      const style = document.createElement("style");
      style.id = "notification-styles";
      style.textContent = `
                @keyframes slideIn {
                    from { transform: translateX(100%); opacity: 0; }
                    to { transform: translateX(0); opacity: 1; }
                }
                @keyframes slideOut {
                    from { transform: translateX(0); opacity: 1; }
                    to { transform: translateX(100%); opacity: 0; }
                }
            `;
      document.head.appendChild(style);
      this.notificationStyleAdded = true;
    }

    // 限制最多同时显示3个通知
    if (this.activeNotifications.length >= 3) {
      const oldest = this.activeNotifications.shift();
      this.removeNotification(oldest);
    }

    // 创建通知元素
    const notification = document.createElement("div");
    const bgColor =
      type === "success"
        ? "#66bb6a"
        : type === "error"
          ? "#ef5350"
          : type === "warning"
            ? "#ffa726"
            : "#42a5f5";

    // 计算通知的位置（堆叠显示）
    const topOffset = 20 + this.activeNotifications.length * 70;

    notification.style.cssText = `
            position: fixed;
            top: ${topOffset}px;
            right: 20px;
            max-width: calc(100vw - 40px);
            background: ${bgColor};
            color: white;
            padding: 12px 20px;
            border-radius: 6px;
            font-size: 14px;
            z-index: 1000;
            box-shadow: 0 2px 8px rgba(0,0,0,0.2);
            animation: slideIn 0.3s ease;
            transition: top 0.3s ease;
            word-break: break-all;
            box-sizing: border-box;
        `;
    notification.textContent = message;

    document.body.appendChild(notification);
    this.activeNotifications.push(notification);

    // 3秒后自动移除
    setTimeout(() => {
      this.removeNotification(notification);
    }, 3000);
  }

  removeNotification(notification) {
    if (!notification || !notification.parentNode) return;

    notification.style.animation = "slideOut 0.3s ease";
    setTimeout(() => {
      if (notification.parentNode) {
        document.body.removeChild(notification);
      }

      // 从活动列表中移除
      const index = this.activeNotifications.indexOf(notification);
      if (index > -1) {
        this.activeNotifications.splice(index, 1);
      }

      // 更新剩余通知的位置
      this.activeNotifications.forEach((notif, idx) => {
        notif.style.top = 20 + idx * 70 + "px";
      });
    }, 300);
  }

  // ===== 手绘风格 Tooltip 系统 =====
  initTooltips() {
    this._tooltipEl = null;
    this._tooltipTimer = null;

    document.addEventListener("mouseover", (e) => {
      const el = e.target;
      if (!el || typeof el.closest !== "function") return;
      const target = el.closest("[data-tooltip]");
      if (!target) return;
      if (this._currentTooltipTarget === target) return;
      this._currentTooltipTarget = target;
      this._showTooltip(target);
    });

    document.addEventListener("mouseout", (e) => {
      const el = e.target;
      if (!el || typeof el.closest !== "function") return;
      const target = el.closest("[data-tooltip]");
      if (!target) return;
      const rel = e.relatedTarget;
      const related =
        rel && typeof rel.closest === "function"
          ? rel.closest("[data-tooltip]")
          : null;
      if (related === target) return;
      this._currentTooltipTarget = null;
      this._hideTooltip();
    });
  }

  _showTooltip(target) {
    clearTimeout(this._tooltipTimer);

    this._tooltipTimer = setTimeout(() => {
      const text = target.getAttribute("data-tooltip");
      if (!text) return;

      if (!this._tooltipEl) {
        this._tooltipEl = document.createElement("div");
        this._tooltipEl.className = "sketch-tooltip";
        this._tooltipEl.innerHTML =
          '<span class="sketch-tooltip-text"></span><span class="sketch-tooltip-arrow"></span>';
        document.body.appendChild(this._tooltipEl);
      }

      const tooltip = this._tooltipEl;
      tooltip.querySelector(".sketch-tooltip-text").textContent = text;
      tooltip.classList.remove("visible");
      tooltip.style.display = "block";

      const rect = target.getBoundingClientRect();
      const tipRect = tooltip.getBoundingClientRect();
      const arrow = tooltip.querySelector(".sketch-tooltip-arrow");
      arrow.className = "sketch-tooltip-arrow";

      const placement = this._getTooltipPlacement(target, rect, tipRect);

      let top, left;
      switch (placement) {
        case "right":
          top = rect.top + rect.height / 2 - tipRect.height / 2;
          left = rect.right + 8;
          arrow.classList.add("arrow-left");
          break;
        case "left":
          top = rect.top + rect.height / 2 - tipRect.height / 2;
          left = rect.left - tipRect.width - 8;
          arrow.classList.add("arrow-right");
          break;
        case "top":
          top = rect.top - tipRect.height - 8;
          left = rect.left + rect.width / 2 - tipRect.width / 2;
          arrow.classList.add("arrow-bottom");
          break;
        case "bottom":
          top = rect.bottom + 8;
          left = rect.left + rect.width / 2 - tipRect.width / 2;
          arrow.classList.add("arrow-top");
          break;
      }

      const vw = window.innerWidth;
      const vh = window.innerHeight;
      if (left < 4) left = 4;
      if (left + tipRect.width > vw - 4) left = vw - tipRect.width - 4;
      if (top < 4) top = 4;
      if (top + tipRect.height > vh - 4) top = vh - tipRect.height - 4;

      tooltip.style.top = top + "px";
      tooltip.style.left = left + "px";
      tooltip.classList.add("visible");
    }, 200);
  }

  _hideTooltip() {
    clearTimeout(this._tooltipTimer);
    if (this._tooltipEl) {
      this._tooltipEl.classList.remove("visible");
    }
  }

  _getTooltipPlacement(target, rect, tipRect) {
    if (target.closest(".history-item-actions")) {
      return "top";
    }
    const vw = window.innerWidth;
    if (rect.right + tipRect.width + 12 < vw) return "right";
    if (rect.left - tipRect.width - 12 > 0) return "left";
    if (rect.top - tipRect.height - 12 > 0) return "top";
    return "bottom";
  }
}

document.addEventListener("DOMContentLoaded", () => {
  window.app = new FKTeamsChat();
  window.fkteamsChat = window.app; // 保持向后兼容
});
