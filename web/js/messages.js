/**
 * messages.js - 消息处理与渲染
 */

FKTeamsChat.prototype.sendMessage = function () {
    const message = this.messageInput.value.trim();
    const hasAttachments = this.attachments && this.attachments.length > 0;
    if ((!message && !hasAttachments) || this.isProcessing) return;

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

    // 显示用户消息（包含附件预览）
    this.addUserMessage(message, this.attachments);

    // 构建发送 payload
    const payload = {
        type: 'chat',
        session_id: this.sessionId,
        message: message,
        mode: this.mode
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
            contents.push({ type: 'text', text: message });
        }
        for (const att of this.attachments) {
            if (att.type === 'image') {
                contents.push({
                    type: 'image_base64',
                    base64_data: att.base64,
                    mime_type: att.mimeType
                });
            }
        }
        payload.contents = contents;
    }

    this.ws.send(JSON.stringify(payload));

    this.messageInput.value = '';
    this.clearAttachments();
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
            // 流式结束后，对所有含 data-raw 的消息做一次脚注最终渲染
            this._finalizeFootnotes();
            this.currentMessageElement = null;
            this.hasToolCallAfterMessage = false;
            // 刷新侧边栏历史列表
            this.loadSidebarHistory();
            break;
        case 'cancelled':
            this.handleCancelled(event);
            break;
        case 'history_cleared':
            if (this._suppressHistoryClearedNotification) {
                this._suppressHistoryClearedNotification = false;
            } else {
                this.showNotification('历史记录已清除', 'success');
            }
            break;
        case 'history_loaded':
            this.handleHistoryLoaded(event);
            break;
        case 'stream_chunk':
            this.handleStreamChunk(event);
            break;
        case 'reasoning_chunk':
            this.handleReasoningChunk(event);
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
        case 'dispatch_progress':
            this.handleDispatchProgress(event);
            break;
        case 'approval_required':
            this.showApprovalRequest(event.message);
            this.showApprovalDialog(event.message);
            break;
        case 'error':
            this.handleError(event);
            break;
        default:
            console.log('Unknown event:', event);
    }
};

FKTeamsChat.prototype.trimLeadingWhitespace = function (text) {
    if (!text) return '';
    return text.replace(/^[\s\n\r\u00A0\u2000-\u200B\uFEFF]+/, '');
};

// 渲染 Markdown（streaming 为 true 时跳过脚注处理以提升流式性能）
FKTeamsChat.prototype.renderMarkdown = function (text, streaming) {
    if (!text) return '';
    try {
        if (typeof marked !== 'undefined') {
            if (!this._markedInstance) {
                this._markedInstance = new marked.Marked({ breaks: true, gfm: true });
                // 链接在新标签中打开
                this._markedInstance.use({
                    renderer: {
                        link: function (token) {
                            var href = token.href || '';
                            var title = token.title ? ' title="' + token.title + '"' : '';
                            var text = token.text || href;
                            if (href.startsWith('#')) {
                                return '<a href="' + href + '"' + title + '>' + text + '</a>';
                            }
                            return '<a href="' + href + '"' + title + ' target="_blank" rel="noopener noreferrer">' + text + '</a>';
                        }
                    }
                });
            }

            // 流式渲染时跳过脚注处理，仅在最终渲染时处理
            if (!streaming) {
                var footnotes = this._extractFootnotes(text);
                text = footnotes.text;
                var html = this._markedInstance.parse(text);
                // 在 marked 解析后替换占位符为真正的脚注链接
                if (footnotes.orderedNums && footnotes.orderedNums.length > 0) {
                    html = this._replaceFootnotePlaceholders(html, footnotes.definitions, footnotes.orderedNums);
                }
                if (footnotes.items.length > 0) {
                    html = this._buildSourcesCard(html, footnotes.items);
                }
                return html;
            }
            return this._markedInstance.parse(text);
        }
    } catch (e) {
        console.error('Markdown parse error:', e);
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
        var url = '', label = '';

        // 尝试匹配 markdown 链接: [text](url)
        var mdLink = content.match(/^\[([^\]]*)\]\((https?:\/\/[^)]+)\)(.*)$/);
        if (mdLink) {
            url = mdLink[2];
            label = (mdLink[1] + ' ' + mdLink[3]).trim() || url;
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
    text = text.replace(/\n*^\[\^(\d+)\]:\s*(.+)$/gm, '');

    // 将行内引用 [^N] 替换为占位符（marked 会当作普通文本保留）
    var items = [];
    orderedNums.forEach(function (num) {
        items.push(definitions[num]);
    });

    text = text.replace(/\[\^(\d+)\]/g, function (match, num) {
        var def = definitions[num];
        if (!def) return match;
        var idx = orderedNums.indexOf(num);
        return '<!--fnref:' + idx + ':' + num + '-->';
    });

    return { text: text, items: items, definitions: definitions, orderedNums: orderedNums };
};

// 将占位符替换为真正的脚注链接（在 marked.parse 之后调用）
FKTeamsChat.prototype._replaceFootnotePlaceholders = function (html, definitions, orderedNums) {
    return html.replace(/<!--fnref:(\d+):(\d+)-->/g, function (match, idx, num) {
        var def = definitions[num];
        if (!def) return match;
        var displayNum = parseInt(idx, 10) + 1;
        if (def.url) {
            return '<a class="footnote-cite" href="' + def.url + '" data-url="' + def.url + '" target="_blank" rel="noopener noreferrer">' + displayNum + '</a>';
        }
        return '<span class="footnote-cite">' + displayNum + '</span>';
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
            } catch (e) { /* ignore */ }
        }
    });

    // 构建图标堆叠（最多显示5个）
    var iconsHtml = '';
    var showCount = Math.min(favicons.length, 5);
    if (showCount > 0) {
        for (var i = 0; i < showCount; i++) {
            iconsHtml += '<img class="source-favicon" src="https://www.google.com/s2/favicons?domain=' + favicons[i] + '&sz=32" alt="" style="z-index:' + (showCount - i) + ';margin-left:' + (i === 0 ? '0' : '-6px') + ';">';
        }
    } else {
        iconsHtml = '<span class="source-icon-fallback"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg></span>';
    }

    // 构建来源列表
    var listHtml = '';
    items.forEach(function (item, idx) {
        var favicon = '';
        if (item.url && /^https?:\/\//.test(item.url)) {
            try {
                var d = new URL(item.url).hostname;
                favicon = '<img class="source-item-favicon" src="https://www.google.com/s2/favicons?domain=' + d + '&sz=16" alt="">';
            } catch (e) { /* ignore */ }
        }
        if (!favicon) {
            favicon = '<span class="source-item-icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="14" height="14"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg></span>';
        }
        var linkAttr = item.url ? ' href="' + item.url + '" target="_blank" rel="noopener noreferrer"' : '';
        var tag = item.url ? 'a' : 'span';
        listHtml += '<' + tag + ' class="source-item"' + linkAttr + '>' + favicon + '<span class="source-item-label">' + (idx + 1) + '. ' + item.label + '</span></' + tag + '>';
    });

    var cardHtml = '<div class="sources-card">' +
        '<div class="sources-header" onclick="this.parentElement.classList.toggle(\'expanded\')">' +
        '<div class="sources-icons">' + iconsHtml + '</div>' +
        '<span class="sources-count">' + items.length + ' 个来源</span>' +
        '<svg class="sources-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>' +
        '</div>' +
        '<div class="sources-list">' + listHtml + '</div>' +
        '</div>';

    return html + cardHtml;
};

// 流式结束后，对含脚注的消息体做一次最终渲染（仅处理尚未完成脚注渲染的消息）
FKTeamsChat.prototype._finalizeFootnotes = function () {
    var bodies = this.messagesContainer.querySelectorAll('.message.assistant .message-body[data-raw]:not([data-fn-done])');
    for (var i = 0; i < bodies.length; i++) {
        var body = bodies[i];
        var raw = body.getAttribute('data-raw');
        if (raw && /\[\^\d+\]/.test(raw)) {
            body.innerHTML = this.renderMarkdown(raw, false);
        }
        // 标记已完成脚注渲染，后续不再重复处理
        body.setAttribute('data-fn-done', '1');
    }
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

        // 推理结束后开始正式内容：折叠推理块并更新标题
        const reasoningBlock = bodyEl.querySelector('.reasoning-block.expanded');
        if (reasoningBlock) {
            reasoningBlock.classList.remove('expanded');
            const title = reasoningBlock.querySelector('.reasoning-title');
            if (title) title.textContent = '思考过程';
        }

        // 获取原始文本
        let rawText = bodyEl.getAttribute('data-raw') || '';
        let newContent = event.content || '';

        if (rawText === '') {
            newContent = this.trimLeadingWhitespace(newContent);
        }

        rawText += newContent;
        bodyEl.setAttribute('data-raw', rawText);

        // 流式渲染 Markdown（跳过脚注处理，但剥离脚注定义行避免原文可见）
        // 1. 剥离完整定义行：[^N]: 内容
        var streamText = rawText.replace(/\n*^\[\^(\d+)\]:\s*(.+)$/gm, '');
        // 2. 剥离尾部不完整的定义行（如 [^、[^1、[^1]、[^1]: 等正在输入中的部分）
        streamText = streamText.replace(/\n\[\^[^\]]*\]?:?\s*$/, '');
        // 3. 规范化尾部空白，避免换行符差异导致无意义的 DOM 更新
        streamText = streamText.replace(/\s+$/, '');
        // 仅当可见内容变化时更新 DOM（避免脚注定义行到达时的无意义重绘）
        var lastStreamText = bodyEl.getAttribute('data-stream-text') || '';
        if (streamText !== lastStreamText) {
            bodyEl.setAttribute('data-stream-text', streamText);
            const existingReasoning = bodyEl.querySelector('.reasoning-block');
            if (existingReasoning) {
                let textContainer = bodyEl.querySelector('.message-text-content');
                if (!textContainer) {
                    textContainer = document.createElement('div');
                    textContainer.className = 'message-text-content';
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
    // 检查是否需要创建新卡片
    const currentAgentName = this.currentMessageElement?.getAttribute('data-agent');
    const needNewCard = this.hasToolCallAfterMessage ||
        !this.currentMessageElement ||
        (event.agent_name && currentAgentName !== event.agent_name);

    if (needNewCard) {
        this.currentMessageElement = this.createAssistantMessage(event.agent_name);
        this.hasToolCallAfterMessage = false;
    }

    const bodyEl = this.currentMessageElement.querySelector('.message-body');
    if (!bodyEl) return;

    const indicator = bodyEl.querySelector('.streaming-indicator');
    if (indicator) indicator.remove();

    // 查找或创建推理内容块
    let reasoningBlock = bodyEl.querySelector('.reasoning-block');
    if (!reasoningBlock) {
        reasoningBlock = document.createElement('div');
        reasoningBlock.className = 'reasoning-block expanded';
        reasoningBlock.innerHTML = `
            <div class="reasoning-header" onclick="this.parentElement.classList.toggle('expanded')">
                <svg class="reasoning-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9.663 17h4.673M12 3v1M6.5 5.5l.7.7M3 12h1M20 12h1M16.8 6.2l.7-.7M17.5 12A5.5 5.5 0 1 0 7 14.5V17a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1v-2.5A5.5 5.5 0 0 0 17.5 12z"/></svg>
                <span class="reasoning-title">思考中...</span>
                <svg class="reasoning-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>
            </div>
            <div class="reasoning-content"></div>
        `;
        bodyEl.prepend(reasoningBlock);
    }

    const contentEl = reasoningBlock.querySelector('.reasoning-content');
    if (contentEl) {
        let rawReasoning = contentEl.getAttribute('data-raw') || '';
        rawReasoning += event.content || '';
        contentEl.setAttribute('data-raw', rawReasoning);
        contentEl.innerHTML = this.renderMarkdown(rawReasoning, true);
    }

    this.scrollToBottom();
};

FKTeamsChat.prototype.handleMessage = function (event) {
    if (!event.content && !event.reasoning_content) return;

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

        // 处理推理/思考内容（非流式完整消息）
        if (event.reasoning_content) {
            let reasoningBlock = bodyEl.querySelector('.reasoning-block');
            if (!reasoningBlock) {
                reasoningBlock = document.createElement('div');
                reasoningBlock.className = 'reasoning-block';
                reasoningBlock.innerHTML = `
                    <div class="reasoning-header" onclick="this.parentElement.classList.toggle('expanded')">
                        <svg class="reasoning-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9.663 17h4.673M12 3v1M6.5 5.5l.7.7M3 12h1M20 12h1M16.8 6.2l.7-.7M17.5 12A5.5 5.5 0 1 0 7 14.5V17a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1v-2.5A5.5 5.5 0 0 0 17.5 12z"/></svg>
                        <span class="reasoning-title">思考过程</span>
                        <svg class="reasoning-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>
                    </div>
                    <div class="reasoning-content">${this.renderMarkdown(event.reasoning_content)}</div>
                `;
                bodyEl.prepend(reasoningBlock);
            }
        }

        if (event.content) {
            const content = this.trimLeadingWhitespace(event.content);
            bodyEl.setAttribute('data-raw', content);
            bodyEl.setAttribute('data-fn-done', '1');
            // 保留已有的推理块
            const existingReasoning = bodyEl.querySelector('.reasoning-block');
            if (existingReasoning) {
                // 保留推理块，创建新的文本内容容器
                let textContainer = bodyEl.querySelector('.message-text-content');
                if (!textContainer) {
                    textContainer = document.createElement('div');
                    textContainer.className = 'message-text-content';
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
    if (!event.tool_calls || event.tool_calls.length === 0) return;

    this.hasToolCallAfterMessage = true;
    // 记录最近调用的工具名，供 handleToolResult 识别
    this.lastToolName = event.tool_calls[0].name;

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
    this.scrollToBottom();
};

FKTeamsChat.prototype.handleToolCalls = function (event) {
    if (!event.tool_calls || event.tool_calls.length === 0) return;

    // dispatch_tasks: 暂存任务列表，卡片在审批通过后由 dispatch_progress 触发创建
    if (this.lastToolName === 'dispatch_tasks' && event.tool_calls[0].arguments) {
        try {
            const args = JSON.parse(event.tool_calls[0].arguments);
            if (args.tasks && args.tasks.length > 0) {
                this._pendingDispatchTasks = args.tasks;
                // 移除 tool-call 占位
                const toolCalls = this.messagesContainer.querySelectorAll('.tool-call');
                const lastToolCall = toolCalls[toolCalls.length - 1];
                if (lastToolCall) lastToolCall.remove();
                this.scrollToBottom();
                return;
            }
        } catch { /* fall through */ }
    }

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
    this.scrollToBottom();
};

FKTeamsChat.prototype.handleToolResult = function (event) {
    let content = event.content || '';

    // dispatch_tasks 专用渲染
    if (this.lastToolName === 'dispatch_tasks') {
        // 移除实时进度容器
        const progress = document.getElementById('dispatch-progress');
        if (progress) progress.remove();
        const el = this.renderDispatchResult(content);
        if (el) {
            this.messagesContainer.appendChild(el);
            this.scrollToBottom();
            return;
        }
    }

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
    this.scrollToBottom();
};

// 初始化 dispatch 实时进度容器（从 tool_calls 解析得到全部任务）
FKTeamsChat.prototype._initDispatchProgress = function (tasks) {
    // 如果已存在则不重复创建
    if (document.getElementById('dispatch-progress')) return;

    const container = document.createElement('div');
    container.id = 'dispatch-progress';
    container.className = 'dispatch-result dispatch-progress-live';

    const header = document.createElement('div');
    header.className = 'dispatch-header dispatch-status-partial';
    header.innerHTML = `<span class="dispatch-progress-title">并行分发 ${tasks.length} 个子任务</span>
        <span class="dispatch-progress-counter" data-total="${tasks.length}" data-done="0">0/${tasks.length}</span>`;
    container.appendChild(header);

    const cardsWrap = document.createElement('div');
    cardsWrap.className = 'dispatch-cards';

    for (let i = 0; i < tasks.length; i++) {
        const t = tasks[i];
        const card = document.createElement('div');
        card.id = 'dispatch-task-' + i;
        card.className = 'dispatch-card dispatch-card-waiting';
        card.innerHTML = `
            <div class="dispatch-card-head">
                <span class="dispatch-card-status-dot"></span>
                <span class="dispatch-card-desc">${this.escapeHtml(t.description || '')}</span>
            </div>
            <div class="dispatch-card-detail" style="display:none">
                <div class="dispatch-card-ops-list"></div>
                <div class="dispatch-card-error" style="display:none"></div>
            </div>
        `;
        // 点击展开/收起详情
        card.addEventListener('click', function () {
            const detail = card.querySelector('.dispatch-card-detail');
            const hasCont = detail.querySelector('.dispatch-card-ops-list').childElementCount > 0
                || detail.querySelector('.dispatch-card-error').style.display !== 'none';
            if (!hasCont) return;
            card.classList.toggle('dispatch-card-expanded');
            detail.style.display = card.classList.contains('dispatch-card-expanded') ? '' : 'none';
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
            const el = document.createElement('div');
            el.className = 'dispatch-result';
            el.innerHTML = `<div class="dispatch-header dispatch-status-partial"><span>${this.escapeHtml(data.error)}</span></div>`;
            return el;
        }
        if (data.results) {
            return this._buildDispatchCards(data.results);
        }
    } catch { /* fallback */ }
    return null;
};

FKTeamsChat.prototype._buildDispatchCards = function (results) {
    const total = results.length;
    const success = results.filter(r => r.status === 'success').length;
    const failed = total - success;

    const container = document.createElement('div');
    container.className = 'dispatch-result';

    const statusClass = failed === 0 ? 'dispatch-status-ok' : 'dispatch-status-partial';
    const header = document.createElement('div');
    header.className = 'dispatch-header ' + statusClass;
    header.innerHTML = `<span>子任务分发完成: ${success}/${total} 成功${failed > 0 ? '，' + failed + ' 失败' : ''}</span>`;
    container.appendChild(header);

    const cardsWrap = document.createElement('div');
    cardsWrap.className = 'dispatch-cards';

    const self = this;
    for (const r of results) {
        const isOk = r.status === 'success';
        const card = document.createElement('div');
        card.className = 'dispatch-card ' + (isOk ? 'dispatch-card-done' : 'dispatch-card-fail');

        // 操作摘要
        let opsText = '';
        if (r.operations && r.operations.length > 0) {
            opsText = `<span class="dispatch-card-ops-count">${r.operations.length} 项操作</span>`;
        }

        card.innerHTML = `
            <div class="dispatch-card-head">
                <span class="dispatch-card-status-dot"></span>
                <span class="dispatch-card-desc">${self.escapeHtml(r.description || '')}</span>
                ${opsText}
                <span class="dispatch-card-toggle"></span>
            </div>
            <div class="dispatch-card-detail" style="display:none"></div>
        `;

        // 构建详情面板
        const detailEl = card.querySelector('.dispatch-card-detail');
        let hasDetail = false;

        if (r.error) {
            const errDiv = document.createElement('div');
            errDiv.className = 'dispatch-card-error';
            errDiv.textContent = r.error;
            detailEl.appendChild(errDiv);
            hasDetail = true;
        }

        if (r.operations && r.operations.length > 0) {
            const opsList = document.createElement('div');
            opsList.className = 'dispatch-card-ops-list';
            for (const op of r.operations) {
                const line = document.createElement('div');
                line.className = 'dispatch-card-op-item';
                line.textContent = op;
                opsList.appendChild(line);
            }
            detailEl.appendChild(opsList);
            hasDetail = true;
        }

        if (r.result) {
            const resDiv = document.createElement('div');
            resDiv.className = 'dispatch-card-result';
            resDiv.innerHTML = self.renderMarkdown(r.result);
            detailEl.appendChild(resDiv);
            hasDetail = true;
        }

        if (hasDetail) {
            card.style.cursor = 'pointer';
            card.addEventListener('click', function () {
                card.classList.toggle('dispatch-card-expanded');
                detailEl.style.display = card.classList.contains('dispatch-card-expanded') ? '' : 'none';
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
        detail = JSON.parse(event.detail || '{}');
    } catch { return; }

    const idx = detail.task_index;
    const evtType = detail.event_type;
    const desc = detail.description || '';

    // 如果进度容器不存在，用暂存的任务列表创建（审批通过后首次收到进度事件时）
    let container = document.getElementById('dispatch-progress');
    if (!container) {
        const tasks = this._pendingDispatchTasks || [{ description: desc }];
        this._initDispatchProgress(tasks);
        this._pendingDispatchTasks = null;
        container = document.getElementById('dispatch-progress');
    }

    let card = document.getElementById('dispatch-task-' + idx);
    if (!card) {
        // 动态追加（不应该到这里，但保险起见）
        const cardsWrap = container.querySelector('.dispatch-cards');
        const c = document.createElement('div');
        c.id = 'dispatch-task-' + idx;
        c.className = 'dispatch-card dispatch-card-waiting';
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
        cardsWrap.appendChild(c);
        card = c;
    }

    const opsListEl = card.querySelector('.dispatch-card-ops-list');
    const errEl = card.querySelector('.dispatch-card-error');

    switch (evtType) {
        case 'start':
            card.className = card.className.replace(/dispatch-card-waiting/, 'dispatch-card-running');
            break;
        case 'op':
            if (opsListEl) {
                const line = document.createElement('div');
                line.className = 'dispatch-card-op-item';
                line.textContent = detail.event_detail || '';
                opsListEl.appendChild(line);
            }
            break;
        case 'content':
            // 内容会在最终 tool_result 中呈现，进度阶段不显示
            break;
        case 'done': {
            card.className = card.className.replace(/dispatch-card-(waiting|running)/, 'dispatch-card-done');
            this._updateDispatchCounter(container, 1);
            break;
        }
        case 'error':
            card.className = card.className.replace(/dispatch-card-(waiting|running)/, 'dispatch-card-fail');
            if (detail.event_detail && errEl) {
                errEl.style.display = '';
                errEl.textContent = detail.event_detail;
            }
            this._updateDispatchCounter(container, 1);
            break;
        case 'timeout':
            card.className = card.className.replace(/dispatch-card-(waiting|running)/, 'dispatch-card-fail');
            if (errEl) {
                errEl.style.display = '';
                errEl.textContent = '任务超时';
            }
            this._updateDispatchCounter(container, 1);
            break;
    }

    this.scrollToBottom();
};

// 更新进度计数器
FKTeamsChat.prototype._updateDispatchCounter = function (container, increment) {
    const counter = container.querySelector('.dispatch-progress-counter');
    if (!counter) return;
    let done = parseInt(counter.getAttribute('data-done') || '0') + increment;
    const total = parseInt(counter.getAttribute('data-total') || '0');
    counter.setAttribute('data-done', done);
    counter.textContent = done + '/' + total;
    if (done >= total) {
        // 全部完成，移除闪烁
        container.classList.remove('dispatch-progress-live');
        const title = container.querySelector('.dispatch-progress-title');
        if (title) title.textContent = '子任务执行完成，等待汇总...';
    }
};

FKTeamsChat.prototype.handleAction = function (event) {
    let actionClass = '';
    let actionIcon = '';

    const compressIcon = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <polyline points="4 14 10 14 10 20"/><polyline points="20 10 14 10 14 4"/>
        <line x1="14" y1="10" x2="21" y2="3"/><line x1="3" y1="21" x2="10" y2="14"/>
    </svg>`;

    // 上下文压缩开始：创建带 ID 的临时卡片
    if (event.action_type === 'context_compress_start') {
        const startEl = document.createElement('div');
        startEl.className = 'action-event context-compress';
        startEl.id = 'context-compress-pending';
        startEl.innerHTML = `${compressIcon}<span>[${this.escapeHtml(event.agent_name)}] ${this.escapeHtml(event.content || event.action_type)}</span>`;
        this.messagesContainer.appendChild(startEl);
        this.scrollToBottom();
        return;
    }

    // 上下文压缩完成：替换临时卡片为可展开的最终卡片
    if (event.action_type === 'context_compress') {
        const pendingEl = document.getElementById('context-compress-pending');
        if (pendingEl) pendingEl.remove();

        const cardEl = document.createElement('div');
        cardEl.className = 'action-event context-compress';
        if (event.detail) {
            cardEl.style.cursor = 'pointer';
            cardEl.style.flexWrap = 'wrap';
            const toggleIcon = `<svg class="toggle-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width:12px;height:12px;transition:transform 0.2s;margin-left:auto;">
                <polyline points="6 9 12 15 18 9"/>
            </svg>`;
            cardEl.innerHTML = `${compressIcon}<span>[${this.escapeHtml(event.agent_name)}] ${this.escapeHtml(event.content || event.action_type)}</span>${toggleIcon}
                <div class="compress-detail" style="display:none;width:100%;margin-top:8px;padding:10px;background:var(--bg-primary);border-radius:6px;font-size:12px;line-height:1.6;white-space:pre-wrap;word-break:break-word;color:var(--text-primary);max-height:300px;overflow-y:auto;">${this.escapeHtml(event.detail)}</div>`;
            cardEl.addEventListener('click', function () {
                const detail = cardEl.querySelector('.compress-detail');
                const toggle = cardEl.querySelector('.toggle-icon');
                if (detail.style.display === 'none') {
                    detail.style.display = 'block';
                    toggle.style.transform = 'rotate(180deg)';
                } else {
                    detail.style.display = 'none';
                    toggle.style.transform = 'rotate(0deg)';
                }
            });
        } else {
            cardEl.innerHTML = `${compressIcon}<span>[${this.escapeHtml(event.agent_name)}] ${this.escapeHtml(event.content || event.action_type)}</span>`;
        }
        this.messagesContainer.appendChild(cardEl);
        this.scrollToBottom();
        return;
    }

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
    this.scrollToBottom();
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
    this.scrollToBottom();
    this.isProcessing = false;
    this.updateStatus('connected', '已连接');
    this.updateSendButtonState();
};

FKTeamsChat.prototype.addUserMessage = function (content, attachments) {
    const messageEl = document.createElement('div');
    messageEl.className = 'message user';
    messageEl.setAttribute('data-message-id', `msg-${Date.now()}`);

    let attachmentsHtml = '';
    if (attachments && attachments.length > 0) {
        const previews = attachments.map(att => {
            if (att.type === 'image') {
                return `<img class="attachment-preview-img" src="data:${att.mimeType};base64,${att.base64}" alt="uploaded image" />`;
            }
            return '';
        }).join('');
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

// 在聊天区域显示审批请求卡片
FKTeamsChat.prototype.showApprovalRequest = function (message) {
    var el = document.createElement('div');
    el.className = 'action-event approval-request';
    el.innerHTML = '<span>' + this.escapeHtml(message || '需要审批') + '</span>';
    this.messagesContainer.appendChild(el);
    this.scrollToBottom();
};

// 显示审批弹窗
FKTeamsChat.prototype.showApprovalDialog = function (message) {
    var modal = document.getElementById('approval-modal');
    var msgEl = document.getElementById('approval-message');
    msgEl.textContent = message || '需要审批';
    modal.style.display = 'flex';

    // 更新状态提示
    this.updateStatus('processing', '等待审批...');
};

// 发送审批决定
FKTeamsChat.prototype.sendApprovalDecision = function (decision) {
    var modal = document.getElementById('approval-modal');
    modal.style.display = 'none';

    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        this.ws.send(JSON.stringify({
            type: 'approval',
            decision: decision
        }));
    }

    // 在聊天中显示审批结果
    var labels = { 0: '已拒绝', 1: '已允许（一次）', 2: '已允许（该项）', 3: '已全部允许' };
    var classes = { 0: 'rejected', 1: 'approved', 2: 'approved', 3: 'approved' };

    var el = document.createElement('div');
    el.className = 'action-event approval-result ' + (classes[decision] || '');
    el.innerHTML = '<span>' + this.escapeHtml(labels[decision] || '审批完成') + '</span>';
    this.messagesContainer.appendChild(el);
    this.scrollToBottom();

    this.updateStatus('processing', '处理中...');
};

// 附件管理
FKTeamsChat.prototype.clearAttachments = function () {
    this.attachments = [];
    const preview = document.getElementById('attachments-preview');
    if (preview) {
        preview.innerHTML = '';
        preview.style.display = 'none';
    }
    this.updateSendButtonState();
};

FKTeamsChat.prototype.removeAttachment = function (index) {
    this.attachments.splice(index, 1);
    this.renderAttachmentPreviews();
    this.updateSendButtonState();
};

FKTeamsChat.prototype.renderAttachmentPreviews = function () {
    const preview = document.getElementById('attachments-preview');
    if (!preview) return;

    if (this.attachments.length === 0) {
        preview.innerHTML = '';
        preview.style.display = 'none';
        return;
    }

    preview.style.display = 'flex';
    preview.innerHTML = this.attachments.map((att, i) => {
        if (att.type === 'image') {
            return `<div class="attachment-item">
                <img src="data:${att.mimeType};base64,${att.base64}" alt="preview" />
                <button class="attachment-remove" onclick="app.removeAttachment(${i})">&times;</button>
            </div>`;
        }
        return '';
    }).join('');
};

FKTeamsChat.prototype.initFileUpload = function () {
    const fileInput = document.getElementById('file-upload');
    const uploadBtn = document.getElementById('upload-btn');
    if (!fileInput || !uploadBtn) return;

    uploadBtn.addEventListener('click', () => fileInput.click());

    fileInput.addEventListener('change', (e) => {
        const files = Array.from(e.target.files);
        files.forEach(file => {
            if (!file.type.startsWith('image/')) return;
            const reader = new FileReader();
            reader.onload = (ev) => {
                const base64 = ev.target.result.split(',')[1];
                this.attachments.push({
                    type: 'image',
                    mimeType: file.type,
                    base64: base64,
                    name: file.name
                });
                this.renderAttachmentPreviews();
                this.updateSendButtonState();
            };
            reader.readAsDataURL(file);
        });
        fileInput.value = '';
    });
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

    // 隐藏回到底部按钮（切换到空页面时需要重置）
    this.showScrollToBottomBtn(false);
    this.userScrolledUp = false;

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
