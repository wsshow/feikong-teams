/**
 * history.js - 历史记录管理
 */

// ===== 历史记录管理 =====

FKTeamsChat.prototype.showHistoryModal = async function () {
    this.historyModal.style.display = 'flex';
    await this.loadHistoryFiles();
};

FKTeamsChat.prototype.hideHistoryModal = function () {
    this.historyModal.style.display = 'none';
};

FKTeamsChat.prototype.loadHistoryFiles = async function () {
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
};

FKTeamsChat.prototype.renderHistoryList = function (files) {
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
};

FKTeamsChat.prototype.loadHistoryFile = function (filename) {
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
};

FKTeamsChat.prototype.showChatLoading = function () {
    if (this.chatLoading) {
        this.chatLoading.style.display = 'flex';
    }
};

FKTeamsChat.prototype.hideChatLoading = function () {
    if (this.chatLoading) {
        this.chatLoading.style.display = 'none';
    }
};

FKTeamsChat.prototype.checkAndLoadSessionHistory = async function (sessionId) {
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
};

FKTeamsChat.prototype.handleHistoryLoaded = function (event) {
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
};

FKTeamsChat.prototype.renderHistoryUserMessage = function (msg) {
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
};

FKTeamsChat.prototype.renderHistoryToolCalls = function (toolCalls) {
    toolCalls.forEach(tc => {
        this.renderSingleToolCall(tc);
    });
};

FKTeamsChat.prototype.renderSingleToolCall = function (tc) {
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
};

FKTeamsChat.prototype.renderHistoryActions = function (actions, agentName) {
    actions.forEach(action => {
        this.renderSingleAction(action, agentName);
    });
};

FKTeamsChat.prototype.renderSingleAction = function (action, agentName) {
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
};

FKTeamsChat.prototype.deleteHistoryFile = function (filename) {
    this.currentDeleteFilename = filename;
    this.deleteFilenameSpan.textContent = filename;
    this.showDeleteModal();
};

FKTeamsChat.prototype.showDeleteModal = function () {
    this.deleteModal.style.display = 'flex';
    setTimeout(() => {
        this.deleteConfirmBtn.focus();
    }, 100);
};

FKTeamsChat.prototype.hideDeleteModal = function () {
    this.deleteModal.style.display = 'none';
    this.currentDeleteFilename = null;
};

FKTeamsChat.prototype.confirmDelete = async function () {
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
};

FKTeamsChat.prototype.renameHistoryFile = async function (filename) {
    this.currentRenameFilename = filename;
    this.renameInput.value = filename;
    this.showRenameModal();
};

FKTeamsChat.prototype.showRenameModal = function () {
    this.renameModal.style.display = 'flex';
    setTimeout(() => {
        this.renameInput.focus();
        this.renameInput.select();
    }, 100);
};

FKTeamsChat.prototype.hideRenameModal = function () {
    this.renameModal.style.display = 'none';
    this.currentRenameFilename = null;
};

FKTeamsChat.prototype.confirmRename = async function () {
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
};

FKTeamsChat.prototype.formatTime = function (timeString) {
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
};

FKTeamsChat.prototype.formatSize = function (bytes) {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
};

FKTeamsChat.prototype.formatHistoryTime = function (timeInfo) {
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
};
