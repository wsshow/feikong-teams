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
}

document.addEventListener('DOMContentLoaded', () => {
    window.app = new FKTeamsChat();
    window.fkteamsChat = window.app; // 保持向后兼容
});
