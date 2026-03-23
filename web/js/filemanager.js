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
    this.fmUploadBar = document.getElementById("fm-upload-bar");
    this.fmUploadText = document.getElementById("fm-upload-text");
    this.fmUploadProgressFill = document.getElementById("fm-upload-progress-fill");
    this.fmUploadCancel = document.getElementById("fm-upload-cancel");
    this.fmPreviewModal = document.getElementById("fm-preview-modal");
    this.fmPreviewClose = document.getElementById("fm-preview-close");
    this.fmPreviewTitle = document.getElementById("fm-preview-title");
    this.fmPreviewBody = document.getElementById("fm-preview-body");
    // Confirm modal elements
    this.fmConfirmModal = document.getElementById("fm-confirm-modal");
    this.fmConfirmClose = document.getElementById("fm-confirm-close");
    this.fmConfirmMsg = document.getElementById("fm-confirm-msg");
    this.fmConfirmWarn = document.getElementById("fm-confirm-warn");
    this.fmConfirmCancel = document.getElementById("fm-confirm-cancel");
    this.fmConfirmOk = document.getElementById("fm-confirm-ok");
    // File/folder upload inputs (hidden)
    this.fmFileInput = document.getElementById("fm-file-input");
    this.fmFolderInput = document.getElementById("fm-folder-input");

    // State
    this.fmCurrentPath = "";
    this.fmFiles = [];
    this.fmSelected = new Set();
    this.fmUploadAbort = null;

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
    if (this.fmModal) {
        this.fmModal.addEventListener("click", (e) => {
            if (e.target === this.fmModal) this.closeFileManager();
        });
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
    if (this.fmPreviewModal) {
        this.fmPreviewModal.addEventListener("click", (e) => {
            if (e.target === this.fmPreviewModal) this._fmClosePreview();
        });
    }

    // Confirm modal events
    if (this.fmConfirmClose) {
        this.fmConfirmClose.addEventListener("click", () => this._fmCloseConfirm());
    }
    if (this.fmConfirmCancel) {
        this.fmConfirmCancel.addEventListener("click", () => this._fmCloseConfirm());
    }
    if (this.fmConfirmModal) {
        this.fmConfirmModal.addEventListener("click", (e) => {
            if (e.target === this.fmConfirmModal) this._fmCloseConfirm();
        });
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
            if (this.fmConfirmModal && this.fmConfirmModal.style.display === "flex") {
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
        ${!file.is_dir ? `<button class="fm-item-action-btn download-action" title="下载">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
        </button>` : ""}
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
    // Only download files, not directories
    const filesToDownload = selected.filter((f) => !f.is_dir);
    filesToDownload.forEach((f) => this._fmDownloadFile(f.path));
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
    if (this.fmPreviewModal) this.fmPreviewModal.style.display = "none";
    if (this.fmPreviewBody) this.fmPreviewBody.innerHTML = "";
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
