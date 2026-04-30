// config.js - 系统配置页面逻辑

FKTeamsChat.prototype.initConfig = function () {
  this.configModal = document.getElementById("config-modal");
  this.configOpenBtn = document.getElementById("config-open-btn");
  this.configCloseBtn = document.getElementById("config-modal-close");
  this.configSaveBtn = document.getElementById("config-save-btn");
  this.configCancelBtn = document.getElementById("config-cancel-btn");
  this.configTabs = document.getElementById("config-tabs");
  this.configModelList = document.getElementById("config-model-list");
  this.agentCardsGrid = document.getElementById("agent-cards-grid");

  this._configData = null;
  this._toolNames = [];
  this._modelCache = {}; // provider+baseUrl+apiKey → [{id}]

  // 打开配置
  this.configOpenBtn.addEventListener("click", () => this.openConfig());
  this.configCloseBtn.addEventListener("click", () => this.closeConfig());
  this.configCancelBtn.addEventListener("click", () => this.closeConfig());
  this.configSaveBtn.addEventListener("click", () => this.saveConfig());

  // Tab 切换
  this.configTabs.addEventListener("click", (e) => {
    const btn = e.target.closest(".config-tab-btn");
    if (!btn) return;
    this.switchConfigTab(btn.dataset.tab);
  });

  // 添加模型
  document.getElementById("config-add-model").addEventListener("click", () => {
    this.addModelCard(
      { name: "", provider: "openai", base_url: "", api_key: "", model: "" },
      true,
    );
  });

  // Auth 开关联动
  document
    .getElementById("config-auth-enabled")
    .addEventListener("change", (e) => {
      document.getElementById("config-auth-fields").style.display = e.target
        .checked
        ? "block"
        : "none";
    });

  // SSH 开关联动
  document
    .getElementById("config-ssh-enabled")
    .addEventListener("change", (e) => {
      document.getElementById("config-ssh-fields").style.display = e.target
        .checked
        ? "block"
        : "none";
    });



  // 初始化静态下拉选择器为自定义组件
  initAllCustomSelects(this.configModal);
};

FKTeamsChat.prototype.openConfig = async function () {
  this.configModal.style.display = "flex";
  this.configSaveBtn.disabled = true;
  try {
    await Promise.all([this.loadConfig(), this.loadToolNames()]);
    this.configSaveBtn.disabled = false;
  } catch (err) {
    console.error("load config failed:", err);
  }
};

FKTeamsChat.prototype.closeConfig = function () {
  this.configModal.style.display = "none";
};

FKTeamsChat.prototype.switchConfigTab = function (tab) {
  this.configTabs.querySelectorAll(".config-tab-btn").forEach((btn) => {
    btn.classList.toggle("active", btn.dataset.tab === tab);
  });
  document.querySelectorAll(".config-panel").forEach((panel) => {
    panel.classList.toggle("active", panel.id === "config-panel-" + tab);
  });
  if (tab === "skills") {
    this._initSkillsTab();
  } else if (tab === "mcp") {
    this._initMcpTab();
  }
};

// ===== 加载配置 =====

FKTeamsChat.prototype.loadConfig = async function () {
  const resp = await this.fetchWithAuth("/api/fkteams/config");
  const result = await resp.json();
  if (result.code !== 0) throw new Error(result.message);
  this._configData = result.data;
  this.fillConfigForm(this._configData);
};

FKTeamsChat.prototype.loadToolNames = async function () {
  const resp = await this.fetchWithAuth("/api/fkteams/config/tools");
  const result = await resp.json();
  if (result.code !== 0) throw new Error(result.message);
  this._toolNames = result.data || [];
};

// ===== 填充表单 =====

FKTeamsChat.prototype.fillConfigForm = function (cfg) {
  // 模型
  this.configModelList.innerHTML = "";
  (cfg.models || []).forEach((m) => this.addModelCard(m));

  // 通用
  document.getElementById("config-memory-enabled").checked =
    cfg.memory?.enabled || false;
  document.getElementById("config-server-host").value = cfg.server?.host || "";
  document.getElementById("config-server-port").value = cfg.server?.port || "";
  document.getElementById("config-server-loglevel").value =
    cfg.server?.log_level || "info";

  // Auth
  const authEnabled = cfg.server?.auth?.enabled || false;
  document.getElementById("config-auth-enabled").checked = authEnabled;
  document.getElementById("config-auth-fields").style.display = authEnabled
    ? "block"
    : "none";
  document.getElementById("config-auth-username").value =
    cfg.server?.auth?.username || "";
  document.getElementById("config-auth-password").value =
    cfg.server?.auth?.password || "";
  document.getElementById("config-auth-secret").value =
    cfg.server?.auth?.secret || "";

  // 智能体
  document.getElementById("config-agent-searcher").checked =
    cfg.agents?.searcher || false;
  document.getElementById("config-agent-assistant").checked =
    cfg.agents?.assistant || false;
  document.getElementById("config-agent-analyst").checked =
    cfg.agents?.analyst || false;

  // SSH
  const sshEnabled = cfg.agents?.ssh_visitor?.enabled || false;
  document.getElementById("config-ssh-enabled").checked = sshEnabled;
  document.getElementById("config-ssh-fields").style.display = sshEnabled
    ? "block"
    : "none";
  document.getElementById("config-ssh-host").value =
    cfg.agents?.ssh_visitor?.host || "";
  document.getElementById("config-ssh-username").value =
    cfg.agents?.ssh_visitor?.username || "";
  document.getElementById("config-ssh-password").value =
    cfg.agents?.ssh_visitor?.password || "";

  // 自定义智能体
  this.renderAgentCards(cfg.custom?.agents || []);

  // 通道
  this.fillChannelForm(cfg.channels || {});
};

FKTeamsChat.prototype.fillChannelForm = function (ch) {
  // 动态填充智能体名称到通道模式选择框
  const builtinModes = new Set(["team", "deep", "roundtable", "custom"]);
  document.querySelectorAll(".channel-mode-select").forEach((sel) => {
    // 移除之前动态添加的选项
    sel.querySelectorAll("[data-agent]").forEach((el) => el.remove());

    // 添加系统内置智能体
    const addedNames = new Set();
    (this.agents || []).forEach((a) => {
      const name = a.name || a;
      if (name && !builtinModes.has(name) && !addedNames.has(name)) {
        addedNames.add(name);
        const opt = document.createElement("option");
        opt.value = name;
        opt.textContent = name;
        opt.setAttribute("data-agent", "true");
        sel.appendChild(opt);
      }
    });

    // 添加自定义智能体名称（去重）
    (this._configData?.custom?.agents || []).forEach((a) => {
      if (a.name && !builtinModes.has(a.name) && !addedNames.has(a.name)) {
        addedNames.add(a.name);
        const opt = document.createElement("option");
        opt.value = a.name;
        opt.textContent = a.name;
        opt.setAttribute("data-agent", "true");
        sel.appendChild(opt);
      }
    });

    // 更新自定义组件
    if (sel._customSelect) sel._customSelect.rebuild();
  });

  // QQ
  document.getElementById("config-qq-enabled").checked =
    ch.qq?.enabled || false;
  document.getElementById("config-qq-appid").value = ch.qq?.app_id || "";
  document.getElementById("config-qq-appsecret").value =
    ch.qq?.app_secret || "";
  document.getElementById("config-qq-mode").value = ch.qq?.mode || "team";
  document.getElementById("config-qq-sandbox").checked =
    ch.qq?.sandbox || false;

  // Discord
  document.getElementById("config-discord-enabled").checked =
    ch.discord?.enabled || false;
  document.getElementById("config-discord-token").value =
    ch.discord?.token || "";
  document.getElementById("config-discord-allowfrom").value =
    ch.discord?.allow_from || "";
  document.getElementById("config-discord-mode").value =
    ch.discord?.mode || "team";

  // 微信
  document.getElementById("config-weixin-enabled").checked =
    ch.weixin?.enabled || false;
  document.getElementById("config-weixin-baseurl").value =
    ch.weixin?.base_url || "";
  document.getElementById("config-weixin-credpath").value =
    ch.weixin?.cred_path || "";
  document.getElementById("config-weixin-allowfrom").value =
    ch.weixin?.allow_from || "";
  document.getElementById("config-weixin-mode").value =
    ch.weixin?.mode || "team";
  document.getElementById("config-weixin-loglevel").value =
    ch.weixin?.log_level || "info";
};

// ===== 模型卡片 =====

FKTeamsChat.prototype.addModelCard = function (m, expanded) {
  const card = document.createElement("div");
  card.className = "config-model-card" + (expanded ? " open" : "");
  card.dataset.apiLookupName = m.name || "";

  const displayName = m.name || "未命名模型";
  const displayModel = m.model || "";
  const displayProvider = m.provider || "openai";
  const apiKeyPlaceholder = m.has_api_key ? "已配置，留空则不修改" : "sk-...";

  const isDefault = m.name === "default";

  card.innerHTML = `
    <div class="config-model-header">
      <div class="config-model-summary">
        <span class="config-model-title">${this.escapeHtml(displayName)}</span>
        <span class="config-model-info">${this.escapeHtml(displayProvider)}${displayModel ? " / " + this.escapeHtml(displayModel) : ""}</span>
        ${isDefault ? '<span class="config-model-default-badge">默认</span>' : ""}
      </div>
      <div class="config-model-header-actions">
        <button class="config-model-set-default" title="${isDefault ? "当前默认模型" : "设为默认模型"}" ${isDefault ? "disabled" : ""}>
          <svg viewBox="0 0 24 24" fill="${isDefault ? "var(--accent-color)" : "none"}" stroke="${isDefault ? "var(--accent-color)" : "currentColor"}" stroke-width="2">
            <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
          </svg>
        </button>
        <button class="config-model-remove" title="删除此模型">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
          </svg>
        </button>
        <svg class="config-model-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <polyline points="6 9 12 15 18 9" />
        </svg>
      </div>
    </div>
    <div class="config-model-body">
      <div class="config-row">
        <div class="config-field">
          <label>名称</label>
          <input type="text" class="config-input model-name" value="${this.escapeHtml(m.name || "")}" placeholder="default" />
        </div>
        <div class="config-field">
          <label>服务商</label>
          <select class="config-select model-provider">
            <option value="openai" ${m.provider === "openai" ? "selected" : ""}>OpenAI</option>
            <option value="claude" ${m.provider === "claude" ? "selected" : ""}>Claude</option>
            <option value="deepseek" ${m.provider === "deepseek" ? "selected" : ""}>DeepSeek</option>
            <option value="gemini" ${m.provider === "gemini" ? "selected" : ""}>Gemini</option>
            <option value="qwen" ${m.provider === "qwen" ? "selected" : ""}>通义千问</option>
            <option value="ollama" ${m.provider === "ollama" ? "selected" : ""}>Ollama</option>
            <option value="openrouter" ${m.provider === "openrouter" ? "selected" : ""}>OpenRouter</option>
            <option value="ark" ${m.provider === "ark" ? "selected" : ""}>火山方舟</option>
            <option value="copilot" ${m.provider === "copilot" ? "selected" : ""}>GitHub Copilot</option>
          </select>
        </div>
      </div>
      <div class="config-row">
        <div class="config-field">
          <label>接口地址</label>
          <input type="text" class="config-input model-baseurl" value="${this.escapeHtml(m.base_url || "")}" placeholder="https://api.openai.com/v1" />
        </div>
        <div class="config-field">
          <label>模型</label>
          <div class="config-input-group">
            <input type="text" class="config-input model-model" value="${this.escapeHtml(m.model || "")}" placeholder="gpt-4o" />
            <button class="config-fetch-models-btn" title="从服务商获取可用模型列表">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16">
                <path d="M21 12a9 9 0 11-6.22-8.56" /><polyline points="21 3 21 9 15 9" />
              </svg>
            </button>
          </div>
        </div>
      </div>
      <div class="config-row">
        <div class="config-field">
          <label>密钥</label>
          <input type="password" class="config-input model-apikey" value="" placeholder="${apiKeyPlaceholder}" autocomplete="new-password" />
        </div>
        <div class="config-field">
          <label>额外请求头</label>
          <input type="text" class="config-input model-extraheaders" value="${this.escapeHtml(m.extra_headers || "")}" placeholder="Key1:Value1,Key2:Value2" />
        </div>
      </div>
    </div>
  `;

  // 折叠/展开
  card.querySelector(".config-model-header").addEventListener("click", (e) => {
    if (
      e.target.closest(".config-model-remove") ||
      e.target.closest(".config-model-set-default")
    )
      return;
    card.classList.toggle("open");
  });

  // 删除
  card.querySelector(".config-model-remove").addEventListener("click", (e) => {
    e.stopPropagation();
    card.remove();
  });

  // 设为默认模型
  card
    .querySelector(".config-model-set-default")
    .addEventListener("click", (e) => {
      e.stopPropagation();
      const allCards =
        this.configModelList.querySelectorAll(".config-model-card");

      // 将当前 default 的卡片名称改回它原本的名称
      allCards.forEach((c) => {
        const nameInput = c.querySelector(".model-name");
        if (nameInput.value.trim() === "default" && c !== card) {
          // 优先从 dataset 恢复原始名称
          const restored = c.dataset.originalName;
          if (restored) {
            nameInput.value = restored;
            delete c.dataset.originalName;
          } else {
            const p = c.querySelector(".model-provider").value;
            const m = c.querySelector(".model-model").value.trim();
            nameInput.value = p + (m ? "-" + m : "");
          }
          nameInput.dispatchEvent(new Event("input", { bubbles: true }));
        }
      });

      // 记录当前卡片的原始名称，然后设为 default
      const myName = card.querySelector(".model-name").value.trim();
      if (myName && myName !== "default") {
        card.dataset.originalName = myName;
      }
      card.querySelector(".model-name").value = "default";
      card
        .querySelector(".model-name")
        .dispatchEvent(new Event("input", { bubbles: true }));

      // 更新所有卡片的默认标识
      this._refreshDefaultBadges();
    });

  // 实时更新摘要
  const self = this;
  const updateSummary = () => {
    const name = card.querySelector(".model-name").value.trim() || "未命名模型";
    const provider = card.querySelector(".model-provider").value;
    const model = card.querySelector(".model-model").value.trim();
    card.querySelector(".config-model-title").textContent = name;
    card.querySelector(".config-model-info").textContent =
      provider + (model ? " / " + model : "");
    self._refreshDefaultBadges();
  };
  card.querySelector(".model-name").addEventListener("input", updateSummary);
  card
    .querySelector(".model-provider")
    .addEventListener("change", updateSummary);
  card.querySelector(".model-model").addEventListener("input", updateSummary);

  // Copilot 等无需 API Key 的服务商，隐藏密钥和地址字段
  const toggleProviderFields = () => {
    const provider = card.querySelector(".model-provider").value;
    const needsKey = provider !== "copilot";
    card.querySelector(".model-apikey").closest(".config-field").style.display =
      needsKey ? "" : "none";
    card
      .querySelector(".model-baseurl")
      .closest(".config-field").style.display = needsKey ? "" : "none";
  };
  card
    .querySelector(".model-provider")
    .addEventListener("change", toggleProviderFields);
  toggleProviderFields();

  // 获取模型列表（带缓存）
  const getCacheKey = () => {
    const provider = card.querySelector(".model-provider").value;
    const baseUrl = card.querySelector(".model-baseurl").value.trim();
    const apiKey = card.querySelector(".model-apikey").value.trim();
    return provider + "|" + baseUrl + "|" + apiKey;
  };

  const fetchAndShowModels = async (forceRefresh) => {
    const provider = card.querySelector(".model-provider").value;
    const baseUrl = card.querySelector(".model-baseurl").value.trim();
    const apiKey = card.querySelector(".model-apikey").value.trim();
    const input = card.querySelector(".model-model");
    const cacheKey = getCacheKey();

    // 尝试使用缓存
    if (!forceRefresh && self._modelCache[cacheKey]) {
      self._showModelDropdown(input, self._modelCache[cacheKey]);
      return;
    }

    const btn = card.querySelector(".config-fetch-models-btn");
    btn.disabled = true;
    btn.classList.add("loading");
    try {
      const resp = await self.fetchWithAuth("/api/fkteams/providers/models", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          provider,
          base_url: baseUrl,
          api_key: apiKey,
        }),
      });
      const result = await resp.json();
      if (result.code !== 0 || !result.data) {
        const msg = result.message || "未知错误";
        const notifType = msg.includes("不支持模型列表") ? "warning" : "error";
        self.showNotification("获取模型列表失败: " + msg, notifType);
        return;
      }
      self._modelCache[cacheKey] = result.data;
      self._showModelDropdown(input, result.data);
      self.showNotification(
        `已获取 ${result.data.length} 个可用模型`,
        "success",
      );
    } catch (err) {
      self.showNotification("获取模型列表失败: " + err.message, "error");
    } finally {
      btn.disabled = false;
      btn.classList.remove("loading");
    }
  };

  // 点击刷新按钮：强制刷新
  card
    .querySelector(".config-fetch-models-btn")
    .addEventListener("click", (e) => {
      e.preventDefault();
      fetchAndShowModels(true);
    });

  // 点击模型输入框：有缓存时直接展示下拉
  card.querySelector(".model-model").addEventListener("click", () => {
    const cacheKey = getCacheKey();
    if (self._modelCache[cacheKey]) {
      const input = card.querySelector(".model-model");
      self._showModelDropdown(input, self._modelCache[cacheKey]);
    }
  });

  this.configModelList.appendChild(card);

  // 转换 select 为自定义组件
  card
    .querySelectorAll("select.config-select")
    .forEach((sel) => createCustomSelect(sel));
};

// 模型列表下拉面板（匹配自定义主题组件）
FKTeamsChat.prototype._showModelDropdown = function (input, models) {
  // 清理旧面板
  const oldDrop = document.querySelector(".fk-model-dropdown");
  if (oldDrop) oldDrop.remove();

  const dropdown = document.createElement("div");
  dropdown.className = "fk-model-dropdown fk-select-dropdown";
  document.body.appendChild(dropdown);

  const allModels = models.map((m) => m.id);

  const renderItems = (filter) => {
    dropdown.innerHTML = "";
    const filtered = filter
      ? allModels.filter((id) =>
        id.toLowerCase().includes(filter.toLowerCase()),
      )
      : allModels;
    filtered.forEach((id) => {
      const item = document.createElement("div");
      item.className =
        "fk-select-option" + (input.value === id ? " active" : "");
      item.textContent = id;
      item.addEventListener("click", () => {
        input.value = id;
        input.dispatchEvent(new Event("input", { bubbles: true }));
        close();
      });
      dropdown.appendChild(item);
    });
    if (filtered.length === 0) {
      const empty = document.createElement("div");
      empty.className = "fk-select-option disabled";
      empty.textContent = "无匹配模型";
      dropdown.appendChild(empty);
    }
  };

  const position = () => {
    const rect = input.getBoundingClientRect();
    dropdown.style.position = "fixed";
    dropdown.style.left = rect.left + "px";
    dropdown.style.width = rect.width + "px";
    const spaceBelow = window.innerHeight - rect.bottom;
    if (spaceBelow >= 200) {
      dropdown.style.top = rect.bottom + 4 + "px";
      dropdown.style.bottom = "auto";
    } else {
      dropdown.style.top = "auto";
      dropdown.style.bottom = window.innerHeight - rect.top + 4 + "px";
    }
  };

  const close = () => {
    dropdown.remove();
    input.removeEventListener("input", onInput);
    document.removeEventListener("click", onDocClick);
  };

  const onInput = () => renderItems(input.value);
  const onDocClick = (e) => {
    if (!dropdown.contains(e.target) && e.target !== input) close();
  };

  input.addEventListener("input", onInput);
  document.addEventListener("click", onDocClick);

  renderItems("");
  position();
  dropdown.style.display = "block";
  input.focus();
};

// 刷新所有模型卡片的默认标识
FKTeamsChat.prototype._refreshDefaultBadges = function () {
  const allCards = this.configModelList.querySelectorAll(".config-model-card");
  allCards.forEach((c) => {
    const name = c.querySelector(".model-name").value.trim();
    const isDefault = name === "default";

    // 更新 badge
    let badge = c.querySelector(".config-model-default-badge");
    if (isDefault && !badge) {
      badge = document.createElement("span");
      badge.className = "config-model-default-badge";
      badge.textContent = "默认";
      c.querySelector(".config-model-summary").appendChild(badge);
    } else if (!isDefault && badge) {
      badge.remove();
    }

    // 更新星标按钮
    const starBtn = c.querySelector(".config-model-set-default");
    if (starBtn) {
      starBtn.disabled = isDefault;
      starBtn.title = isDefault ? "当前默认模型" : "设为默认模型";
      const svg = starBtn.querySelector("svg");
      if (svg) {
        svg.setAttribute("fill", isDefault ? "var(--accent-color)" : "none");
        svg.setAttribute(
          "stroke",
          isDefault ? "var(--accent-color)" : "currentColor",
        );
      }
    }
  });
};

// ===== 自定义智能体卡片 =====

FKTeamsChat.prototype.renderAgentCards = function (agents) {
  this._customAgents = agents.map((a) => ({ ...a }));
  this._rebuildAgentCardsDOM();
};

FKTeamsChat.prototype._rebuildAgentCardsDOM = function () {
  const grid = this.agentCardsGrid;
  grid.innerHTML = "";

  this._customAgents.forEach((agent, idx) => {
    const card = document.createElement("div");
    card.className = "agent-card";
    const toolTags = (agent.tools || [])
      .slice(0, 4)
      .map(
        (t) =>
          `<span class="agent-card-tag tool-tag">${this.escapeHtml(t)}</span>`,
      )
      .join("");
    const moreTag =
      (agent.tools || []).length > 4
        ? `<span class="agent-card-tag tool-tag">+${agent.tools.length - 4}</span>`
        : "";

    card.innerHTML = `
      <div class="agent-card-actions">
        <button class="agent-card-action-btn edit" title="编辑">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" /><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" /></svg>
        </button>
        <button class="agent-card-action-btn delete" title="删除">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6" /><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" /></svg>
        </button>
      </div>
      <div class="agent-card-name">${this.escapeHtml(agent.name || "未命名")}</div>
      <div class="agent-card-desc">${this.escapeHtml(agent.desc || "暂无描述")}</div>
      <div class="agent-card-meta">
        <span class="agent-card-tag model-tag">${this.escapeHtml(agent.model || "default")}</span>
        ${toolTags}${moreTag}
      </div>
    `;

    card.querySelector(".edit").addEventListener("click", (e) => {
      e.stopPropagation();
      this.openAgentEditor(idx);
    });
    card.querySelector(".delete").addEventListener("click", (e) => {
      e.stopPropagation();
      this._customAgents.splice(idx, 1);
      this._rebuildAgentCardsDOM();
    });
    card.addEventListener("click", () => this.openAgentEditor(idx));
    grid.appendChild(card);
  });

  // 添加卡片
  const addCard = document.createElement("div");
  addCard.className = "agent-card-add";
  addCard.innerHTML = `
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
      <line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" />
    </svg>
    <span>添加智能体</span>
  `;
  addCard.addEventListener("click", () => {
    this._customAgents.push({
      name: "",
      desc: "",
      system_prompt: "",
      model: "default",
      tools: [],
    });
    this.openAgentEditor(this._customAgents.length - 1);
  });
  grid.appendChild(addCard);
};

// ===== 智能体编辑器 =====

FKTeamsChat.prototype.openAgentEditor = function (idx) {
  const agent = this._customAgents[idx];
  const overlay = document.createElement("div");
  overlay.className = "agent-edit-overlay";

  const seenNames = new Set(["default"]);
  const modelOptions = (this._configData?.models || [])
    .filter((m) => {
      if (!m.name || seenNames.has(m.name)) return false;
      seenNames.add(m.name);
      return true;
    })
    .map(
      (m) =>
        `<option value="${this.escapeHtml(m.name)}" ${agent.model === m.name ? "selected" : ""}>${this.escapeHtml(m.name)}</option>`,
    )
    .join("");

  const toolChips = this._toolNames
    .map((t) => {
      const selected = (agent.tools || []).includes(t) ? "selected" : "";
      return `<span class="config-tool-chip ${selected}" data-tool="${this.escapeHtml(t)}">${this.escapeHtml(t)}</span>`;
    })
    .join("");

  // MCP 工具也加入选项
  const mcpServers = this._configData?.custom?.mcp_servers || [];
  const mcpChips = mcpServers
    .filter((s) => s.enabled)
    .map((s) => {
      const toolName = "mcp-" + s.name;
      const selected = (agent.tools || []).includes(toolName) ? "selected" : "";
      return `<span class="config-tool-chip ${selected}" data-tool="${this.escapeHtml(toolName)}">${this.escapeHtml(toolName)}</span>`;
    })
    .join("");

  overlay.innerHTML = `
    <div class="agent-edit-panel">
      <div class="agent-edit-title">${idx < this._customAgents.length - 1 || agent.name ? "编辑智能体" : "新建智能体"}</div>
      <div class="config-field">
        <label>名称</label>
        <input type="text" class="config-input" id="ae-name" value="${this.escapeHtml(agent.name || "")}" placeholder="智能体名称" />
      </div>
      <div class="config-field">
        <label>描述</label>
        <input type="text" class="config-input" id="ae-desc" value="${this.escapeHtml(agent.desc || "")}" placeholder="一句话描述" />
      </div>
      <div class="config-field">
        <label>系统提示词</label>
        <textarea class="config-textarea" id="ae-prompt" placeholder="你是一个有帮助的助手。">${this.escapeHtml(agent.system_prompt || "")}</textarea>
      </div>
      <div class="config-field">
        <label>模型</label>
        <select class="config-select" id="ae-model">
          <option value="default" ${agent.model === "default" || !agent.model ? "selected" : ""}>default (系统默认)</option>
          ${modelOptions}
        </select>
      </div>
      <div class="config-field">
        <label>工具 (点击选择/取消)</label>
        <div class="config-tools-select" id="ae-tools">${toolChips}${mcpChips}</div>
      </div>
      <div class="agent-edit-footer">
        <button class="config-btn config-btn-cancel" id="ae-cancel">取消</button>
        <button class="config-btn config-btn-save" id="ae-save">确定</button>
      </div>
    </div>
  `;

  // 工具选择交互
  overlay.querySelector("#ae-tools").addEventListener("click", (e) => {
    const chip = e.target.closest(".config-tool-chip");
    if (chip) chip.classList.toggle("selected");
  });

  // 转换 select 为自定义组件
  overlay
    .querySelectorAll("select.config-select")
    .forEach((sel) => createCustomSelect(sel));

  // 提示词模板变量补全
  this._setupPromptVarAutocomplete(overlay.querySelector("#ae-prompt"));

  // 取消
  overlay.querySelector("#ae-cancel").addEventListener("click", () => {
    // 如果是新建且未填名称，移除
    if (!agent.name && idx === this._customAgents.length - 1) {
      this._customAgents.pop();
    }
    overlay.remove();
  });

  // 保存
  overlay.querySelector("#ae-save").addEventListener("click", () => {
    const name = overlay.querySelector("#ae-name").value.trim();
    if (!name) {
      overlay.querySelector("#ae-name").style.borderColor = "var(--error)";
      return;
    }
    this._customAgents[idx] = {
      name: name,
      desc: overlay.querySelector("#ae-desc").value.trim(),
      system_prompt: overlay.querySelector("#ae-prompt").value,
      model: overlay.querySelector("#ae-model").value,
      tools: Array.from(
        overlay.querySelectorAll(".config-tool-chip.selected"),
      ).map((c) => c.dataset.tool),
    };
    overlay.remove();
    this._rebuildAgentCardsDOM();
  });

  // 点击背景关闭
  overlay.addEventListener("click", (e) => {
    if (e.target === overlay) {
      if (!agent.name && idx === this._customAgents.length - 1) {
        this._customAgents.pop();
      }
      overlay.remove();
    }
  });

  document.body.appendChild(overlay);
  overlay.querySelector("#ae-name").focus();
};

// ===== 模板变量补全 =====

FKTeamsChat.prototype._loadTemplateVars = async function () {
  if (this._templateVars) return this._templateVars;
  try {
    const resp = await this.fetchWithAuth("/api/fkteams/config/template-vars");
    const result = await resp.json();
    if (result.code === 0 && result.data) {
      this._templateVars = result.data;
      return this._templateVars;
    }
  } catch (e) {
    console.error("load template vars:", e);
  }
  // 兜底
  this._templateVars = [
    { name: "current_time", description: "当前时间" },
    { name: "os_type", description: "操作系统类型" },
    { name: "os_arch", description: "系统架构" },
    { name: "workspace_dir", description: "工作目录路径" },
  ];
  return this._templateVars;
};

FKTeamsChat.prototype._setupPromptVarAutocomplete = function (textarea) {
  if (!textarea) return;
  const self = this;

  textarea.addEventListener("input", async function () {
    const pos = textarea.selectionStart;
    const text = textarea.value.substring(0, pos);

    // 检测是否刚输入了 { 或正在 {xxx 中
    const match = text.match(/\{([a-z_]*)$/);
    if (!match) {
      self._hideVarDropdown();
      return;
    }

    const filter = match[1].toLowerCase();
    const vars = await self._loadTemplateVars();
    const filtered = vars.filter((v) =>
      v.name.toLowerCase().includes(filter),
    );
    if (filtered.length === 0) {
      self._hideVarDropdown();
      return;
    }

    self._showVarDropdown(textarea, filtered, match[0].length);
  });

  textarea.addEventListener("blur", function () {
    setTimeout(() => self._hideVarDropdown(), 200);
  });

  textarea.addEventListener("keydown", function (e) {
    const dropdown = document.querySelector(".template-var-dropdown");
    if (!dropdown || dropdown.style.display === "none") return;
    const items = dropdown.querySelectorAll(".template-var-item");
    let idx = Array.from(items).findIndex((i) =>
      i.classList.contains("active"),
    );
    if (e.key === "ArrowDown") {
      e.preventDefault();
      if (idx >= 0) items[idx].classList.remove("active");
      idx = (idx + 1) % items.length;
      items[idx].classList.add("active");
      items[idx].scrollIntoView({ block: "nearest" });
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      if (idx >= 0) items[idx].classList.remove("active");
      idx = idx <= 0 ? items.length - 1 : idx - 1;
      items[idx].classList.add("active");
      items[idx].scrollIntoView({ block: "nearest" });
    } else if (e.key === "Enter" || e.key === "Tab") {
      if (idx >= 0) {
        e.preventDefault();
        items[idx].click();
      }
    } else if (e.key === "Escape") {
      self._hideVarDropdown();
    }
  });
};

FKTeamsChat.prototype._showVarDropdown = function (textarea, vars, matchLen) {
  let dropdown = document.querySelector(".template-var-dropdown");
  if (!dropdown) {
    dropdown = document.createElement("div");
    dropdown.className = "template-var-dropdown";
    document.body.appendChild(dropdown);
  }

  dropdown.innerHTML = "";
  const self = this;
  vars.forEach((v, i) => {
    const item = document.createElement("div");
    item.className = "template-var-item" + (i === 0 ? " active" : "");
    item.innerHTML = `<span class="template-var-name">{${v.name}}</span><span class="template-var-desc">${v.description}</span>`;
    item.addEventListener("mousedown", (e) => {
      e.preventDefault();
      const pos = textarea.selectionStart;
      const before = textarea.value.substring(0, pos - matchLen);
      const after = textarea.value.substring(pos);
      const insert = "{" + v.name + "}";
      textarea.value = before + insert + after;
      const newPos = before.length + insert.length;
      textarea.selectionStart = textarea.selectionEnd = newPos;
      textarea.focus();
      self._hideVarDropdown();
    });
    dropdown.appendChild(item);
  });

  // 定位到光标附近
  const rect = textarea.getBoundingClientRect();
  // 粗略估算光标位置
  const lineHeight = parseInt(getComputedStyle(textarea).lineHeight) || 20;
  const lines = textarea.value.substring(0, textarea.selectionStart).split("\n");
  const lineNum = lines.length - 1;
  const scrollTop = textarea.scrollTop;

  dropdown.style.display = "block";
  dropdown.style.position = "fixed";
  dropdown.style.left = rect.left + "px";
  dropdown.style.top = Math.min(rect.top + (lineNum + 1) * lineHeight - scrollTop + 4, rect.bottom) + "px";
  dropdown.style.width = Math.min(280, rect.width) + "px";
  dropdown.style.zIndex = "12000";
};

FKTeamsChat.prototype._hideVarDropdown = function () {
  const dropdown = document.querySelector(".template-var-dropdown");
  if (dropdown) dropdown.style.display = "none";
};

// ===== 收集并保存 =====

FKTeamsChat.prototype.collectConfigData = function () {
  const cfg = {};

  // 模型
  cfg.models = [];
  this.configModelList
    .querySelectorAll(".config-model-card")
    .forEach((card) => {
      cfg.models.push({
        name: card.querySelector(".model-name").value.trim(),
        original_name: card.dataset.apiLookupName || "",
        provider: card.querySelector(".model-provider").value,
        base_url: card.querySelector(".model-baseurl").value.trim(),
        api_key: card.querySelector(".model-apikey").value,
        model: card.querySelector(".model-model").value.trim(),
        extra_headers: card.querySelector(".model-extraheaders").value.trim(),
      });
    });

  // 记忆
  cfg.memory = {
    enabled: document.getElementById("config-memory-enabled").checked,
  };

  // 服务器
  cfg.server = {
    host: document.getElementById("config-server-host").value.trim(),
    port: parseInt(document.getElementById("config-server-port").value) || 0,
    log_level: document.getElementById("config-server-loglevel").value,
    auth: {
      enabled: document.getElementById("config-auth-enabled").checked,
      username: document.getElementById("config-auth-username").value.trim(),
      password: document.getElementById("config-auth-password").value,
      secret: document.getElementById("config-auth-secret").value,
    },
  };

  // 智能体
  cfg.agents = {
    searcher: document.getElementById("config-agent-searcher").checked,
    assistant: document.getElementById("config-agent-assistant").checked,
    analyst: document.getElementById("config-agent-analyst").checked,
    ssh_visitor: {
      enabled: document.getElementById("config-ssh-enabled").checked,
      host: document.getElementById("config-ssh-host").value.trim(),
      username: document.getElementById("config-ssh-username").value.trim(),
      password: document.getElementById("config-ssh-password").value,
    },
  };

  // 自定义智能体 + 保留原有的 moderator 和 mcp_servers
  cfg.custom = {
    moderator: this._configData?.custom?.moderator || {},
    agents: this._customAgents || [],
    mcp_servers: this._configData?.custom?.mcp_servers || [],
  };

  // 通道
  cfg.channels = {
    qq: {
      enabled: document.getElementById("config-qq-enabled").checked,
      app_id: document.getElementById("config-qq-appid").value.trim(),
      app_secret: document.getElementById("config-qq-appsecret").value,
      sandbox: document.getElementById("config-qq-sandbox").checked,
      mode: document.getElementById("config-qq-mode").value,
    },
    discord: {
      enabled: document.getElementById("config-discord-enabled").checked,
      token: document.getElementById("config-discord-token").value,
      allow_from: document
        .getElementById("config-discord-allowfrom")
        .value.trim(),
      mode: document.getElementById("config-discord-mode").value,
    },
    weixin: {
      enabled: document.getElementById("config-weixin-enabled").checked,
      base_url: document.getElementById("config-weixin-baseurl").value.trim(),
      cred_path: document.getElementById("config-weixin-credpath").value.trim(),
      log_level: document.getElementById("config-weixin-loglevel").value,
      allow_from: document
        .getElementById("config-weixin-allowfrom")
        .value.trim(),
      mode: document.getElementById("config-weixin-mode").value,
    },
  };

  // 保留圆桌讨论配置
  cfg.roundtable = this._configData?.roundtable || {};

  return cfg;
};

FKTeamsChat.prototype.saveConfig = async function () {
  this.configSaveBtn.disabled = true;
  this.configSaveBtn.textContent = "保存中...";

  try {
    const oldCfg = this._configData;
    const cfg = this.collectConfigData();

    // 校验模型名称唯一性
    const names = cfg.models.map((m) => m.name).filter(Boolean);
    const seen = new Set();
    for (const n of names) {
      if (seen.has(n)) {
        this.showNotification(`模型名称 "${n}" 重复，请确保每个模型名称唯一`, "error");
        this.configSaveBtn.disabled = false;
        this.configSaveBtn.textContent = "保存配置";
        return;
      }
      seen.add(n);
    }

    const resp = await this.fetchWithAuth("/api/fkteams/config", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(cfg),
    });
    const result = await resp.json();
    if (result.code !== 0) {
      this.showNotification("保存失败: " + result.message, "error");
      return;
    }

    // 刷新智能体列表
    this.loadAgents();

    // Auth 变更检测
    if (result.data?.auth_changed) {
      this.showNotification("认证配置已变更，请重新登录", "info");
      localStorage.removeItem("fk_token");
      document.cookie = "fk_token=; path=/; max-age=0";
      window.location.href = "/login";
      return;
    }

    this.closeConfig();
    this.showNotification("配置已保存并生效", "success");

    // 检测是否有需要重启才能生效的配置变更
    if (this._detectRestartNeeded(oldCfg, cfg)) {
      this._showRestartHint();
    }
  } catch (err) {
    this.showNotification("保存配置失败: " + err.message, "error");
  } finally {
    this.configSaveBtn.disabled = false;
    this.configSaveBtn.textContent = "保存配置";
  }
};

// 检测是否有需要重启服务才能生效的配置变更（服务器、通道）
FKTeamsChat.prototype._detectRestartNeeded = function (oldCfg, newCfg) {
  if (!oldCfg) return false;
  const os = oldCfg.server || {},
    ns = newCfg.server || {};
  if (
    os.host !== ns.host ||
    os.port !== ns.port ||
    os.log_level !== ns.log_level
  )
    return true;
  if (
    JSON.stringify(oldCfg.channels || {}) !==
    JSON.stringify(newCfg.channels || {})
  )
    return true;
  return false;
};

// 弹窗提示用户需要重启，并提供关闭服务按钮
FKTeamsChat.prototype._showRestartHint = function () {
  const overlay = document.createElement("div");
  overlay.style.cssText =
    "position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(40,35,25,0.4);backdrop-filter:blur(4px);z-index:1200;display:flex;align-items:center;justify-content:center;animation:fadeIn 0.15s ease;";

  const box = document.createElement("div");
  box.style.cssText =
    "background:var(--bg-secondary);border:2px solid var(--border-sketch);border-radius:16px;width:90%;max-width:420px;box-shadow:4px 6px 0px rgba(60,50,30,0.12);animation:sketchSlideUp 0.25s ease;";

  const header = document.createElement("div");
  header.style.cssText =
    "padding:16px 20px;border-bottom:2px dashed var(--border-color);";
  header.innerHTML =
    '<h3 style="margin:0;font-family:var(--font-hand);font-size:22px;font-weight:600;color:var(--warning,#f0a500);">提示</h3>';

  const body = document.createElement("div");
  body.style.cssText =
    "padding:20px;text-align:center;line-height:1.6;font-size:15px;color:var(--text-primary);";
  body.textContent =
    "配置已保存。服务器或通道相关配置的变更需要重启服务后才能生效。";

  const footer = document.createElement("div");
  footer.style.cssText =
    "display:flex;justify-content:flex-end;gap:10px;padding:16px 20px;border-top:2px dashed var(--border-color);";

  const laterBtn = document.createElement("button");
  laterBtn.textContent = "稍后再说";
  laterBtn.style.cssText =
    "padding:8px 20px;border:2px solid var(--border-color);border-radius:10px;font-size:14px;cursor:pointer;background:transparent;color:var(--text-primary);transition:all var(--transition);";

  const restartBtn = document.createElement("button");
  restartBtn.textContent = "立即重启";
  restartBtn.style.cssText =
    "padding:8px 20px;border:2px solid var(--accent-primary);border-radius:10px;font-size:14px;font-weight:500;cursor:pointer;background:var(--accent-primary);color:white;transition:all var(--transition);";

  const close = () => {
    overlay.style.animation = "fadeIn 0.15s ease reverse";
    setTimeout(() => overlay.remove(), 150);
  };

  laterBtn.addEventListener("click", close);
  overlay.addEventListener("click", (e) => {
    if (e.target === overlay) close();
  });

  const self = this;
  restartBtn.addEventListener("click", async () => {
    restartBtn.disabled = true;
    restartBtn.textContent = "正在重启...";
    try {
      await self.fetchWithAuth("/api/fkteams/restart", { method: "POST" });
      body.textContent = "服务正在重启，请稍候...";
      restartBtn.style.display = "none";
      laterBtn.style.display = "none";

      // 轮询等待服务恢复后自动刷新
      const poll = setInterval(async () => {
        try {
          const r = await fetch("/health");
          if (r.ok) {
            clearInterval(poll);
            window.location.reload();
          }
        } catch (_) { }
      }, 2000);
    } catch (err) {
      self.showNotification("重启失败: " + err.message, "error");
      restartBtn.disabled = false;
      restartBtn.textContent = "立即重启";
    }
  });

  footer.append(laterBtn, restartBtn);
  box.append(header, body, footer);
  overlay.appendChild(box);
  document.body.appendChild(overlay);
};

// ===== Skills Management =====

FKTeamsChat.prototype._initSkillsTab = function () {
  if (this._skillsInited) return;
  this._skillsInited = true;
  this._bindSkillsTabs();
  this._bindSkillsSearch();
  this._loadInstalledSkills();
};

FKTeamsChat.prototype._bindSkillsTabs = function () {
  var self = this;
  var container = document.getElementById("config-panel-skills");
  if (!container) return;
  container.querySelector(".skills-tabs").addEventListener("click", function (e) {
    var btn = e.target.closest(".skills-tab-btn");
    if (!btn) return;
    var tab = btn.dataset.skillsTab;
    container.querySelectorAll(".skills-tab-btn").forEach(function (b) {
      b.classList.toggle("active", b.dataset.skillsTab === tab);
    });
    container.querySelectorAll(".skills-panel").forEach(function (p) {
      p.classList.toggle("active", p.id === "skills-panel-" + tab);
    });
    if (tab === "installed") { self._loadInstalledSkills(); }
  });
};

FKTeamsChat.prototype._bindSkillsSearch = function () {
  var self = this;
  var input = document.getElementById("skills-search-input");
  var btn = document.getElementById("skills-search-btn");
  if (!input || !btn) return;
  var doSearch = function () {
    var q = input.value.trim();
    if (!q) return;
    self._searchSkills(q);
  };
  btn.addEventListener("click", doSearch);
  input.addEventListener("keydown", function (e) { if (e.key === "Enter") doSearch(); });
};

FKTeamsChat.prototype._loadInstalledSkills = async function () {
  var list = document.getElementById("skills-installed-list");
  if (!list) return;
  list.innerHTML = '<div class="skills-placeholder">loading...</div>';
  try {
    var resp = await this.fetchWithAuth("/api/fkteams/skills");
    var r = await resp.json();
    if (r.code !== 0 || !r.data) { list.innerHTML = '<div class="skills-placeholder">load failed</div>'; return; }
    var skills = r.data.skills || [];
    if (skills.length === 0) {
      list.innerHTML = '<div class="skills-placeholder">no installed skills</div>';
      return;
    }
    this._renderSkillCards(list, skills, "remove");
  } catch (e) { list.innerHTML = '<div class="skills-placeholder">load failed</div>'; }
};

FKTeamsChat.prototype._searchSkills = async function (keyword, page) {
  page = page || 1;
  var list = document.getElementById("skills-market-list");
  var sortEl = document.getElementById("skills-sort-select");
  if (!list) return;
  list.innerHTML = '<div class="skills-placeholder">searching...</div>';
  var sortBy = sortEl ? sortEl.value : "downloads";

  try {
    var url = "/api/fkteams/skills/search?q=" + encodeURIComponent(keyword) +
      "&sort=" + sortBy + "&order=desc&page=" + page + "&size=10";
    var results = await Promise.all([
      this.fetchWithAuth(url),
      this.fetchWithAuth("/api/fkteams/skills")
    ]);
    var searchR = await results[0].json();
    var installedR = await results[1].json();

    var installedSlugs = {};
    if (installedR.code === 0 && installedR.data && installedR.data.skills) {
      installedR.data.skills.forEach(function (s) { installedSlugs[s.slug] = true; });
    }

    if (searchR.code !== 0 || !searchR.data) { list.innerHTML = '<div class="skills-placeholder">search failed</div>'; return; }
    var skills = searchR.data.skills || [];
    if (skills.length === 0) {
      list.innerHTML = '<div class="skills-placeholder">no results for "' + this.escapeHtml(keyword) + '"</div>';
      this._renderPagination(keyword, page, 0, 0);
      return;
    }
    this._renderSkillCards(list, skills, "install", installedSlugs);
    this._renderPagination(keyword, page, searchR.data.total, searchR.data.size || 10);
  } catch (e) { list.innerHTML = '<div class="skills-placeholder">search failed</div>'; }
};

FKTeamsChat.prototype._renderPagination = function (keyword, page, total, size) {
  var pager = document.getElementById("skills-pagination");
  if (!pager) return;
  var totalPages = Math.ceil(total / size);
  if (totalPages <= 1) { pager.style.display = "none"; return; }

  var self = this;
  pager.style.display = "flex";
  var html = '<span class="skills-page-info">' + total + ' results, page ' + page + '/' + totalPages + '</span>';
  if (page > 1) {
    html += '<button class="skills-page-btn" data-page="' + (page - 1) + '">prev</button>';
  }
  if (page < totalPages) {
    html += '<button class="skills-page-btn" data-page="' + (page + 1) + '">next</button>';
  }
  pager.innerHTML = html;

  pager.querySelectorAll(".skills-page-btn").forEach(function (btn) {
    btn.addEventListener("click", function () {
      self._searchSkills(keyword, parseInt(btn.dataset.page));
    });
  });
};

FKTeamsChat.prototype._renderSkillCards = function (container, skills, mode, installedSlugs) {
  var self = this;
  installedSlugs = installedSlugs || {};
  var html = "";
  skills.forEach(function (s) {
    var slug = s.slug || "";
    var name = s.name || slug || "";
    var desc = s.description || "";
    if (desc.length > 80) desc = desc.substring(0, 80) + "...";

    var metaHtml = "";
    if (mode === "install") {
      var metaParts = [];
      if (s.owner) metaParts.push("@" + s.owner);
      if (s.version) metaParts.push("v" + s.version);
      metaParts.push((s.downloads || 0) + " downloads");
      if (s.stars) metaParts.push("★ " + s.stars);
      metaHtml = '<div class="skill-market-meta">' + self.escapeHtml(metaParts.join(" · ")) + '</div>';
    }

    var actionHtml = "";
    if (mode === "install") {
      if (installedSlugs[slug]) {
        actionHtml = '<span class="skill-installed-tag">已安装</span>';
      } else {
        actionHtml = '<button class="skill-card-btn skill-install-btn" data-slug="' + self.escapeHtml(slug) + '">install</button>';
      }
    } else {
      actionHtml = '<button class="skill-card-btn skill-remove-btn" data-slug="' + self.escapeHtml(slug) + '">remove</button>';
    }

    var cardId = "skill-card-" + self.escapeHtml(slug);

    html +=
      '<div class="skill-card" id="' + cardId + '">' +
        '<div class="skill-card-info">' +
          '<div class="skill-card-name">' + self.escapeHtml(name) + '</div>' +
          '<div class="skill-card-desc">' + self.escapeHtml(desc) + '</div>' +
          metaHtml +
        '</div>' +
        '<div class="skill-card-actions">' + actionHtml + '</div>' +
      '</div>';
  });
  container.innerHTML = html;

  container.querySelectorAll(".skill-install-btn").forEach(function (btn) {
    btn.addEventListener("click", function (e) { e.stopPropagation(); self._installSkill(btn.dataset.slug, btn); });
  });
  container.querySelectorAll(".skill-remove-btn").forEach(function (btn) {
    btn.addEventListener("click", function (e) { e.stopPropagation(); self._removeSkill(btn.dataset.slug); });
  });

  // click card to toggle file tree (installed mode only)
  if (mode === "remove") {
    container.querySelectorAll(".skill-card").forEach(function (card) {
      card.style.cursor = "pointer";
      card.addEventListener("click", function (e) {
        if (e.target.closest(".skill-card-btn")) return;
        var slug = card.id.replace("skill-card-", "");
        self._toggleSkillTree(slug);
      });
    });
  }
};

FKTeamsChat.prototype._toggleSkillTree = function (slug) {
  this._openSkillBrowser(slug, "");
};

FKTeamsChat.prototype._openSkillBrowser = function (slug, subPath) {
  var self = this;
  var overlay = document.createElement("div");
  overlay.className = "skill-preview-overlay";
  overlay.dataset.slug = slug;
  overlay.dataset.path = subPath;

  overlay.innerHTML =
    '<div class="skill-preview-modal">' +
      '<div class="skill-preview-header">' +
        '<span class="skill-preview-title">' + self.escapeHtml(slug) + '</span>' +
        '<button class="skill-preview-close">&times;</button>' +
      '</div>' +
      '<div class="skill-preview-paths" id="skill-breadcrumb"></div>' +
      '<div class="skill-preview-list" id="skill-file-list">' +
        '<div class="skills-placeholder">loading...</div>' +
      '</div>' +
      '<div class="skill-preview-content" id="skill-file-content" style="display:none"></div>' +
    '</div>';

  document.body.appendChild(overlay);

  var close = function () {
    overlay.style.animation = "fadeIn 0.15s ease reverse";
    setTimeout(function () { document.body.removeChild(overlay); }, 150);
  };
  overlay.addEventListener("click", function (e) { if (e.target === overlay) close(); });
  overlay.querySelector(".skill-preview-close").addEventListener("click", close);

  self._loadSkillBrowser(overlay, slug, subPath);
};

FKTeamsChat.prototype._loadSkillBrowser = async function (overlay, slug, subPath) {
  var self = this;
  subPath = (subPath || "").replace(/^\/+/, ""); // normalize: remove leading slashes
  var list = overlay.querySelector("#skill-file-list");
  var breadcrumb = overlay.querySelector("#skill-breadcrumb");
  var content = overlay.querySelector("#skill-file-content");
  list.style.display = "";
  list.innerHTML = '<div class="skills-placeholder">loading...</div>';
  content.style.display = "none";

  try {
    var resp = await this.fetchWithAuth("/api/fkteams/skills/" + slug + "/files?path=" + encodeURIComponent(subPath));
    var r = await resp.json();
    if (r.code !== 0 || !r.data) { list.innerHTML = '<div class="skills-placeholder">load failed</div>'; return; }
    var files = r.data.files || [];

    // breadcrumb
    var parts = subPath ? subPath.split("/").filter(Boolean) : [];
    var bcHtml = '<span class="skill-bc-item" data-path="">root</span>';
    var accum = "";
    parts.forEach(function (p) {
      accum = accum ? accum + "/" + p : p;
      bcHtml += '<span class="skill-bc-sep">/</span><span class="skill-bc-item" data-path="' + self.escapeHtml(accum) + '">' + self.escapeHtml(p) + '</span>';
    });
    breadcrumb.innerHTML = bcHtml;

    breadcrumb.querySelectorAll(".skill-bc-item").forEach(function (el) {
      el.addEventListener("click", function () {
        overlay.dataset.path = el.dataset.path;
        self._loadSkillBrowser(overlay, slug, el.dataset.path);
      });
    });

    // sort: dirs first
    files.sort(function (a, b) {
      if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;
      return a.name.localeCompare(b.name);
    });

    var html = "";
    if (subPath) {
      var parentPath = subPath.split("/").filter(Boolean).slice(0, -1).join("/");
      html += '<div class="skill-file-entry skill-file-up" data-path="' + self.escapeHtml(parentPath) + '">..</div>';
    }

    files.forEach(function (f) {
      var cls = f.is_dir ? "skill-file-entry skill-file-dir" : "skill-file-entry skill-file-file";
      html +=
        '<div class="' + cls + '" data-path="' + self.escapeHtml(f.path) + '">' +
          '<span class="skill-file-name">' + self.escapeHtml(f.name) + '</span>' +
          (f.is_dir ? '' : '<span class="skill-file-size">' + self._formatFileSize(f.size) + '</span>') +
        '</div>';
    });
    list.innerHTML = html;

    list.querySelectorAll(".skill-file-dir, .skill-file-up").forEach(function (el) {
      el.addEventListener("click", function () {
        overlay.dataset.path = el.dataset.path;
        self._loadSkillBrowser(overlay, slug, el.dataset.path);
      });
    });
    list.querySelectorAll(".skill-file-file").forEach(function (el) {
      el.addEventListener("click", function () {
        self._loadSkillFileContent(overlay, slug, el.dataset.path);
      });
    });
  } catch (e) {
    list.innerHTML = '<div class="skills-placeholder">load failed</div>';
  }
};

FKTeamsChat.prototype._loadSkillFileContent = async function (overlay, slug, filePath) {
  var self = this;
  var list = overlay.querySelector("#skill-file-list");
  var breadcrumb = overlay.querySelector("#skill-breadcrumb");
  var content = overlay.querySelector("#skill-file-content");

  // update breadcrumb: append filename
  var parts = filePath.replace(/^\/+/, "").split("/").filter(Boolean);
  var accum = "";
  var bcHtml = '<span class="skill-bc-item" data-path="">root</span>';
  parts.forEach(function (p) {
    accum = accum ? accum + "/" + p : p;
    var isLast = accum === filePath.replace(/^\/+/, "");
    bcHtml += '<span class="skill-bc-sep">/</span>';
    bcHtml += isLast
      ? '<span class="skill-bc-item skill-bc-last">' + self.escapeHtml(p) + '</span>'
      : '<span class="skill-bc-item" data-path="' + self.escapeHtml(accum) + '">' + self.escapeHtml(p) + '</span>';
  });
  breadcrumb.innerHTML = bcHtml;

  breadcrumb.querySelectorAll(".skill-bc-item:not(.skill-bc-last)").forEach(function (el) {
    el.addEventListener("click", function () {
      overlay.dataset.path = el.dataset.path;
      self._loadSkillBrowser(overlay, slug, el.dataset.path);
    });
  });

  list.style.display = "none";
  content.style.display = "";
  content.innerHTML = '<div class="skills-placeholder">loading...</div>';

  try {
    var resp = await this.fetchWithAuth("/api/fkteams/skills/" + slug + "/file?path=" + encodeURIComponent(filePath));
    var r = await resp.json();
    if (r.code !== 0 || !r.data) { content.innerHTML = '<div class="skills-placeholder">load failed</div>'; return; }

    var ext = (filePath.split(".").pop() || "").toLowerCase();
    content.innerHTML =
      (ext === "md"
        ? '<div class="skill-file-content markdown-body">' + self.renderMarkdown(r.data.content) + '</div>'
        : '<pre class="skill-file-content"><code>' + self.escapeHtml(r.data.content) + '</code></pre>');
  } catch (e) {
    content.innerHTML = '<div class="skills-placeholder">load failed</div>';
  }
};

FKTeamsChat.prototype._formatFileSize = function (bytes) {
  if (!bytes || bytes === 0) return "";
  if (bytes < 1024) return bytes + " B";
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
  return (bytes / (1024 * 1024)).toFixed(1) + " MB";
};

FKTeamsChat.prototype._installSkill = async function (slug, btn) {
  btn.disabled = true;
  btn.textContent = "installing...";
  try {
    var resp = await this.fetchWithAuth("/api/fkteams/skills/install", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ slug: slug })
    });
    var r = await resp.json();
    if (r.code === 0) {
      this.showNotification(slug + " installed", "success");
      this._loadInstalledSkills();
    } else {
      this.showNotification(r.message || "install failed", "error");
    }
  } catch (e) {
    this.showNotification("install failed", "error");
  }
  btn.disabled = false;
  btn.textContent = "install";
};

FKTeamsChat.prototype._removeSkill = async function (slug) {
  if (!confirm("remove " + slug + "?")) return;
  try {
    var resp = await this.fetchWithAuth("/api/fkteams/skills/" + slug, { method: "DELETE" });
    var r = await resp.json();
    if (r.code === 0) {
      this.showNotification(slug + " removed", "success");
      this._loadInstalledSkills();
    } else {
      this.showNotification(r.message || "remove failed", "error");
    }
  } catch (e) {
    this.showNotification("remove failed", "error");
  }
};

// ===== MCP Server Management =====

FKTeamsChat.prototype._initMcpTab = function () {
  if (this._mcpInited) return;
  this._mcpInited = true;
  this._bindMcpEvents();
  this._renderMcpList();
};

FKTeamsChat.prototype._getMcpServers = function () {
  return this._configData?.custom?.mcp_servers || [];
};

FKTeamsChat.prototype._setMcpServers = function (servers) {
  if (!this._configData.custom) this._configData.custom = {};
  this._configData.custom.mcp_servers = servers;
};

FKTeamsChat.prototype._bindMcpEvents = function () {
  var self = this;
  var addBtn = document.getElementById("mcp-add-btn");
  if (addBtn) addBtn.addEventListener("click", function () { self._openMcpEditor(-1); });

  var closeBtn = document.getElementById("mcp-editor-close");
  if (closeBtn) closeBtn.addEventListener("click", function () { self._closeMcpEditor(); });

  var saveBtn = document.getElementById("mcp-editor-save");
  if (saveBtn) saveBtn.addEventListener("click", function () { self._saveMcpServer(); });

  var delBtn = document.getElementById("mcp-editor-delete");
  if (delBtn) delBtn.addEventListener("click", function () { self._deleteMcpServer(); });

  var transport = document.getElementById("mcp-edit-transport");
  if (transport) transport.addEventListener("change", function () { self._onMcpTransportChange(); });
};

FKTeamsChat.prototype._renderMcpList = function () {
  var list = document.getElementById("mcp-server-list");
  var count = document.getElementById("mcp-server-count");
  if (!list) return;
  var servers = this._getMcpServers();
  if (count) count.textContent = servers.length + " 个服务";

  if (servers.length === 0) {
    list.innerHTML = '<div class="mcp-empty">暂无 MCP 服务</div>';
    return;
  }

  var self = this;
  var html = "";
  servers.forEach(function (s, idx) {
    var transport = s.transport_type || "http";
    var desc = s.desc || "";
    var metaParts = [transport];
    if (transport === "http" || transport === "sse") {
      metaParts.push(s.url || "");
    } else {
      metaParts.push((s.command || "") + " " + (s.args || []).join(" "));
    }
    html +=
      '<div class="mcp-card">' +
        '<div class="mcp-toggle' + (s.enabled ? " on" : "") + '" data-idx="' + idx + '"></div>' +
        '<div class="mcp-card-info">' +
          '<div class="mcp-card-name">' + self.escapeHtml(s.name || "未命名") + '</div>' +
          (desc ? '<div class="mcp-card-desc">' + self.escapeHtml(desc) + '</div>' : '') +
          '<div class="mcp-card-meta">' + self.escapeHtml(metaParts.join(" - ")) + '</div>' +
        '</div>' +
        '<div class="mcp-card-actions">' +
          '<button class="mcp-card-btn mcp-edit-btn" data-idx="' + idx + '">编辑</button>' +
        '</div>' +
      '</div>';
  });
  list.innerHTML = html;

  list.querySelectorAll(".mcp-toggle").forEach(function (tog) {
    tog.addEventListener("click", function () {
      self._toggleMcpServer(parseInt(tog.dataset.idx));
    });
  });
  list.querySelectorAll(".mcp-edit-btn").forEach(function (btn) {
    btn.addEventListener("click", function () {
      self._openMcpEditor(parseInt(btn.dataset.idx));
    });
  });
};

FKTeamsChat.prototype._openMcpEditor = function (idx) {
  var editor = document.getElementById("mcp-editor");
  var list = document.getElementById("mcp-server-list");
  if (!editor || !list) return;
  this._mcpEditIdx = idx;
  var servers = this._getMcpServers();
  var s = idx >= 0 && idx < servers.length ? servers[idx] : {};

  document.getElementById("mcp-edit-name").value = s.name || "";
  document.getElementById("mcp-edit-desc").value = s.desc || "";
  document.getElementById("mcp-edit-transport").value = s.transport_type || "http";
  document.getElementById("mcp-edit-url").value = s.url || "";
  document.getElementById("mcp-edit-command").value = s.command || "";
  document.getElementById("mcp-edit-args").value = (s.args || []).join("\n");
  document.getElementById("mcp-edit-env").value = (s.env_vars || []).join("\n");
  document.getElementById("mcp-edit-timeout").value = s.timeout || 30;
  document.getElementById("mcp-editor-delete").style.display = idx >= 0 ? "" : "none";

  this._onMcpTransportChange();
  list.style.display = "none";
  editor.style.display = "";
};

FKTeamsChat.prototype._closeMcpEditor = function () {
  var editor = document.getElementById("mcp-editor");
  var list = document.getElementById("mcp-server-list");
  if (!editor || !list) return;
  editor.style.display = "none";
  list.style.display = "";
};

FKTeamsChat.prototype._onMcpTransportChange = function () {
  var t = document.getElementById("mcp-edit-transport").value;
  var urlGroup = document.getElementById("mcp-edit-url-group");
  var cmdGroup = document.getElementById("mcp-edit-command-group");
  var argsGroup = document.getElementById("mcp-edit-args-group");
  var envGroup = document.getElementById("mcp-edit-env-group");
  var isStdio = t === "stdio";
  if (urlGroup) urlGroup.style.display = isStdio ? "none" : "";
  if (cmdGroup) cmdGroup.style.display = isStdio ? "" : "none";
  if (argsGroup) argsGroup.style.display = isStdio ? "" : "none";
  if (envGroup) envGroup.style.display = isStdio ? "" : "none";
};

FKTeamsChat.prototype._saveMcpServer = function () {
  var servers = JSON.parse(JSON.stringify(this._getMcpServers()));
  var s = {
    name: document.getElementById("mcp-edit-name").value.trim(),
    desc: document.getElementById("mcp-edit-desc").value.trim(),
    enabled: true,
    transport_type: document.getElementById("mcp-edit-transport").value,
    url: document.getElementById("mcp-edit-url").value.trim(),
    command: document.getElementById("mcp-edit-command").value.trim(),
    args: document.getElementById("mcp-edit-args").value.split("\n").map(function (l) { return l.trim(); }).filter(Boolean),
    env_vars: document.getElementById("mcp-edit-env").value.split("\n").map(function (l) { return l.trim(); }).filter(Boolean),
    timeout: parseInt(document.getElementById("mcp-edit-timeout").value) || 30,
  };

  if (!s.name) { this.showNotification("名称不能为空", "error"); return; }

  if (this._mcpEditIdx >= 0 && this._mcpEditIdx < servers.length) {
    s.enabled = servers[this._mcpEditIdx].enabled;
    servers[this._mcpEditIdx] = s;
  } else {
    servers.push(s);
  }

  this._setMcpServers(servers);
  this._renderMcpList();
  this._closeMcpEditor();
  this.showNotification(s.name + " 已保存", "success");
};

FKTeamsChat.prototype._deleteMcpServer = function () {
  if (this._mcpEditIdx < 0) return;
  var servers = this._getMcpServers();
  if (this._mcpEditIdx >= servers.length) return;
  var name = servers[this._mcpEditIdx].name || "服务";
  if (!confirm("确认删除 " + name + "？")) return;
  servers = JSON.parse(JSON.stringify(servers));
  servers.splice(this._mcpEditIdx, 1);
  this._setMcpServers(servers);
  this._renderMcpList();
  this._closeMcpEditor();
  this.showNotification(name + " 已删除", "success");
};

FKTeamsChat.prototype._toggleMcpServer = function (idx) {
  var servers = this._getMcpServers();
  if (idx < 0 || idx >= servers.length) return;
  servers = JSON.parse(JSON.stringify(servers));
  servers[idx].enabled = !servers[idx].enabled;
  this._setMcpServers(servers);
  this._renderMcpList();
};
