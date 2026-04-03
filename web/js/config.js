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
        this.addModelCard({ name: "", provider: "openai", base_url: "", api_key: "", model: "" }, true);
    });

    // Auth 开关联动
    document.getElementById("config-auth-enabled").addEventListener("change", (e) => {
        document.getElementById("config-auth-fields").style.display = e.target.checked ? "block" : "none";
    });

    // SSH 开关联动
    document.getElementById("config-ssh-enabled").addEventListener("change", (e) => {
        document.getElementById("config-ssh-fields").style.display = e.target.checked ? "block" : "none";
    });

    // 点击背景关闭
    this.configModal.addEventListener("click", (e) => {
        if (e.target === this.configModal) this.closeConfig();
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
    document.getElementById("config-proxy-url").value = cfg.proxy?.url || "";
    document.getElementById("config-memory-enabled").checked = cfg.memory?.enabled || false;
    document.getElementById("config-server-host").value = cfg.server?.host || "";
    document.getElementById("config-server-port").value = cfg.server?.port || "";
    document.getElementById("config-server-loglevel").value = cfg.server?.log_level || "info";

    // Auth
    const authEnabled = cfg.server?.auth?.enabled || false;
    document.getElementById("config-auth-enabled").checked = authEnabled;
    document.getElementById("config-auth-fields").style.display = authEnabled ? "block" : "none";
    document.getElementById("config-auth-username").value = cfg.server?.auth?.username || "";
    document.getElementById("config-auth-password").value = cfg.server?.auth?.password || "";
    document.getElementById("config-auth-secret").value = cfg.server?.auth?.secret || "";

    // 智能体
    document.getElementById("config-agent-searcher").checked = cfg.agents?.searcher || false;
    document.getElementById("config-agent-assistant").checked = cfg.agents?.assistant || false;
    document.getElementById("config-agent-analyst").checked = cfg.agents?.analyst || false;

    // SSH
    const sshEnabled = cfg.agents?.ssh_visitor?.enabled || false;
    document.getElementById("config-ssh-enabled").checked = sshEnabled;
    document.getElementById("config-ssh-fields").style.display = sshEnabled ? "block" : "none";
    document.getElementById("config-ssh-host").value = cfg.agents?.ssh_visitor?.host || "";
    document.getElementById("config-ssh-username").value = cfg.agents?.ssh_visitor?.username || "";
    document.getElementById("config-ssh-password").value = cfg.agents?.ssh_visitor?.password || "";

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
    document.getElementById("config-qq-enabled").checked = ch.qq?.enabled || false;
    document.getElementById("config-qq-appid").value = ch.qq?.app_id || "";
    document.getElementById("config-qq-appsecret").value = ch.qq?.app_secret || "";
    document.getElementById("config-qq-mode").value = ch.qq?.mode || "team";
    document.getElementById("config-qq-sandbox").checked = ch.qq?.sandbox || false;

    // Discord
    document.getElementById("config-discord-enabled").checked = ch.discord?.enabled || false;
    document.getElementById("config-discord-token").value = ch.discord?.token || "";
    document.getElementById("config-discord-allowfrom").value = ch.discord?.allow_from || "";
    document.getElementById("config-discord-mode").value = ch.discord?.mode || "team";

    // 微信
    document.getElementById("config-weixin-enabled").checked = ch.weixin?.enabled || false;
    document.getElementById("config-weixin-baseurl").value = ch.weixin?.base_url || "";
    document.getElementById("config-weixin-credpath").value = ch.weixin?.cred_path || "";
    document.getElementById("config-weixin-allowfrom").value = ch.weixin?.allow_from || "";
    document.getElementById("config-weixin-mode").value = ch.weixin?.mode || "team";
    document.getElementById("config-weixin-loglevel").value = ch.weixin?.log_level || "info";
};

// ===== 模型卡片 =====

FKTeamsChat.prototype.addModelCard = function (m, expanded) {
    const card = document.createElement("div");
    card.className = "config-model-card" + (expanded ? " open" : "");

    const displayName = m.name || "未命名模型";
    const displayModel = m.model || "";
    const displayProvider = m.provider || "openai";

    card.innerHTML = `
    <div class="config-model-header">
      <div class="config-model-summary">
        <span class="config-model-title">${this.escapeHtml(displayName)}</span>
        <span class="config-model-info">${this.escapeHtml(displayProvider)}${displayModel ? " / " + this.escapeHtml(displayModel) : ""}</span>
      </div>
      <div class="config-model-header-actions">
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
          <input type="text" class="config-input model-model" value="${this.escapeHtml(m.model || "")}" placeholder="gpt-4o" />
        </div>
      </div>
      <div class="config-row">
        <div class="config-field">
          <label>密钥</label>
          <input type="password" class="config-input model-apikey" value="${this.escapeHtml(m.api_key || "")}" placeholder="sk-..." autocomplete="new-password" />
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
        if (e.target.closest(".config-model-remove")) return;
        card.classList.toggle("open");
    });

    // 删除
    card.querySelector(".config-model-remove").addEventListener("click", (e) => {
        e.stopPropagation();
        card.remove();
    });

    // 实时更新摘要
    const updateSummary = () => {
        const name = card.querySelector(".model-name").value.trim() || "未命名模型";
        const provider = card.querySelector(".model-provider").value;
        const model = card.querySelector(".model-model").value.trim();
        card.querySelector(".config-model-title").textContent = name;
        card.querySelector(".config-model-info").textContent = provider + (model ? " / " + model : "");
    };
    card.querySelector(".model-name").addEventListener("input", updateSummary);
    card.querySelector(".model-provider").addEventListener("change", updateSummary);
    card.querySelector(".model-model").addEventListener("input", updateSummary);

    this.configModelList.appendChild(card);

    // 转换 select 为自定义组件
    card.querySelectorAll("select.config-select").forEach((sel) => createCustomSelect(sel));
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
            .map((t) => `<span class="agent-card-tag tool-tag">${this.escapeHtml(t)}</span>`)
            .join("");
        const moreTag = (agent.tools || []).length > 4 ? `<span class="agent-card-tag tool-tag">+${agent.tools.length - 4}</span>` : "";

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
        this._customAgents.push({ name: "", desc: "", system_prompt: "", model: "default", tools: [] });
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
        .map((m) => `<option value="${this.escapeHtml(m.name)}" ${agent.model === m.name ? "selected" : ""}>${this.escapeHtml(m.name)}</option>`)
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
    overlay.querySelectorAll("select.config-select").forEach((sel) => createCustomSelect(sel));

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
            tools: Array.from(overlay.querySelectorAll(".config-tool-chip.selected")).map((c) => c.dataset.tool),
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

// ===== 收集并保存 =====

FKTeamsChat.prototype.collectConfigData = function () {
    const cfg = {};

    // 模型
    cfg.models = [];
    this.configModelList.querySelectorAll(".config-model-card").forEach((card) => {
        cfg.models.push({
            name: card.querySelector(".model-name").value.trim(),
            provider: card.querySelector(".model-provider").value,
            base_url: card.querySelector(".model-baseurl").value.trim(),
            api_key: card.querySelector(".model-apikey").value,
            model: card.querySelector(".model-model").value.trim(),
            extra_headers: card.querySelector(".model-extraheaders").value.trim(),
        });
    });

    // 代理
    cfg.proxy = { url: document.getElementById("config-proxy-url").value.trim() };

    // 记忆
    cfg.memory = { enabled: document.getElementById("config-memory-enabled").checked };

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
            allow_from: document.getElementById("config-discord-allowfrom").value.trim(),
            mode: document.getElementById("config-discord-mode").value,
        },
        weixin: {
            enabled: document.getElementById("config-weixin-enabled").checked,
            base_url: document.getElementById("config-weixin-baseurl").value.trim(),
            cred_path: document.getElementById("config-weixin-credpath").value.trim(),
            log_level: document.getElementById("config-weixin-loglevel").value,
            allow_from: document.getElementById("config-weixin-allowfrom").value.trim(),
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
        const resp = await this.fetchWithAuth("/api/fkteams/config", {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(cfg),
        });
        const result = await resp.json();
        if (result.code !== 0) {
            alert("保存失败: " + result.message);
            return;
        }

        // 刷新智能体列表
        this.loadAgents();

        // Auth 变更检测
        if (result.data?.auth_changed) {
            alert("认证配置已变更，请重新登录");
            localStorage.removeItem("fk_token");
            document.cookie = "fk_token=; path=/; max-age=0";
            window.location.href = "/login";
            return;
        }

        // 检测是否有需要重启才能生效的配置变更
        const needsRestart = this._detectRestartNeeded(oldCfg, cfg);

        this.closeConfig();
        if (needsRestart) {
            this._showRestartHint();
        } else {
            this.showNotification("配置已保存并生效", "success");
        }
    } catch (err) {
        alert("保存配置失败: " + err.message);
    } finally {
        this.configSaveBtn.disabled = false;
        this.configSaveBtn.textContent = "保存配置";
    }
};

// 检测是否有需要重启服务才能生效的配置变更（服务器、通道）
FKTeamsChat.prototype._detectRestartNeeded = function (oldCfg, newCfg) {
    if (!oldCfg) return false;
    // 服务器 host/port/log_level 变更
    const os = oldCfg.server || {}, ns = newCfg.server || {};
    if (os.host !== ns.host || os.port !== ns.port || os.log_level !== ns.log_level) return true;
    // 通道配置变更
    if (JSON.stringify(oldCfg.channels || {}) !== JSON.stringify(newCfg.channels || {})) return true;
    return false;
};

// 弹窗提示用户需要手动重启服务
FKTeamsChat.prototype._showRestartHint = function () {
    const overlay = document.createElement("div");
    overlay.style.cssText = "position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(40,35,25,0.4);backdrop-filter:blur(4px);z-index:1200;display:flex;align-items:center;justify-content:center;animation:fadeIn 0.15s ease;";

    const box = document.createElement("div");
    box.style.cssText = "background:var(--bg-secondary);border:2px solid var(--border-sketch);border-radius:16px;width:90%;max-width:420px;box-shadow:4px 6px 0px rgba(60,50,30,0.12);animation:sketchSlideUp 0.25s ease;";

    const header = document.createElement("div");
    header.style.cssText = "padding:16px 20px;border-bottom:2px dashed var(--border-color);";
    header.innerHTML = '<h3 style="margin:0;font-family:var(--font-hand);font-size:22px;font-weight:600;color:var(--warning,#f0a500);">提示</h3>';

    const body = document.createElement("div");
    body.style.cssText = "padding:20px;text-align:center;line-height:1.6;font-size:15px;color:var(--text-primary);";
    body.textContent = "配置已保存。服务器或通道相关配置的变更需要重启服务后才能生效。";

    const footer = document.createElement("div");
    footer.style.cssText = "display:flex;justify-content:flex-end;padding:16px 20px;border-top:2px dashed var(--border-color);";

    const okBtn = document.createElement("button");
    okBtn.textContent = "我知道了";
    okBtn.style.cssText = "padding:8px 20px;border:2px solid var(--accent-primary);border-radius:10px;font-size:14px;font-weight:500;cursor:pointer;background:var(--accent-primary);color:white;transition:all var(--transition);";
    okBtn.addEventListener("mouseenter", () => { okBtn.style.opacity = "0.85"; });
    okBtn.addEventListener("mouseleave", () => { okBtn.style.opacity = "1"; });

    const close = () => {
        overlay.style.animation = "fadeIn 0.15s ease reverse";
        setTimeout(() => overlay.remove(), 150);
    };
    okBtn.addEventListener("click", close);
    overlay.addEventListener("click", (e) => { if (e.target === overlay) close(); });

    footer.appendChild(okBtn);
    box.append(header, body, footer);
    overlay.appendChild(box);
    document.body.appendChild(overlay);
};
