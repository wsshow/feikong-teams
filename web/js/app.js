/**
 * 非空小队 - AI 对话前端应用
 */

class FKTeamsChat {
    constructor() {
        this.ws = null;
        this.sessionId = 'default';
        this.mode = 'supervisor';
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
        this.fileSuggestions = null; // 文件建议弹窗
        this.selectedFileIndex = -1; // 当前选中的文件索引
        this.currentPath = ''; // 当前浏览的路径

        this.init();
    }

    init() {
        this.bindElements();
        this.bindEvents();
        this.restoreSidebarState();
        this.loadAgents(); // 加载智能体列表
        this.connect();
    }

    bindElements() {
        this.messagesContainer = document.getElementById('messages');
        this.messagesWrapper = document.getElementById('messages-wrapper');
        this.messageInput = document.getElementById('message-input');
        this.sendBtn = document.getElementById('send-btn');
        this.cancelBtn = document.getElementById('cancel-btn');
        this.sessionIdInput = document.getElementById('session-id');
        this.statusIndicator = document.getElementById('status-indicator');
        this.clearBtn = document.getElementById('clear-chat');
        this.exportBtn = document.getElementById('export-html');
        this.historyBtn = document.getElementById('history-btn');
        this.historyModal = document.getElementById('history-modal');
        this.historyModalClose = document.getElementById('history-modal-close');
        this.historyList = document.getElementById('history-list');
        this.renameModal = document.getElementById('rename-modal');
        this.renameModalClose = document.getElementById('rename-modal-close');
        this.renameInput = document.getElementById('rename-input');
        this.renameCancelBtn = document.getElementById('rename-cancel-btn');
        this.renameConfirmBtn = document.getElementById('rename-confirm-btn');
        this.deleteModal = document.getElementById('delete-modal');
        this.deleteModalClose = document.getElementById('delete-modal-close');
        this.deleteCancelBtn = document.getElementById('delete-cancel-btn');
        this.deleteConfirmBtn = document.getElementById('delete-confirm-btn');
        this.deleteFilenameSpan = document.getElementById('delete-filename');
        this.modeButtons = document.querySelectorAll('.mode-btn');
        this.sidebar = document.getElementById('sidebar');
        this.sidebarToggle = document.getElementById('sidebar-toggle');
        this.mainContent = document.getElementById('main-content');
        this.scrollToBottomBtn = document.getElementById('scroll-to-bottom');
        this.chatLoading = document.getElementById('chat-loading');
        this.quickNavBars = document.getElementById('quick-nav-bars');
        this.quickNavPanel = document.getElementById('quick-nav-panel');
        this.quickNavPanelList = document.getElementById('quick-nav-panel-list');
    }

    bindEvents() {
        this.sendBtn.addEventListener('click', () => this.sendMessage());
        this.cancelBtn.addEventListener('click', () => this.cancelTask());
        this.messageInput.addEventListener('input', () => {
            this.handleInputChange();
            this.handleInputForMention();
        });
        this.messageInput.addEventListener('keydown', (e) => this.handleKeyDown(e));
        this.sessionIdInput.addEventListener('change', () => {
            const newSessionId = this.sessionIdInput.value || 'default';
            if (newSessionId !== this.sessionId) {
                this.sessionId = newSessionId;
                this.checkAndLoadSessionHistory(newSessionId);
            }
        });
        this.sessionIdInput.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                this.sessionIdInput.blur(); // 失去焦点，触发 change 事件
            }
        });
        this.clearBtn.addEventListener('click', () => this.clearChat());
        this.exportBtn.addEventListener('click', () => this.exportToHTML());
        this.historyBtn.addEventListener('click', () => this.showHistoryModal());
        this.historyModalClose.addEventListener('click', () => this.hideHistoryModal());
        // 点击背景关闭弹窗
        this.historyModal.addEventListener('click', (e) => {
            if (e.target === this.historyModal) {
                this.hideHistoryModal();
            }
        });
        // 重命名弹窗事件
        this.renameModalClose.addEventListener('click', () => this.hideRenameModal());
        this.renameCancelBtn.addEventListener('click', () => this.hideRenameModal());
        this.renameConfirmBtn.addEventListener('click', () => this.confirmRename());
        this.renameModal.addEventListener('click', (e) => {
            if (e.target === this.renameModal) {
                this.hideRenameModal();
            }
        });
        this.renameInput.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                this.confirmRename();
            } else if (e.key === 'Escape') {
                this.hideRenameModal();
            }
        });
        // 删除弹窗事件
        this.deleteModalClose.addEventListener('click', () => this.hideDeleteModal());
        this.deleteCancelBtn.addEventListener('click', () => this.hideDeleteModal());
        this.deleteConfirmBtn.addEventListener('click', () => this.confirmDelete());
        this.deleteModal.addEventListener('click', (e) => {
            if (e.target === this.deleteModal) {
                this.hideDeleteModal();
            }
        });
        this.modeButtons.forEach(btn => {
            btn.addEventListener('click', () => this.setMode(btn.dataset.mode));
        });
        if (this.sidebarToggle) {
            this.sidebarToggle.addEventListener('click', () => this.toggleSidebar());
        }
        // 监听滚动事件
        if (this.mainContent) {
            this.mainContent.addEventListener('scroll', () => this.handleScroll());
        }
        // 回到底部按钮
        if (this.scrollToBottomBtn) {
            this.scrollToBottomBtn.addEventListener('click', () => this.scrollToBottomAndResume());
        }
    }

    handleScroll() {
        const { scrollTop, scrollHeight, clientHeight } = this.mainContent;
        const distanceFromBottom = scrollHeight - scrollTop - clientHeight;

        // 如果距离底部超过 100px，认为用户向上滚动了
        if (distanceFromBottom > 100) {
            this.userScrolledUp = true;
            this.showScrollToBottomBtn(true);
        } else {
            // 用户回到了底部附近
            this.userScrolledUp = false;
            this.showScrollToBottomBtn(false);
        }
    }

    showScrollToBottomBtn(show) {
        if (this.scrollToBottomBtn) {
            if (show) {
                this.scrollToBottomBtn.style.display = 'flex';
                // 触发重排以启动动画
                this.scrollToBottomBtn.offsetHeight;
                this.scrollToBottomBtn.style.opacity = '1';
                this.scrollToBottomBtn.style.transform = 'translateX(calc(-50% + var(--sidebar-width) / 2)) translateY(0)';
            } else {
                this.scrollToBottomBtn.style.opacity = '0';
                this.scrollToBottomBtn.style.transform = 'translateX(calc(-50% + var(--sidebar-width) / 2)) translateY(20px)';
                setTimeout(() => {
                    if (this.scrollToBottomBtn.style.opacity === '0') {
                        this.scrollToBottomBtn.style.display = 'none';
                    }
                }, 200); // 等待动画完成
            }
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
        const isCollapsed = this.sidebar.classList.toggle('collapsed');
        this.sidebarToggle.classList.toggle('collapsed', isCollapsed);
        // 调整回到底部按钮的位置
        if (this.scrollToBottomBtn) {
            this.scrollToBottomBtn.classList.toggle('sidebar-collapsed', isCollapsed);
        }
        // 调整快速导航按钮和菜单的位置
        localStorage.setItem('sidebarCollapsed', isCollapsed);
    }

    restoreSidebarState() {
        const isCollapsed = localStorage.getItem('sidebarCollapsed') === 'true';
        if (isCollapsed) {
            this.sidebar.classList.add('collapsed');
            this.sidebarToggle.classList.add('collapsed');
            // 同步调整回到底部按钮位置
            if (this.scrollToBottomBtn) {
                this.scrollToBottomBtn.classList.add('sidebar-collapsed');
            }
            // 同步调整快速导航按钮和菜单位置
        }
    }

    connect() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws`;

        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = () => {
            this.updateStatus('connected', '已连接');
            this.reconnectAttempts = 0;
            // 连接成功后，检查并加载默认会话的历史记录
            this.checkAndLoadSessionHistory(this.sessionId);
        };

        this.ws.onclose = () => {
            this.updateStatus('disconnected', '连接断开');
            this.tryReconnect();
        };

        this.ws.onerror = (error) => {
            console.error('WebSocket error:', error);
            this.updateStatus('disconnected', '连接错误');
        };

        this.ws.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                this.handleServerEvent(data);
            } catch (e) {
                console.error('Failed to parse message:', e);
            }
        };
    }

    tryReconnect() {
        if (this.reconnectAttempts < this.maxReconnectAttempts) {
            this.reconnectAttempts++;
            this.updateStatus('disconnected', `重连中 (${this.reconnectAttempts}/${this.maxReconnectAttempts})...`);
            setTimeout(() => this.connect(), 2000 * this.reconnectAttempts);
        }
    }

    updateStatus(status, text) {
        const dot = this.statusIndicator.querySelector('.status-dot');
        const textEl = this.statusIndicator.querySelector('.status-text');
        dot.className = 'status-dot ' + status;
        textEl.textContent = text;
    }

    handleInputChange() {
        const hasContent = this.messageInput.value.trim().length > 0;
        this.sendBtn.disabled = !hasContent || this.isProcessing;
        this.messageInput.style.height = 'auto';
        this.messageInput.style.height = Math.min(this.messageInput.scrollHeight, 120) + 'px';
        this.updateSendButtonState();
    }

    updateSendButtonState() {
        if (this.isProcessing) {
            this.sendBtn.textContent = '处理中';
            this.sendBtn.classList.add('processing');
            this.sendBtn.disabled = true;
        } else {
            this.sendBtn.textContent = '发送';
            this.sendBtn.classList.remove('processing');
            const hasContent = this.messageInput.value.trim().length > 0;
            this.sendBtn.disabled = !hasContent;
        }
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

        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            if (!this.sendBtn.disabled) {
                this.sendMessage();
            }
        }
    }

    setMode(mode) {
        this.mode = mode;
        this.modeButtons.forEach(btn => {
            btn.classList.toggle('active', btn.dataset.mode === mode);
        });

        // 切换模式时，清除当前智能体
        if (this.currentAgent) {
            this.currentAgent = null;
            this.showNotification('已切换回团队模式', 'success');
        }

        // 更新状态显示
        let modeText = '未知模式';
        switch (mode) {
            case 'supervisor':
                modeText = '团队模式';
                break;
            case 'roundtable':
                modeText = '圆桌讨论模式';
                break;
            case 'custom':
                modeText = '自定义会议模式';
                break;
            case 'deep':
                modeText = '深度模式';
                break;
        }
        console.log(`已切换到: ${modeText}`);
    }

    updateSendButtonState() {
        if (this.isProcessing) {
            this.sendBtn.style.display = 'none';
            this.cancelBtn.style.display = 'flex';
            this.messageInput.disabled = true;
        } else {
            this.sendBtn.style.display = 'flex';
            this.cancelBtn.style.display = 'none';
            this.messageInput.disabled = false;
            const hasContent = this.messageInput.value.trim().length > 0;
            this.sendBtn.disabled = !hasContent;
        }
    }

    sendMessage() {
        const message = this.messageInput.value.trim();
        if (!message || this.isProcessing) return;

        const welcomeMsg = this.messagesContainer.querySelector('.welcome-message');
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
            const agent = this.agents.find(a => a.name === mention.agentName);
            if (agent) {
                this.currentAgent = agent;

                // 显示切换通知
                this.showAgentSwitchNotification(agent.name, agent.description);
            } else {
                // 智能体不存在，显示错误
                this.showNotification(`未找到智能体: ${mention.agentName}`, 'error');
                return;
            }
        }

        this.addUserMessage(message);

        // 发送消息 - 始终发送完整的原始消息（包括@智能体和#文件部分）
        // 如果指定了智能体则包含agent_name字段
        // 如果有文件路径则包含file_paths字段
        const payload = {
            type: 'chat',
            session_id: this.sessionId,
            message: message,  // 发送完整的原始消息
            mode: this.mode
        };

        if (this.currentAgent) {
            payload.agent_name = this.currentAgent.name;
        }

        if (filePaths.length > 0) {
            payload.file_paths = filePaths;
        }

        this.ws.send(JSON.stringify(payload));

        this.messageInput.value = '';
        this.handleInputChange();
        this.isProcessing = true;
        this.updateSendButtonState();
        this.updateStatus('processing', '处理中...');
    }

    // 显示智能体切换通知
    showAgentSwitchNotification(agentName, description) {
        const notificationEl = document.createElement('div');
        notificationEl.className = 'action-event agent-switch';
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
    }

    // 重置回团队模式
    resetToTeamMode() {
        this.currentAgent = null;
        const resetNotificationEl = document.createElement('div');
        resetNotificationEl.className = 'action-event agent-switch';
        resetNotificationEl.innerHTML = `
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
                <circle cx="9" cy="7" r="4" />
                <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
                <path d="M16 3.13a4 4 0 0 1 0 7.75" />
            </svg>
            <span>已切换回 <strong>${this.mode === 'supervisor' ? '团队模式' : this.mode === 'roundtable' ? '圆桌讨论模式' : '自定义会议模式'}</strong></span>
        `;
        this.messagesContainer.appendChild(resetNotificationEl);
        this.scrollToBottom();
        this.showNotification('已切换回团队模式', 'success');
    }

    handleServerEvent(event) {
        switch (event.type) {
            case 'connected':
                break;
            case 'processing_start':
                this.isProcessing = true;
                this.updateStatus('processing', '处理中...');
                break;
            case 'processing_end':
                this.isProcessing = false;
                this.updateStatus('connected', '已连接');
                this.updateSendButtonState();
                this.currentMessageElement = null;
                this.hasToolCallAfterMessage = false;
                break;
            case 'cancelled':
                this.handleCancelled(event);
                break;
            case 'history_cleared':
                this.showNotification('历史记录已清除', 'success');
                break;
            case 'history_loaded':
                this.handleHistoryLoaded(event);
                break;
            case 'stream_chunk':
                this.handleStreamChunk(event);
                break;
            case 'message':
                this.handleMessage(event);
                break;
            case 'tool_calls_preparing':
                this.handleToolCallsPreparing(event);
                break;
            case 'tool_calls':
                this.handleToolCalls(event);
                break;
            case 'tool_result':
            case 'tool_result_chunk':
                this.handleToolResult(event);
                break;
            case 'action':
                this.handleAction(event);
                break;
            case 'error':
                this.handleError(event);
                break;
            default:
                console.log('Unknown event:', event);
        }
        this.scrollToBottom();
    }

    trimLeadingWhitespace(text) {
        if (!text) return '';
        return text.replace(/^[\s\n\r\u00A0\u2000-\u200B\uFEFF]+/, '');
    }

    // 渲染 Markdown
    renderMarkdown(text) {
        if (!text) return '';
        try {
            if (typeof marked !== 'undefined') {
                marked.setOptions({
                    breaks: true,
                    gfm: true
                });
                return marked.parse(text);
            }
        } catch (e) {
            console.error('Markdown parse error:', e);
        }
        return this.escapeHtml(text);
    }

    handleStreamChunk(event) {
        // 检查是否需要创建新卡片：工具调用后、没有当前元素、或者 agent 名称变化
        const currentAgentName = this.currentMessageElement?.getAttribute('data-agent');
        const needNewCard = this.hasToolCallAfterMessage ||
            !this.currentMessageElement ||
            (event.agent_name && currentAgentName !== event.agent_name);

        if (needNewCard) {
            this.currentMessageElement = this.createAssistantMessage(event.agent_name);
            this.hasToolCallAfterMessage = false;
        }

        const bodyEl = this.currentMessageElement.querySelector('.message-body');
        if (bodyEl) {
            const indicator = bodyEl.querySelector('.streaming-indicator');
            if (indicator) indicator.remove();

            // 获取原始文本
            let rawText = bodyEl.getAttribute('data-raw') || '';
            let newContent = event.content || '';

            if (rawText === '') {
                newContent = this.trimLeadingWhitespace(newContent);
            }

            rawText += newContent;
            bodyEl.setAttribute('data-raw', rawText);

            // 实时渲染 Markdown
            bodyEl.innerHTML = this.renderMarkdown(rawText);
        }
    }

    handleMessage(event) {
        if (!event.content) return;

        // 检查是否需要创建新卡片：工具调用后、没有当前元素、或者 agent 名称变化
        const currentAgentName = this.currentMessageElement?.getAttribute('data-agent');
        const needNewCard = this.hasToolCallAfterMessage ||
            !this.currentMessageElement ||
            (event.agent_name && currentAgentName !== event.agent_name);

        if (needNewCard) {
            this.currentMessageElement = this.createAssistantMessage(event.agent_name);
            this.hasToolCallAfterMessage = false;
        }

        const bodyEl = this.currentMessageElement.querySelector('.message-body');
        if (bodyEl) {
            const indicator = bodyEl.querySelector('.streaming-indicator');
            if (indicator) indicator.remove();

            const content = this.trimLeadingWhitespace(event.content);
            bodyEl.setAttribute('data-raw', content);
            bodyEl.innerHTML = this.renderMarkdown(content);
        }
    }

    handleToolCallsPreparing(event) {
        if (!event.tool_calls || event.tool_calls.length === 0) return;

        this.hasToolCallAfterMessage = true;

        const toolName = event.tool_calls[0].name;
        const toolCallEl = document.createElement('div');
        toolCallEl.className = 'tool-call';
        toolCallEl.innerHTML = `
            <div class="tool-call-header">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <circle cx="12" cy="12" r="3"/>
                    <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>
                </svg>
                <span>准备调用工具:</span>
                <code class="tool-call-name">${this.escapeHtml(toolName)}</code>
            </div>
            <pre class="tool-call-args">参数准备中...</pre>
        `;
        this.messagesContainer.appendChild(toolCallEl);
    }

    handleToolCalls(event) {
        if (!event.tool_calls || event.tool_calls.length === 0) return;

        const toolCalls = this.messagesContainer.querySelectorAll('.tool-call');
        const lastToolCall = toolCalls[toolCalls.length - 1];
        if (lastToolCall) {
            const argsEl = lastToolCall.querySelector('.tool-call-args');
            if (argsEl && event.tool_calls[0].arguments) {
                try {
                    const args = JSON.parse(event.tool_calls[0].arguments);
                    argsEl.textContent = JSON.stringify(args, null, 2);
                } catch {
                    argsEl.textContent = event.tool_calls[0].arguments;
                }
            }
        }
    }

    handleToolResult(event) {
        let content = event.content || '';
        let formattedContent = content;

        try {
            const parsed = JSON.parse(content);
            formattedContent = JSON.stringify(parsed, null, 2);
            if (formattedContent.length > 2048) {
                formattedContent = formattedContent.substring(0, 2048) + '\n...';
            }
        } catch {
            if (content.length > 2048) {
                formattedContent = content.substring(0, 2048) + '\n...';
            }
        }

        const toolResultEl = document.createElement('div');
        toolResultEl.className = 'tool-result';
        toolResultEl.innerHTML = `
            <div class="tool-result-header">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <polyline points="20 6 9 17 4 12"/>
                </svg>
                <span>执行结果</span>
            </div>
            <pre class="tool-result-content">${this.escapeHtml(formattedContent)}</pre>
        `;
        this.messagesContainer.appendChild(toolResultEl);
    }

    handleAction(event) {
        let actionClass = '';
        let actionIcon = '';

        switch (event.action_type) {
            case 'transfer':
                actionClass = 'transfer';
                actionIcon = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M17 1l4 4-4 4"/><path d="M3 11V9a4 4 0 0 1 4-4h14"/>
                    <path d="M7 23l-4-4 4-4"/><path d="M21 13v2a4 4 0 0 1-4 4H3"/>
                </svg>`;
                break;
            case 'exit':
                actionClass = 'exit';
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

        const actionEl = document.createElement('div');
        actionEl.className = `action-event ${actionClass}`;
        actionEl.innerHTML = `${actionIcon}<span>[${this.escapeHtml(event.agent_name)}] ${this.escapeHtml(event.content || event.action_type)}</span>`;
        this.messagesContainer.appendChild(actionEl);
    }

    handleError(event) {
        const errorEl = document.createElement('div');
        errorEl.className = 'error-message';
        errorEl.innerHTML = `
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10"/>
                <line x1="15" y1="9" x2="9" y2="15"/>
                <line x1="9" y1="9" x2="15" y2="15"/>
            </svg>
            <span>${event.agent_name ? `[${this.escapeHtml(event.agent_name)}] ` : ''}${this.escapeHtml(event.error)}</span>
        `;
        this.messagesContainer.appendChild(errorEl);
        this.isProcessing = false;
        this.updateStatus('connected', '已连接');
        this.updateSendButtonState();
    }

    addUserMessage(content) {
        const messageEl = document.createElement('div');
        messageEl.className = 'message user';
        messageEl.setAttribute('data-message-id', `msg-${Date.now()}`);
        messageEl.innerHTML = `
            <div class="message-content">
                <div class="message-header">
                    <span class="message-name">您</span>
                    <span class="message-time">${this.getCurrentTime()}</span>
                </div>
                <div class="message-body">${this.escapeHtml(content)}</div>
            </div>
        `;
        this.messagesContainer.appendChild(messageEl);

        // 添加到问题列表
        this.addQuestionToNav(content, messageEl);

        this.scrollToBottom();
    }

    createAssistantMessage(agentName, timeInfo = null) {
        const messageEl = document.createElement('div');
        messageEl.className = 'message assistant';
        messageEl.setAttribute('data-agent', agentName || '');

        // 如果提供了时间信息，使用历史时间；否则使用当前时间
        const timeDisplay = timeInfo ? this.formatHistoryTime(timeInfo) : this.getCurrentTime();

        messageEl.innerHTML = `
            <div class="message-content">
                <div class="message-header">
                    <span class="message-name">${this.escapeHtml(agentName || 'Assistant')}</span>
                    <span class="agent-tag">${this.escapeHtml(agentName || 'AI')}</span>
                    <span class="message-time">${timeDisplay}</span>
                </div>
                <div class="message-body"><span class="streaming-indicator"><span></span><span></span><span></span></span></div>
            </div>
        `;
        this.messagesContainer.appendChild(messageEl);
        this.scrollToBottom();
        return messageEl;
    }

    cancelTask() {
        if (!this.isProcessing) return;

        // 发送取消消息
        this.ws.send(JSON.stringify({
            type: 'cancel'
        }));

        this.showNotification('正在取消任务...', 'info');
    }

    handleCancelled(event) {
        this.isProcessing = false;
        this.updateStatus('connected', '已连接');
        this.updateSendButtonState();
        this.currentMessageElement = null;
        this.hasToolCallAfterMessage = false;

        // 添加取消提示
        const cancelEl = document.createElement('div');
        cancelEl.className = 'action-event cancelled';
        cancelEl.innerHTML = `
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10"/>
                <line x1="15" y1="9" x2="9" y2="15"/>
                <line x1="9" y1="9" x2="15" y2="15"/>
            </svg>
            <span>${this.escapeHtml(event.message || '任务已取消')}</span>
        `;
        this.messagesContainer.appendChild(cancelEl);

        this.showNotification('任务已取消', 'success');
    }

    clearChat() {
        // 发送清除历史的消息到后端
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.ws.send(JSON.stringify({
                type: 'clear_history',
                session_id: this.sessionId
            }));
        }

        this.clearChatUI();
    }

    clearChatUI() {
        // 只清空界面，不发送删除历史的消息到后端
        this.messagesContainer.innerHTML = `
            <div class="welcome-message">
                <div class="welcome-icon">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                        <path d="M12 2L2 7l10 5 10-5-10-5z"/>
                        <path d="M2 17l10 5 10-5"/>
                        <path d="M2 12l10 5 10-5"/>
                    </svg>
                </div>
                <h2>非空小队</h2>
                <p>多智能体协作系统，开始您的对话</p>
            </div>
        `;
        this.currentMessageElement = null;
        this.hasToolCallAfterMessage = false;

        // 清空问题导航
        this.clearQuickNav();
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
        return new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
    }

    escapeHtml(text) {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    exportToHTML() {
        const messagesContainer = document.getElementById('messages');
        if (!messagesContainer) return;

        // 获取当前会话ID用于文件名
        const sessionId = this.sessionId || 'default';
        const timestamp = new Date().toISOString().slice(0, 19).replace(/[:.]/g, '-');
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
            <div>导出时间: ${new Date().toLocaleString('zh-CN')}</div>
        </div>
    </div>
    <div class="messages">
        ${messagesContainer.innerHTML}
    </div>
</body>
</html>`;

        // 创建并下载文件
        const blob = new Blob([htmlTemplate], { type: 'text/html;charset=utf-8' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);

        // 显示成功提示
        this.showNotification(`对话记录已导出为 ${filename}`, 'success');
    }

    showNotification(message, type = 'info') {
        // 添加动画样式（只添加一次）
        if (!this.notificationStyleAdded) {
            const style = document.createElement('style');
            style.id = 'notification-styles';
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
        const notification = document.createElement('div');
        const bgColor = type === 'success' ? '#66bb6a' : type === 'error' ? '#ef5350' : '#42a5f5';

        // 计算通知的位置（堆叠显示）
        const topOffset = 20 + (this.activeNotifications.length * 70);

        notification.style.cssText = `
            position: fixed;
            top: ${topOffset}px;
            right: 20px;
            background: ${bgColor};
            color: white;
            padding: 12px 20px;
            border-radius: 6px;
            font-size: 14px;
            z-index: 1000;
            box-shadow: 0 2px 8px rgba(0,0,0,0.2);
            animation: slideIn 0.3s ease;
            transition: top 0.3s ease;
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

        notification.style.animation = 'slideOut 0.3s ease';
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
                notif.style.top = (20 + idx * 70) + 'px';
            });
        }, 300);
    }

    // ===== 历史记录管理 =====

    async showHistoryModal() {
        this.historyModal.style.display = 'flex';
        await this.loadHistoryFiles();
    }

    hideHistoryModal() {
        this.historyModal.style.display = 'none';
    }

    async loadHistoryFiles() {
        this.historyList.innerHTML = '<div class="history-loading">加载中...</div>';

        try {
            const response = await fetch('/api/fkteams/history/files');
            if (!response.ok) {
                throw new Error('加载失败');
            }

            const result = await response.json();
            if (result.code !== 0) {
                throw new Error(result.message || '加载失败');
            }
            this.renderHistoryList(result.data.files);
        } catch (error) {
            console.error('Error loading history files:', error);
            this.historyList.innerHTML = '<div class="history-error">加载历史文件失败</div>';
        }
    }

    renderHistoryList(files) {
        if (!files || files.length === 0) {
            this.historyList.innerHTML = '<div class="history-empty">暂无历史记录</div>';
            return;
        }

        // 按修改时间排序（最新的在前）
        files.sort((a, b) => new Date(b.mod_time) - new Date(a.mod_time));

        const listHTML = files.map(file => `
            <div class="history-item" data-filename="${this.escapeHtml(file.filename)}">
                <div class="history-item-info">
                    <div class="history-item-name">${this.escapeHtml(file.display_name)}</div>
                    <div class="history-item-meta">
                        <span class="history-item-time">${this.formatTime(file.mod_time)}</span>
                        <span class="history-item-size">${this.formatSize(file.size)}</span>
                    </div>
                </div>
                <div class="history-item-actions">
                    <button class="history-action-btn load-btn" title="加载">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <path d="M3 15v4a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-4M7 10l5 5 5-5M12 15V3"/>
                        </svg>
                    </button>
                    <button class="history-action-btn rename-btn" title="重命名">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <path d="M12 20h9M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"/>
                        </svg>
                    </button>
                    <button class="history-action-btn delete-btn" title="删除">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <polyline points="3 6 5 6 21 6"/>
                            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
                        </svg>
                    </button>
                </div>
            </div>
        `).join('');

        this.historyList.innerHTML = listHTML;

        // 绑定事件
        this.historyList.querySelectorAll('.load-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const item = e.target.closest('.history-item');
                const filename = item.dataset.filename;
                this.loadHistoryFile(filename);
            });
        });

        this.historyList.querySelectorAll('.rename-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const item = e.target.closest('.history-item');
                const filename = item.dataset.filename;
                this.renameHistoryFile(filename);
            });
        });

        this.historyList.querySelectorAll('.delete-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const item = e.target.closest('.history-item');
                const filename = item.dataset.filename;
                this.deleteHistoryFile(filename);
            });
        });
    }

    loadHistoryFile(filename) {
        // 通过 WebSocket 加载历史文件
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.showChatLoading();
            this.ws.send(JSON.stringify({
                type: 'load_history',
                message: filename
            }));
            this.hideHistoryModal();
        } else {
            this.showNotification('WebSocket 未连接', 'error');
        }
    }

    showChatLoading() {
        if (this.chatLoading) {
            this.chatLoading.style.display = 'flex';
        }
    }

    hideChatLoading() {
        if (this.chatLoading) {
            this.chatLoading.style.display = 'none';
        }
    }

    async checkAndLoadSessionHistory(sessionId) {
        try {
            // 构造文件名
            const filename = `fkteams_chat_history_${sessionId}`;

            // 检查文件是否存在
            const response = await fetch('/api/fkteams/history/files');
            if (!response.ok) {
                // 如果无法获取文件列表，只清空界面不删除历史
                this.clearChatUI();
                return;
            }

            const result = await response.json();
            if (result.code !== 0 || !result.data || !result.data.files) {
                // 如果没有历史文件，只清空界面不删除历史
                this.clearChatUI();
                return;
            }

            // 查找是否存在该会话的历史文件
            const fileExists = result.data.files.some(file =>
                file.filename === filename || file === filename
            );

            if (fileExists) {
                // 存在历史记录，加载它
                this.loadHistoryFile(filename);
            } else {
                // 不存在历史记录，只清空界面显示新会话
                this.clearChatUI();
                this.showNotification(`已切换到新会话: ${sessionId}`, 'success');
            }
        } catch (error) {
            console.error('Error checking session history:', error);
            // 出错时只清空界面，不删除历史
            this.clearChatUI();
        }
    }

    handleHistoryLoaded(event) {
        // 隐藏loading
        this.hideChatLoading();

        // 清空当前消息
        this.messagesContainer.innerHTML = '';
        this.currentMessageElement = null;
        this.hasToolCallAfterMessage = false;

        // 清空快速导航（将重新构建）
        this.clearQuickNav();

        // 更新 session ID
        if (event.session_id) {
            this.sessionId = event.session_id;
            this.sessionIdInput.value = event.session_id;
        }

        // 渲染历史消息
        if (event.messages && event.messages.length > 0) {
            event.messages.forEach(msg => {
                // 检查是否是用户消息
                if (msg.agent_name === '用户') {
                    // 渲染用户消息
                    this.renderHistoryUserMessage(msg);
                    return;
                }

                const timeInfo = {
                    startTime: msg.start_time,
                    endTime: msg.end_time
                };

                // 如果有 events 数组，按时间顺序渲染每个事件
                if (msg.events && msg.events.length > 0) {
                    let currentMessageEl = null;
                    let currentContent = '';

                    msg.events.forEach(evt => {
                        switch (evt.type) {
                            case 'text':
                                // 累积文本内容
                                currentContent += evt.content;
                                // 如果还没有创建消息元素，创建一个
                                if (!currentMessageEl) {
                                    currentMessageEl = this.createAssistantMessage(msg.agent_name, timeInfo);
                                }
                                // 更新消息体
                                const bodyEl = currentMessageEl.querySelector('.message-body');
                                if (bodyEl) {
                                    bodyEl.setAttribute('data-raw', currentContent);
                                    bodyEl.innerHTML = this.renderMarkdown(currentContent);
                                }
                                break;

                            case 'tool_call':
                                // 渲染单个工具调用
                                if (evt.tool_call) {
                                    this.renderSingleToolCall(evt.tool_call);
                                }
                                // 重置当前消息元素和内容，后续文本会创建新卡片
                                currentMessageEl = null;
                                currentContent = '';
                                break;

                            case 'action':
                                // 渲染单个 action 事件
                                if (evt.action) {
                                    this.renderSingleAction(evt.action, msg.agent_name);
                                }
                                break;
                        }
                    });
                } else {
                    // 兼容旧格式（没有 events 字段的历史记录）
                    const messageEl = this.createAssistantMessage(msg.agent_name, timeInfo);
                    const bodyEl = messageEl.querySelector('.message-body');
                    if (bodyEl && msg.content) {
                        bodyEl.setAttribute('data-raw', msg.content);
                        bodyEl.innerHTML = this.renderMarkdown(msg.content);
                    }

                    // 渲染工具调用（如果有）
                    if (msg.tool_calls && msg.tool_calls.length > 0) {
                        this.renderHistoryToolCalls(msg.tool_calls);
                    }

                    // 渲染 action 事件（如果有）
                    if (msg.actions && msg.actions.length > 0) {
                        this.renderHistoryActions(msg.actions, msg.agent_name);
                    }
                }
            });
            this.showNotification(`已加载 ${event.messages.length} 条历史消息`, 'success');
        } else {
            this.showNotification('历史记录为空', 'info');
        }

        this.scrollToBottom();
    }

    renderHistoryUserMessage(msg) {
        // 从events中提取用户输入的文本
        let userContent = '';
        if (msg.events && msg.events.length > 0) {
            msg.events.forEach(evt => {
                if (evt.type === 'text' && evt.content) {
                    userContent += evt.content;
                }
            });
        }

        if (!userContent) return;

        // 创建用户消息元素
        const messageEl = document.createElement('div');
        messageEl.className = 'message user';
        const messageId = `msg-${msg.start_time || Date.now()}`;
        messageEl.setAttribute('data-message-id', messageId);

        // 格式化时间
        const timeDisplay = msg.start_time ? this.formatHistoryTime({ startTime: msg.start_time }) : this.getCurrentTime();

        messageEl.innerHTML = `
            <div class="message-content">
                <div class="message-header">
                    <span class="message-name">您</span>
                    <span class="message-time">${timeDisplay}</span>
                </div>
                <div class="message-body">${this.escapeHtml(userContent)}</div>
            </div>
        `;
        this.messagesContainer.appendChild(messageEl);

        // 添加到快速导航
        const question = {
            id: messageId,
            content: userContent,
            time: timeDisplay,
            element: messageEl
        };
        this.userQuestions.push(question);
        this.updateQuickNav();
    }

    renderHistoryToolCalls(toolCalls) {
        toolCalls.forEach(tc => {
            this.renderSingleToolCall(tc);
        });
    }

    renderSingleToolCall(tc) {
        // 渲染工具调用
        const toolCallEl = document.createElement('div');
        toolCallEl.className = 'tool-call';

        let argsDisplay = tc.arguments || '无参数';
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
                <span>工具调用:</span>
                <code class="tool-call-name">${this.escapeHtml(tc.name)}</code>
            </div>
            <pre class="tool-call-args">${this.escapeHtml(argsDisplay)}</pre>
        `;
        this.messagesContainer.appendChild(toolCallEl);

        // 渲染工具结果（如果有）
        if (tc.result) {
            let formattedResult = tc.result;
            try {
                const parsed = JSON.parse(tc.result);
                formattedResult = JSON.stringify(parsed, null, 2);
                if (formattedResult.length > 2048) {
                    formattedResult = formattedResult.substring(0, 2048) + '\n...';
                }
            } catch {
                if (tc.result.length > 2048) {
                    formattedResult = tc.result.substring(0, 2048) + '\n...';
                }
            }

            const toolResultEl = document.createElement('div');
            toolResultEl.className = 'tool-result';
            toolResultEl.innerHTML = `
                <div class="tool-result-header">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <polyline points="20 6 9 17 4 12"/>
                    </svg>
                    <span>执行结果</span>
                </div>
                <pre class="tool-result-content">${this.escapeHtml(formattedResult)}</pre>
            `;
            this.messagesContainer.appendChild(toolResultEl);
        }
    }

    renderHistoryActions(actions, agentName) {
        actions.forEach(action => {
            this.renderSingleAction(action, agentName);
        });
    }

    renderSingleAction(action, agentName) {
        let actionClass = '';
        let actionIcon = '';

        switch (action.action_type) {
            case 'transfer':
                actionClass = 'transfer';
                actionIcon = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M17 1l4 4-4 4"/><path d="M3 11V9a4 4 0 0 1 4-4h14"/>
                    <path d="M7 23l-4-4 4-4"/><path d="M21 13v2a4 4 0 0 1-4 4H3"/>
                </svg>`;
                break;
            case 'exit':
                actionClass = 'exit';
                actionIcon = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <polyline points="20 6 9 17 4 12"/>
                </svg>`;
                break;
            case 'interrupted':
                actionClass = 'interrupted';
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

        const actionEl = document.createElement('div');
        actionEl.className = `action-event ${actionClass}`;
        actionEl.innerHTML = `${actionIcon}<span>[${this.escapeHtml(agentName)}] ${this.escapeHtml(action.content || action.action_type)}</span>`;
        this.messagesContainer.appendChild(actionEl);
    }

    deleteHistoryFile(filename) {
        this.currentDeleteFilename = filename;
        this.deleteFilenameSpan.textContent = filename;
        this.showDeleteModal();
    }

    showDeleteModal() {
        this.deleteModal.style.display = 'flex';
        setTimeout(() => {
            this.deleteConfirmBtn.focus();
        }, 100);
    }

    hideDeleteModal() {
        this.deleteModal.style.display = 'none';
        this.currentDeleteFilename = null;
    }

    async confirmDelete() {
        const filename = this.currentDeleteFilename;
        this.hideDeleteModal();

        if (!filename) return;

        try {
            const response = await fetch(`/api/fkteams/history/files/${encodeURIComponent(filename)}`, {
                method: 'DELETE'
            });

            if (!response.ok) {
                throw new Error('删除失败');
            }

            const result = await response.json();
            if (result.code !== 0) {
                throw new Error(result.message || '删除失败');
            }

            this.showNotification('删除成功', 'success');
            await this.loadHistoryFiles(); // 重新加载列表
        } catch (error) {
            console.error('Error deleting file:', error);
            this.showNotification(error.message || '删除失败', 'error');
        }
    }

    async renameHistoryFile(filename) {
        this.currentRenameFilename = filename;
        this.renameInput.value = filename;
        this.showRenameModal();
    }

    showRenameModal() {
        this.renameModal.style.display = 'flex';
        setTimeout(() => {
            this.renameInput.focus();
            this.renameInput.select();
        }, 100);
    }

    hideRenameModal() {
        this.renameModal.style.display = 'none';
        this.currentRenameFilename = null;
    }

    async confirmRename() {
        const newName = this.renameInput.value.trim();
        const oldFilename = this.currentRenameFilename;

        if (!newName || newName === oldFilename) {
            this.hideRenameModal();
            return;
        }

        try {
            const response = await fetch('/api/fkteams/history/files/rename', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    old_filename: oldFilename,
                    new_filename: newName
                })
            });

            if (!response.ok) {
                const result = await response.json();
                throw new Error(result.message || '重命名失败');
            }

            const result = await response.json();
            if (result.code !== 0) {
                throw new Error(result.message || '重命名失败');
            }

            this.showNotification('重命名成功', 'success');
            this.hideRenameModal();
            await this.loadHistoryFiles(); // 重新加载列表
        } catch (error) {
            console.error('Error renaming file:', error);
            this.showNotification(error.message || '重命名失败', 'error');
        }
    }

    formatTime(timeString) {
        const date = new Date(timeString);
        const now = new Date();
        const diff = now - date;
        const days = Math.floor(diff / (1000 * 60 * 60 * 24));

        if (days === 0) {
            return '今天 ' + date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
        } else if (days === 1) {
            return '昨天 ' + date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
        } else if (days < 7) {
            return days + ' 天前';
        } else {
            return date.toLocaleDateString('zh-CN');
        }
    }

    formatSize(bytes) {
        if (bytes < 1024) return bytes + ' B';
        if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
        return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
    }

    formatHistoryTime(timeInfo) {
        if (!timeInfo || !timeInfo.startTime) {
            return this.getCurrentTime();
        }

        const startDate = new Date(timeInfo.startTime);
        const endDate = timeInfo.endTime ? new Date(timeInfo.endTime) : null;

        // 格式化开始时间
        const timeStr = startDate.toLocaleTimeString('zh-CN', {
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit'
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
    }

    // ===== 快速导航功能 =====

    addQuestionToNav(content, messageElement) {
        const questionId = messageElement.getAttribute('data-message-id');
        const question = {
            id: questionId,
            content: content,
            time: this.getCurrentTime(),
            element: messageElement
        };

        this.userQuestions.push(question);
        this.updateQuickNav();
    }

    updateQuickNav() {
        if (!this.quickNavBars || !this.quickNavPanelList) return;

        if (this.userQuestions.length === 0) {
            if (this.quickNavBars.parentElement) {
                this.quickNavBars.parentElement.style.display = 'none';
            }
            return;
        }

        if (this.quickNavBars.parentElement) {
            this.quickNavBars.parentElement.style.display = 'flex';
        }

        // 生成右侧的短横线（倒序显示，最新的在上）
        this.quickNavBars.innerHTML = '';
        const reversedQuestions = [...this.userQuestions].reverse();

        reversedQuestions.forEach((question, index) => {
            const bar = document.createElement('div');
            bar.className = 'quick-nav-bar';
            bar.setAttribute('data-question-id', question.id);

            // 最新的一个高亮显示
            if (index === 0) {
                bar.classList.add('active');
            }

            bar.addEventListener('click', () => {
                this.scrollToQuestion(question.id);
            });

            this.quickNavBars.appendChild(bar);
        });

        // 生成面板中的问题列表
        this.quickNavPanelList.innerHTML = '';

        reversedQuestions.forEach((question, index) => {
            // 实际序号（从1开始）
            const actualIndex = this.userQuestions.length - index;

            const item = document.createElement('div');
            item.className = 'quick-nav-item';
            item.setAttribute('data-question-id', question.id);

            item.innerHTML = `
                <div class="quick-nav-item-index">${actualIndex}</div>
                <div class="quick-nav-item-content">
                    <div class="quick-nav-item-text">${this.escapeHtml(question.content)}</div>
                    <div class="quick-nav-item-time">${question.time}</div>
                </div>
            `;

            item.addEventListener('click', () => {
                this.scrollToQuestion(question.id);
            });

            this.quickNavPanelList.appendChild(item);
        });
    }

    scrollToQuestion(questionId) {
        const messageElement = this.messagesContainer.querySelector(`[data-message-id="${questionId}"]`);
        if (messageElement) {
            // 滚动到该消息位置
            messageElement.scrollIntoView({ behavior: 'smooth', block: 'center' });

            // 添加高亮效果
            messageElement.classList.add('message-highlight');
            setTimeout(() => {
                messageElement.classList.remove('message-highlight');
            }, 2000);
        }

        // 更新导航面板中的高亮状态
        this.updateQuickNavHighlight(questionId);
    }

    updateQuickNavHighlight(questionId) {
        // 更新短横线的激活状态
        if (this.quickNavBars) {
            const allBars = this.quickNavBars.querySelectorAll('.quick-nav-bar');
            allBars.forEach(bar => {
                if (bar.getAttribute('data-question-id') === questionId) {
                    bar.classList.add('active');
                } else {
                    bar.classList.remove('active');
                }
            });
        }

        // 更新面板列表项的高亮状态
        if (this.quickNavPanelList) {
            const allItems = this.quickNavPanelList.querySelectorAll('.quick-nav-item');
            allItems.forEach(item => {
                if (item.getAttribute('data-question-id') === questionId) {
                    item.classList.add('active');
                    // 滚动到可视区域
                    item.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
                } else {
                    item.classList.remove('active');
                }
            });
        }
    }

    clearQuickNav() {
        this.userQuestions = [];
        this.updateQuickNav();
    }

    // 加载智能体列表
    async loadAgents() {
        try {
            const response = await fetch('/api/fkteams/agents');
            const result = await response.json();
            if (result.code === 0 && result.data) {
                this.agents = result.data;
            }
        } catch (error) {
            console.error('加载智能体列表失败:', error);
        }
    }

    // 处理输入框输入，检测@提及和#文件
    handleInputForMention(e) {
        const textarea = this.messageInput;
        const value = textarea.value;
        const cursorPos = textarea.selectionStart;

        // 获取光标前的文本
        const textBefore = value.substring(0, cursorPos);

        // 匹配最后一个@符号及其后的内容
        const atMatch = textBefore.match(/@([\u4e00-\u9fa5\w]*)$/);

        // 匹配最后一个#符号及其后的内容
        const hashMatch = textBefore.match(/#([^\s]*)$/);

        if (atMatch) {
            const searchText = atMatch[1];
            this.showAgentSuggestions(searchText, cursorPos);
            this.hideFileSuggestions();
        } else if (hashMatch) {
            const searchText = hashMatch[1];
            this.showFileSuggestions(searchText, cursorPos);
            this.hideAgentSuggestions();
        } else {
            this.hideAgentSuggestions();
            this.hideFileSuggestions();
        }
    }

    // 显示智能体建议列表
    showAgentSuggestions(searchText, cursorPos) {
        // 过滤智能体列表
        const filteredAgents = this.agents.filter(agent => {
            const name = agent.name.toLowerCase();
            const search = searchText.toLowerCase();
            return name.includes(search);
        });

        if (filteredAgents.length === 0) {
            this.hideAgentSuggestions();
            return;
        }

        // 创建或更新建议弹窗
        if (!this.agentSuggestions) {
            this.agentSuggestions = document.createElement('div');
            this.agentSuggestions.className = 'agent-suggestions';
            document.body.appendChild(this.agentSuggestions);
        }

        // 生成建议列表HTML
        this.agentSuggestions.innerHTML = filteredAgents.map((agent, index) => `
            <div class="agent-suggestion-item ${index === 0 ? 'selected' : ''}" data-index="${index}" data-name="${this.escapeHtml(agent.name)}">
                <div class="agent-suggestion-name">@${this.escapeHtml(agent.name)}</div>
                <div class="agent-suggestion-desc">${this.escapeHtml(agent.description)}</div>
            </div>
        `).join('');

        this.agentSuggestions.style.display = 'block';
        this.selectedAgentIndex = 0;

        // 计算弹窗位置（显示在输入框上方，使用input-wrapper的尺寸）
        const inputWrapper = this.messageInput.closest('.input-wrapper');
        const wrapperRect = inputWrapper ? inputWrapper.getBoundingClientRect() : this.messageInput.getBoundingClientRect();

        // 设置宽度与input-wrapper一致
        this.agentSuggestions.style.width = wrapperRect.width + 'px';
        this.agentSuggestions.style.left = wrapperRect.left + 'px';

        // 获取建议框的实际高度来更精确定位
        const suggestionsHeight = this.agentSuggestions.offsetHeight;
        // 定位在输入框正上方，留10px间隙
        this.agentSuggestions.style.bottom = (window.innerHeight - wrapperRect.top + 10) + 'px';
        // 移除可能冲突的top属性
        this.agentSuggestions.style.top = 'auto';

        // 绑定点击事件
        this.agentSuggestions.querySelectorAll('.agent-suggestion-item').forEach(item => {
            item.addEventListener('click', () => {
                const agentName = item.getAttribute('data-name');
                this.insertAgentMention(agentName);
            });
        });
    }

    // 隐藏智能体建议列表
    hideAgentSuggestions() {
        if (this.agentSuggestions) {
            this.agentSuggestions.style.display = 'none';
        }
        this.selectedAgentIndex = -1;
    }

    // 插入智能体提及
    insertAgentMention(agentName) {
        const textarea = this.messageInput;
        const value = textarea.value;
        const cursorPos = textarea.selectionStart;

        // 获取光标前的文本，找到最后一个@符号的位置
        const textBefore = value.substring(0, cursorPos);
        const atMatch = textBefore.match(/@([\u4e00-\u9fa5\w]*)$/);

        if (atMatch) {
            const atPos = cursorPos - atMatch[0].length;
            const textAfter = value.substring(cursorPos);

            // 替换@及其后的部分文本为完整的智能体名称
            textarea.value = value.substring(0, atPos) + '@' + agentName + ' ' + textAfter;

            // 设置光标位置到插入文本之后
            const newCursorPos = atPos + agentName.length + 2;
            textarea.setSelectionRange(newCursorPos, newCursorPos);

            // 触发input事件更新高度
            this.handleInputChange();
        }

        this.hideAgentSuggestions();
        textarea.focus();
    }

    // 处理建议列表的键盘导航
    handleSuggestionKeyDown(e) {
        if (!this.agentSuggestions || this.agentSuggestions.style.display === 'none') {
            return false;
        }

        const items = this.agentSuggestions.querySelectorAll('.agent-suggestion-item');
        if (items.length === 0) return false;

        switch (e.key) {
            case 'ArrowDown':
                e.preventDefault();
                this.selectedAgentIndex = (this.selectedAgentIndex + 1) % items.length;
                this.updateSuggestionSelection(items);
                return true;

            case 'ArrowUp':
                e.preventDefault();
                this.selectedAgentIndex = (this.selectedAgentIndex - 1 + items.length) % items.length;
                this.updateSuggestionSelection(items);
                return true;

            case 'Enter':
            case 'Tab':
                if (this.selectedAgentIndex >= 0 && this.selectedAgentIndex < items.length) {
                    e.preventDefault();
                    const selectedItem = items[this.selectedAgentIndex];
                    const agentName = selectedItem.getAttribute('data-name');
                    this.insertAgentMention(agentName);
                    return true;
                }
                break;

            case 'Escape':
                e.preventDefault();
                this.hideAgentSuggestions();
                return true;
        }

        return false;
    }

    // 更新建议列表选中状态
    updateSuggestionSelection(items) {
        items.forEach((item, index) => {
            item.classList.toggle('selected', index === this.selectedAgentIndex);
        });

        // 滚动到可视区域
        if (this.selectedAgentIndex >= 0 && this.selectedAgentIndex < items.length) {
            items[this.selectedAgentIndex].scrollIntoView({ block: 'nearest' });
        }
    }

    // 提取智能体提及
    extractAgentMention(input) {
        const trimmed = input.trim();
        const match = trimmed.match(/^@([\u4e00-\u9fa5\w]+)\s*(.*)$/);
        if (match) {
            return {
                agentName: match[1],
                query: match[2].trim()
            };
        }
        return null;
    }

    // 加载文件列表
    async loadFiles(path = '') {
        try {
            const url = path ? `/api/fkteams/files?path=${encodeURIComponent(path)}` : '/api/fkteams/files';
            const response = await fetch(url);
            const result = await response.json();
            if (result.code === 0 && result.data) {
                this.files = result.data;
                this.currentPath = path;
                return this.files;
            }
        } catch (error) {
            console.error('加载文件列表失败:', error);
        }
        return [];
    }

    // 显示文件建议列表
    async showFileSuggestions(searchText, cursorPos) {
        // 解析路径和搜索文本
        const parts = searchText.split('/');
        const searchFileName = parts[parts.length - 1];
        const searchPath = parts.slice(0, -1).join('/');

        // 加载文件列表
        const files = await this.loadFiles(searchPath);

        // 过滤文件列表
        const filteredFiles = files.filter(file => {
            const name = file.name.toLowerCase();
            const search = searchFileName.toLowerCase();
            return name.includes(search);
        });

        if (filteredFiles.length === 0 && !searchPath) {
            this.hideFileSuggestions();
            return;
        }

        // 创建或更新建议弹窗
        if (!this.fileSuggestions) {
            this.fileSuggestions = document.createElement('div');
            this.fileSuggestions.className = 'file-suggestions';
            document.body.appendChild(this.fileSuggestions);
        }

        // 生成建议列表HTML
        let html = '';

        // 如果不在根目录，添加"返回上级"选项
        if (searchPath) {
            const parentPath = searchPath.split('/').slice(0, -1).join('/');
            html += `
                <div class="file-suggestion-item file-suggestion-parent" 
                     data-index="-1" 
                     data-path="${parentPath}"
                     data-is-dir="true"
                     data-is-parent="true">
                    <div class="file-suggestion-icon">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <path d="M19 12H5M12 19l-7-7 7-7"/>
                        </svg>
                    </div>
                    <div class="file-suggestion-info">
                        <div class="file-suggestion-name">返回上级</div>
                        <div class="file-suggestion-path">#${this.escapeHtml(parentPath || '.')}</div>
                    </div>
                </div>
            `;
        }

        html += filteredFiles.map((file, index) => `
            <div class="file-suggestion-item ${index === 0 && !searchPath ? 'selected' : ''}" 
                 data-index="${index}" 
                 data-path="${this.escapeHtml(file.path)}"
                 data-is-dir="${file.is_dir}">
                <div class="file-suggestion-icon">
                    ${file.is_dir ? `
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/>
                        </svg>
                    ` : `
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
                            <polyline points="14 2 14 8 20 8"/>
                        </svg>
                    `}
                </div>
                <div class="file-suggestion-info">
                    <div class="file-suggestion-name">${this.escapeHtml(file.name)}</div>
                    <div class="file-suggestion-path">#${this.escapeHtml(file.path)}</div>
                    ${file.is_dir ? '' : `<div class="file-suggestion-meta">${this.formatFileSize(file.size)} · ${this.formatFileTime(file.mod_time)}</div>`}
                </div>
            </div>
        `).join('');

        this.fileSuggestions.innerHTML = html;

        this.fileSuggestions.style.display = 'block';
        this.selectedFileIndex = searchPath ? -1 : 0;

        // 计算弹窗位置
        const inputWrapper = this.messageInput.closest('.input-wrapper');
        const wrapperRect = inputWrapper ? inputWrapper.getBoundingClientRect() : this.messageInput.getBoundingClientRect();

        this.fileSuggestions.style.width = wrapperRect.width + 'px';
        this.fileSuggestions.style.left = wrapperRect.left + 'px';
        this.fileSuggestions.style.bottom = (window.innerHeight - wrapperRect.top + 10) + 'px';
        this.fileSuggestions.style.top = 'auto';

        // 绑定点击事件
        this.fileSuggestions.querySelectorAll('.file-suggestion-item').forEach(item => {
            // 单击：选择文件或文件夹
            item.addEventListener('click', () => {
                const filePath = item.getAttribute('data-path');
                const isParent = item.getAttribute('data-is-parent') === 'true';

                if (isParent) {
                    // 返回上级目录
                    const newPath = filePath ? filePath + '/' : '';
                    this.showFileSuggestions(newPath, cursorPos);
                } else {
                    // 选择文件或文件夹
                    this.insertFileMention(filePath);
                }
            });

            // 双击：进入文件夹
            item.addEventListener('dblclick', async (e) => {
                e.stopPropagation();
                const filePath = item.getAttribute('data-path');
                const isDir = item.getAttribute('data-is-dir') === 'true';
                const isParent = item.getAttribute('data-is-parent') === 'true';

                if (isDir && !isParent) {
                    // 进入子目录
                    await this.showFileSuggestions(filePath + '/', cursorPos);
                }
            });
        });
    }

    // 隐藏文件建议列表
    hideFileSuggestions() {
        if (this.fileSuggestions) {
            this.fileSuggestions.style.display = 'none';
        }
        this.selectedFileIndex = -1;
    }

    // 插入文件提及
    insertFileMention(filePath) {
        const textarea = this.messageInput;
        const value = textarea.value;
        const cursorPos = textarea.selectionStart;

        // 获取光标前的文本，找到最后一个#符号的位置
        const textBefore = value.substring(0, cursorPos);
        const hashMatch = textBefore.match(/#([^\s]*)$/);

        if (hashMatch) {
            const hashPos = cursorPos - hashMatch[0].length;
            const textAfter = value.substring(cursorPos);

            // 替换#及其后的部分文本为完整的文件路径
            textarea.value = value.substring(0, hashPos) + '#' + filePath + ' ' + textAfter;

            // 设置光标位置到插入文本之后
            const newCursorPos = hashPos + filePath.length + 2;
            textarea.setSelectionRange(newCursorPos, newCursorPos);

            // 触发input事件更新高度
            this.handleInputChange();
        }

        this.hideFileSuggestions();
        textarea.focus();
    }

    // 处理文件建议列表的键盘导航
    handleFileSuggestionKeyDown(e) {
        if (!this.fileSuggestions || this.fileSuggestions.style.display === 'none') {
            return false;
        }

        const items = this.fileSuggestions.querySelectorAll('.file-suggestion-item');
        if (items.length === 0) return false;

        const hasParent = items[0] && items[0].getAttribute('data-is-parent') === 'true';
        const maxIndex = hasParent ? items.length - 2 : items.length - 1;

        switch (e.key) {
            case 'ArrowDown':
                e.preventDefault();
                if (hasParent) {
                    this.selectedFileIndex = this.selectedFileIndex >= maxIndex ? -1 : this.selectedFileIndex + 1;
                } else {
                    this.selectedFileIndex = (this.selectedFileIndex + 1) % items.length;
                }
                this.updateFileSuggestionSelection(items);
                return true;

            case 'ArrowUp':
                e.preventDefault();
                if (hasParent) {
                    this.selectedFileIndex = this.selectedFileIndex <= -1 ? maxIndex : this.selectedFileIndex - 1;
                } else {
                    this.selectedFileIndex = (this.selectedFileIndex - 1 + items.length) % items.length;
                }
                this.updateFileSuggestionSelection(items);
                return true;

            case 'Enter':
                // Enter键：选择文件或文件夹
                if (this.selectedFileIndex >= -1 && this.selectedFileIndex <= maxIndex) {
                    e.preventDefault();
                    let selectedItem;
                    if (hasParent) {
                        selectedItem = this.selectedFileIndex === -1 ? items[0] : items[this.selectedFileIndex + 1];
                    } else {
                        selectedItem = items[this.selectedFileIndex];
                    }

                    if (!selectedItem) return false;

                    const filePath = selectedItem.getAttribute('data-path');
                    const isParent = selectedItem.getAttribute('data-is-parent') === 'true';

                    if (isParent) {
                        // 返回上级目录
                        const newPath = filePath ? filePath + '/' : '';
                        this.showFileSuggestions(newPath, this.messageInput.selectionStart);
                    } else {
                        // 选择文件或文件夹
                        this.insertFileMention(filePath);
                    }
                    return true;
                }
                break;

            case 'Tab':
                // Tab键：进入文件夹或选择文件
                if (this.selectedFileIndex >= -1 && this.selectedFileIndex <= maxIndex) {
                    e.preventDefault();
                    let selectedItem;
                    if (hasParent) {
                        selectedItem = this.selectedFileIndex === -1 ? items[0] : items[this.selectedFileIndex + 1];
                    } else {
                        selectedItem = items[this.selectedFileIndex];
                    }

                    if (!selectedItem) return false;

                    const filePath = selectedItem.getAttribute('data-path');
                    const isDir = selectedItem.getAttribute('data-is-dir') === 'true';
                    const isParent = selectedItem.getAttribute('data-is-parent') === 'true';

                    if (isDir) {
                        if (isParent) {
                            // 返回上级目录
                            const newPath = filePath ? filePath + '/' : '';
                            this.showFileSuggestions(newPath, this.messageInput.selectionStart);
                        } else {
                            // 进入子目录
                            this.showFileSuggestions(filePath + '/', this.messageInput.selectionStart);
                        }
                    } else {
                        // 文件则选择
                    }
                    return true;
                }
                break;

            case 'Escape':
                e.preventDefault();
                this.hideFileSuggestions();
                return true;
        }

        return false;
    }

    // 更新文件建议列表选中状态
    updateFileSuggestionSelection(items) {
        const hasParent = items[0] && items[0].getAttribute('data-is-parent') === 'true';

        items.forEach((item, index) => {
            const itemIndex = hasParent ? index - 1 : index;
            item.classList.toggle('selected', itemIndex === this.selectedFileIndex);
        });

        // 滚动到可视区域
        const actualIndex = hasParent ? this.selectedFileIndex + 1 : this.selectedFileIndex;
        if (actualIndex >= 0 && actualIndex < items.length) {
            items[actualIndex].scrollIntoView({ block: 'nearest' });
        }
    }

    // 提取文件路径
    extractFilePaths(input) {
        const paths = [];
        const regex = /#([^\s]+)/g;
        let match;
        while ((match = regex.exec(input)) !== null) {
            paths.push(match[1]);
        }
        return paths;
    }

    // 格式化文件大小
    formatFileSize(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
    }

    // 格式化文件修改时间
    formatFileTime(timestamp) {
        const date = new Date(timestamp * 1000);
        const now = new Date();
        const diff = now - date;
        const days = Math.floor(diff / (1000 * 60 * 60 * 24));

        if (days === 0) {
            const hours = Math.floor(diff / (1000 * 60 * 60));
            if (hours === 0) {
                const minutes = Math.floor(diff / (1000 * 60));
                return minutes === 0 ? '刚刚' : `${minutes}分钟前`;
            }
            return `${hours}小时前`;
        } else if (days === 1) {
            return '昨天';
        } else if (days < 7) {
            return `${days}天前`;
        } else {
            const year = date.getFullYear();
            const month = String(date.getMonth() + 1).padStart(2, '0');
            const day = String(date.getDate()).padStart(2, '0');
            return now.getFullYear() === year ? `${month}-${day}` : `${year}-${month}-${day}`;
        }
    }
}

document.addEventListener('DOMContentLoaded', () => {
    window.app = new FKTeamsChat();
    window.fkteamsChat = window.app; // 保持向后兼容
});
