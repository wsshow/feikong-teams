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
        this.updateMobileNavCount();
        return;
    }

    this.quickNavWrapper.style.display = '';
    this.quickNavList.innerHTML = '';
    this.updateMobileNavCount();

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
    this.updateMobileNavCount();
};

// ===== 移动端工具栏 =====

FKTeamsChat.prototype.initMobileToolbar = function () {
    const inputContainer = document.querySelector('.input-container');
    if (!inputContainer) return;

    this.mobileToolbar = document.createElement('div');
    this.mobileToolbar.className = 'mobile-toolbar';
    this.mobileToolbar.style.position = 'relative';
    this.mobileToolbar.innerHTML = `
        <button class="mobile-tool-btn" id="mobile-at-btn">@</button>
        <button class="mobile-tool-btn" id="mobile-hash-btn">#</button>
        <button class="mobile-tool-btn mobile-nav-btn" id="mobile-nav-btn">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M4 6h16M4 12h16M4 18h16"/>
            </svg>
            <span class="nav-count" id="mobile-nav-count"></span>
        </button>
    `;

    inputContainer.insertBefore(this.mobileToolbar, inputContainer.firstChild);

    // @ 按钮：插入 @ 并触发智能体建议
    this.mobileToolbar.querySelector('#mobile-at-btn').addEventListener('click', () => {
        this.insertCharAtCursor('@');
        this.messageInput.focus();
        this.handleInputForMention();
    });

    // # 按钮：插入 # 并触发文件引用建议
    this.mobileToolbar.querySelector('#mobile-hash-btn').addEventListener('click', () => {
        this.insertCharAtCursor('#');
        this.messageInput.focus();
        this.handleInputForMention();
    });

    // 导航按钮：切换快速导航弹窗
    this.mobileToolbar.querySelector('#mobile-nav-btn').addEventListener('click', (e) => {
        e.stopPropagation();
        this.toggleMobileNav();
    });

    // 点击其他区域关闭弹窗
    document.addEventListener('click', (e) => {
        if (this._mobileNavPopup && !this._mobileNavPopup.contains(e.target)) {
            this.hideMobileNav();
        }
    });
};

FKTeamsChat.prototype.insertCharAtCursor = function (char) {
    const textarea = this.messageInput;
    const start = textarea.selectionStart;
    const end = textarea.selectionEnd;
    const value = textarea.value;
    textarea.value = value.substring(0, start) + char + value.substring(end);
    textarea.selectionStart = textarea.selectionEnd = start + char.length;
    this.handleInputChange();
};

FKTeamsChat.prototype.toggleMobileNav = function () {
    if (this._mobileNavPopup) {
        this.hideMobileNav();
    } else {
        this.showMobileNav();
    }
};

FKTeamsChat.prototype.showMobileNav = function () {
    this.hideMobileNav();

    const popup = document.createElement('div');
    popup.className = 'mobile-nav-popup';

    if (this.userQuestions.length === 0) {
        popup.innerHTML = '<span class="mobile-nav-empty">暂无问题</span>';
    } else {
        const reversedQuestions = [...this.userQuestions].reverse();
        reversedQuestions.forEach((question, index) => {
            const actualIndex = this.userQuestions.length - index;
            const shortText = question.content.length > 16
                ? question.content.substring(0, 16) + '…'
                : question.content;
            const item = document.createElement('div');
            item.className = 'mobile-nav-item';
            item.innerHTML = `<span class="mobile-nav-index">${actualIndex}</span><span class="mobile-nav-text">${this.escapeHtml(shortText)}</span>`;
            item.addEventListener('click', (e) => {
                e.stopPropagation();
                this.scrollToQuestion(question.id);
                this.hideMobileNav();
            });
            popup.appendChild(item);
        });
    }

    this.mobileToolbar.appendChild(popup);
    this._mobileNavPopup = popup;
};

FKTeamsChat.prototype.hideMobileNav = function () {
    if (this._mobileNavPopup) {
        this._mobileNavPopup.remove();
        this._mobileNavPopup = null;
    }
};

FKTeamsChat.prototype.updateMobileNavCount = function () {
    const countEl = document.getElementById('mobile-nav-count');
    if (countEl) {
        countEl.textContent = this.userQuestions.length > 0 ? this.userQuestions.length : '';
    }
};
