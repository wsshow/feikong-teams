/**
 * agents.js - 智能体提及功能
 */

// 加载智能体列表
FKTeamsChat.prototype.loadAgents = async function () {
    try {
        const response = await fetch('/api/fkteams/agents');
        const result = await response.json();
        if (result.code === 0 && result.data) {
            this.agents = result.data;
        }
    } catch (error) {
        console.error('加载智能体列表失败:', error);
    }
};

// 处理输入框输入，检测@提及和#文件
FKTeamsChat.prototype.handleInputForMention = function (e) {
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
};

// 显示智能体建议列表
FKTeamsChat.prototype.showAgentSuggestions = function (searchText, cursorPos) {
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
};

// 隐藏智能体建议列表
FKTeamsChat.prototype.hideAgentSuggestions = function () {
    if (this.agentSuggestions) {
        this.agentSuggestions.style.display = 'none';
    }
    this.selectedAgentIndex = -1;
};

// 插入智能体提及
FKTeamsChat.prototype.insertAgentMention = function (agentName) {
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
};

// 处理建议列表的键盘导航
FKTeamsChat.prototype.handleSuggestionKeyDown = function (e) {
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
};

// 更新建议列表选中状态
FKTeamsChat.prototype.updateSuggestionSelection = function (items) {
    items.forEach((item, index) => {
        item.classList.toggle('selected', index === this.selectedAgentIndex);
    });

    // 滚动到可视区域
    if (this.selectedAgentIndex >= 0 && this.selectedAgentIndex < items.length) {
        items[this.selectedAgentIndex].scrollIntoView({ block: 'nearest' });
    }
};

// 提取智能体提及
FKTeamsChat.prototype.extractAgentMention = function (input) {
    const trimmed = input.trim();
    const match = trimmed.match(/^@([\u4e00-\u9fa5\w]+)\s*(.*)$/);
    if (match) {
        return {
            agentName: match[1],
            query: match[2].trim()
        };
    }
    return null;
};
