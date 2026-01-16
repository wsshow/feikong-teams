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

        this.init();
    }

    init() {
        this.bindElements();
        this.bindEvents();
        this.restoreSidebarState();
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
        this.modeButtons = document.querySelectorAll('.mode-btn');
        this.sidebar = document.getElementById('sidebar');
        this.sidebarToggle = document.getElementById('sidebar-toggle');
        this.mainContent = document.getElementById('main-content');
        this.scrollToBottomBtn = document.getElementById('scroll-to-bottom');
    }

    bindEvents() {
        this.sendBtn.addEventListener('click', () => this.sendMessage());
        this.cancelBtn.addEventListener('click', () => this.cancelTask());
        this.messageInput.addEventListener('input', () => this.handleInputChange());
        this.messageInput.addEventListener('keydown', (e) => this.handleKeyDown(e));
        this.sessionIdInput.addEventListener('change', () => {
            this.sessionId = this.sessionIdInput.value || 'default';
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
            this.scrollToBottomBtn.style.display = show ? 'flex' : 'none';
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
        }
    }

    connect() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws`;

        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = () => {
            this.updateStatus('connected', '已连接');
            this.reconnectAttempts = 0;
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

        this.addUserMessage(message);

        this.ws.send(JSON.stringify({
            type: 'chat',
            session_id: this.sessionId,
            message: message,
            mode: this.mode
        }));

        this.messageInput.value = '';
        this.handleInputChange();
        this.isProcessing = true;
        this.updateSendButtonState();
        this.updateStatus('processing', '处理中...');
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
        // 创建通知元素
        const notification = document.createElement('div');
        notification.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            background: ${type === 'success' ? '#66bb6a' : '#42a5f5'};
            color: white;
            padding: 12px 20px;
            border-radius: 6px;
            font-size: 14px;
            z-index: 1000;
            animation: slideIn 0.3s ease;
        `;
        notification.textContent = message;

        // 添加滑入动画
        const style = document.createElement('style');
        style.textContent = `
            @keyframes slideIn {
                from { transform: translateX(100%); opacity: 0; }
                to { transform: translateX(0); opacity: 1; }
            }
        `;
        document.head.appendChild(style);

        document.body.appendChild(notification);

        // 3秒后自动移除
        setTimeout(() => {
            notification.style.animation = 'slideIn 0.3s ease reverse';
            setTimeout(() => {
                if (notification.parentNode) {
                    document.body.removeChild(notification);
                }
                if (style.parentNode) {
                    document.head.removeChild(style);
                }
            }, 300);
        }, 3000);
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
            this.ws.send(JSON.stringify({
                type: 'load_history',
                message: filename
            }));
            this.hideHistoryModal();
            this.showNotification('正在加载历史记录...', 'info');
        } else {
            this.showNotification('WebSocket 未连接', 'error');
        }
    }

    handleHistoryLoaded(event) {
        // 清空当前消息
        this.messagesContainer.innerHTML = '';
        this.currentMessageElement = null;
        this.hasToolCallAfterMessage = false;

        // 更新 session ID
        if (event.session_id) {
            this.sessionId = event.session_id;
            this.sessionIdInput.value = event.session_id;
        }

        // 渲染历史消息
        if (event.messages && event.messages.length > 0) {
            event.messages.forEach(msg => {
                const timeInfo = {
                    startTime: msg.start_time,
                    endTime: msg.end_time
                };
                const messageEl = this.createAssistantMessage(msg.agent_name, timeInfo);
                const bodyEl = messageEl.querySelector('.message-body');
                if (bodyEl) {
                    bodyEl.setAttribute('data-raw', msg.content);
                    bodyEl.innerHTML = this.renderMarkdown(msg.content);
                }
            });
            this.showNotification(`已加载 ${event.messages.length} 条历史消息`, 'success');
        } else {
            this.showNotification('历史记录为空', 'info');
        }

        this.scrollToBottom();
    }

    async deleteHistoryFile(filename) {
        if (!confirm(`确定要删除 "${filename}" 吗？此操作不可恢复！`)) {
            return;
        }

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
}

document.addEventListener('DOMContentLoaded', () => {
    window.fkteamsChat = new FKTeamsChat();
});
