/**
 * filemanager.js - 文件管理器
 */

// ===== 常量 =====

const FM_CHUNK_SIZE = 2 * 1024 * 1024; // 2MB per chunk
const FM_CHUNK_THRESHOLD = 5 * 1024 * 1024; // >5MB use chunk upload

// ===== 初始化 =====

FKTeamsChat.prototype.initFileManager = function () {
    this.fmModal = document.getElementById("fm-modal");
    this.fmCloseBtn = document.getElementById("fm-close-btn");
    this.fmRefreshBtn = document.getElementById("fm-refresh-btn");
    this.fmOpenBtn = document.getElementById("fm-open-btn");
    this.fmBreadcrumb = document.getElementById("fm-breadcrumb");
    this.fmList = document.getElementById("fm-list");
    this.fmBody = document.getElementById("fm-body");
    this.fmDropzone = document.getElementById("fm-dropzone");
    this.fmSelectBar = document.getElementById("fm-select-bar");
    this.fmSelectAllCb = document.getElementById("fm-select-all-cb");
    this.fmSelectCount = document.getElementById("fm-select-count");
    this.fmUploadBtn = document.getElementById("fm-upload-btn");
    this.fmDownloadBtn = document.getElementById("fm-download-btn");
    this.fmDeleteBtn = document.getElementById("fm-delete-btn");
    this.fmShareSelectedBtn = document.getElementById("fm-share-selected-btn");
    this.fmUploadBar = document.getElementById("fm-upload-bar");
    this.fmUploadText = document.getElementById("fm-upload-text");
    this.fmUploadProgressFill = document.getElementById("fm-upload-progress-fill");
    this.fmUploadCancel = document.getElementById("fm-upload-cancel");
    this.fmPreviewModal = document.getElementById("fm-preview-modal");
    this.fmPreviewClose = document.getElementById("fm-preview-close");
    this.fmPreviewRender = document.getElementById("fm-preview-render");
    this.fmPreviewFullscreen = document.getElementById("fm-preview-fullscreen");
    this.fmPreviewTitle = document.getElementById("fm-preview-title");
    this.fmPreviewBody = document.getElementById("fm-preview-body");
    this._fmPreviewRawText = null;
    this._fmPreviewExt = null;
    this._fmPreviewRendered = false;
    // Confirm modal elements
    this.fmConfirmModal = document.getElementById("fm-confirm-modal");
    this.fmConfirmClose = document.getElementById("fm-confirm-close");
    this.fmConfirmMsg = document.getElementById("fm-confirm-msg");
    this.fmConfirmWarn = document.getElementById("fm-confirm-warn");
    this.fmConfirmCancel = document.getElementById("fm-confirm-cancel");
    this.fmConfirmOk = document.getElementById("fm-confirm-ok");
    // Share modal elements
    this.fmShareModal = document.getElementById("fm-share-modal");
    this.fmShareClose = document.getElementById("fm-share-close");
    this.fmShareCancel = document.getElementById("fm-share-cancel");
    this.fmShareOk = document.getElementById("fm-share-ok");
    this.fmShareFilename = document.getElementById("fm-share-filename");
    this.fmSharePassword = document.getElementById("fm-share-password");
    this.fmShareExpiry = document.getElementById("fm-share-expiry");
    this.fmShareResult = document.getElementById("fm-share-result");
    this.fmShareLink = document.getElementById("fm-share-link");
    this.fmShareCopyBtn = document.getElementById("fm-share-copy-btn");
    this.fmShareLinksBtn = document.getElementById("fm-share-links-btn");
    this.fmShareManageBtn = document.getElementById("fm-share-manage-btn");
    // Share list modal elements
    this.fmShareListModal = document.getElementById("fm-share-list-modal");
    this.fmShareListClose = document.getElementById("fm-share-list-close");
    this.fmShareList = document.getElementById("fm-share-list");
    // File/folder upload inputs (hidden)
    this.fmFileInput = document.getElementById("fm-file-input");
    this.fmFolderInput = document.getElementById("fm-folder-input");
    // Search elements
    this.fmSearchInput = document.getElementById("fm-search-input");
    this.fmSearchClear = document.getElementById("fm-search-clear");

    // State
    this.fmCurrentPath = "";
    this.fmFiles = [];
    this.fmSelected = new Set();
    this.fmUploadAbort = null;
    this.fmSearchQuery = "";
    this.fmSearchTimer = null;

    this._bindFmEvents();
};

// ===== 事件绑定 =====

FKTeamsChat.prototype._bindFmEvents = function () {
    if (this.fmOpenBtn) {
        this.fmOpenBtn.addEventListener("click", () => this.openFileManager());
    }
    if (this.fmCloseBtn) {
        this.fmCloseBtn.addEventListener("click", () => this.closeFileManager());
    }

    if (this.fmRefreshBtn) {
        this.fmRefreshBtn.addEventListener("click", () => this.fmLoadFiles());
    }

    // Upload buttons - directly trigger file inputs
    this.fmUploadFolderBtn = document.getElementById("fm-upload-folder-btn");
    if (this.fmUploadBtn && this.fmFileInput) {
        this.fmUploadBtn.addEventListener("click", () => this.fmFileInput.click());
    }
    if (this.fmUploadFolderBtn && this.fmFolderInput) {
        this.fmUploadFolderBtn.addEventListener("click", () => this.fmFolderInput.click());
    }
    if (this.fmFileInput) {
        this.fmFileInput.addEventListener("change", (e) => {
            if (e.target.files.length > 0) {
                this._fmUploadFiles(Array.from(e.target.files));
                e.target.value = "";
            }
        });
    }
    if (this.fmFolderInput) {
        this.fmFolderInput.addEventListener("change", (e) => {
            if (e.target.files.length > 0) {
                this._fmUploadFiles(Array.from(e.target.files));
                e.target.value = "";
            }
        });
    }

    // Batch actions
    if (this.fmDownloadBtn) {
        this.fmDownloadBtn.addEventListener("click", () => this._fmDownloadSelected());
    }
    if (this.fmDeleteBtn) {
        this.fmDeleteBtn.addEventListener("click", () => this._fmDeleteSelected());
    }
    if (this.fmShareSelectedBtn) {
        this.fmShareSelectedBtn.addEventListener("click", () => {
            const selected = this._fmGetSelectedFiles();
            if (selected.length > 0) this._fmShowShareDialog(selected);
        });
    }

    // Select all
    if (this.fmSelectAllCb) {
        this.fmSelectAllCb.addEventListener("change", (e) => {
            if (e.target.checked) {
                this.fmFiles.forEach((f) => this.fmSelected.add(f.path));
            } else {
                this.fmSelected.clear();
            }
            this._fmRenderSelection();
        });
    }

    // Upload cancel
    if (this.fmUploadCancel) {
        this.fmUploadCancel.addEventListener("click", () => {
            if (this.fmUploadAbort) {
                this.fmUploadAbort.abort();
                this.fmUploadAbort = null;
            }
        });
    }

    // Preview close
    if (this.fmPreviewClose) {
        this.fmPreviewClose.addEventListener("click", () => this._fmClosePreview());
    }
    // Preview render toggle
    if (this.fmPreviewRender) {
        this.fmPreviewRender.addEventListener("click", () => this._fmToggleRender());
    }
    // Preview fullscreen toggle
    if (this.fmPreviewFullscreen) {
        this.fmPreviewFullscreen.addEventListener("click", () => this._fmToggleFullscreen());
    }


    // Confirm modal events
    if (this.fmConfirmClose) {
        this.fmConfirmClose.addEventListener("click", () => this._fmCloseConfirm());
    }
    if (this.fmConfirmCancel) {
        this.fmConfirmCancel.addEventListener("click", () => this._fmCloseConfirm());
    }


    // Share modal events
    if (this.fmShareClose) {
        this.fmShareClose.addEventListener("click", () => this._fmCloseShareDialog());
    }
    if (this.fmShareCancel) {
        this.fmShareCancel.addEventListener("click", () => this._fmCloseShareDialog());
    }

    if (this.fmShareOk) {
        this.fmShareOk.addEventListener("click", () => this._fmCreateShareLink());
    }
    if (this.fmShareCopyBtn) {
        this.fmShareCopyBtn.addEventListener("click", () => this._fmCopyShareLink());
    }
    if (this.fmShareLinksBtn) {
        this.fmShareLinksBtn.addEventListener("click", () => {
            this._fmCloseShareDialog();
            this._fmShowShareList();
        });
    }
    if (this.fmShareManageBtn) {
        this.fmShareManageBtn.addEventListener("click", () => this._fmShowShareList());
    }
    // Share list modal events
    if (this.fmShareListClose) {
        this.fmShareListClose.addEventListener("click", () => this._fmCloseShareList());
    }


    // Search events
    if (this.fmSearchInput) {
        this.fmSearchInput.addEventListener("input", () => {
            clearTimeout(this.fmSearchTimer);
            const q = this.fmSearchInput.value.trim();
            if (this.fmSearchClear) {
                this.fmSearchClear.classList.toggle("hidden", !q);
            }
            if (!q) {
                this._fmClearSearch();
                return;
            }
            this.fmSearchTimer = setTimeout(() => this._fmPerformSearch(q), 300);
        });
        this.fmSearchInput.addEventListener("keydown", (e) => {
            if (e.key === "Enter") {
                e.preventDefault();
                clearTimeout(this.fmSearchTimer);
                const q = this.fmSearchInput.value.trim();
                if (q) this._fmPerformSearch(q);
            }
        });
    }
    if (this.fmSearchClear) {
        this.fmSearchClear.addEventListener("click", () => this._fmClearSearch());
    }

    // Drag and drop on file list body
    if (this.fmBody) {
        this.fmBody.addEventListener("dragover", (e) => {
            e.preventDefault();
            if (this.fmDropzone) this.fmDropzone.classList.add("active");
        });
        this.fmBody.addEventListener("dragleave", (e) => {
            if (e.target === this.fmBody || !this.fmBody.contains(e.relatedTarget)) {
                if (this.fmDropzone) this.fmDropzone.classList.remove("active");
            }
        });
        this.fmBody.addEventListener("drop", (e) => {
            e.preventDefault();
            if (this.fmDropzone) this.fmDropzone.classList.remove("active");
            const items = e.dataTransfer.items;
            if (items && items.length > 0) {
                this._fmHandleDrop(items);
            }
        });
    }
    // Keyboard: Escape
    document.addEventListener("keydown", (e) => {
        if (e.key === "Escape") {
            if (this.fmShareModal && this.fmShareModal.style.display === "flex") {
                this._fmCloseShareDialog();
            } else if (this.fmShareListModal && this.fmShareListModal.style.display === "flex") {
                this._fmCloseShareList();
            } else if (this.fmConfirmModal && this.fmConfirmModal.style.display === "flex") {
                this._fmCloseConfirm();
            } else if (this.fmPreviewModal && this.fmPreviewModal.style.display === "flex") {
                this._fmClosePreview();
            } else if (this.fmModal && this.fmModal.style.display === "flex") {
                this.closeFileManager();
            }
        }
    });
};

// ===== 弹窗控制 =====

FKTeamsChat.prototype.openFileManager = function () {
    if (!this.fmModal) return;
    this.fmModal.style.display = "flex";
    this.fmCurrentPath = "";
    this.fmSelected.clear();
    this.fmSearchQuery = "";
    if (this.fmSearchInput) this.fmSearchInput.value = "";
    if (this.fmSearchClear) this.fmSearchClear.classList.add("hidden");
    this.fmLoadFiles();
};

FKTeamsChat.prototype.closeFileManager = function () {
    if (!this.fmModal) return;
    this.fmModal.style.display = "none";
};

// ===== 加载文件列表 =====

FKTeamsChat.prototype.fmLoadFiles = async function () {
    if (!this.fmList) return;
    this.fmList.innerHTML = '<div class="fm-loading">加载中...</div>';
    this.fmSelected.clear();
    this._fmUpdateToolbar();

    try {
        const url = this.fmCurrentPath
            ? `/api/fkteams/files?path=${encodeURIComponent(this.fmCurrentPath)}`
            : "/api/fkteams/files";
        const resp = await this.fetchWithAuth(url);
        if (!resp.ok) {
            this.fmList.innerHTML = '<div class="fm-empty">加载失败</div>';
            return;
        }
        const result = await resp.json();
        if (result.code !== 0) {
            this.fmList.innerHTML = `<div class="fm-empty">${this.escapeHtml(result.message || "加载失败")}</div>`;
            return;
        }

        this.fmFiles = result.data || [];
        this._fmRenderList();
        this._fmRenderBreadcrumb();
    } catch (err) {
        console.error("fm load error:", err);
        this.fmList.innerHTML = '<div class="fm-empty">加载失败</div>';
    }
};

// ===== 渲染文件列表 =====

FKTeamsChat.prototype._fmRenderList = function () {
    if (!this.fmList) return;

    if (!this.fmFiles || this.fmFiles.length === 0) {
        this.fmList.innerHTML = '<div class="fm-empty">空目录</div>';
        this._fmUpdateToolbar();
        return;
    }

    this.fmList.innerHTML = "";

    this.fmFiles.forEach((file) => {
        const item = document.createElement("div");
        item.className = "fm-item" + (this.fmSelected.has(file.path) ? " selected" : "");
        item.dataset.path = file.path;

        const iconClass = file.is_dir ? "folder" : "file";
        const iconSvg = file.is_dir
            ? '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>'
            : '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>';

        const sizeTxt = file.is_dir ? "" : this._fmFormatSize(file.size);
        const timeTxt = this._fmFormatTime(file.mod_time);

        const metaParts = [timeTxt];
        if (sizeTxt) metaParts.unshift(sizeTxt);

        item.innerHTML = `
      <input type="checkbox" class="fm-item-cb" ${this.fmSelected.has(file.path) ? "checked" : ""} />
      <div class="fm-item-icon ${iconClass}">${iconSvg}</div>
      <div class="fm-item-info">
        <div class="fm-item-name">${this.escapeHtml(file.name)}</div>
        <div class="fm-item-meta">${metaParts.join(" / ")}</div>
      </div>
      <div class="fm-item-actions">
        ${!file.is_dir ? `<button class="fm-item-action-btn preview-action" title="预览">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>
        </button>` : ""}
        <button class="fm-item-action-btn share-action" title="分享">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/><line x1="8.59" y1="13.51" x2="15.42" y2="17.49"/><line x1="15.41" y1="6.51" x2="8.59" y2="10.49"/></svg>
        </button>
        <button class="fm-item-action-btn download-action" title="下载">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
        </button>
        <button class="fm-item-action-btn delete-action" title="删除">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
        </button>
      </div>`;

        // Events
        const cb = item.querySelector(".fm-item-cb");
        cb.addEventListener("click", (e) => e.stopPropagation());
        cb.addEventListener("change", (e) => {
            if (e.target.checked) {
                this.fmSelected.add(file.path);
                item.classList.add("selected");
            } else {
                this.fmSelected.delete(file.path);
                item.classList.remove("selected");
            }
            this._fmRenderSelection();
        });

        // Click row: enter folder or preview file
        item.addEventListener("click", (e) => {
            if (e.target.closest(".fm-item-cb") || e.target.closest(".fm-item-actions")) return;
            if (file.is_dir) {
                this.fmCurrentPath = file.path;
                this.fmLoadFiles();
            } else {
                this._fmPreviewFile(file);
            }
        });

        // Action buttons
        const previewBtn = item.querySelector(".preview-action");
        if (previewBtn) {
            previewBtn.addEventListener("click", (e) => {
                e.stopPropagation();
                this._fmPreviewFile(file);
            });
        }
        const shareBtn = item.querySelector(".share-action");
        if (shareBtn) {
            shareBtn.addEventListener("click", (e) => {
                e.stopPropagation();
                this._fmShowShareDialog(file);
            });
        }
        const downloadBtn = item.querySelector(".download-action");
        if (downloadBtn) {
            downloadBtn.addEventListener("click", (e) => {
                e.stopPropagation();
                this._fmDownloadFile(file.path);
            });
        }
        const deleteBtn = item.querySelector(".delete-action");
        if (deleteBtn) {
            deleteBtn.addEventListener("click", (e) => {
                e.stopPropagation();
                this._fmDeleteItems([file]);
            });
        }

        this.fmList.appendChild(item);
    });

    this._fmUpdateToolbar();
};

// ===== 面包屑 =====

FKTeamsChat.prototype._fmRenderBreadcrumb = function () {
    if (!this.fmBreadcrumb) return;
    this.fmBreadcrumb.innerHTML = "";

    const root = document.createElement("button");
    root.className = "fm-breadcrumb-item" + (!this.fmCurrentPath ? " active" : "");
    root.textContent = "workspace";
    root.addEventListener("click", () => {
        if (this.fmCurrentPath) {
            this.fmCurrentPath = "";
            this.fmLoadFiles();
        }
    });
    this.fmBreadcrumb.appendChild(root);

    if (this.fmCurrentPath) {
        const parts = this.fmCurrentPath.split("/").filter(Boolean);
        parts.forEach((part, i) => {
            const sep = document.createElement("span");
            sep.className = "fm-breadcrumb-sep";
            sep.textContent = "/";
            this.fmBreadcrumb.appendChild(sep);

            const btn = document.createElement("button");
            const isLast = i === parts.length - 1;
            btn.className = "fm-breadcrumb-item" + (isLast ? " active" : "");
            btn.textContent = part;
            if (!isLast) {
                const path = parts.slice(0, i + 1).join("/");
                btn.addEventListener("click", () => {
                    this.fmCurrentPath = path;
                    this.fmLoadFiles();
                });
            }
            this.fmBreadcrumb.appendChild(btn);
        });
    }
};

// ===== 文件搜索 =====

FKTeamsChat.prototype._fmPerformSearch = async function (query) {
    if (!this.fmList) return;
    this.fmSearchQuery = query;
    this.fmList.innerHTML = '<div class="fm-loading">搜索中...</div>';
    this.fmSelected.clear();
    this._fmUpdateToolbar();

    try {
        const resp = await this.fetchWithAuth(
            `/api/fkteams/files/search?q=${encodeURIComponent(query)}`
        );
        if (!resp.ok) {
            this.fmList.innerHTML = '<div class="fm-empty">搜索失败</div>';
            return;
        }
        const result = await resp.json();
        if (result.code !== 0) {
            this.fmList.innerHTML = `<div class="fm-empty">${this.escapeHtml(result.message || "搜索失败")}</div>`;
            return;
        }

        const files = result.data || [];
        this.fmFiles = files;
        this._fmRenderSearchResults(files, query);
    } catch (err) {
        console.error("fm search error:", err);
        this.fmList.innerHTML = '<div class="fm-empty">搜索失败</div>';
    }
};

FKTeamsChat.prototype._fmRenderSearchResults = function (files, query) {
    if (!this.fmList) return;

    this.fmList.innerHTML = "";

    // hint bar
    const hint = document.createElement("div");
    hint.className = "fm-search-hint";
    hint.innerHTML = `找到 ${files.length} 个结果`;
    const clearLink = document.createElement("a");
    clearLink.textContent = "清除搜索";
    clearLink.addEventListener("click", () => this._fmClearSearch());
    hint.appendChild(clearLink);
    this.fmList.appendChild(hint);

    if (files.length === 0) {
        const empty = document.createElement("div");
        empty.className = "fm-empty";
        empty.textContent = "未找到匹配的文件";
        this.fmList.appendChild(empty);
        this._fmUpdateToolbar();
        return;
    }

    files.forEach((file) => {
        const item = document.createElement("div");
        item.className = "fm-item" + (this.fmSelected.has(file.path) ? " selected" : "");
        item.dataset.path = file.path;

        const iconSvg = file.is_dir
            ? '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>'
            : '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>';

        // Show path below name for search results
        const parentPath = file.path.includes("/")
            ? file.path.substring(0, file.path.lastIndexOf("/"))
            : "";
        const pathInfo = parentPath
            ? `<div class="fm-search-result-path">${this.escapeHtml(parentPath)}</div>`
            : "";

        item.innerHTML = `
            <input type="checkbox" class="fm-item-cb" ${this.fmSelected.has(file.path) ? "checked" : ""}>
            <div class="fm-item-icon ${file.is_dir ? "folder" : "file"}">${iconSvg}</div>
            <div class="fm-item-info">
                <div class="fm-item-name">${this.escapeHtml(file.name)}</div>
                ${pathInfo}
            </div>
            <div class="fm-item-meta">
                <span class="fm-item-size">${file.is_dir ? "" : this._fmFormatSize(file.size)}</span>
            </div>
            <div class="fm-item-actions">
                <button class="fm-item-action-btn" data-action="locate" title="定位到所在目录"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 10c0 7-9 13-9 13s-9-6-9-13a9 9 0 0 1 18 0z"/><circle cx="12" cy="10" r="3"/></svg></button>
                ${!file.is_dir ? '<button class="fm-item-action-btn" data-action="preview" title="预览"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg></button>' : ""}
                ${!file.is_dir ? '<button class="fm-item-action-btn" data-action="share" title="分享"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/><line x1="8.59" y1="13.51" x2="15.42" y2="17.49"/><line x1="15.41" y1="6.51" x2="8.59" y2="10.49"/></svg></button>' : ""}
                <button class="fm-item-action-btn" data-action="download" title="下载"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg></button>
            </div>`;

        // Click handlers
        const cb = item.querySelector(".fm-item-cb");
        if (cb) {
            cb.addEventListener("click", (e) => e.stopPropagation());
            cb.addEventListener("change", () => {
                if (cb.checked) this.fmSelected.add(file.path);
                else this.fmSelected.delete(file.path);
                item.classList.toggle("selected", cb.checked);
                this._fmUpdateToolbar();
            });
        }

        item.querySelectorAll(".fm-item-action-btn").forEach((btn) => {
            btn.addEventListener("click", (e) => {
                e.stopPropagation();
                const action = btn.dataset.action;
                if (action === "download") this._fmDownloadFile(file);
                else if (action === "preview") this._fmPreviewFile(file);
                else if (action === "share") this._fmShowShareDialog(file);
                else if (action === "locate") {
                    const parentPath = file.is_dir
                        ? file.path
                        : (file.path.includes("/") ? file.path.substring(0, file.path.lastIndexOf("/")) : "");
                    this.fmCurrentPath = parentPath;
                    this._fmClearSearch();
                }
            });
        });

        // Double-click: navigate into directory, or to file's parent directory
        item.addEventListener("dblclick", () => {
            if (file.is_dir) {
                this.fmCurrentPath = file.path;
                this._fmClearSearch();
            } else {
                // Navigate to the file's parent directory
                const parentPath = file.path.includes("/")
                    ? file.path.substring(0, file.path.lastIndexOf("/"))
                    : "";
                this.fmCurrentPath = parentPath;
                this._fmClearSearch();
            }
        });

        // Single click: toggle select
        item.addEventListener("click", (e) => {
            if (e.target.closest(".fm-item-action-btn") || e.target.closest(".fm-item-cb")) return;
            const isSelected = this.fmSelected.has(file.path);
            if (isSelected) this.fmSelected.delete(file.path);
            else this.fmSelected.add(file.path);
            item.classList.toggle("selected", !isSelected);
            if (cb) cb.checked = !isSelected;
            this._fmUpdateToolbar();
        });

        this.fmList.appendChild(item);
    });

    this._fmUpdateToolbar();
};

FKTeamsChat.prototype._fmClearSearch = function () {
    this.fmSearchQuery = "";
    if (this.fmSearchInput) this.fmSearchInput.value = "";
    if (this.fmSearchClear) this.fmSearchClear.classList.add("hidden");
    this.fmLoadFiles();
};

// ===== 选择状态 =====

FKTeamsChat.prototype._fmRenderSelection = function () {
    // Update checkboxes in list
    const items = this.fmList ? this.fmList.querySelectorAll(".fm-item") : [];
    items.forEach((item) => {
        const path = item.dataset.path;
        const cb = item.querySelector(".fm-item-cb");
        const isSelected = this.fmSelected.has(path);
        if (cb) cb.checked = isSelected;
        item.classList.toggle("selected", isSelected);
    });
    this._fmUpdateToolbar();
};

FKTeamsChat.prototype._fmUpdateToolbar = function () {
    const count = this.fmSelected.size;
    const hasFiles = this.fmFiles && this.fmFiles.length > 0;

    // Select bar
    if (this.fmSelectBar) {
        this.fmSelectBar.classList.toggle("hidden", !hasFiles);
    }
    if (this.fmSelectAllCb) {
        this.fmSelectAllCb.checked = hasFiles && count === this.fmFiles.length;
        this.fmSelectAllCb.indeterminate = count > 0 && count < this.fmFiles.length;
    }
    if (this.fmSelectCount) {
        this.fmSelectCount.textContent = count > 0 ? `已选 ${count} 项` : `共 ${this.fmFiles.length} 项`;
    }

    // Batch action buttons
    if (this.fmDownloadBtn) this.fmDownloadBtn.disabled = count === 0;
    if (this.fmDeleteBtn) this.fmDeleteBtn.disabled = count === 0;
    if (this.fmShareSelectedBtn) this.fmShareSelectedBtn.disabled = count === 0;
};

// ===== 上传 =====

FKTeamsChat.prototype._fmUploadFiles = async function (files) {
    if (!files || files.length === 0) return;

    const abortCtrl = new AbortController();
    this.fmUploadAbort = abortCtrl;

    // Show progress bar
    if (this.fmUploadBar) this.fmUploadBar.classList.add("active");
    this._fmSetUploadProgress(0, `准备上传 ${files.length} 个文件...`);

    let completed = 0;
    const total = files.length;

    for (const file of files) {
        if (abortCtrl.signal.aborted) break;

        // Determine actual target path (for folder uploads, preserve relative directory)
        let targetPath = this.fmCurrentPath;
        if (file.webkitRelativePath) {
            const parts = file.webkitRelativePath.split("/");
            if (parts.length > 1) {
                const relDir = parts.slice(0, -1).join("/");
                targetPath = this.fmCurrentPath ? this.fmCurrentPath + "/" + relDir : relDir;
            }
        }

        try {
            if (file.size > FM_CHUNK_THRESHOLD) {
                await this._fmChunkUpload(file, targetPath, abortCtrl, (pct) => {
                    const overall = ((completed + pct) / total) * 100;
                    this._fmSetUploadProgress(overall, `上传中: ${file.name} (${this._fmFormatSize(file.size)})`);
                });
            } else {
                await this._fmSimpleUpload(file, targetPath, abortCtrl.signal);
            }
            completed++;
            this._fmSetUploadProgress((completed / total) * 100, `已完成 ${completed}/${total}`);
        } catch (err) {
            if (err.name === "AbortError") break;
            console.error("upload error:", file.name, err);
        }
    }

    this.fmUploadAbort = null;
    // Hide progress bar after a short delay
    setTimeout(() => {
        if (this.fmUploadBar) this.fmUploadBar.classList.remove("active");
    }, 1000);

    // Refresh file list
    this.fmLoadFiles();
};

FKTeamsChat.prototype._fmSimpleUpload = async function (file, targetPath, signal) {
    const formData = new FormData();
    formData.append("file", file);
    if (targetPath) formData.append("path", targetPath);

    const resp = await this.fetchWithAuth("/api/fkteams/files/upload", {
        method: "POST",
        body: formData,
        signal,
    });
    if (!resp.ok) throw new Error("upload failed");
};

FKTeamsChat.prototype._fmChunkUpload = async function (file, targetPath, abortCtrl, onProgress) {
    const totalChunks = Math.ceil(file.size / FM_CHUNK_SIZE);
    const uploadId = `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;

    for (let i = 0; i < totalChunks; i++) {
        if (abortCtrl.signal.aborted) throw new DOMException("Aborted", "AbortError");

        const start = i * FM_CHUNK_SIZE;
        const end = Math.min(start + FM_CHUNK_SIZE, file.size);
        const chunk = file.slice(start, end);

        const formData = new FormData();
        formData.append("file", chunk);
        formData.append("uploadId", uploadId);
        formData.append("chunkIndex", String(i));
        formData.append("totalChunks", String(totalChunks));
        formData.append("fileName", file.name);
        if (targetPath) formData.append("path", targetPath);

        const resp = await this.fetchWithAuth("/api/fkteams/files/upload/chunk", {
            method: "POST",
            body: formData,
            signal: abortCtrl.signal,
        });
        if (!resp.ok) throw new Error(`chunk ${i} failed`);

        if (onProgress) onProgress((i + 1) / totalChunks);
    }
};

FKTeamsChat.prototype._fmSetUploadProgress = function (pct, text) {
    if (this.fmUploadText) this.fmUploadText.textContent = text;
    if (this.fmUploadProgressFill) this.fmUploadProgressFill.style.width = Math.min(pct, 100) + "%";
};

// ===== Drag and Drop (支持文件夹) =====

FKTeamsChat.prototype._fmHandleDrop = async function (items) {
    const files = [];

    // Use DataTransferItem.webkitGetAsEntry for folder support
    const entries = [];
    for (let i = 0; i < items.length; i++) {
        const entry = items[i].webkitGetAsEntry ? items[i].webkitGetAsEntry() : null;
        if (entry) entries.push(entry);
    }

    if (entries.length > 0 && entries.some((e) => e.isDirectory)) {
        // Recursively read directory entries
        const readEntry = (entry, path) => {
            return new Promise((resolve) => {
                if (entry.isFile) {
                    entry.file((f) => {
                        // Attach relative path for folder structure
                        Object.defineProperty(f, "webkitRelativePath", {
                            value: path ? path + "/" + f.name : f.name,
                            writable: false,
                        });
                        files.push(f);
                        resolve();
                    }, () => resolve());
                } else if (entry.isDirectory) {
                    const reader = entry.createReader();
                    const readAll = (allEntries) => {
                        reader.readEntries(async (batch) => {
                            if (batch.length === 0) {
                                await Promise.all(
                                    allEntries.map((e) => readEntry(e, path ? path + "/" + entry.name : entry.name))
                                );
                                resolve();
                            } else {
                                readAll(allEntries.concat(Array.from(batch)));
                            }
                        }, () => resolve());
                    };
                    readAll([]);
                } else {
                    resolve();
                }
            });
        };

        await Promise.all(entries.map((e) => readEntry(e, "")));
    } else {
        // Plain files
        for (let i = 0; i < items.length; i++) {
            const f = items[i].getAsFile();
            if (f) files.push(f);
        }
    }

    if (files.length > 0) {
        this._fmUploadFiles(files);
    }
};

// ===== 下载 =====

FKTeamsChat.prototype._fmDownloadFile = function (path) {
    const url = `/api/fkteams/files/download?path=${encodeURIComponent(path)}`;
    const a = document.createElement("a");
    a.href = url;
    // Let fetchWithAuth handle token in a hidden iframe won't work for auth,
    // so we use a direct link with token as query param is not ideal.
    // Use fetch + blob approach.
    this.fetchWithAuth(url)
        .then((resp) => {
            if (!resp.ok) throw new Error("download failed");
            return resp.blob();
        })
        .then((blob) => {
            const objUrl = URL.createObjectURL(blob);
            const link = document.createElement("a");
            link.href = objUrl;
            link.download = path.split("/").pop() || "file";
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);
            URL.revokeObjectURL(objUrl);
        })
        .catch((err) => console.error("download error:", err));
};

FKTeamsChat.prototype._fmDownloadSelected = function () {
    const selected = this._fmGetSelectedFiles();
    if (selected.length === 0) return;

    // Single file (non-directory): direct download
    if (selected.length === 1 && !selected[0].is_dir) {
        this._fmDownloadFile(selected[0].path);
        return;
    }

    // Multiple items or directory: batch zip download
    const paths = selected.map((f) => f.path);
    this.fetchWithAuth("/api/fkteams/files/download/batch", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ paths }),
    })
        .then((resp) => {
            if (!resp.ok) throw new Error("batch download failed");
            return resp.blob();
        })
        .then((blob) => {
            const url = URL.createObjectURL(blob);
            const link = document.createElement("a");
            link.href = url;
            link.download = "download.zip";
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);
            URL.revokeObjectURL(url);
        })
        .catch((err) => {
            console.error("batch download error:", err);
            this.showNotification("下载失败", "error");
        });
};

// ===== 删除 =====

FKTeamsChat.prototype._fmDeleteItems = function (items) {
    if (!items || items.length === 0) return;

    const hasDirs = items.some((f) => f.is_dir);
    let msg, warn;
    if (items.length === 1) {
        msg = `确定要删除 "${this.escapeHtml(items[0].name || items[0].path)}" 吗？`;
    } else {
        msg = `确定要删除选中的 ${items.length} 个项目吗？`;
    }
    warn = hasDirs ? "包含文件夹，将强制递归删除，此操作不可恢复！" : "此操作不可恢复！";

    this._fmShowConfirm(msg, warn, () => {
        const promises = items.map((item) => {
            const path = item.path || item;
            return this.fetchWithAuth("/api/fkteams/files", {
                method: "DELETE",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ path, force: true }),
            }).catch((err) => console.error("delete error:", path, err));
        });

        Promise.all(promises).then(() => this.fmLoadFiles());
    });
};

FKTeamsChat.prototype._fmDeleteSelected = function () {
    const selected = this._fmGetSelectedFiles();
    if (selected.length === 0) return;
    this._fmDeleteItems(selected);
};

FKTeamsChat.prototype._fmGetSelectedFiles = function () {
    return this.fmFiles.filter((f) => this.fmSelected.has(f.path));
};

// ===== 预览 =====

FKTeamsChat.prototype._fmPreviewFile = async function (file) {
    if (!this.fmPreviewModal || !this.fmPreviewBody) return;

    this.fmPreviewModal.style.display = "flex";
    if (this.fmPreviewTitle) this.fmPreviewTitle.textContent = file.name;
    this.fmPreviewBody.innerHTML = '<div class="fm-loading">加载中...</div>';
    this._fmPreviewRawText = null;
    this._fmPreviewExt = null;
    this._fmPreviewFilePath = null;
    this._fmPreviewRendered = false;
    if (this.fmPreviewRender) this.fmPreviewRender.style.display = "none";

    const ext = (file.name.split(".").pop() || "").toLowerCase();
    const url = `/api/fkteams/files/download?path=${encodeURIComponent(file.path)}`;

    try {
        // Image
        if (["png", "jpg", "jpeg", "gif", "svg", "webp", "ico", "bmp"].includes(ext)) {
            const resp = await this.fetchWithAuth(url);
            const blob = await resp.blob();
            const objUrl = URL.createObjectURL(blob);
            this.fmPreviewBody.innerHTML = `<img src="${objUrl}" alt="${this.escapeHtml(file.name)}" />`;
            return;
        }

        // Video
        if (["mp4", "webm", "ogg"].includes(ext)) {
            const resp = await this.fetchWithAuth(url);
            const blob = await resp.blob();
            const objUrl = URL.createObjectURL(blob);
            this.fmPreviewBody.innerHTML = `<video controls src="${objUrl}"></video>`;
            return;
        }

        // Audio
        if (["mp3", "wav", "ogg", "flac", "aac"].includes(ext)) {
            const resp = await this.fetchWithAuth(url);
            const blob = await resp.blob();
            const objUrl = URL.createObjectURL(blob);
            this.fmPreviewBody.innerHTML = `<audio controls src="${objUrl}"></audio>`;
            return;
        }

        // PDF
        if (ext === "pdf") {
            const resp = await this.fetchWithAuth(url);
            const blob = await resp.blob();
            const objUrl = URL.createObjectURL(blob);
            this.fmPreviewBody.innerHTML = `<iframe src="${objUrl}" style="width:100%;height:60vh;border:none;border-radius:8px;"></iframe>`;
            return;
        }

        // Text-like files
        const textExts = [
            "txt", "md", "json", "yaml", "yml", "toml", "xml", "html", "css", "js", "ts",
            "go", "py", "sh", "bash", "zsh", "fish", "rs", "c", "cpp", "h", "hpp",
            "java", "kt", "swift", "rb", "php", "sql", "lua", "r", "csv", "log",
            "env", "ini", "conf", "cfg", "dockerfile", "makefile", "gitignore",
        ];
        if (textExts.includes(ext) || file.size < 256 * 1024) {
            const resp = await this.fetchWithAuth(url);
            const text = await resp.text();
            this.fmPreviewBody.innerHTML = `<pre>${this.escapeHtml(text)}</pre>`;
            // 为 HTML 和 MD 文件显示渲染预览按钮
            if (["html", "htm", "md"].includes(ext)) {
                this._fmPreviewRawText = text;
                this._fmPreviewExt = ext;
                this._fmPreviewFilePath = file.path;
                if (this.fmPreviewRender) this.fmPreviewRender.style.display = "";
            }
            return;
        }

        // Unsupported
        this.fmPreviewBody.innerHTML = `<div class="fm-preview-unsupported">
      不支持预览此文件类型 (.${this.escapeHtml(ext)})<br/>
      <button class="fm-action-btn" style="margin-top:12px;" onclick="this.closest('.fm-preview-modal').style.display='none'">关闭</button>
    </div>`;
    } catch (err) {
        console.error("preview error:", err);
        this.fmPreviewBody.innerHTML = '<div class="fm-empty">预览加载失败</div>';
    }
};

FKTeamsChat.prototype._fmClosePreview = function () {
    if (this.fmPreviewModal) {
        this.fmPreviewModal.style.display = "none";
        var content = this.fmPreviewModal.querySelector(".fm-preview-content");
        if (content) content.classList.remove("fm-preview-fullscreen");
    }
    if (this.fmPreviewBody) this.fmPreviewBody.innerHTML = "";
    this._fmPreviewRawText = null;
    this._fmPreviewExt = null;
    this._fmPreviewFilePath = null;
    this._fmPreviewRendered = false;
    if (this.fmPreviewRender) this.fmPreviewRender.style.display = "none";
    if (this.fmPreviewFullscreen) {
        this.fmPreviewFullscreen.title = "全屏预览";
        this.fmPreviewFullscreen.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="15 3 21 3 21 9"/><polyline points="9 21 3 21 3 15"/><line x1="21" y1="3" x2="14" y2="10"/><line x1="3" y1="21" x2="10" y2="14"/></svg>';
    }
};

// 切换源码/渲染视图
FKTeamsChat.prototype._fmToggleRender = function () {
    if (!this._fmPreviewRawText || !this.fmPreviewBody) return;

    this._fmPreviewRendered = !this._fmPreviewRendered;

    if (this._fmPreviewRendered) {
        // 渲染模式
        if (this._fmPreviewExt === "md") {
            // Markdown 渲染
            if (typeof marked !== "undefined") {
                var inst = this._markedInstance || new marked.Marked({ breaks: true, gfm: true });
                this.fmPreviewBody.innerHTML = '<div class="fm-preview-rendered">' + inst.parse(this._fmPreviewRawText) + '</div>';
            } else {
                this.fmPreviewBody.innerHTML = '<div class="fm-preview-rendered">' + this.escapeHtml(this._fmPreviewRawText) + '</div>';
            }
        } else {
            // HTML 渲染（通过后端 serve 端点，支持 CDN 和相对路径资源）
            var iframe = document.createElement("iframe");
            iframe.sandbox = "allow-scripts allow-popups allow-forms";
            iframe.style.cssText = "width:100%;height:60vh;border:none;border-radius:8px;background:#fff;";
            this.fmPreviewBody.innerHTML = "";
            this.fmPreviewBody.appendChild(iframe);
            if (this._fmPreviewFilePath) {
                iframe.src = "/api/fkteams/files/serve/" + encodeURI(this._fmPreviewFilePath);
            } else {
                iframe.contentDocument.open();
                iframe.contentDocument.write(this._fmPreviewRawText);
                iframe.contentDocument.close();
            }
        }
        this.fmPreviewRender.title = "查看源码";
    } else {
        // 源码模式
        this.fmPreviewBody.innerHTML = '<pre>' + this.escapeHtml(this._fmPreviewRawText) + '</pre>';
        this.fmPreviewRender.title = "渲染预览";
    }
};

// 切换全屏预览
FKTeamsChat.prototype._fmToggleFullscreen = function () {
    if (!this.fmPreviewModal) return;
    var content = this.fmPreviewModal.querySelector(".fm-preview-content");
    if (!content) return;

    content.classList.toggle("fm-preview-fullscreen");
    var isFS = content.classList.contains("fm-preview-fullscreen");
    if (this.fmPreviewFullscreen) {
        this.fmPreviewFullscreen.title = isFS ? "退出全屏" : "全屏预览";
        // 切换图标
        this.fmPreviewFullscreen.innerHTML = isFS
            ? '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="4 14 10 14 10 20"/><polyline points="20 10 14 10 14 4"/><line x1="14" y1="10" x2="21" y2="3"/><line x1="3" y1="21" x2="10" y2="14"/></svg>'
            : '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="15 3 21 3 21 9"/><polyline points="9 21 3 21 3 15"/><line x1="21" y1="3" x2="14" y2="10"/><line x1="3" y1="21" x2="10" y2="14"/></svg>';
    }
};

// ===== 工具方法 =====

FKTeamsChat.prototype._fmFormatSize = function (bytes) {
    if (bytes === 0) return "0 B";
    const units = ["B", "KB", "MB", "GB"];
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    const val = (bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0);
    return val + " " + units[i];
};

FKTeamsChat.prototype._fmFormatTime = function (ts) {
    if (!ts) return "";
    const d = new Date(ts * 1000);
    const pad = (n) => String(n).padStart(2, "0");
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
};

// ===== 确认弹窗 =====

FKTeamsChat.prototype._fmShowConfirm = function (msg, warn, onConfirm) {
    if (!this.fmConfirmModal) {
        // Fallback if modal elements missing
        if (confirm(msg + "\n" + warn)) onConfirm();
        return;
    }
    if (this.fmConfirmMsg) this.fmConfirmMsg.innerHTML = msg;
    if (this.fmConfirmWarn) this.fmConfirmWarn.textContent = warn;
    this.fmConfirmModal.style.display = "flex";

    // Store callback, remove previous listener to avoid stacking
    if (this._fmConfirmHandler) {
        this.fmConfirmOk.removeEventListener("click", this._fmConfirmHandler);
    }
    this._fmConfirmHandler = () => {
        this._fmCloseConfirm();
        onConfirm();
    };
    this.fmConfirmOk.addEventListener("click", this._fmConfirmHandler);
};

FKTeamsChat.prototype._fmCloseConfirm = function () {
    if (this.fmConfirmModal) this.fmConfirmModal.style.display = "none";
    if (this._fmConfirmHandler && this.fmConfirmOk) {
        this.fmConfirmOk.removeEventListener("click", this._fmConfirmHandler);
        this._fmConfirmHandler = null;
    }
};

// ===== 分享功能 =====

FKTeamsChat.prototype._fmShowShareDialog = function (fileOrFiles) {
    if (!this.fmShareModal) return;
    // Support single file or array of files
    const files = Array.isArray(fileOrFiles) ? fileOrFiles : [fileOrFiles];
    this._fmShareFilePaths = files.map((f) => f.path);
    const displayName = files.length === 1
        ? files[0].name
        : `${files.length} 个文件/文件夹`;
    if (this.fmShareFilename) this.fmShareFilename.textContent = displayName;
    if (this.fmSharePassword) this.fmSharePassword.value = "";
    if (this.fmShareExpiry) this.fmShareExpiry.value = "86400";
    if (this.fmShareResult) this.fmShareResult.style.display = "none";
    if (this.fmShareOk) this.fmShareOk.style.display = "";
    this.fmShareModal.style.display = "flex";
};

FKTeamsChat.prototype._fmCloseShareDialog = function () {
    if (this.fmShareModal) this.fmShareModal.style.display = "none";
    this._fmShareFilePaths = null;
};

FKTeamsChat.prototype._fmCreateShareLink = async function () {
    if (!this._fmShareFilePaths || this._fmShareFilePaths.length === 0) return;
    const btn = this.fmShareOk;
    if (btn) {
        btn.disabled = true;
        btn.textContent = "创建中...";
    }
    try {
        const body = {
            expires_in: parseInt(this.fmShareExpiry ? this.fmShareExpiry.value : "86400", 10),
        };
        // Use file_paths for multi-file, file_path for single
        if (this._fmShareFilePaths.length === 1) {
            body.file_path = this._fmShareFilePaths[0];
        } else {
            body.file_paths = this._fmShareFilePaths;
        }
        const pwd = this.fmSharePassword ? this.fmSharePassword.value.trim() : "";
        if (pwd) body.password = pwd;

        const resp = await this.fetchWithAuth("/api/fkteams/preview", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(body),
        });
        const data = await resp.json();
        if (data.code !== 0) {
            this.showNotification(data.message || "创建分享失败", "error");
            return;
        }
        // Show result
        const linkUrl = `${location.origin}/p/${data.data.id}`;
        if (this.fmShareLink) this.fmShareLink.value = linkUrl;
        if (this.fmShareResult) this.fmShareResult.style.display = "";
        if (btn) btn.style.display = "none";
    } catch (err) {
        console.error("share error:", err);
        this.showNotification("创建分享链接失败", "error");
    } finally {
        if (btn) {
            btn.disabled = false;
            btn.textContent = "创建分享";
        }
    }
};

FKTeamsChat.prototype._fmCopyToClipboard = function (text, btn) {
    const showSuccess = () => {
        if (!btn) return;
        btn.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg>';
        setTimeout(() => {
            btn.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>';
        }, 1500);
    };
    if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(showSuccess).catch(() => {
            this._fmFallbackCopy(text);
            showSuccess();
        });
    } else {
        this._fmFallbackCopy(text);
        showSuccess();
    }
};

FKTeamsChat.prototype._fmFallbackCopy = function (text) {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.cssText = "position:fixed;left:-9999px;top:-9999px;opacity:0";
    document.body.appendChild(ta);
    ta.select();
    try { document.execCommand("copy"); } catch (_) { /* ignore */ }
    document.body.removeChild(ta);
};

FKTeamsChat.prototype._fmCopyShareLink = function () {
    const link = this.fmShareLink ? this.fmShareLink.value : "";
    if (!link) return;
    this._fmCopyToClipboard(link, this.fmShareCopyBtn);
};

FKTeamsChat.prototype._fmShowShareList = async function () {
    if (!this.fmShareListModal || !this.fmShareList) return;
    this.fmShareListModal.style.display = "flex";
    this.fmShareList.innerHTML = '<div class="fm-empty">加载中...</div>';

    try {
        const resp = await this.fetchWithAuth("/api/fkteams/preview");
        const data = await resp.json();
        if (data.code !== 0) {
            this.fmShareList.innerHTML = '<div class="fm-empty">加载失败</div>';
            return;
        }
        const links = data.data || [];
        if (links.length === 0) {
            this.fmShareList.innerHTML = '<div class="fm-empty">暂无分享链接</div>';
            return;
        }
        this.fmShareList.innerHTML = "";
        links.forEach((link) => {
            const el = document.createElement("div");
            el.className = "fm-share-list-item";
            // expires_at === 0 表示永不过期
            const isNeverExpire = !link.expires_at;
            const expiresDate = isNeverExpire ? "永不过期" : this._fmFormatTime(link.expires_at);
            const linkUrl = `${location.origin}/p/${link.id}`;
            // 构建显示名称
            let nameHtml = "";
            const paths = link.file_paths || [];
            if (paths.length > 1) {
                // 批量分享：显示文件数和各文件名
                const fileNames = paths.map(p => p.split("/").pop());
                nameHtml = `<div class="fm-share-list-name">${this.escapeHtml(paths.length + " 个文件/文件夹")}</div>`;
                nameHtml += `<div class="fm-share-list-files">${fileNames.map(n => `<span class="fm-share-file-tag">${this.escapeHtml(n)}</span>`).join("")}</div>`;
            } else {
                nameHtml = `<div class="fm-share-list-name">${this.escapeHtml(paths[0] || link.file_path)}</div>`;
            }
            el.innerHTML = `
                <div class="fm-share-list-info">
                    ${nameHtml}
                    <div class="fm-share-list-meta">过期时间: ${expiresDate}</div>
                </div>
                <div class="fm-share-list-actions">
                    <button class="fm-item-action-btn" title="复制链接" data-link="${this.escapeHtml(linkUrl)}">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
                    </button>
                    <button class="fm-item-action-btn delete-action" title="删除" data-id="${this.escapeHtml(link.id)}">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
                    </button>
                </div>`;
            // Copy button
            const copyBtn = el.querySelector("[data-link]");
            if (copyBtn) {
                copyBtn.addEventListener("click", () => {
                    this._fmCopyToClipboard(copyBtn.dataset.link, copyBtn);
                });
            }
            // Delete button
            const delBtn = el.querySelector("[data-id]");
            if (delBtn) {
                delBtn.addEventListener("click", async () => {
                    try {
                        await this.fetchWithAuth(`/api/fkteams/preview/${encodeURIComponent(delBtn.dataset.id)}`, {
                            method: "DELETE",
                        });
                        el.remove();
                        // Check if list is now empty
                        if (this.fmShareList && this.fmShareList.children.length === 0) {
                            this.fmShareList.innerHTML = '<div class="fm-empty">暂无分享链接</div>';
                        }
                    } catch (err) {
                        console.error("delete share error:", err);
                    }
                });
            }
            this.fmShareList.appendChild(el);
        });
    } catch (err) {
        console.error("load shares error:", err);
        this.fmShareList.innerHTML = '<div class="fm-empty">加载失败</div>';
    }
};

FKTeamsChat.prototype._fmCloseShareList = function () {
    if (this.fmShareListModal) this.fmShareListModal.style.display = "none";
};
