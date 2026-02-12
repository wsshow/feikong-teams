/**
 * navigation.js - 快速导航
 */

// ===== 快速导航功能 =====

FKTeamsChat.prototype.addQuestionToNav = function (content, messageElement) {
    const questionId = messageElement.getAttribute('data-message-id');
    const question = {
        id: questionId,
        content: content,
        time: this.getCurrentTime(),
        element: messageElement
    };

    this.userQuestions.push(question);
    this.updateQuickNav();
};

FKTeamsChat.prototype.updateQuickNav = function () {
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
};

FKTeamsChat.prototype.scrollToQuestion = function (questionId) {
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
};

FKTeamsChat.prototype.updateQuickNavHighlight = function (questionId) {
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
};

FKTeamsChat.prototype.clearQuickNav = function () {
    this.userQuestions = [];
    this.updateQuickNav();
};
