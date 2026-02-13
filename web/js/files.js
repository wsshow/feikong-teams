/**
 * files.js - 文件引用功能
 */

// 加载文件列表
FKTeamsChat.prototype.loadFiles = async function (path = '') {
    try {
        const url = path ? `/api/fkteams/files?path=${encodeURIComponent(path)}` : '/api/fkteams/files';
        const response = await this.fetchWithAuth(url);
        const result = await response.json();
        if (result.code === 0 && result.data) {
            this.files = result.data;
            this.currentPath = path;
            return this.files;
        }
    } catch (error) {
        console.error('加载文件列表失败:', error);
    }
    return [];
};

// 显示文件建议列表
FKTeamsChat.prototype.showFileSuggestions = async function (searchText, cursorPos) {
    // 解析路径和搜索文本
    const parts = searchText.split('/');
    const searchFileName = parts[parts.length - 1];
    const searchPath = parts.slice(0, -1).join('/');

    // 加载文件列表
    const files = await this.loadFiles(searchPath);

    // 过滤文件列表
    const filteredFiles = files.filter(file => {
        const name = file.name.toLowerCase();
        const search = searchFileName.toLowerCase();
        return name.includes(search);
    });

    if (filteredFiles.length === 0 && !searchPath) {
        this.hideFileSuggestions();
        return;
    }

    // 创建或更新建议弹窗
    if (!this.fileSuggestions) {
        this.fileSuggestions = document.createElement('div');
        this.fileSuggestions.className = 'file-suggestions';
        document.body.appendChild(this.fileSuggestions);
    }

    // 生成建议列表HTML
    let html = '';

    // 如果不在根目录，添加"返回上级"选项
    if (searchPath) {
        const parentPath = searchPath.split('/').slice(0, -1).join('/');
        html += `
            <div class="file-suggestion-item file-suggestion-parent" 
                 data-index="-1" 
                 data-path="${parentPath}"
                 data-is-dir="true"
                 data-is-parent="true">
                <div class="file-suggestion-icon">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M19 12H5M12 19l-7-7 7-7"/>
                    </svg>
                </div>
                <div class="file-suggestion-info">
                    <div class="file-suggestion-name">返回上级</div>
                    <div class="file-suggestion-path">#${this.escapeHtml(parentPath || '.')}</div>
                </div>
            </div>
        `;
    }

    html += filteredFiles.map((file, index) => `
        <div class="file-suggestion-item ${index === 0 && !searchPath ? 'selected' : ''}" 
             data-index="${index}" 
             data-path="${this.escapeHtml(file.path)}"
             data-is-dir="${file.is_dir}">
            <div class="file-suggestion-icon">
                ${file.is_dir ? `
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/>
                    </svg>
                ` : `
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
                        <polyline points="14 2 14 8 20 8"/>
                    </svg>
                `}
            </div>
            <div class="file-suggestion-info">
                <div class="file-suggestion-name">${this.escapeHtml(file.name)}</div>
                <div class="file-suggestion-path">#${this.escapeHtml(file.path)}</div>
                ${file.is_dir ? '' : `<div class="file-suggestion-meta">${this.formatFileSize(file.size)} · ${this.formatFileTime(file.mod_time)}</div>`}
            </div>
        </div>
    `).join('');

    this.fileSuggestions.innerHTML = html;

    this.fileSuggestions.style.display = 'block';
    this.selectedFileIndex = searchPath ? -1 : 0;

    // 计算弹窗位置
    const inputWrapper = this.messageInput.closest('.input-wrapper');
    const wrapperRect = inputWrapper ? inputWrapper.getBoundingClientRect() : this.messageInput.getBoundingClientRect();

    this.fileSuggestions.style.width = wrapperRect.width + 'px';
    this.fileSuggestions.style.left = wrapperRect.left + 'px';
    this.fileSuggestions.style.bottom = (window.innerHeight - wrapperRect.top + 10) + 'px';
    this.fileSuggestions.style.top = 'auto';

    // 绑定点击事件
    this.fileSuggestions.querySelectorAll('.file-suggestion-item').forEach(item => {
        // 单击：选择文件或文件夹
        item.addEventListener('click', () => {
            const filePath = item.getAttribute('data-path');
            const isParent = item.getAttribute('data-is-parent') === 'true';

            if (isParent) {
                // 返回上级目录
                const newPath = filePath ? filePath + '/' : '';
                this.showFileSuggestions(newPath, cursorPos);
            } else {
                // 选择文件或文件夹
                this.insertFileMention(filePath);
            }
        });

        // 双击：进入文件夹
        item.addEventListener('dblclick', async (e) => {
            e.stopPropagation();
            const filePath = item.getAttribute('data-path');
            const isDir = item.getAttribute('data-is-dir') === 'true';
            const isParent = item.getAttribute('data-is-parent') === 'true';

            if (isDir && !isParent) {
                // 进入子目录
                await this.showFileSuggestions(filePath + '/', cursorPos);
            }
        });
    });
};

// 隐藏文件建议列表
FKTeamsChat.prototype.hideFileSuggestions = function () {
    if (this.fileSuggestions) {
        this.fileSuggestions.style.display = 'none';
    }
    this.selectedFileIndex = -1;
};

// 插入文件提及
FKTeamsChat.prototype.insertFileMention = function (filePath) {
    const textarea = this.messageInput;
    const value = textarea.value;
    const cursorPos = textarea.selectionStart;

    // 获取光标前的文本，找到最后一个#符号的位置
    const textBefore = value.substring(0, cursorPos);
    const hashMatch = textBefore.match(/#([^\s]*)$/);

    if (hashMatch) {
        const hashPos = cursorPos - hashMatch[0].length;
        const textAfter = value.substring(cursorPos);

        // 替换#及其后的部分文本为完整的文件路径
        textarea.value = value.substring(0, hashPos) + '#' + filePath + ' ' + textAfter;

        // 设置光标位置到插入文本之后
        const newCursorPos = hashPos + filePath.length + 2;
        textarea.setSelectionRange(newCursorPos, newCursorPos);

        // 触发input事件更新高度
        this.handleInputChange();
    }

    this.hideFileSuggestions();
    textarea.focus();
};

// 处理文件建议列表的键盘导航
FKTeamsChat.prototype.handleFileSuggestionKeyDown = function (e) {
    if (!this.fileSuggestions || this.fileSuggestions.style.display === 'none') {
        return false;
    }

    const items = this.fileSuggestions.querySelectorAll('.file-suggestion-item');
    if (items.length === 0) return false;

    const hasParent = items[0] && items[0].getAttribute('data-is-parent') === 'true';
    const maxIndex = hasParent ? items.length - 2 : items.length - 1;

    switch (e.key) {
        case 'ArrowDown':
            e.preventDefault();
            if (hasParent) {
                this.selectedFileIndex = this.selectedFileIndex >= maxIndex ? -1 : this.selectedFileIndex + 1;
            } else {
                this.selectedFileIndex = (this.selectedFileIndex + 1) % items.length;
            }
            this.updateFileSuggestionSelection(items);
            return true;

        case 'ArrowUp':
            e.preventDefault();
            if (hasParent) {
                this.selectedFileIndex = this.selectedFileIndex <= -1 ? maxIndex : this.selectedFileIndex - 1;
            } else {
                this.selectedFileIndex = (this.selectedFileIndex - 1 + items.length) % items.length;
            }
            this.updateFileSuggestionSelection(items);
            return true;

        case 'Enter':
            // Enter键：选择文件或文件夹
            if (this.selectedFileIndex >= -1 && this.selectedFileIndex <= maxIndex) {
                e.preventDefault();
                let selectedItem;
                if (hasParent) {
                    selectedItem = this.selectedFileIndex === -1 ? items[0] : items[this.selectedFileIndex + 1];
                } else {
                    selectedItem = items[this.selectedFileIndex];
                }

                if (!selectedItem) return false;

                const filePath = selectedItem.getAttribute('data-path');
                const isParent = selectedItem.getAttribute('data-is-parent') === 'true';

                if (isParent) {
                    // 返回上级目录
                    const newPath = filePath ? filePath + '/' : '';
                    this.showFileSuggestions(newPath, this.messageInput.selectionStart);
                } else {
                    // 选择文件或文件夹
                    this.insertFileMention(filePath);
                }
                return true;
            }
            break;

        case 'Tab':
            // Tab键：进入文件夹或选择文件
            if (this.selectedFileIndex >= -1 && this.selectedFileIndex <= maxIndex) {
                e.preventDefault();
                let selectedItem;
                if (hasParent) {
                    selectedItem = this.selectedFileIndex === -1 ? items[0] : items[this.selectedFileIndex + 1];
                } else {
                    selectedItem = items[this.selectedFileIndex];
                }

                if (!selectedItem) return false;

                const filePath = selectedItem.getAttribute('data-path');
                const isDir = selectedItem.getAttribute('data-is-dir') === 'true';
                const isParent = selectedItem.getAttribute('data-is-parent') === 'true';

                if (isDir) {
                    if (isParent) {
                        // 返回上级目录
                        const newPath = filePath ? filePath + '/' : '';
                        this.showFileSuggestions(newPath, this.messageInput.selectionStart);
                    } else {
                        // 进入子目录
                        this.showFileSuggestions(filePath + '/', this.messageInput.selectionStart);
                    }
                } else {
                    // 文件则选择
                }
                return true;
            }
            break;

        case 'Escape':
            e.preventDefault();
            this.hideFileSuggestions();
            return true;
    }

    return false;
};

// 更新文件建议列表选中状态
FKTeamsChat.prototype.updateFileSuggestionSelection = function (items) {
    const hasParent = items[0] && items[0].getAttribute('data-is-parent') === 'true';

    items.forEach((item, index) => {
        const itemIndex = hasParent ? index - 1 : index;
        item.classList.toggle('selected', itemIndex === this.selectedFileIndex);
    });

    // 滚动到可视区域
    const actualIndex = hasParent ? this.selectedFileIndex + 1 : this.selectedFileIndex;
    if (actualIndex >= 0 && actualIndex < items.length) {
        items[actualIndex].scrollIntoView({ block: 'nearest' });
    }
};

// 提取文件路径
FKTeamsChat.prototype.extractFilePaths = function (input) {
    const paths = [];
    const regex = /#([^\s]+)/g;
    let match;
    while ((match = regex.exec(input)) !== null) {
        paths.push(match[1]);
    }
    return paths;
};

// 格式化文件大小
FKTeamsChat.prototype.formatFileSize = function (bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
};

// 格式化文件修改时间
FKTeamsChat.prototype.formatFileTime = function (timestamp) {
    const date = new Date(timestamp * 1000);
    const now = new Date();
    const diff = now - date;
    const days = Math.floor(diff / (1000 * 60 * 60 * 24));

    if (days === 0) {
        const hours = Math.floor(diff / (1000 * 60 * 60));
        if (hours === 0) {
            const minutes = Math.floor(diff / (1000 * 60));
            return minutes === 0 ? '刚刚' : `${minutes}分钟前`;
        }
        return `${hours}小时前`;
    } else if (days === 1) {
        return '昨天';
    } else if (days < 7) {
        return `${days}天前`;
    } else {
        const year = date.getFullYear();
        const month = String(date.getMonth() + 1).padStart(2, '0');
        const day = String(date.getDate()).padStart(2, '0');
        return now.getFullYear() === year ? `${month}-${day}` : `${year}-${month}-${day}`;
    }
};
