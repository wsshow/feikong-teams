/**
 * messages.js - 消息处理与渲染
 */

FKTeamsChat.prototype.sendMessage = function () {
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
};

// 显示智能体切换通知
FKTeamsChat.prototype.showAgentSwitchNotification = function (agentName, description) {
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
};

// 重置回团队模式
FKTeamsChat.prototype.resetToTeamMode = function () {
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
};

FKTeamsChat.prototype.handleServerEvent = function (event) {
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
            // 刷新侧边栏历史列表
            this.loadSidebarHistory();
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
};

FKTeamsChat.prototype.trimLeadingWhitespace = function (text) {
    if (!text) return '';
    return text.replace(/^[\s\n\r\u00A0\u2000-\u200B\uFEFF]+/, '');
};

// 渲染 Markdown
FKTeamsChat.prototype.renderMarkdown = function (text) {
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
};

FKTeamsChat.prototype.handleStreamChunk = function (event) {
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
};

FKTeamsChat.prototype.handleMessage = function (event) {
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
};

FKTeamsChat.prototype.handleToolCallsPreparing = function (event) {
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
};

FKTeamsChat.prototype.handleToolCalls = function (event) {
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
};

FKTeamsChat.prototype.handleToolResult = function (event) {
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
};

FKTeamsChat.prototype.handleAction = function (event) {
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
};

FKTeamsChat.prototype.handleError = function (event) {
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
};

FKTeamsChat.prototype.addUserMessage = function (content) {
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
};

FKTeamsChat.prototype.createAssistantMessage = function (agentName, timeInfo = null) {
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
};

FKTeamsChat.prototype.cancelTask = function () {
    if (!this.isProcessing) return;

    // 发送取消消息
    this.ws.send(JSON.stringify({
        type: 'cancel'
    }));

    this.showNotification('正在取消任务...', 'info');
};

FKTeamsChat.prototype.handleCancelled = function (event) {
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
};

FKTeamsChat.prototype.clearChat = function () {
    // 发送清除历史的消息到后端
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        this.ws.send(JSON.stringify({
            type: 'clear_history',
            session_id: this.sessionId
        }));
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
};

FKTeamsChat.prototype.exportToHTML = function () {
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
};
