import { Bot, Brain, Cable, FileCode2, Database, KeyRound, Layers, ListPlus, MessageSquare, RefreshCcw, Save, Server, Wrench } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { getConfig, getToolCatalog, saveConfig } from "@/api/config";
import { configActions, appActions } from "@/app/store";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { authRestoredEvent, clearServerAuthentication, expireAuthentication } from "@/lib/auth-session";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ConfirmDialog } from "@/components/ui/action-dialog";
import { Panel, PanelBody, PanelHeader } from "@/components/ui/panel";
import { cn } from "@/lib/cn";
import type { AppConfig } from "@/types/config";
import { knownTopLevelConfigKeys, approvalStoreOptions, type EditorProps, setServer, setAuth, setQQ, setDiscord, setWeixin, normalizeConfig, normalizeConfigForSave, normalizeApprovalStores } from "./configModel";
import { ModelsTab, AgentsTab, RoundtableTab, DeepTab } from "./AgentEditors";
import { ToolsTab, JavaScriptTab } from "./IntegrationEditors";
import { ChannelCard, SectionTitle, TextField, NumberField, SelectField, ModeField, ToggleField, StringListField, MetricCard } from "./ConfigFields";

export type ConfigTab = "models" | "server" | "agents" | "roundtable" | "deep" | "memory" | "channels" | "permissions" | "tools" | "javascript" | "other";

export const tabs: Array<{ key: ConfigTab; label: string; icon: typeof Bot }> = [
  { key: "models", label: "模型", icon: Bot },
  { key: "server", label: "服务", icon: Server },
  { key: "agents", label: "智能体", icon: Brain },
  { key: "roundtable", label: "圆桌", icon: ListPlus },
  { key: "deep", label: "深度", icon: Layers },
  { key: "memory", label: "记忆", icon: Database },
  { key: "channels", label: "通道", icon: MessageSquare },
  { key: "permissions", label: "权限", icon: KeyRound },
  { key: "tools", label: "工具", icon: Wrench },
  { key: "javascript", label: "扩展", icon: FileCode2 },
  { key: "other", label: "其他", icon: Cable },
];

export function ConfigPanel() {
  const dispatch = useAppDispatch();
  const persisted = useAppSelector((state) => state.config.value);
  const tools = useAppSelector((state) => state.config.tools);
  const [draft, setDraft] = useState<AppConfig | undefined>(persisted);
  const [activeTab, setActiveTab] = useState<ConfigTab>("models");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const modelIDs = useMemo(() => (draft?.models || []).map((model) => model.id).filter(Boolean), [draft?.models]);

  async function load() {
    setLoading(true);
    try {
      const [cfg, catalog] = await Promise.all([getConfig(), getToolCatalog().catch(() => [])]);
      dispatch(configActions.setConfig(cfg));
      dispatch(configActions.setTools(catalog));
      setDraft(normalizeConfig(cfg));
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    } finally {
      setLoading(false);
    }
  }

  async function persistConfig(nextDraft: AppConfig, message = "配置已保存") {
    setSaving(true);
    try {
      const result = await saveConfig(normalizeConfigForSave(nextDraft));
      dispatch(appActions.showToast(message));
      if (result.auth_changed && nextDraft.server?.auth?.enabled) {
        await clearServerAuthentication();
        expireAuthentication();
        return;
      }
      if (result.auth_changed) await clearServerAuthentication();
      await load();
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    } finally {
      setSaving(false);
    }
  }

  async function save() {
    if (!draft) return;
    await persistConfig(draft);
  }

  function updateDraft(mutator: (next: AppConfig) => void) {
    setDraft((current) => {
      const next = normalizeConfig(current || {});
      mutator(next);
      return next;
    });
  }

  useEffect(() => {
    void load();
  }, []);

  useEffect(() => {
    const onAuthRestored = () => void load();
    window.addEventListener(authRestoredEvent, onAuthRestored);
    return () => window.removeEventListener(authRestoredEvent, onAuthRestored);
  }, []);

  if (!draft) {
    return (
      <div className="flex h-full items-center justify-center p-3 sm:p-6">
        <Panel className="w-full max-w-sm">
          <PanelBody className="py-10 text-center text-muted-foreground">{loading ? "正在加载配置" : "暂无配置"}</PanelBody>
        </Panel>
      </div>
    );
  }

  return (
    <div className="chat-scroll h-full overflow-auto p-3 sm:p-6">
      <div className="mx-auto flex max-w-7xl flex-col gap-4">
        <Panel>
          <PanelHeader className="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
            <div>
              <div className="text-xl font-semibold">系统配置</div>
              <div className="mt-1 text-sm text-muted-foreground">使用结构化表单编辑配置，敏感字段留空会保留当前值。</div>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" onClick={load} disabled={loading || saving}>
                <RefreshCcw className="h-4 w-4" />
                重新加载
              </Button>
              <Button onClick={save} disabled={loading || saving}>
                <Save className="h-4 w-4" />
                保存配置
              </Button>
            </div>
          </PanelHeader>
          <PanelBody className="border-t border-border/70 p-0">
            <div className="chat-scroll flex gap-1 overflow-x-auto px-4 py-3">
              {tabs.map((tab) => {
                const Icon = tab.icon;
                return (
                  <button
                    key={tab.key}
                    className={cn(
                      "inline-flex h-10 shrink-0 items-center gap-2 rounded-lg border px-3 text-sm transition-colors",
                      activeTab === tab.key
                        ? "border-primary/50 bg-primary/10 text-primary"
                        : "border-transparent text-muted-foreground hover:border-border hover:bg-card",
                    )}
                    onClick={() => setActiveTab(tab.key)}
                  >
                    <Icon className="h-4 w-4" />
                    {tab.label}
                  </button>
                );
              })}
            </div>
          </PanelBody>
        </Panel>

        {activeTab === "models" ? <ModelsTab draft={draft} updateDraft={updateDraft} /> : null}
        {activeTab === "server" ? <ServerTab draft={draft} updateDraft={updateDraft} /> : null}
        {activeTab === "agents" ? <AgentsTab draft={draft} modelIDs={modelIDs} updateDraft={updateDraft} autoSaveDraft={(next) => persistConfig(next, "智能体配置已保存")} saving={saving} /> : null}
        {activeTab === "roundtable" ? <RoundtableTab draft={draft} modelIDs={modelIDs} updateDraft={updateDraft} /> : null}
        {activeTab === "deep" ? <DeepTab draft={draft} updateDraft={updateDraft} /> : null}
        {activeTab === "memory" ? <MemoryTab draft={draft} updateDraft={updateDraft} /> : null}
        {activeTab === "channels" ? <ChannelsTab draft={draft} updateDraft={updateDraft} /> : null}
        {activeTab === "permissions" ? <PermissionsTab draft={draft} updateDraft={updateDraft} autoSaveDraft={(next) => persistConfig(next, "权限配置已保存")} saving={saving} /> : null}
        {activeTab === "tools" ? <ToolsTab draft={draft} updateDraft={updateDraft} /> : null}
        {activeTab === "javascript" ? <JavaScriptTab draft={draft} updateDraft={updateDraft} /> : null}
        {activeTab === "other" ? <OtherTab draft={draft} toolsCount={tools.length} /> : null}
      </div>
    </div>
  );
}

export function ServerTab({ draft, updateDraft }: EditorProps) {
  const server = draft.server || {};
  const auth = server.auth || {};
  const openaiAPI = draft.openai_api || {};
  return (
    <div className="grid gap-4 xl:grid-cols-[1fr_420px]">
      <Panel>
        <PanelHeader>
          <SectionTitle icon={Server} title="服务配置" description="控制 Web/API 服务监听地址、日志和跨域来源。" />
        </PanelHeader>
        <PanelBody className="grid gap-4 md:grid-cols-2">
          <TextField label="监听地址" value={server.host} onChange={(value) => updateDraft((next) => setServer(next, { host: value }))} />
          <NumberField label="端口" value={server.port} onChange={(value) => updateDraft((next) => setServer(next, { port: value }))} />
          <SelectField
            label="日志级别"
            value={server.log_level}
            options={["debug", "info", "warn", "error"]}
            onChange={(value) => updateDraft((next) => setServer(next, { log_level: value }))}
          />
          <StringListField
            label="允许跨域来源"
            values={server.allow_origins || []}
            placeholder="http://localhost:5173"
            onChange={(values) => updateDraft((next) => setServer(next, { allow_origins: values }))}
          />
          <StringListField
            label="可信反向代理"
            values={server.trusted_proxies || []}
            placeholder="127.0.0.1 或 10.0.0.0/8（重启生效）"
            onChange={(values) => updateDraft((next) => setServer(next, { trusted_proxies: values }))}
          />
        </PanelBody>
      </Panel>

      <div className="space-y-4">
        <Panel>
          <PanelHeader>
            <SectionTitle icon={KeyRound} title="Web 认证" description="启用后访问 Web UI 需要登录。" />
          </PanelHeader>
          <PanelBody className="space-y-4">
            <ToggleField label="启用认证" checked={Boolean(auth.enabled)} onChange={(value) => updateDraft((next) => setAuth(next, { enabled: value }))} />
            <TextField label="用户名" value={auth.username} onChange={(value) => updateDraft((next) => setAuth(next, { username: value }))} />
            <TextField label="密码" type="password" value={auth.password} onChange={(value) => updateDraft((next) => setAuth(next, { password: value }))} />
            <TextField label="JWT Secret" type="password" value={auth.secret} onChange={(value) => updateDraft((next) => setAuth(next, { secret: value }))} />
          </PanelBody>
        </Panel>
        <Panel>
          <PanelHeader>
            <SectionTitle icon={Cable} title="OpenAI 兼容 API" description="用于兼容接口访问的 API Key。" />
          </PanelHeader>
          <PanelBody>
            <StringListField
              label="访问密钥"
              values={openaiAPI.api_keys || []}
              placeholder="sk-fkteams-..."
              secret
              onChange={(values) =>
                updateDraft((next) => {
                  next.openai_api = { ...(next.openai_api || {}), api_keys: values };
                })
              }
            />
          </PanelBody>
        </Panel>
      </div>
    </div>
  );
}

export function MemoryTab({ draft, updateDraft }: EditorProps) {
  return (
    <Panel>
      <PanelHeader>
        <SectionTitle icon={Database} title="长期记忆" description="控制是否启用长期记忆检索、注入和提取流程。" />
      </PanelHeader>
      <PanelBody>
        <ToggleField
          label="启用长期记忆"
          checked={Boolean(draft.memory?.enabled)}
          onChange={(value) =>
            updateDraft((next) => {
              next.memory = { ...(next.memory || {}), enabled: value };
            })
          }
        />
      </PanelBody>
    </Panel>
  );
}

export function ChannelsTab({ draft, updateDraft }: EditorProps) {
  const qq = draft.channels?.qq || {};
  const discord = draft.channels?.discord || {};
  const weixin = draft.channels?.weixin || {};
  return (
    <div className="grid gap-4 xl:grid-cols-3">
      <ChannelCard title="QQ" description="QQ 官方机器人通道">
        <ToggleField label="启用" checked={Boolean(qq.enabled)} onChange={(value) => updateDraft((next) => setQQ(next, { enabled: value }))} />
        <TextField label="App ID" value={qq.app_id} onChange={(value) => updateDraft((next) => setQQ(next, { app_id: value }))} />
        <TextField label="App Secret" type="password" value={qq.app_secret} onChange={(value) => updateDraft((next) => setQQ(next, { app_secret: value }))} />
        <ToggleField label="沙箱模式" checked={Boolean(qq.sandbox)} onChange={(value) => updateDraft((next) => setQQ(next, { sandbox: value }))} />
        <ModeField value={qq.mode} onChange={(value) => updateDraft((next) => setQQ(next, { mode: value }))} />
        {qq.mode === "agent" ? <TextField label="智能体 ID" value={qq.agent_id} onChange={(value) => updateDraft((next) => setQQ(next, { agent_id: value }))} /> : null}
      </ChannelCard>
      <ChannelCard title="Discord" description="Discord Bot 通道">
        <ToggleField label="启用" checked={Boolean(discord.enabled)} onChange={(value) => updateDraft((next) => setDiscord(next, { enabled: value }))} />
        <TextField label="Token" type="password" value={discord.token} onChange={(value) => updateDraft((next) => setDiscord(next, { token: value }))} />
        <TextField label="允许用户" value={discord.allow_from} placeholder="多个 ID 用逗号分隔" onChange={(value) => updateDraft((next) => setDiscord(next, { allow_from: value }))} />
        <ModeField value={discord.mode} onChange={(value) => updateDraft((next) => setDiscord(next, { mode: value }))} />
        {discord.mode === "agent" ? <TextField label="智能体 ID" value={discord.agent_id} onChange={(value) => updateDraft((next) => setDiscord(next, { agent_id: value }))} /> : null}
      </ChannelCard>
      <ChannelCard title="微信" description="iLinkAI 微信通道">
        <ToggleField label="启用" checked={Boolean(weixin.enabled)} onChange={(value) => updateDraft((next) => setWeixin(next, { enabled: value }))} />
        <TextField label="Base URL" value={weixin.base_url} onChange={(value) => updateDraft((next) => setWeixin(next, { base_url: value }))} />
        <TextField label="凭证路径" value={weixin.cred_path} onChange={(value) => updateDraft((next) => setWeixin(next, { cred_path: value }))} />
        <SelectField label="日志级别" value={weixin.log_level} options={["debug", "info", "warn", "error", "silent"]} onChange={(value) => updateDraft((next) => setWeixin(next, { log_level: value }))} />
        <TextField label="允许用户" value={weixin.allow_from} placeholder="多个 ID 用逗号分隔" onChange={(value) => updateDraft((next) => setWeixin(next, { allow_from: value }))} />
        <ModeField value={weixin.mode} onChange={(value) => updateDraft((next) => setWeixin(next, { mode: value }))} />
        {weixin.mode === "agent" ? <TextField label="智能体 ID" value={weixin.agent_id} onChange={(value) => updateDraft((next) => setWeixin(next, { agent_id: value }))} /> : null}
      </ChannelCard>
    </div>
  );
}

export function PermissionsTab({ draft, updateDraft, autoSaveDraft, saving }: EditorProps) {
  const autoApprove = normalizeApprovalStores(draft.tools?.approval?.auto_approve || []);
  const autoApproveAll = autoApprove.includes("all");
  const popupEnabled = !autoApproveAll;
  const [disableApprovalConfirmOpen, setDisableApprovalConfirmOpen] = useState(false);

  function buildApprovalDraft(stores: string[]) {
    const next = normalizeConfig(draft);
    next.tools = {
      ...(next.tools || {}),
      approval: {
        ...(next.tools?.approval || {}),
        auto_approve: normalizeApprovalStores(stores),
      },
    };
    return next;
  }

  function setAutoApprove(stores: string[]) {
    const nextDraft = buildApprovalDraft(stores);
    updateDraft((next) => {
      next.tools = nextDraft.tools;
    });
    void autoSaveDraft?.(nextDraft);
  }

  function setPopupEnabled(enabled: boolean) {
    if (enabled) {
      setAutoApprove(autoApprove.filter((store) => store !== "all"));
      return;
    }
    setDisableApprovalConfirmOpen(true);
  }

  function toggleStore(store: string, enabled: boolean) {
    const withoutAll = autoApprove.filter((item) => item !== "all");
    if (enabled) {
      setAutoApprove([...withoutAll, store]);
      return;
    }
    setAutoApprove(withoutAll.filter((item) => item !== store));
  }

  return (
    <div className="space-y-4">
      <Panel>
        <PanelHeader>
          <SectionTitle icon={KeyRound} title="权限审批" description="配置 Web 对话中的工具审批弹窗和自动允许类别。" />
        </PanelHeader>
        <PanelBody className="space-y-3">
          <ToggleField label={saving ? "弹出审批框（保存中）" : "弹出审批框"} checked={popupEnabled} disabled={saving} onChange={setPopupEnabled} />
          {!popupEnabled ? (
            <div className="rounded-lg border border-destructive/35 bg-destructive/5 px-3 py-2 text-sm leading-6 text-destructive">
              审批框已关闭，危险命令、外部文件访问、Git 写操作和任务分发会自动允许。
            </div>
          ) : null}
          <div className="grid gap-3 md:grid-cols-2">
            {approvalStoreOptions.map((option) => (
              <ToggleField
                key={option.value}
                label={`自动允许${option.label}`}
                checked={autoApproveAll || autoApprove.includes(option.value)}
                disabled={autoApproveAll || saving}
                onChange={(value) => toggleStore(option.value, value)}
              />
            ))}
          </div>
        </PanelBody>
      </Panel>
      <ConfirmDialog
        open={disableApprovalConfirmOpen}
        title="关闭审批框"
        description="关闭后，危险命令、外部文件访问、Git 写操作和任务分发将自动允许，不再等待人工确认。请仅在可信环境中使用。"
        confirmLabel="确认关闭"
        destructive
        onCancel={() => setDisableApprovalConfirmOpen(false)}
        onConfirm={() => {
          setAutoApprove(["all"]);
          setDisableApprovalConfirmOpen(false);
        }}
      />
    </div>
  );
}

export function OtherTab({ draft, toolsCount }: { draft: AppConfig; toolsCount: number }) {
  const unknownKeys = Object.keys(draft).filter((key) => !knownTopLevelConfigKeys.has(key));
  return (
    <Panel>
      <PanelHeader>
        <SectionTitle icon={Cable} title="其他信息" description="当前页面会保留未知顶层配置，但不提供原始文本编辑入口。" />
      </PanelHeader>
      <PanelBody className="grid gap-4 md:grid-cols-3">
        <MetricCard label="模型数量" value={draft.models?.length || 0} />
        <MetricCard label="工具组数量" value={toolsCount} />
        <MetricCard label="未知配置块" value={unknownKeys.length} />
        {unknownKeys.length ? (
          <div className="md:col-span-3">
            <div className="mb-2 text-sm text-muted-foreground">已保留的未知配置块</div>
            <div className="flex flex-wrap gap-2">
              {unknownKeys.map((key) => (
                <Badge key={key}>{key}</Badge>
              ))}
            </div>
          </div>
        ) : null}
      </PanelBody>
    </Panel>
  );
}
