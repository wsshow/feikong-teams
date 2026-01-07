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
        this.sessionIdInput = document.getElementById('session-id');
        this.statusIndicator = document.getElementById('status-indicator');
        this.clearBtn = document.getElementById('clear-chat');
        this.modeButtons = document.querySelectorAll('.mode-btn');
        this.sidebar = document.getElementById('sidebar');
        this.sidebarToggle = document.getElementById('sidebar-toggle');
    }

    bindEvents() {
        this.sendBtn.addEventListener('click', () => this.sendMessage());
        this.messageInput.addEventListener('input', () => this.handleInputChange());
        this.messageInput.addEventListener('keydown', (e) => this.handleKeyDown(e));
        this.sessionIdInput.addEventListener('change', () => {
            this.sessionId = this.sessionIdInput.value || 'default';
        });
        this.clearBtn.addEventListener('click', () => this.clearChat());
        this.modeButtons.forEach(btn => {
            btn.addEventListener('click', () => this.setMode(btn.dataset.mode));
        });
        if (this.sidebarToggle) {
            this.sidebarToggle.addEventListener('click', () => this.toggleSidebar());
        }
    }

    toggleSidebar() {
        const isCollapsed = this.sidebar.classList.toggle('collapsed');
        this.sidebarToggle.classList.toggle('collapsed', isCollapsed);
        localStorage.setItem('sidebarCollapsed', isCollapsed);
    }

    restoreSidebarState() {
        const isCollapsed = localStorage.getItem('sidebarCollapsed') === 'true';
        if (isCollapsed) {
            this.sidebar.classList.add('collapsed');
            this.sidebarToggle.classList.add('collapsed');
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
    }

    sendMessage() {
        const message = this.messageInput.value.trim();
        if (!message || this.isProcessing) return;

        const welcomeMsg = this.messagesContainer.querySelector('.welcome-message');
        if (welcomeMsg) welcomeMsg.remove();

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
                this.sendBtn.disabled = this.messageInput.value.trim().length === 0;
                this.currentMessageElement = null;
                this.hasToolCallAfterMessage = false;
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

    handleStreamChunk(event) {
        if (this.hasToolCallAfterMessage || !this.currentMessageElement) {
            this.currentMessageElement = this.createAssistantMessage(event.agent_name);
            this.hasToolCallAfterMessage = false;
        }

        const bodyEl = this.currentMessageElement.querySelector('.message-body');
        if (bodyEl) {
            const indicator = bodyEl.querySelector('.streaming-indicator');
            if (indicator) indicator.remove();

            let currentText = bodyEl.textContent || '';
            let newContent = event.content || '';

            if (currentText === '') {
                newContent = this.trimLeadingWhitespace(newContent);
            }

            bodyEl.textContent = currentText + newContent;
        }
    }

    handleMessage(event) {
        if (!event.content) return;

        if (this.hasToolCallAfterMessage || !this.currentMessageElement) {
            this.currentMessageElement = this.createAssistantMessage(event.agent_name);
            this.hasToolCallAfterMessage = false;
        }

        const bodyEl = this.currentMessageElement.querySelector('.message-body');
        if (bodyEl) {
            const indicator = bodyEl.querySelector('.streaming-indicator');
            if (indicator) indicator.remove();
            bodyEl.textContent = this.trimLeadingWhitespace(event.content);
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
            if (formattedContent.length > 800) {
                formattedContent = formattedContent.substring(0, 800) + '\n...';
            }
        } catch {
            if (content.length > 800) {
                formattedContent = content.substring(0, 800) + '\n...';
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

    createAssistantMessage(agentName) {
        const messageEl = document.createElement('div');
        messageEl.className = 'message assistant';
        messageEl.innerHTML = `
            <div class="message-content">
                <div class="message-header">
                    <span class="message-name">${this.escapeHtml(agentName || 'Assistant')}</span>
                    <span class="agent-tag">${this.escapeHtml(agentName || 'AI')}</span>
                    <span class="message-time">${this.getCurrentTime()}</span>
                </div>
                <div class="message-body"><span class="streaming-indicator"><span></span><span></span><span></span></span></div>
            </div>
        `;
        this.messagesContainer.appendChild(messageEl);
        this.scrollToBottom();
        return messageEl;
    }

    clearChat() {
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
                <p>多智能体协作系统</p>
            </div>
        `;
        this.currentMessageElement = null;
        this.hasToolCallAfterMessage = false;
    }

    scrollToBottom() {
        requestAnimationFrame(() => {
            if (this.messagesWrapper) {
                this.messagesWrapper.scrollTop = this.messagesWrapper.scrollHeight;
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
}

document.addEventListener('DOMContentLoaded', () => {
    window.fkteamsChat = new FKTeamsChat();
});
