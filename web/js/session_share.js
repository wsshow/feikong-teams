/**
 * session_share.js - 会话分享与分享管理
 */

FKTeamsChat.prototype.initSessionShare = function () {
  this.sessionShareManageBtn = document.getElementById("session-share-manage-btn");
  this.sessionShareModal = document.getElementById("session-share-modal");
  this.sessionShareClose = document.getElementById("session-share-close");
  this.sessionShareCancel = document.getElementById("session-share-cancel");
  this.sessionShareOk = document.getElementById("session-share-ok");
  this.sessionShareTitle = document.getElementById("session-share-title");
  this.sessionSharePassword = document.getElementById("session-share-password");
  this.sessionShareExpiry = document.getElementById("session-share-expiry");
  this.sessionShareToolDetails = document.getElementById("session-share-tool-details");
  this.sessionShareResult = document.getElementById("session-share-result");
  this.sessionShareLink = document.getElementById("session-share-link");
  this.sessionShareCopy = document.getElementById("session-share-copy");
  this.sessionShareLinksBtn = document.getElementById("session-share-links-btn");
  this.sessionShareListModal = document.getElementById("session-share-list-modal");
  this.sessionShareListClose = document.getElementById("session-share-list-close");
  this.sessionShareList = document.getElementById("session-share-list");

  if (this.sessionShareManageBtn) {
    this.sessionShareManageBtn.addEventListener("click", () => this.showSessionShareList());
  }
  if (this.sessionShareClose) this.sessionShareClose.addEventListener("click", () => this.hideSessionShareModal());
  if (this.sessionShareCancel) this.sessionShareCancel.addEventListener("click", () => this.hideSessionShareModal());
  if (this.sessionShareOk) this.sessionShareOk.addEventListener("click", () => this.createSessionShare());
  if (this.sessionShareCopy) this.sessionShareCopy.addEventListener("click", () => this.copySessionShareLink());
  if (this.sessionShareLinksBtn) this.sessionShareLinksBtn.addEventListener("click", () => this.showSessionShareList());
  if (this.sessionShareListClose) this.sessionShareListClose.addEventListener("click", () => this.hideSessionShareList());

  if (this.sessionShareModal) {
    this.sessionShareModal.addEventListener("click", (e) => {
      if (e.target === this.sessionShareModal) this.hideSessionShareModal();
    });
  }
  if (this.sessionShareListModal) {
    this.sessionShareListModal.addEventListener("click", (e) => {
      if (e.target === this.sessionShareListModal) this.hideSessionShareList();
    });
  }
};

FKTeamsChat.prototype.showSessionShareModal = function (sessionId, title) {
  if (!sessionId || !this.sessionShareModal) return;
  this._sessionShareTarget = { sessionId, title: title || sessionId };
  if (this.sessionShareTitle) this.sessionShareTitle.textContent = title || sessionId;
  if (this.sessionSharePassword) this.sessionSharePassword.value = "";
  if (this.sessionShareExpiry) this.sessionShareExpiry.value = "604800";
  if (this.sessionShareToolDetails) this.sessionShareToolDetails.checked = false;
  if (this.sessionShareResult) this.sessionShareResult.style.display = "none";
  if (this.sessionShareLink) this.sessionShareLink.value = "";
  if (this.sessionShareOk) {
    this.sessionShareOk.disabled = false;
    this.sessionShareOk.textContent = "创建分享";
    this.sessionShareOk.style.display = "";
  }
  this.sessionShareModal.style.display = "flex";
  setTimeout(() => this.sessionSharePassword?.focus(), 50);
};

FKTeamsChat.prototype.hideSessionShareModal = function () {
  if (this.sessionShareModal) this.sessionShareModal.style.display = "none";
};

FKTeamsChat.prototype.createSessionShare = async function () {
  const target = this._sessionShareTarget;
  if (!target || !target.sessionId) return;
  const btn = this.sessionShareOk;
  if (btn) {
    btn.disabled = true;
    btn.textContent = "创建中...";
  }
  try {
    const body = {
      session_id: target.sessionId,
      expires_in: parseInt(this.sessionShareExpiry?.value || "604800", 10),
      allow_tool_details: !!this.sessionShareToolDetails?.checked,
    };
    const password = (this.sessionSharePassword?.value || "").trim();
    if (password) body.password = password;

    const resp = await this.fetchWithAuth("/api/fkteams/session-shares", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    const data = await resp.json();
    if (data.code !== 0) {
      this.showNotification(this.sessionShareErrorText(data.message), "error");
      return;
    }
    const linkURL = `${location.origin}/s/${data.data.id}`;
    if (this.sessionShareLink) this.sessionShareLink.value = linkURL;
    if (this.sessionShareResult) this.sessionShareResult.style.display = "";
    if (btn) btn.style.display = "none";
    this.showNotification("分享链接已创建", "success");
  } catch (err) {
    console.error("create session share error:", err);
    this.showNotification("创建会话分享失败", "error");
  } finally {
    if (btn) {
      btn.disabled = false;
      if (btn.style.display !== "none") btn.textContent = "创建分享";
    }
  }
};

FKTeamsChat.prototype.sessionShareErrorText = function (message) {
  if (message === "session has no shareable messages") return "该会话暂无可分享内容";
  if (message === "session history not found") return "会话历史不存在";
  if (message === "invalid session ID") return "会话 ID 无效";
  return message || "操作失败";
};

FKTeamsChat.prototype.copySessionShareLink = function () {
  const link = this.sessionShareLink?.value || "";
  if (!link) return;
  this.copyTextToClipboard(link, this.sessionShareCopy);
};

FKTeamsChat.prototype.copyTextToClipboard = function (text, btn) {
  const done = () => {
    if (!btn) return;
    const oldHTML = btn.innerHTML;
    btn.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg>';
    setTimeout(() => {
      btn.innerHTML = oldHTML;
    }, 1400);
  };
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).then(done).catch(() => {
      this.fallbackCopyText(text);
      done();
    });
    return;
  }
  this.fallbackCopyText(text);
  done();
};

FKTeamsChat.prototype.fallbackCopyText = function (text) {
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.style.cssText = "position:fixed;left:-9999px;top:-9999px;opacity:0";
  document.body.appendChild(ta);
  ta.select();
  try {
    document.execCommand("copy");
  } catch (_) {
    /* ignore */
  }
  document.body.removeChild(ta);
};

FKTeamsChat.prototype.showSessionShareList = async function () {
  if (!this.sessionShareListModal || !this.sessionShareList) return;
  this.sessionShareListModal.style.display = "flex";
  this.sessionShareList.innerHTML = '<div class="session-share-empty">加载中...</div>';

  try {
    const resp = await this.fetchWithAuth("/api/fkteams/session-shares");
    const data = await resp.json();
    if (data.code !== 0) {
      this.sessionShareList.innerHTML = '<div class="session-share-empty">加载失败</div>';
      return;
    }
    const shares = data.data || [];
    if (shares.length === 0) {
      this.sessionShareList.innerHTML = '<div class="session-share-empty">暂无会话分享</div>';
      return;
    }
    this.sessionShareList.innerHTML = "";
    shares.forEach((share) => this.renderSessionShareListItem(share));
  } catch (err) {
    console.error("load session shares error:", err);
    this.sessionShareList.innerHTML = '<div class="session-share-empty">加载失败</div>';
  }
};

FKTeamsChat.prototype.hideSessionShareList = function () {
  if (this.sessionShareListModal) this.sessionShareListModal.style.display = "none";
};

FKTeamsChat.prototype.renderSessionShareListItem = function (share) {
  const el = document.createElement("div");
  el.className = "session-share-list-item";
  const linkURL = `${location.origin}/s/${share.id}`;
  const expiresText = share.expires_at ? this.formatUnixTime(share.expires_at) : "永不过期";
  const badges = [
    share.has_password ? "有密码" : "无密码",
    share.allow_tool_details ? "含工具详情" : "仅对话",
    `${share.message_count || 0} 条消息`,
  ];
  el.innerHTML = `
    <div class="session-share-list-info">
      <div class="session-share-list-name">${this.escapeHtml(share.title || share.session_id || share.id)}</div>
      <div class="session-share-list-meta">
        <span>过期时间: ${this.escapeHtml(expiresText)}</span>
        <span>创建时间: ${this.escapeHtml(this.formatUnixTime(share.created_at))}</span>
      </div>
      <div class="session-share-list-badges">
        ${badges.map((badge) => `<span>${this.escapeHtml(badge)}</span>`).join("")}
      </div>
    </div>
    <div class="session-share-list-actions">
      <button class="session-share-icon-btn" title="复制链接" data-link="${this.escapeHtml(linkURL)}">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <rect x="9" y="9" width="13" height="13" rx="2" ry="2"/>
          <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
        </svg>
      </button>
      <button class="session-share-icon-btn delete-action" title="删除分享" data-id="${this.escapeHtml(share.id)}">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <polyline points="3 6 5 6 21 6"/>
          <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
        </svg>
      </button>
    </div>
  `;
  el.querySelector("[data-link]")?.addEventListener("click", (e) => {
    this.copyTextToClipboard(e.currentTarget.dataset.link, e.currentTarget);
  });
  el.querySelector("[data-id]")?.addEventListener("click", async (e) => {
    const id = e.currentTarget.dataset.id;
    try {
      const resp = await this.fetchWithAuth(`/api/fkteams/session-shares/${encodeURIComponent(id)}`, {
        method: "DELETE",
      });
      const data = await resp.json();
      if (data.code !== 0) {
        this.showNotification(data.message || "删除分享失败", "error");
        return;
      }
      el.remove();
      if (this.sessionShareList && this.sessionShareList.children.length === 0) {
        this.sessionShareList.innerHTML = '<div class="session-share-empty">暂无会话分享</div>';
      }
      this.showNotification("分享已删除", "success");
    } catch (err) {
      console.error("delete session share error:", err);
      this.showNotification("删除分享失败", "error");
    }
  });
  this.sessionShareList.appendChild(el);
};

FKTeamsChat.prototype.formatUnixTime = function (value) {
  const unix = Number(value);
  if (!Number.isFinite(unix) || unix <= 0) return "";
  return new Date(unix * 1000).toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
};
