/**
 * navigation.js - 一体化问题导航
 * 默认收缩为小圆点列，悬浮展开显示问题文本
 */

// ===== 快速导航功能 =====

FKTeamsChat.prototype.addQuestionToNav = function (content, messageElement) {
    const questionId = messageElement.getAttribute('data-message-id');
    const question = {
        id: questionId,
        content: content,
        element: messageElement
    };

    this.userQuestions.push(question);
    this.updateQuickNav();
};

FKTeamsChat.prototype.updateQuickNav = function () {
    if (!this.quickNavList || !this.quickNavWrapper) return;

    if (this.userQuestions.length === 0) {
        this.quickNavWrapper.style.display = 'none';
        return;
    }

    this.quickNavWrapper.style.display = '';
    this.quickNavList.innerHTML = '';

    // 倒序显示（最新在上）
    const reversedQuestions = [...this.userQuestions].reverse();

    reversedQuestions.forEach((question, index) => {
        const actualIndex = this.userQuestions.length - index;

        const item = document.createElement('div');
        item.className = 'quick-nav-item';
        item.setAttribute('data-question-id', question.id);

        // 最新的一个高亮
        if (index === 0) {
            item.classList.add('active');
        }

        // 截取问题文本（最多 20 字符）
        const shortText = question.content.length > 20
            ? question.content.substring(0, 20) + '…'
            : question.content;

        item.innerHTML = `
            <span class="quick-nav-dot"></span>
            <span class="quick-nav-index">${actualIndex}</span>
            <span class="quick-nav-text">${this.escapeHtml(shortText)}</span>
        `;

        item.addEventListener('click', () => {
            this.scrollToQuestion(question.id);
        });

        this.quickNavList.appendChild(item);
    });
};

FKTeamsChat.prototype.scrollToQuestion = function (questionId) {
    const messageElement = this.messagesContainer.querySelector(`[data-message-id="${questionId}"]`);
    if (messageElement) {
        messageElement.scrollIntoView({ behavior: 'smooth', block: 'center' });

        // 只高亮消息文本部分
        const bodyEl = messageElement.querySelector('.message-body');
        const target = bodyEl || messageElement;
        target.classList.add('message-highlight');
        setTimeout(() => {
            target.classList.remove('message-highlight');
        }, 2000);
    }

    this.updateQuickNavHighlight(questionId);
};

FKTeamsChat.prototype.updateQuickNavHighlight = function (questionId) {
    if (!this.quickNavList) return;

    const allItems = this.quickNavList.querySelectorAll('.quick-nav-item');
    allItems.forEach(item => {
        if (item.getAttribute('data-question-id') === questionId) {
            item.classList.add('active');
        } else {
            item.classList.remove('active');
        }
    });
};

FKTeamsChat.prototype.clearQuickNav = function () {
    this.userQuestions = [];
    this.updateQuickNav();
};
