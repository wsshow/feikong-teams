(function () {
  const state = {
    shareID: decodeURIComponent(location.pathname.split("/").filter(Boolean).pop() || ""),
    info: null,
  };

  const els = {
    title: document.getElementById("share-title"),
    meta: document.getElementById("share-meta"),
    content: document.getElementById("share-content"),
    passwordCard: document.getElementById("share-password-card"),
    passwordInput: document.getElementById("share-password-input"),
    passwordSubmit: document.getElementById("share-password-submit"),
    passwordError: document.getElementById("share-password-error"),
  };

  function escapeHtml(text) {
    const div = document.createElement("div");
    div.textContent = text || "";
    return div.innerHTML;
  }

  function renderMarkdown(text) {
    if (window.marked && typeof window.marked.parse === "function") {
      return window.marked.parse(text || "");
    }
    return escapeHtml(text || "").replace(/\n/g, "<br>");
  }

  function formatUnixTime(value) {
    const unix = Number(value);
    if (!Number.isFinite(unix) || unix <= 0) return "";
    return new Date(unix * 1000).toLocaleString("zh-CN", {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    });
  }

  function setError(message) {
    if (!els.content) return;
    els.passwordCard.style.display = "none";
    els.content.style.display = "";
    els.content.innerHTML = `<div class="share-empty">${escapeHtml(message)}</div>`;
  }

  function renderMeta(data) {
    const items = [];
    if (data.message_count !== undefined) items.push(`${data.message_count || 0} 条消息`);
    items.push(data.expires_at ? `有效期至 ${formatUnixTime(data.expires_at)}` : "永不过期");
    if (data.has_password !== undefined) items.push(data.has_password ? "需要密码" : "无需密码");
    items.push(data.allow_tool_details ? "包含工具详情" : "仅对话内容");
    els.meta.innerHTML = items.map((item) => `<span>${escapeHtml(item)}</span>`).join("");
  }

  async function loadInfo() {
    if (!state.shareID) {
      setError("分享链接无效");
      return;
    }
    try {
      const resp = await fetch(`/api/fkteams/public/session-shares/${encodeURIComponent(state.shareID)}/info`);
      const data = await resp.json();
      if (data.code !== 0) {
        setError(data.message === "share expired" ? "分享链接已过期" : "分享链接不存在或已失效");
        return;
      }
      state.info = data.data;
      els.title.textContent = state.info.title || "会话分享";
      renderMeta(state.info);
      if (state.info.has_password) {
        els.content.style.display = "none";
        els.passwordCard.style.display = "";
        setTimeout(() => els.passwordInput?.focus(), 50);
        return;
      }
      accessShare("");
    } catch (err) {
      console.error("load share info error:", err);
      setError("加载分享信息失败");
    }
  }

  async function accessShare(password) {
    if (els.passwordSubmit) els.passwordSubmit.disabled = true;
    if (els.passwordError) els.passwordError.textContent = "";
    try {
      const resp = await fetch(`/api/fkteams/public/session-shares/${encodeURIComponent(state.shareID)}/access`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password: password || "" }),
      });
      const data = await resp.json();
      if (data.code !== 0) {
        if (resp.status === 401) {
          els.passwordError.textContent = "密码不正确";
          return;
        }
        setError(data.message === "share expired" ? "分享链接已过期" : "分享内容不可访问");
        return;
      }
      els.passwordCard.style.display = "none";
      els.content.style.display = "";
      els.title.textContent = data.data.title || state.info?.title || "会话分享";
      renderMeta({ ...(state.info || {}), ...data.data });
      renderMessages(data.data.messages || []);
    } catch (err) {
      console.error("access share error:", err);
      setError("加载分享内容失败");
    } finally {
      if (els.passwordSubmit) els.passwordSubmit.disabled = false;
    }
  }

  function renderMessages(messages) {
    if (!messages.length) {
      els.content.innerHTML = '<div class="share-empty">这个分享暂无会话内容</div>';
      return;
    }
    els.content.innerHTML = messages.map(renderMessage).join("");
  }

  function renderMessage(msg) {
    const agent = msg.member_name || msg.agent_name || "成员";
    const time = msg.start_time ? new Date(msg.start_time).toLocaleString("zh-CN") : "";
    const events = (msg.events || []).map(renderEvent).join("");
    return `
      <article class="share-message">
        <div class="share-message-head">
          <span class="share-agent">${escapeHtml(agent)}</span>
          <span class="share-time">${escapeHtml(time)}</span>
        </div>
        ${events || '<div class="share-event">无内容</div>'}
      </article>
    `;
  }

  function renderEvent(event) {
    if (!event) return "";
    if (event.type === "text") {
      return `<div class="share-event markdown-body">${renderMarkdown(event.content || "")}</div>`;
    }
    if (event.type === "reasoning") {
      return `<div class="share-event reasoning">${escapeHtml(event.content || "")}</div>`;
    }
    if (event.type === "tool_call" && event.tool_call) {
      const tool = event.tool_call;
      const name = tool.display_name || tool.name || "工具调用";
      const args = tool.arguments ? `<pre>${escapeHtml(tool.arguments)}</pre>` : "";
      const result = tool.result ? `<pre>${escapeHtml(tool.result)}</pre>` : "";
      return `<div class="share-event tool"><strong>${escapeHtml(name)}</strong>${args}${result}</div>`;
    }
    if (event.type === "action" && event.action) {
      const action = event.action.action_type ? `[${event.action.action_type}] ` : "";
      return `<div class="share-event action">${escapeHtml(action + (event.action.content || ""))}</div>`;
    }
    if (event.type === "error") {
      return `<div class="share-event error">${escapeHtml(event.content || "执行失败")}</div>`;
    }
    return "";
  }

  if (els.passwordSubmit) {
    els.passwordSubmit.addEventListener("click", () => accessShare(els.passwordInput.value));
  }
  if (els.passwordInput) {
    els.passwordInput.addEventListener("keydown", (e) => {
      if (e.key === "Enter") accessShare(els.passwordInput.value);
    });
  }

  loadInfo();
})();
