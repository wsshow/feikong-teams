import {
  Bot,
  Brain,
  Cable,
  Check,
  ChevronDown,
  Copy,
  Database,
  KeyRound,
  Layers,
  ListPlus,
  MessageSquare,
  Plus,
  RefreshCcw,
  Save,
  Search,
  Server,
  Sparkles,
  Trash2,
  Wrench,
  X,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { generateAgentDrafts, rewriteText } from "@/api/ai";
import { isAbortError } from "@/api/client";
import { getConfig, getToolCatalog, saveConfig } from "@/api/config";
import { listProviderModels } from "@/api/providers";
import { configActions, appActions } from "@/app/store";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { authRestoredEvent, expireAuthentication } from "@/lib/auth-session";
import { setAuthToken } from "@/lib/storage";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { ConfirmDialog } from "@/components/ui/action-dialog";
import { LoadingSurface } from "@/components/ui/loading-surface";
import { Panel, PanelBody, PanelHeader } from "@/components/ui/panel";
import { cn } from "@/lib/cn";
import type {
  AppConfig,
  AgentConfig,
  ChannelDiscordConfig,
  ChannelQQConfig,
  ChannelWeixinConfig,
  DeepConfig,
  MCPServerConfig,
  ModelConfig,
  ServerAuthConfig,
  TeamMemberConfig,
  ToolInfo,
} from "@/types/config";

type ConfigTab = "models" | "server" | "agents" | "roundtable" | "deep" | "memory" | "channels" | "permissions" | "tools" | "other";

const tabs: Array<{ key: ConfigTab; label: string; icon: typeof Bot }> = [
  { key: "models", label: "模型", icon: Bot },
  { key: "server", label: "服务", icon: Server },
  { key: "agents", label: "智能体", icon: Brain },
  { key: "roundtable", label: "圆桌", icon: ListPlus },
  { key: "deep", label: "深度", icon: Layers },
  { key: "memory", label: "记忆", icon: Database },
  { key: "channels", label: "通道", icon: MessageSquare },
  { key: "permissions", label: "权限", icon: KeyRound },
  { key: "tools", label: "工具", icon: Wrench },
  { key: "other", label: "其他", icon: Cable },
];

const knownTopLevelConfigKeys = new Set([
  "models",
  "server",
  "agents",
  "tools",
  "channels",
  "memory",
  "openai_api",
  "roundtable",
  "deep",
]);

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
        expireAuthentication();
        return;
      }
      if (result.auth_changed) setAuthToken("");
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
        {activeTab === "other" ? <OtherTab draft={draft} toolsCount={tools.length} /> : null}
      </div>
    </div>
  );
}

function ModelsTab({ draft, updateDraft }: EditorProps) {
  const models = draft.models || [];
  const [modelLookup, setModelLookup] = useState<Record<number, ModelLookupState>>({});
  const [expandedModelIndex, setExpandedModelIndex] = useState<number | null>(null);

  function removeModel(index: number) {
    updateDraft((next) => {
      next.models = (next.models || []).filter((_, itemIndex) => itemIndex !== index);
    });
    setExpandedModelIndex((current) => {
      if (current === null) return null;
      if (current === index) return null;
      return current > index ? current - 1 : current;
    });
  }

  async function loadProviderModels(model: ModelConfig, index: number) {
    if (!model.provider) {
      setModelLookup((current) => ({ ...current, [index]: { error: "请先选择提供商" } }));
      return;
    }
    setModelLookup((current) => ({ ...current, [index]: { ...current[index], loading: true, error: undefined } }));
    try {
      const result = await listProviderModels({
        provider: model.provider,
        base_url: model.base_url,
        api_key: model.api_key,
        model_id: model.id,
        original_id: model.original_id,
        extra_headers: model.extra_headers,
      });
      const providerModels = result.map((item) => item.id).filter(Boolean);
      setModelLookup((current) => ({
        ...current,
        [index]: {
          loading: false,
          models: providerModels,
          error: providerModels.length ? undefined : "供应商没有返回可用模型",
        },
      }));
    } catch (error) {
      setModelLookup((current) => ({
        ...current,
        [index]: {
          ...current[index],
          loading: false,
          error: error instanceof Error ? error.message : String(error),
        },
      }));
    }
  }

  return (
    <Panel>
      <SectionHeader icon={Bot} title="模型池" description="模型通过稳定 ID 被智能体、圆桌和自定义配置引用。">
        <Button
          className="w-full sm:w-auto"
          variant="outline"
          onClick={() => {
            updateDraft((next) => {
              next.models = [
                ...(next.models || []),
                {
                  id: uniqueModelID(next.models || [], "main"),
                  name: uniqueName(next.models || [], "model"),
                  use_for: (next.models || []).some((item) => item.use_for?.includes("chat")) ? [] : ["chat"],
                  provider: "openai",
                  base_url: "",
                  api_key: "",
                  model: "",
                },
              ];
            });
            setExpandedModelIndex(models.length);
          }}
        >
          <Plus className="h-4 w-4" />
          添加模型
        </Button>
      </SectionHeader>
      <PanelBody className="grid gap-4 xl:grid-cols-2">
        {models.map((model, index) => {
          const expanded = expandedModelIndex === index;
          return (
            <ModelConfigCard
              key={`${model.original_id || model.id || "model"}-${index}`}
              model={model}
              expanded={expanded}
              onToggle={() => setExpandedModelIndex(expanded ? null : index)}
              onRemove={() => removeModel(index)}
            >
              <div className="grid gap-3 md:grid-cols-2">
                <TextField label="ID" value={model.id} onChange={(value) => updateModel(updateDraft, index, { id: value })} />
                <TextField label="显示名称" value={model.name} onChange={(value) => updateModel(updateDraft, index, { name: value })} />
                <SelectField
                  label="提供商"
                  value={model.provider}
                  options={["openai", "deepseek", "claude", "ollama", "ark", "gemini", "qwen", "openrouter", "copilot"]}
                  onChange={(value) => updateModel(updateDraft, index, { provider: value })}
                />
                <ModelNameField
                  index={index}
                  model={model}
                  state={modelLookup[index]}
                  onChange={(value) => updateModel(updateDraft, index, { model: value })}
                  onLoad={() => void loadProviderModels(model, index)}
                />
                <TextField label="Base URL" value={model.base_url} onChange={(value) => updateModel(updateDraft, index, { base_url: value })} />
                <ModelAPIKeyField model={model} onChange={(value) => updateModel(updateDraft, index, { api_key: value })} />
                <TextField
                  label="额外请求头"
                  value={model.extra_headers}
                  placeholder="X-Key: value, X-Trace: value"
                  onChange={(value) => updateModel(updateDraft, index, { extra_headers: value })}
                />
                <ModelUseField uses={model.use_for || []} onChange={(use_for) => updateModel(updateDraft, index, { use_for })} />
              </div>
            </ModelConfigCard>
          );
        })}
        {!models.length ? <EmptyState title="暂无模型配置" description="添加一个 chat 用途模型后即可开始使用。" /> : null}
      </PanelBody>
    </Panel>
  );
}

interface ModelLookupState {
  loading?: boolean;
  models?: string[];
  error?: string;
}

function ModelConfigCard({
  model,
  expanded,
  onToggle,
  onRemove,
  children,
}: {
  model: ModelConfig;
  expanded: boolean;
  onToggle: () => void;
  onRemove: () => void;
  children: React.ReactNode;
}) {
  return (
    <div className={cn("rounded-xl border border-border/75 bg-card/65 transition-colors", expanded && "xl:col-span-2")}>
      <div className="flex min-h-32 flex-col gap-3 p-4">
        <button type="button" className="flex min-w-0 flex-1 items-start gap-3 text-left" onClick={onToggle} aria-expanded={expanded}>
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <span className="truncate font-medium">{model.name || model.id || "未命名模型"}</span>
              <Badge>{model.provider || "provider"}</Badge>
              {model.use_for?.includes("chat") ? <Badge>chat</Badge> : null}
            </div>
            <div className="mt-1 text-xs text-muted-foreground">{model.id || "未设置 ID"}</div>
            <div className="mt-3 line-clamp-2 text-sm leading-6 text-muted-foreground">{model.model || "尚未指定供应商模型"}</div>
          </div>
          <ChevronDown className={cn("mt-1 h-4 w-4 shrink-0 text-muted-foreground transition-transform", expanded && "rotate-180")} />
        </button>
        <div className="grid grid-cols-2 gap-2 text-xs text-muted-foreground sm:grid-cols-4">
          <AgentSummaryMetric label="提供商" value={model.provider || "未设置"} />
          <AgentSummaryMetric label="模型" value={model.model || "未设置"} />
          <AgentSummaryMetric label="用途" value={modelUsesSummary(model.use_for || [])} />
          <AgentSummaryMetric label="密钥" value={model.has_api_key || model.api_key ? "已配置" : "未配置"} />
        </div>
        <div className="flex items-center justify-between gap-2">
          <Button size="sm" variant={expanded ? "secondary" : "outline"} onClick={onToggle}>
            {expanded ? "收起" : "编辑"}
          </Button>
          <Button size="icon" variant="ghost" onClick={onRemove} aria-label="删除">
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </div>
      {expanded ? <div className="border-t border-border/70 p-4">{children}</div> : null}
    </div>
  );
}

function modelUsesSummary(uses: string[]) {
  if (!uses.length) return "未设置";
  return uses.join(", ");
}

function ModelNameField({
  index,
  model,
  state,
  onChange,
  onLoad,
}: {
  index: number;
  model: ModelConfig;
  state?: ModelLookupState;
  onChange: (value: string) => void;
  onLoad: () => void;
}) {
  const providerModels = state?.models || [];
  const [open, setOpen] = useState(false);
  const [filtering, setFiltering] = useState(false);
  const filteredModels = providerModels.filter((name) => {
    const query = (model.model || "").trim().toLowerCase();
    return !query || name.toLowerCase().includes(query);
  });
  const visibleModels = filtering ? filteredModels : providerModels;

  useEffect(() => {
    if (!providerModels.length) {
      setOpen(false);
      return;
    }
    setFiltering(false);
    setOpen(true);
  }, [providerModels.length]);

  return (
    <Field label="模型">
      <div className="relative">
        <Input
          className="pr-20"
          value={model.model || ""}
          placeholder="gpt-5"
          onChange={(event) => {
            onChange(event.target.value);
            setFiltering(true);
            if (providerModels.length) setOpen(true);
          }}
          onFocus={() => {
            if (providerModels.length) {
              setFiltering(false);
              setOpen(true);
            }
          }}
          onBlur={() => {
            window.setTimeout(() => setOpen(false), 120);
          }}
        />
        {providerModels.length ? (
          <button
            type="button"
            className="absolute right-8 top-1/2 flex h-7 w-7 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent/70 hover:text-foreground"
            aria-label="展开供应商模型"
            title="展开供应商模型"
            onMouseDown={(event) => {
              event.preventDefault();
              setFiltering(false);
              setOpen((value) => !value);
            }}
          >
            <ChevronDown className={cn("h-4 w-4 transition-transform", open && "rotate-180")} />
          </button>
        ) : null}
        <button
          type="button"
          className="absolute right-1 top-1/2 flex h-7 w-7 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent/70 hover:text-foreground disabled:pointer-events-none disabled:opacity-45"
          aria-label="获取供应商模型"
          title="获取供应商模型"
          onClick={onLoad}
          disabled={state?.loading || !model.provider}
        >
          <RefreshCcw className={cn("h-4 w-4", state?.loading && "animate-spin")} />
        </button>
        {open && providerModels.length ? (
          <div className="sketch-surface absolute left-0 right-0 top-[calc(100%+0.4rem)] z-40 max-h-56 space-y-1 overflow-y-auto rounded-xl bg-card p-2 text-sm shadow-[0_14px_32px_hsl(218_30%_25%/0.16)]">
            {visibleModels.length ? visibleModels.map((name) => (
              <button
                key={name}
                type="button"
                className={cn(
                  "flex w-full items-center rounded-lg px-3 py-2 text-left transition-colors hover:bg-accent/70",
                  model.model === name && "bg-primary/10 text-primary",
                )}
                onMouseDown={(event) => {
                  event.preventDefault();
                  onChange(name);
                  setOpen(false);
                }}
              >
                <span className="min-w-0 truncate">{name}</span>
              </button>
            )) : (
              <div className="px-3 py-3 text-sm text-muted-foreground">没有匹配的模型</div>
            )}
          </div>
        ) : null}
      </div>
      {state?.error ? <div className="text-xs text-destructive">{state.error}</div> : null}
    </Field>
  );
}

function ServerTab({ draft, updateDraft }: EditorProps) {
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

function AgentsTab({ draft, modelIDs, updateDraft, autoSaveDraft, saving }: EditorProps & { modelIDs: string[] }) {
  const dispatch = useAppDispatch();
  const agents = draft.agents || {};
  const agentItems = agents.items || [];
  const tools = useAppSelector((state) => state.config.tools);
  const toolOptions = useMemo(() => buildToolOptions(tools, draft.tools?.mcp_servers || []), [draft.tools?.mcp_servers, tools]);
  const [assistantOpen, setAssistantOpen] = useState(false);
  const [expandedAgentKey, setExpandedAgentKey] = useState<string | null>(null);
  const [deleteAgentTarget, setDeleteAgentTarget] = useState<{ agent: AgentConfig; index: number; key: string } | null>(null);

  function buildAgentsDraft(items: AgentConfig[]) {
    const next = normalizeConfig(draft);
    const current = next.agents?.items || [];
    const merged = [...current];
    for (const item of items) {
      merged.push({
        ...item,
        id: uniqueAgentID(merged, item.id || item.name || "agent"),
        tools: item.tools || [],
        enabled: item.enabled ?? true,
      });
    }
    next.agents = { ...(next.agents || {}), items: merged };
    return next;
  }

  async function appendAgents(items: AgentConfig[]) {
    const nextDraft = buildAgentsDraft(items);
    updateDraft((next) => {
      next.agents = nextDraft.agents;
    });
    if (autoSaveDraft) {
      await autoSaveDraft(nextDraft);
      return;
    }
    dispatch(appActions.showToast(`${items.length} 个智能体草稿已添加`));
  }

  function confirmDeleteAgent() {
    if (!deleteAgentTarget) return;
    const target = deleteAgentTarget;
    updateDraft((next) => {
      next.agents = { ...(next.agents || {}), items: (next.agents?.items || []).filter((_, itemIndex) => itemIndex !== target.index) };
    });
    if (expandedAgentKey === target.key) {
      setExpandedAgentKey(null);
    }
    setDeleteAgentTarget(null);
  }

  function duplicateBuiltinAgent(agent: AgentConfig) {
    const nextID = uniqueAgentID(agentItems, `${agent.id || agent.name || "agent"}_custom`);
    const copy: AgentConfig = {
      ...agent,
      id: nextID,
      name: `${agent.name || agent.id || "智能体"} 副本`,
      builtin: undefined,
      team_member: undefined,
      enabled: true,
    };
    updateDraft((next) => {
      const items = next.agents?.items || [];
      next.agents = { ...(next.agents || {}), items: [...items, copy] };
    });
    setExpandedAgentKey(`${nextID}-${agentItems.length}`);
    dispatch(appActions.showToast("已复制为自定义智能体"));
  }

  return (
    <Panel>
      <SectionHeader icon={Brain} title="智能体目录" description="内置智能体只读展示；需要调整时可复制为自定义智能体。">
        <div className="grid w-full grid-cols-1 gap-2 sm:w-auto sm:grid-cols-2">
          <Button className="w-full sm:w-auto" variant="outline" onClick={() => setAssistantOpen(true)}>
            <Sparkles className="h-4 w-4" />
            AI 创建
          </Button>
          <Button
            className="w-full sm:w-auto"
            variant="outline"
            onClick={() =>
              updateDraft((next) => {
                const items = next.agents?.items || [];
                next.agents = {
                  ...(next.agents || {}),
                  items: [
                    ...items,
                    {
                      id: uniqueAgentID(items, "agent"),
                      name: "",
                      description: "",
                      prompt: "",
                      model_id: modelIDs[0] || "",
                      tools: [],
                      enabled: true,
                    },
                  ],
                };
              })
            }
          >
            <Plus className="h-4 w-4" />
            添加智能体
          </Button>
        </div>
      </SectionHeader>
      {assistantOpen ? (
        <AgentDraftDialog
          agents={agentItems}
          modelIDs={modelIDs}
          toolOptions={toolOptions}
          applying={Boolean(saving)}
          onClose={() => setAssistantOpen(false)}
          onApply={async (items) => {
            await appendAgents(items);
            setAssistantOpen(false);
          }}
        />
      ) : null}
      <ConfirmDialog
        open={Boolean(deleteAgentTarget)}
        title="删除智能体"
        description={
          <>
            智能体「<span className="font-medium text-foreground">{deleteAgentTarget?.agent.name || deleteAgentTarget?.agent.id || "未命名智能体"}</span>」将从当前配置草稿中移除，保存配置后生效。
          </>
        }
        confirmLabel="确认删除"
        destructive
        onCancel={() => setDeleteAgentTarget(null)}
        onConfirm={confirmDeleteAgent}
      />
      <PanelBody className="grid items-start gap-4 xl:grid-cols-2">
        {agentItems.map((agent, index) => {
          const agentKey = `${agent.id || agent.name || "agent"}-${index}`;
          const expanded = expandedAgentKey === agentKey;
          return (
            <AgentConfigCard
              key={agentKey}
              agent={agent}
              expanded={expanded}
              onToggle={() => setExpandedAgentKey(expanded ? null : agentKey)}
              onDuplicate={agent.builtin ? () => duplicateBuiltinAgent(agent) : undefined}
              onRemove={
                agent.builtin
                  ? undefined
                  : () => setDeleteAgentTarget({ agent, index, key: agentKey })
              }
            >
              <AgentCatalogEditor
                agent={agent}
                modelIDs={modelIDs}
                toolOptions={toolOptions}
                readOnly={Boolean(agent.builtin)}
                enabledReadOnly={Boolean(agent.builtin && agent.id === "coordinator")}
                onChange={(value) =>
                  updateDraft((next) => {
                    const items = [...(next.agents?.items || [])];
                    items[index] = value;
                    next.agents = { ...(next.agents || {}), items };
                  })
                }
              />
            </AgentConfigCard>
          );
        })}
        {agentItems.length === 0 ? (
          <div className="rounded-xl border border-dashed border-border p-8 text-center text-sm text-muted-foreground xl:col-span-2">暂无智能体配置，重新加载配置会从后端获取内置智能体默认信息。</div>
        ) : null}
      </PanelBody>
    </Panel>
  );
}

function AgentConfigCard({
  agent,
  expanded,
  onToggle,
  onDuplicate,
  onRemove,
  children,
}: {
  agent: AgentConfig;
  expanded: boolean;
  onToggle: () => void;
  onDuplicate?: () => void;
  onRemove?: () => void;
  children: React.ReactNode;
}) {
  const toolCount = agent.tools?.length || 0;
  return (
    <div className={cn("rounded-xl border border-border/75 bg-card/65 transition-colors", expanded && "xl:col-span-2")}>
      <div className="flex min-h-36 flex-col gap-3 p-4">
        <button type="button" className="flex min-w-0 flex-1 items-start gap-3 text-left" onClick={onToggle} aria-expanded={expanded}>
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <span className="truncate font-medium">{agent.name || agent.id || "未命名智能体"}</span>
              <Badge>{agent.builtin ? "内置" : "自定义"}</Badge>
              <Badge>{agent.enabled ? "已启用" : "已关闭"}</Badge>
            </div>
            <div className="mt-1 text-xs text-muted-foreground">{agent.id || "未设置 ID"}</div>
            <div className="mt-3 line-clamp-2 text-sm leading-6 text-muted-foreground">{agent.description || "暂无描述"}</div>
          </div>
          <ChevronDown className={cn("mt-1 h-4 w-4 shrink-0 text-muted-foreground transition-transform", expanded && "rotate-180")} />
        </button>
        <div className="grid grid-cols-2 gap-2 text-xs text-muted-foreground sm:grid-cols-4">
          <AgentSummaryMetric label="类型" value={agent.builtin ? "内置" : agent.team_member ? "团队" : "自定义"} />
          <AgentSummaryMetric label="模型" value={agent.model_id || "默认"} />
          <AgentSummaryMetric label="工具" value={`${toolCount}`} />
          <AgentSummaryMetric label="状态" value={agent.enabled ? "启用" : "关闭"} />
        </div>
        <div className="flex items-center justify-between gap-2">
          <Button size="sm" variant={expanded ? "secondary" : "outline"} onClick={onToggle}>
            {expanded ? "收起" : agent.builtin ? "查看" : "编辑"}
          </Button>
          <div className="flex items-center gap-1">
            {onDuplicate ? (
              <Button size="icon" variant="ghost" onClick={onDuplicate} aria-label="复制为自定义">
                <Copy className="h-4 w-4" />
              </Button>
            ) : null}
            {onRemove ? (
              <Button size="icon" variant="ghost" onClick={onRemove} aria-label="删除">
                <Trash2 className="h-4 w-4" />
              </Button>
            ) : null}
          </div>
        </div>
      </div>
      {expanded ? <div className="border-t border-border/70 p-4">{children}</div> : null}
    </div>
  );
}

function AgentSummaryMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-lg border border-border/60 bg-background/45 px-2 py-1.5">
      <div>{label}</div>
      <div className="mt-1 truncate text-foreground">{value}</div>
    </div>
  );
}

function RoundtableTab({ draft, modelIDs, updateDraft }: EditorProps & { modelIDs: string[] }) {
  const roundtable = draft.roundtable || {};
  return (
    <Panel>
      <SectionHeader icon={ListPlus} title="圆桌讨论" description="配置 roundtable 模式成员和最大迭代次数。">
        <Button
          className="w-full sm:w-auto"
          variant="outline"
          onClick={() =>
            updateDraft((next) => {
              const members = next.roundtable?.members || [];
              next.roundtable = {
                ...(next.roundtable || {}),
                members: [...members, { id: uniqueMemberID(members, "member"), name: "", description: "", model_id: modelIDs[0] || "", prompt: "" }],
              };
            })
          }
        >
          <Plus className="h-4 w-4" />
          添加成员
        </Button>
      </SectionHeader>
      <PanelBody className="grid gap-4 xl:grid-cols-2">
        <div className="xl:col-span-2">
          <NumberField
            label="最大迭代次数"
            value={roundtable.max_iterations}
            min={0}
            onChange={(value) =>
              updateDraft((next) => {
                next.roundtable = { ...(next.roundtable || {}), max_iterations: value };
              })
            }
          />
        </div>
        {(roundtable.members || []).map((member, index) => (
          <RoundtableMemberEditor key={index} member={member} index={index} modelIDs={modelIDs} updateDraft={updateDraft} />
        ))}
      </PanelBody>
    </Panel>
  );
}

function DeepTab({ draft, updateDraft }: EditorProps) {
  const deep = draft.deep || {};
  const tools = useAppSelector((state) => state.config.tools);
  const toolOptions = useMemo(() => buildToolOptions(tools, draft.tools?.mcp_servers || []), [draft.tools?.mcp_servers, tools]);

  function update(patch: DeepConfig) {
    updateDraft((next) => {
      next.deep = mergeDeepConfig(next.deep || {}, patch);
    });
  }

  return (
    <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_420px]">
      <Panel>
        <PanelHeader>
          <SectionTitle icon={Layers} title="深度智能体" description="配置深度模式的提示词、工具和基础执行能力。" />
        </PanelHeader>
        <PanelBody className="space-y-4">
          <div className="max-w-sm">
            <NumberField
              label="最大迭代次数"
              value={deep.max_iterations}
              min={0}
              onChange={(value) => update({ max_iterations: value })}
            />
          </div>
          <Field label="系统提示词（留空使用内置深度提示词）">
            <Textarea className="min-h-72 text-sm" value={deep.instruction || ""} onChange={(event) => update({ instruction: event.target.value })} />
          </Field>
          <ToolSelectField tools={deep.extra_tools || []} options={toolOptions} onChange={(extraTools) => update({ extra_tools: extraTools })} />
        </PanelBody>
      </Panel>

      <div className="space-y-4">
        <Panel>
          <PanelHeader>
            <SectionTitle icon={Check} title="能力配置" description="选择深度模式可直接使用的核心能力。" />
          </PanelHeader>
          <PanelBody className="space-y-3">
            <ToggleField label="计划清单" checked={Boolean(deep.planning?.enabled)} onChange={(value) => update({ planning: { enabled: value } })} />
            <ToggleField label="工作区文件" checked={Boolean(deep.workspace?.enabled)} onChange={(value) => update({ workspace: { enabled: value } })} />
            <ToggleField label="Shell 命令" checked={Boolean(deep.shell?.enabled)} onChange={(value) => update({ shell: { enabled: value } })} />
          </PanelBody>
        </Panel>
      </div>
    </div>
  );
}

function MemoryTab({ draft, updateDraft }: EditorProps) {
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

function ChannelsTab({ draft, updateDraft }: EditorProps) {
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

const approvalStoreOptions = [
  { value: "command", label: "命令执行" },
  { value: "file", label: "外部文件" },
  { value: "git", label: "Git 操作" },
  { value: "dispatch", label: "任务分发" },
];

function PermissionsTab({ draft, updateDraft, autoSaveDraft, saving }: EditorProps) {
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

function ToolsTab({ draft, updateDraft }: EditorProps) {
  const tools = useAppSelector((state) => state.config.tools);
  const builtinTools = tools.filter((tool) => tool.builtin !== false);
  const mcpTools = tools.filter((tool) => tool.builtin === false);
  const mcpServers = draft.tools?.mcp_servers || [];
  return (
    <div className="space-y-4">
      <Panel>
        <PanelHeader>
          <SectionTitle icon={Wrench} title="内置工具" description={`${builtinTools.length} 个内置工具组，可用于智能体工具配置。`} />
        </PanelHeader>
        <PanelBody className="grid gap-3 xl:grid-cols-2">
          {builtinTools.map((tool) => (
            <ToolInfoCard key={tool.name} tool={tool} />
          ))}
        </PanelBody>
      </Panel>
      <Panel>
        <SectionHeader icon={Cable} title="MCP 工具" description="配置 HTTP 或 stdio MCP 服务，启用后可在智能体工具中选择。">
          <Button
            className="w-full sm:w-auto"
            variant="outline"
            onClick={() =>
              updateDraft((next) => {
                const servers = next.tools?.mcp_servers || [];
                next.tools = {
                  ...(next.tools || {}),
                  mcp_servers: [
                    ...servers,
                    { id: uniqueMCPServerID(servers, "mcp"), name: "", description: "", enabled: false, timeout: "30s", transport: "http", url: "" },
                  ],
                };
              })
            }
          >
            <Plus className="h-4 w-4" />
            添加 MCP
          </Button>
        </SectionHeader>
        <PanelBody className="grid gap-4 xl:grid-cols-2">
          {mcpServers.map((server, index) => (
            <MCPServerEditor key={index} server={server} index={index} updateDraft={updateDraft} />
          ))}
          {mcpServers.length === 0 ? (
            <div className="rounded-xl border border-dashed border-border p-8 text-center text-sm text-muted-foreground xl:col-span-2">暂无 MCP 服务配置。</div>
          ) : null}
        </PanelBody>
      </Panel>
      {mcpTools.length ? (
        <Panel>
          <PanelHeader>
            <SectionTitle icon={Cable} title="已加载 MCP 工具" description={`${mcpTools.length} 个来自已启用 MCP 服务的工具组。`} />
          </PanelHeader>
          <PanelBody className="grid gap-3 xl:grid-cols-2">
            {mcpTools.map((tool) => (
              <ToolInfoCard key={tool.name} tool={tool} />
            ))}
          </PanelBody>
        </Panel>
      ) : null}
    </div>
  );
}

function ToolInfoCard({ tool }: { tool: ToolInfo }) {
  return (
    <div className="rounded-xl border border-border/75 bg-card/65 p-4">
      <div className="flex items-center justify-between gap-2">
        <div className="font-medium">{tool.display_name || tool.name}</div>
        <div className="flex gap-1">
          {tool.read_only ? <Badge>只读</Badge> : null}
          {tool.destructive ? <Badge>破坏性</Badge> : null}
          <Badge>{tool.category || "tool"}</Badge>
        </div>
      </div>
      <div className="mt-2 text-sm text-muted-foreground">{tool.description || "暂无描述"}</div>
      {tool.included_tools?.length ? (
        <div className="mt-3 flex flex-wrap gap-1">
          {tool.included_tools.map((name) => (
            <Badge key={name}>{name}</Badge>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function OtherTab({ draft, toolsCount }: { draft: AppConfig; toolsCount: number }) {
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

interface EditorProps {
  draft: AppConfig;
  updateDraft: (mutator: (next: AppConfig) => void) => void;
  autoSaveDraft?: (next: AppConfig) => Promise<void>;
  saving?: boolean;
}

function RoundtableMemberEditor({
  member,
  index,
  modelIDs,
  updateDraft,
}: {
  member: TeamMemberConfig;
  index: number;
  modelIDs: string[];
  updateDraft: EditorProps["updateDraft"];
}) {
  function update(patch: Partial<TeamMemberConfig>) {
    updateDraft((next) => {
      const members = [...(next.roundtable?.members || [])];
      members[index] = { ...members[index], ...patch };
      next.roundtable = { ...(next.roundtable || {}), members };
    });
  }

  return (
    <ConfigCard
      title={member.name || `成员 ${index + 1}`}
      aside={member.model_id || "model_id"}
      onRemove={() =>
        updateDraft((next) => {
          next.roundtable = { ...(next.roundtable || {}), members: (next.roundtable?.members || []).filter((_, itemIndex) => itemIndex !== index) };
        })
      }
    >
      <div className="grid gap-3 md:grid-cols-3">
        <TextField label="ID" value={member.id} onChange={(value) => update({ id: value })} />
        <TextField label="名称" value={member.name} onChange={(value) => update({ name: value })} />
        <ModelSelect label="模型 ID" value={member.model_id} modelIDs={modelIDs} onChange={(value) => update({ model_id: value })} />
      </div>
      <TextField label="描述" value={member.description} onChange={(value) => update({ description: value })} />
      <Field label="提示词（留空使用内置圆桌讨论提示词）">
        <Textarea className="min-h-28 text-sm" value={member.prompt || ""} onChange={(event) => update({ prompt: event.target.value })} />
      </Field>
    </ConfigCard>
  );
}

function AgentCatalogEditor({
  agent,
  modelIDs,
  toolOptions,
  readOnly = false,
  enabledReadOnly = false,
  onChange,
}: {
  agent: AgentConfig;
  modelIDs: string[];
  toolOptions: ToolSelectOption[];
  readOnly?: boolean;
  enabledReadOnly?: boolean;
  onChange: (value: AgentConfig) => void;
}) {
  const usesSSH = (agent.tools || []).includes("ssh");
  const ssh = agent.ssh || {};
  return (
    <div className="grid gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <Badge>{agent.builtin ? "内置" : "自定义"}</Badge>
        {agent.team_member ? <Badge>团队成员</Badge> : null}
        <Badge>{agent.enabled ? "已启用" : "已关闭"}</Badge>
        {agent.builtin && agent.id === "coordinator" ? <Badge>核心入口</Badge> : null}
        {readOnly ? <Badge>只读</Badge> : null}
      </div>
      <ToggleField label="启用智能体" checked={Boolean(agent.enabled)} disabled={enabledReadOnly} onChange={(value) => onChange({ ...agent, enabled: value })} />
      <div className="grid gap-3 md:grid-cols-3">
        <TextField
          label="ID"
          value={agent.id}
          onChange={(value) => onChange({ ...agent, id: value })}
          disabled={readOnly || Boolean(agent.builtin)}
        />
        <TextField label="名称" value={agent.name} disabled={readOnly} onChange={(value) => onChange({ ...agent, name: value })} />
        <ModelSelect label="模型 ID（可选）" value={agent.model_id} modelIDs={["", ...modelIDs]} disabled={readOnly} onChange={(value) => onChange({ ...agent, model_id: value })} />
      </div>
      <AITextField
        label="描述"
        scenario="agent_description"
        value={agent.description}
        context={{ agent_id: agent.id, agent_name: agent.name }}
        disabled={readOnly}
        onChange={(value) => onChange({ ...agent, description: value })}
      />
      <ToolSelectField
        tools={agent.tools || []}
        options={toolOptions}
        disabled={readOnly}
        onChange={(tools) => onChange({ ...agent, tools, ssh: tools.includes("ssh") ? agent.ssh : undefined })}
      />
      {usesSSH ? (
        <div className="grid gap-3 rounded-xl border border-border/75 bg-background/45 p-3 md:grid-cols-3">
          <TextField label="SSH 主机" value={ssh.host} placeholder="ip:port" disabled={readOnly} onChange={(value) => onChange({ ...agent, ssh: { ...(agent.ssh || {}), host: value } })} />
          <TextField label="SSH 用户名" value={ssh.username} disabled={readOnly} onChange={(value) => onChange({ ...agent, ssh: { ...(agent.ssh || {}), username: value } })} />
          <TextField
            label="SSH 密码"
            type="password"
            value={ssh.password}
            disabled={readOnly}
            onChange={(value) => onChange({ ...agent, ssh: { ...(agent.ssh || {}), password: value } })}
          />
        </div>
      ) : null}
      <Field
        label="系统提示词"
        action={readOnly ? undefined : (
          <AIRewriteButton
            scenario="agent_prompt"
            value={agent.prompt || ""}
            context={{ agent_id: agent.id, agent_name: agent.name, description: agent.description, tools: agent.tools || [] }}
            onApply={(value) => onChange({ ...agent, prompt: value })}
          />
        )}
      >
        <Textarea className="min-h-56 text-sm" value={agent.prompt || ""} readOnly={readOnly} onChange={(event) => onChange({ ...agent, prompt: event.target.value })} />
      </Field>
    </div>
  );
}

function AgentDraftDialog({
  agents,
  modelIDs,
  toolOptions,
  applying = false,
  onClose,
  onApply,
}: {
  agents: AgentConfig[];
  modelIDs: string[];
  toolOptions: ToolSelectOption[];
  applying?: boolean;
  onClose: () => void;
  onApply: (agents: AgentConfig[]) => void | Promise<void>;
}) {
  const [instruction, setInstruction] = useState("");
  const [drafts, setDrafts] = useState<AgentConfig[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const abortControllerRef = useRef<AbortController | null>(null);

  async function generate() {
    const trimmed = instruction.trim();
    if (!trimmed || loading) return;
    const controller = new AbortController();
    abortControllerRef.current = controller;
    setLoading(true);
    setError("");
    try {
      const resp = await generateAgentDrafts(
        {
          instruction: trimmed,
          existing_agents: agents.map((agent) => agent.id || agent.name || "").filter(Boolean),
          available_tools: toolOptions.map((tool) => tool.name),
          available_models: modelIDs,
          default_model_id: modelIDs[0] || "",
        },
        { signal: controller.signal },
      );
      const items = resp.agents || [];
      setDrafts(items);
      setSelected(new Set(items.map((agent) => agent.id || agent.name || "")));
    } catch (err) {
      if (isAbortError(err)) {
        setError("");
        return;
      }
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      if (abortControllerRef.current === controller) {
        abortControllerRef.current = null;
      }
      setLoading(false);
    }
  }

  function cancelGenerate() {
    abortControllerRef.current?.abort();
  }

  function toggle(id: string) {
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }

  const selectedDrafts = drafts.filter((agent) => selected.has(agent.id || agent.name || ""));
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-foreground/15 p-3 backdrop-blur-[1px] sm:p-6" role="dialog" aria-modal="true">
      <div className="sketch-surface relative flex max-h-[calc(100dvh-1.5rem)] w-full max-w-3xl flex-col overflow-hidden rounded-2xl bg-card/95 shadow-[0_18px_48px_hsl(218_30%_20%/0.18)]">
        {loading ? (
          <div className="absolute inset-0 z-10 flex items-center justify-center bg-background/75 backdrop-blur-[1px]">
            <div className="flex flex-col items-center gap-3">
              <LoadingSurface label="正在生成智能体草稿" />
              <Button variant="outline" onClick={cancelGenerate}>
                取消生成
              </Button>
            </div>
          </div>
        ) : null}
        <div className="flex items-start justify-between gap-3 border-b border-border/70 p-4">
          <div className="min-w-0">
            <div className="flex items-center gap-2 font-semibold">
              <Sparkles className="h-4 w-4 text-primary" />
              AI 创建智能体
            </div>
            <div className="mt-1 text-sm leading-6 text-muted-foreground">描述你需要的角色、数量、职责和工具偏好，生成后确认添加到当前配置草稿。</div>
          </div>
          <Button size="icon" variant="ghost" disabled={loading} onClick={onClose} aria-label="关闭">
            <X className="h-4 w-4" />
          </Button>
        </div>

        <div className={cn("chat-scroll min-h-0 flex-1 p-4", loading ? "overflow-hidden" : "overflow-auto")}>
          <div className="grid gap-4">
            <Field label="创建要求">
              <Textarea
                className="min-h-28 text-sm"
                disabled={loading}
                value={instruction}
                placeholder="例如：创建三个研发协作智能体，分别负责前端实现、后端接口和测试验收，提示词要明确边界和输出格式。"
                onChange={(event) => setInstruction(event.target.value)}
              />
            </Field>
            {error ? <div className="rounded-lg border border-destructive/40 bg-destructive/5 px-3 py-2 text-sm text-destructive">{error}</div> : null}
            {drafts.length ? (
              <div className="grid gap-3">
                <div className="text-sm text-muted-foreground">生成结果</div>
                {drafts.map((agent) => {
                  const id = agent.id || agent.name || "";
                  const checked = selected.has(id);
                  return (
                    <label key={id} className="flex cursor-pointer gap-3 rounded-xl border border-border/75 bg-background/45 p-3">
                      <input className="mt-1 h-4 w-4 shrink-0 accent-primary" type="checkbox" disabled={loading} checked={checked} onChange={() => toggle(id)} />
                      <span className="min-w-0 flex-1">
                        <span className="flex min-w-0 flex-wrap items-center gap-2">
                          <span className="font-medium">{agent.name || id}</span>
                          <Badge>{id}</Badge>
                          {agent.model_id ? <Badge>{agent.model_id}</Badge> : null}
                        </span>
                        <span className="mt-1 block text-sm leading-6 text-muted-foreground">{agent.description}</span>
                        {agent.tools?.length ? <span className="mt-2 block text-xs text-muted-foreground">工具：{agent.tools.join("、")}</span> : null}
                        {agent.prompt ? <span className="mt-2 line-clamp-3 block text-xs leading-5 text-muted-foreground">{agent.prompt}</span> : null}
                      </span>
                    </label>
                  );
                })}
              </div>
            ) : null}
          </div>
        </div>

        <div className="flex flex-col gap-2 border-t border-border/70 p-4 sm:flex-row sm:justify-end">
          <Button variant="outline" disabled={loading} onClick={onClose}>
            取消
          </Button>
          <Button variant="outline" disabled={!instruction.trim() || loading} onClick={() => void generate()}>
            <Sparkles className="h-4 w-4" />
            {loading ? "生成中" : "生成草稿"}
          </Button>
          <Button disabled={!selectedDrafts.length || loading || applying} onClick={() => void onApply(selectedDrafts)}>
            <Plus className="h-4 w-4" />
            {applying ? "保存中" : "添加选中"}
          </Button>
        </div>
      </div>
    </div>
  );
}

function AITextField({
  label,
  scenario,
  value,
  context,
  onChange,
  disabled,
}: {
  label: string;
  scenario: string;
  value?: string;
  context?: Record<string, unknown>;
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  return (
    <Field label={label} action={disabled ? undefined : <AIRewriteButton scenario={scenario} value={value || ""} context={context} onApply={onChange} />}>
      <Input value={value || ""} disabled={disabled} onChange={(event) => onChange(event.target.value)} />
    </Field>
  );
}

function AIRewriteButton({
  scenario,
  value,
  context,
  onApply,
}: {
  scenario: string;
  value: string;
  context?: Record<string, unknown>;
  onApply: (value: string) => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button size="sm" variant="ghost" onClick={() => setOpen(true)}>
        <Sparkles className="h-4 w-4" />
        AI 修改
      </Button>
      {open ? <AIRewriteDialog scenario={scenario} value={value} context={context} onApply={onApply} onClose={() => setOpen(false)} /> : null}
    </>
  );
}

function AIRewriteDialog({
  scenario,
  value,
  context,
  onApply,
  onClose,
}: {
  scenario: string;
  value: string;
  context?: Record<string, unknown>;
  onApply: (value: string) => void;
  onClose: () => void;
}) {
  const [instruction, setInstruction] = useState("");
  const [result, setResult] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function runRewrite() {
    const trimmed = instruction.trim();
    if (!trimmed || loading) return;
    setLoading(true);
    setError("");
    try {
      const resp = await rewriteText({ scenario, instruction: trimmed, text: value, context });
      setResult(resp.text || "");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }

  function apply() {
    if (!result.trim()) return;
    onApply(result);
    onClose();
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-foreground/15 p-3 backdrop-blur-[1px] sm:p-6" role="dialog" aria-modal="true">
      <div className="sketch-surface flex max-h-[calc(100dvh-1.5rem)] w-full max-w-2xl flex-col overflow-hidden rounded-2xl bg-card/95 shadow-[0_18px_48px_hsl(218_30%_20%/0.18)]">
        <div className="flex items-start justify-between gap-3 border-b border-border/70 p-4">
          <div>
            <div className="flex items-center gap-2 font-semibold">
              <Sparkles className="h-4 w-4 text-primary" />
              AI 修改
            </div>
            <div className="mt-1 text-sm leading-6 text-muted-foreground">输入修改要求，生成结果确认后再应用到字段。</div>
          </div>
          <Button size="icon" variant="ghost" onClick={onClose} aria-label="关闭">
            <X className="h-4 w-4" />
          </Button>
        </div>
        <div className="chat-scroll min-h-0 flex-1 space-y-4 overflow-auto p-4">
          <Field label="修改要求">
            <Textarea className="min-h-24 text-sm" value={instruction} placeholder="例如：更专业、更简洁，并增加输出边界。" onChange={(event) => setInstruction(event.target.value)} />
          </Field>
          <Field label="当前内容">
            <Textarea className="min-h-28 text-sm" value={value} readOnly />
          </Field>
          {error ? <div className="rounded-lg border border-destructive/40 bg-destructive/5 px-3 py-2 text-sm text-destructive">{error}</div> : null}
          {result ? (
            <Field label="生成结果">
              <Textarea className="min-h-40 text-sm" value={result} onChange={(event) => setResult(event.target.value)} />
            </Field>
          ) : null}
        </div>
        <div className="flex flex-col gap-2 border-t border-border/70 p-4 sm:flex-row sm:justify-end">
          <Button variant="outline" onClick={onClose}>
            取消
          </Button>
          <Button variant="outline" disabled={!instruction.trim() || loading} onClick={() => void runRewrite()}>
            <Sparkles className="h-4 w-4" />
            {loading ? "生成中" : "生成修改"}
          </Button>
          <Button disabled={!result.trim() || loading} onClick={apply}>
            应用
          </Button>
        </div>
      </div>
    </div>
  );
}

interface ToolSelectOption {
  name: string;
  label: string;
  description?: string;
  category?: string;
  source: "builtin" | "mcp";
  readOnly?: boolean;
  destructive?: boolean;
  enabled?: boolean;
}

function ToolSelectField({
  tools,
  options,
  onChange,
  disabled,
}: {
  tools: string[];
  options: ToolSelectOption[];
  onChange: (tools: string[]) => void;
  disabled?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const selectedTools = uniqueToolNames(tools);
  const selectedSet = new Set(selectedTools);
  const optionByName = new Map(options.map((option) => [option.name, option]));
  const normalizedQuery = query.trim().toLowerCase();
  const visibleOptions = options.filter((option) => {
    if (!normalizedQuery) return true;
    return `${option.name} ${option.label} ${option.description || ""} ${option.category || ""}`.toLowerCase().includes(normalizedQuery);
  });
  const canAddCustom = Boolean(query.trim() && !selectedSet.has(query.trim()) && !optionByName.has(query.trim()));

  function addTool(name: string) {
    const trimmed = name.trim();
    if (!trimmed || selectedSet.has(trimmed)) return;
    onChange([...selectedTools, trimmed]);
    setQuery("");
  }

  function removeTool(name: string) {
    onChange(selectedTools.filter((tool) => tool !== name));
  }

  function toggleTool(name: string) {
    if (selectedSet.has(name)) {
      removeTool(name);
      return;
    }
    addTool(name);
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-3">
        <div className="text-sm text-muted-foreground">工具</div>
        <Button variant="outline" size="sm" disabled={disabled} onClick={() => setOpen((value) => !value)}>
          <Plus className="h-4 w-4" />
          添加工具
        </Button>
      </div>

      <div className="min-h-12 rounded-xl border border-border/75 bg-background/45 p-2">
        {selectedTools.length ? (
          <div className="flex flex-wrap gap-2">
            {selectedTools.map((tool) => {
              const option = optionByName.get(tool);
              return (
                <span
                  key={tool}
                  className="inline-flex max-w-full items-center gap-1.5 rounded-lg border border-border/80 bg-card/85 px-2 py-1 text-sm"
                >
                  <Wrench className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                  <span className="truncate">{option?.label || tool}</span>
                  {option?.source === "mcp" ? <Badge>MCP</Badge> : null}
                  {!option ? <Badge>手动</Badge> : null}
                  {disabled ? null : (
                    <button
                      type="button"
                      className="ml-0.5 rounded-full text-muted-foreground transition-colors hover:text-foreground"
                      aria-label={`移除工具 ${tool}`}
                      onClick={() => removeTool(tool)}
                    >
                      <X className="h-3.5 w-3.5" />
                    </button>
                  )}
                </span>
              );
            })}
          </div>
        ) : (
          <div className="flex min-h-8 items-center text-sm text-muted-foreground">还没有选择工具</div>
        )}
      </div>

      {open ? (
        <div className="rounded-xl border border-border/75 bg-card/70 p-2">
          <div className="relative">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              className="pl-8"
              value={query}
              placeholder="搜索工具或 MCP 服务"
              onChange={(event) => setQuery(event.target.value)}
            />
          </div>
          <div className="mt-2 max-h-64 space-y-1 overflow-y-auto pr-1">
            {visibleOptions.map((option) => {
              const selected = selectedSet.has(option.name);
              return (
                <button
                  key={option.name}
                  type="button"
                  className={cn(
                    "flex w-full items-start gap-2 rounded-lg px-2 py-2 text-left transition-colors hover:bg-accent/60",
                    selected && "bg-accent/55",
                  )}
                  onClick={() => toggleTool(option.name)}
                >
                  <span
                    className={cn(
                      "mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-md border",
                      selected ? "border-primary bg-primary text-primary-foreground" : "border-border bg-background/70",
                    )}
                  >
                    {selected ? <Check className="h-3.5 w-3.5" /> : null}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="flex min-w-0 flex-wrap items-center gap-1.5">
                      <span className="truncate font-medium">{option.label}</span>
                      <span className="text-xs text-muted-foreground">{option.name}</span>
                      {option.source === "mcp" ? <Badge>MCP</Badge> : null}
                      {option.readOnly ? <Badge>只读</Badge> : null}
                      {option.destructive ? <Badge>破坏性</Badge> : null}
                      {option.enabled === false ? <Badge>未启用</Badge> : null}
                    </span>
                    {option.description ? <span className="mt-0.5 block line-clamp-2 text-xs text-muted-foreground">{option.description}</span> : null}
                  </span>
                </button>
              );
            })}
            {canAddCustom ? (
              <button
                type="button"
                className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm transition-colors hover:bg-accent/60"
                onClick={() => addTool(query)}
              >
                <Plus className="h-4 w-4 text-muted-foreground" />
                <span>添加自定义工具名</span>
                <span className="min-w-0 truncate font-medium">{query.trim()}</span>
              </button>
            ) : null}
            {!visibleOptions.length && !canAddCustom ? (
              <div className="px-2 py-6 text-center text-sm text-muted-foreground">没有匹配的工具</div>
            ) : null}
          </div>
        </div>
      ) : null}
    </div>
  );
}

function buildToolOptions(tools: ToolInfo[], mcpServers: MCPServerConfig[]): ToolSelectOption[] {
  const options = new Map<string, ToolSelectOption>();
  for (const tool of tools) {
    const name = tool.name.trim();
    if (!name) continue;
    options.set(name, {
      name,
      label: tool.display_name || name,
      description: tool.description || tool.included_tools?.join(", "),
      category: tool.category,
      source: tool.builtin === false ? "mcp" : "builtin",
      readOnly: tool.read_only,
      destructive: tool.destructive,
      enabled: true,
    });
  }
  for (const server of mcpServers) {
    const name = server.id?.trim();
    if (!name || options.has(name)) continue;
    options.set(name, {
      name,
      label: server.name || name,
      description: server.description,
      category: "mcp",
      source: "mcp",
      enabled: Boolean(server.enabled),
    });
  }
  return Array.from(options.values()).sort((a, b) => {
    if (a.source !== b.source) return a.source === "builtin" ? -1 : 1;
    return a.label.localeCompare(b.label);
  });
}

function uniqueToolNames(values: string[]) {
  const seen = new Set<string>();
  const result: string[] = [];
  for (const value of values) {
    const trimmed = value.trim();
    if (!trimmed || seen.has(trimmed)) continue;
    seen.add(trimmed);
    result.push(trimmed);
  }
  return result;
}

function MCPServerEditor({
  server,
  index,
  updateDraft,
}: {
  server: MCPServerConfig;
  index: number;
  updateDraft: EditorProps["updateDraft"];
}) {
  function update(patch: Partial<MCPServerConfig>) {
    updateDraft((next) => {
      const servers = [...(next.tools?.mcp_servers || [])];
      servers[index] = { ...servers[index], ...patch };
      next.tools = { ...(next.tools || {}), mcp_servers: servers };
    });
  }

  return (
    <ConfigCard
      title={server.name || server.id || "未命名 MCP"}
      aside={server.transport || "transport"}
      onRemove={() =>
        updateDraft((next) => {
          next.tools = { ...(next.tools || {}), mcp_servers: (next.tools?.mcp_servers || []).filter((_, itemIndex) => itemIndex !== index) };
        })
      }
    >
      <div className="grid gap-3 md:grid-cols-2">
        <TextField label="ID" value={server.id} onChange={(value) => update({ id: value })} />
        <TextField label="名称" value={server.name} onChange={(value) => update({ name: value })} />
        <SelectField label="传输" value={server.transport} options={["http", "stdio"]} onChange={(value) => update({ transport: value })} />
        <ToggleField label="启用" checked={Boolean(server.enabled)} onChange={(value) => update({ enabled: value })} />
        <TextField label="超时" value={server.timeout} placeholder="30s" onChange={(value) => update({ timeout: value })} />
      </div>
      <TextField label="描述" value={server.description} onChange={(value) => update({ description: value })} />
      {server.transport === "stdio" ? (
        <>
          <TextField label="命令" value={server.command} onChange={(value) => update({ command: value })} />
          <StringListField label="参数" values={server.args || []} placeholder="run" onChange={(values) => update({ args: values })} />
          <KeyValueMapField label="环境变量" values={server.env || {}} onChange={(env) => update({ env })} />
        </>
      ) : (
        <TextField label="URL" value={server.url} onChange={(value) => update({ url: value })} />
      )}
    </ConfigCard>
  );
}

function ChannelCard({ title, description, children }: { title: string; description: string; children: React.ReactNode }) {
  return (
    <Panel>
      <PanelHeader>
        <SectionTitle icon={MessageSquare} title={title} description={description} />
      </PanelHeader>
      <PanelBody className="space-y-4">{children}</PanelBody>
    </Panel>
  );
}

function ConfigCard({
  title,
  aside,
  children,
  onRemove,
}: {
  title: string;
  aside?: string;
  children: React.ReactNode;
  onRemove?: () => void;
}) {
  return (
    <div className="rounded-xl border border-border/75 bg-card/65 p-4">
      <div className="mb-4 flex items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="truncate font-medium">{title}</div>
          {aside ? <div className="mt-0.5 text-xs text-muted-foreground">{aside}</div> : null}
        </div>
        {onRemove ? (
          <Button size="icon" variant="ghost" onClick={onRemove} aria-label="删除">
            <Trash2 className="h-4 w-4" />
          </Button>
        ) : null}
      </div>
      <div className="space-y-3">{children}</div>
    </div>
  );
}

function SectionTitle({ icon: Icon, title, description }: { icon: typeof Bot; title: string; description: string }) {
  return (
    <div className="flex min-w-0 items-start gap-3">
      <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-border bg-card/75">
        <Icon className="h-4 w-4" />
      </div>
      <div className="min-w-0">
        <div className="font-semibold">{title}</div>
        <div className="mt-1 text-sm leading-6 text-muted-foreground">{description}</div>
      </div>
    </div>
  );
}

function SectionHeader({
  icon,
  title,
  description,
  children,
}: {
  icon: typeof Bot;
  title: string;
  description: string;
  children?: React.ReactNode;
}) {
  return (
    <PanelHeader className="flex flex-col items-stretch gap-3 sm:flex-row sm:items-center sm:justify-between">
      <SectionTitle icon={icon} title={title} description={description} />
      {children ? <div className="flex shrink-0 sm:justify-end">{children}</div> : null}
    </PanelHeader>
  );
}

function Field({ label, action, children }: { label: string; action?: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="block space-y-1.5">
      <span className="flex min-h-8 items-center justify-between gap-3">
        <span className="text-sm text-muted-foreground">{label}</span>
        {action ? <span className="shrink-0">{action}</span> : null}
      </span>
      {children}
    </div>
  );
}

function TextField({
  label,
  value,
  onChange,
  placeholder,
  type = "text",
  disabled,
}: {
  label: string;
  value?: string;
  onChange: (value: string) => void;
  placeholder?: string;
  type?: "text" | "password";
  disabled?: boolean;
}) {
  return (
    <Field label={label}>
      <Input type={type} value={value || ""} placeholder={placeholder} disabled={disabled} onChange={(event) => onChange(event.target.value)} />
    </Field>
  );
}

function ModelAPIKeyField({ model, onChange }: { model: ModelConfig; onChange: (value: string) => void }) {
  const [editing, setEditing] = useState(false);
  const configured = Boolean(model.has_api_key);
  const hasDraftValue = Boolean(model.api_key);
  const showEditor = !configured || editing || hasDraftValue;

  useEffect(() => {
    if (configured && !hasDraftValue) {
      setEditing(false);
    }
  }, [configured, hasDraftValue]);

  if (!showEditor) {
    return (
      <Field
        label="API Key"
        action={
          <Button variant="ghost" size="sm" onClick={() => setEditing(true)}>
            修改
          </Button>
        }
      >
        <div className="sketch-inset flex h-9 w-full items-center rounded-md px-3 py-1 text-sm text-muted-foreground">已配置，留空不修改</div>
      </Field>
    );
  }

  return (
    <Field
      label={configured ? "API Key（输入新值后保存）" : "API Key"}
      action={
        configured ? (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              onChange("");
              setEditing(false);
            }}
          >
            取消
          </Button>
        ) : null
      }
    >
      <Input
        type="password"
        value={model.api_key || ""}
        placeholder={configured ? "输入新的 API Key" : undefined}
        autoComplete="new-password"
        spellCheck={false}
        onChange={(event) => onChange(event.target.value)}
      />
    </Field>
  );
}

function NumberField({
  label,
  value,
  min,
  step = 1,
  onChange,
}: {
  label: string;
  value?: number;
  min?: number;
  step?: number;
  onChange: (value: number) => void;
}) {
  function parseValue(raw: string) {
    let next = Number(raw || 0);
    if (!Number.isFinite(next)) {
      next = 0;
    }
    if (min !== undefined) {
      next = Math.max(min, next);
    }
    return next;
  }

  return (
    <Field label={label}>
      <Input type="number" min={min} step={step} value={value ?? 0} onChange={(event) => onChange(parseValue(event.target.value))} />
    </Field>
  );
}

function SelectField({
  label,
  value,
  options,
  onChange,
  disabled,
}: {
  label: string;
  value?: string;
  options: string[];
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const selected = value ?? options[0] ?? "";
  return (
    <Field label={label}>
      <div
        className="relative"
        onBlur={() => {
          window.setTimeout(() => setOpen(false), 120);
        }}
      >
        <button
          type="button"
          className={cn(
            "sketch-inset flex h-9 w-full items-center justify-between gap-2 rounded-md px-3 py-1 text-left text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
            open && "ring-2 ring-ring",
            disabled && "cursor-not-allowed opacity-70",
          )}
          disabled={disabled}
          aria-haspopup="listbox"
          aria-expanded={open}
          onClick={() => setOpen((current) => !current)}
        >
          <span className={cn("min-w-0 truncate", !selected && "text-muted-foreground")}>{selected || "未选择"}</span>
          <ChevronDown className={cn("h-4 w-4 shrink-0 text-muted-foreground transition-transform", open && "rotate-180")} />
        </button>
        {open ? (
          <div
            className="sketch-surface absolute left-0 right-0 top-[calc(100%+0.4rem)] z-40 max-h-56 space-y-1 overflow-y-auto rounded-xl bg-card p-2 text-sm shadow-[0_14px_32px_hsl(218_30%_25%/0.16)]"
            role="listbox"
          >
            {options.map((option) => (
              <button
                key={option}
                type="button"
                role="option"
                aria-selected={selected === option}
                className={cn(
                  "flex w-full items-center rounded-lg px-3 py-2 text-left transition-colors hover:bg-accent/70",
                  selected === option && "bg-primary/10 text-primary",
                )}
                onMouseDown={(event) => {
                  event.preventDefault();
                  onChange(option);
                  setOpen(false);
                }}
              >
                <span className={cn("min-w-0 truncate", !option && "text-muted-foreground")}>{option || "未选择"}</span>
              </button>
            ))}
          </div>
        ) : null}
      </div>
    </Field>
  );
}

const modelUseOptions = [
  { value: "chat", label: "对话" },
  { value: "agent", label: "智能体" },
  { value: "title", label: "标题" },
  { value: "summary", label: "摘要" },
];

function ModelUseField({ uses, onChange }: { uses: string[]; onChange: (uses: string[]) => void }) {
  const selected = new Set(uses);
  function toggle(value: string) {
    if (selected.has(value)) {
      onChange(uses.filter((item) => item !== value));
      return;
    }
    onChange([...uses, value]);
  }
  return (
    <div className="space-y-1.5 md:col-span-2">
      <div className="text-sm text-muted-foreground">用途</div>
      <div className="flex flex-wrap gap-2">
        {modelUseOptions.map((option) => (
          <button
            key={option.value}
            type="button"
            className={cn(
              "inline-flex h-8 items-center rounded-md border px-3 text-sm transition-colors",
              selected.has(option.value) ? "border-primary/60 bg-primary/10 text-primary" : "border-border/75 bg-card/70 text-muted-foreground hover:bg-accent/60",
            )}
            onClick={() => toggle(option.value)}
          >
            {option.label}
          </button>
        ))}
      </div>
    </div>
  );
}

function ModelSelect({
  label,
  value,
  modelIDs,
  onChange,
  disabled,
}: {
  label: string;
  value?: string;
  modelIDs: string[];
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  return <SelectField label={label} value={value} options={modelIDs.length ? modelIDs : [""]} disabled={disabled} onChange={onChange} />;
}

function ModeField({ value, onChange }: { value?: string; onChange: (value: string) => void }) {
  return <SelectField label="运行模式" value={value} options={["team", "deep", "roundtable", "agent"]} onChange={onChange} />;
}

function ToggleField({ label, checked, onChange, disabled }: { label: string; checked: boolean; onChange: (value: boolean) => void; disabled?: boolean }) {
  return (
    <button
      type="button"
      className={cn(
        "flex w-full items-center justify-between gap-3 rounded-lg border border-border/75 bg-card/60 px-3 py-2 text-left transition-colors hover:bg-accent/60",
        disabled && "cursor-not-allowed opacity-70 hover:bg-card/60",
      )}
      disabled={disabled}
      onClick={() => onChange(!checked)}
    >
      <span className="text-sm font-medium">{label}</span>
      <span className={cn("relative h-6 w-11 rounded-full border transition-colors", checked ? "border-primary/70 bg-primary" : "border-border bg-muted")}>
        <span className={cn("absolute top-0.5 h-5 w-5 rounded-full bg-card shadow transition-transform", checked ? "translate-x-5" : "translate-x-0.5")} />
      </span>
    </button>
  );
}

function StringListField({
  label,
  values,
  onChange,
  placeholder,
  secret,
}: {
  label: string;
  values: string[];
  onChange: (values: string[]) => void;
  placeholder?: string;
  secret?: boolean;
}) {
  const rows = values.length ? values : [""];
  return (
    <div className="space-y-2">
      <div className="text-sm text-muted-foreground">{label}</div>
      <div className="space-y-2">
        {rows.map((value, index) => (
          <div key={index} className="flex gap-2">
            <Input
              type={secret ? "password" : "text"}
              value={value}
              placeholder={placeholder}
              onChange={(event) => {
                const next = [...rows];
                next[index] = event.target.value;
                onChange(next.filter((item, itemIndex) => item.trim() || itemIndex !== rows.length - 1));
              }}
            />
            <Button
              size="icon"
              variant="ghost"
              aria-label="删除"
              onClick={() => {
                const next = rows.filter((_, itemIndex) => itemIndex !== index);
                onChange(next.length ? next : []);
              }}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        ))}
      </div>
      <Button variant="outline" size="sm" onClick={() => onChange([...values, ""])}>
        <Plus className="h-4 w-4" />
        添加
      </Button>
    </div>
  );
}

function KeyValueMapField({
  label,
  values,
  onChange,
}: {
  label: string;
  values: Record<string, string>;
  onChange: (values: Record<string, string>) => void;
}) {
  const [rows, setRows] = useState<Array<[string, string]>>(() => {
    const entries = Object.entries(values);
    return entries.length ? entries : [["", ""]];
  });
  useEffect(() => {
    const entries = Object.entries(values);
    setRows(entries.length ? entries : [["", ""]]);
  }, [JSON.stringify(values)]);

  function commit(nextRows: Array<[string, string]>) {
    setRows(nextRows.length ? nextRows : [["", ""]]);
    const next: Record<string, string> = {};
    for (const [key, value] of nextRows) {
      const trimmed = key.trim();
      if (!trimmed) continue;
      next[trimmed] = value;
    }
    onChange(next);
  }
  return (
    <div className="space-y-2">
      <div className="text-sm text-muted-foreground">{label}</div>
      <div className="space-y-2">
        {rows.map(([key, value], index) => (
          <div key={index} className="grid gap-2 md:grid-cols-[1fr_1fr_auto]">
            <Input
              value={key}
              placeholder="KEY"
              onChange={(event) => {
                const nextRows = [...rows];
                nextRows[index] = [event.target.value, value];
                commit(nextRows);
              }}
            />
            <Input
              value={value}
              placeholder="value"
              onChange={(event) => {
                const nextRows = [...rows];
                nextRows[index] = [key, event.target.value];
                commit(nextRows);
              }}
            />
            <Button
              size="icon"
              variant="ghost"
              aria-label="删除"
              onClick={() => {
                const nextRows = rows.filter((_, itemIndex) => itemIndex !== index);
                commit(nextRows);
              }}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        ))}
      </div>
      <Button variant="outline" size="sm" onClick={() => setRows([...rows, ["", ""]])}>
        <Plus className="h-4 w-4" />
        添加
      </Button>
    </div>
  );
}

function EmptyState({ title, description }: { title: string; description: string }) {
  return (
    <div className="rounded-xl border border-dashed border-border p-8 text-center xl:col-span-2">
      <div className="font-medium">{title}</div>
      <div className="mt-1 text-sm text-muted-foreground">{description}</div>
    </div>
  );
}

function MetricCard({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-xl border border-border/75 bg-card/65 p-4">
      <div className="text-sm text-muted-foreground">{label}</div>
      <div className="mt-2 text-3xl font-semibold">{value}</div>
    </div>
  );
}

function updateModel(updateDraft: EditorProps["updateDraft"], index: number, patch: Partial<ModelConfig>) {
  updateDraft((next) => {
    const models = [...(next.models || [])];
    models[index] = { ...models[index], ...patch, original_id: models[index]?.original_id || models[index]?.id };
    next.models = models;
  });
}

function setServer(config: AppConfig, patch: AppConfig["server"]) {
  config.server = { ...(config.server || {}), ...patch };
}

function setAuth(config: AppConfig, patch: Partial<ServerAuthConfig>) {
  config.server = { ...(config.server || {}), auth: { ...(config.server?.auth || {}), ...patch } };
}

function setAgents(config: AppConfig, patch: AppConfig["agents"]) {
  config.agents = { ...(config.agents || {}), ...patch };
}

function setQQ(config: AppConfig, patch: Partial<ChannelQQConfig>) {
  config.channels = { ...(config.channels || {}), qq: { ...(config.channels?.qq || {}), ...patch } };
}

function setDiscord(config: AppConfig, patch: Partial<ChannelDiscordConfig>) {
  config.channels = { ...(config.channels || {}), discord: { ...(config.channels?.discord || {}), ...patch } };
}

function setWeixin(config: AppConfig, patch: Partial<ChannelWeixinConfig>) {
  config.channels = { ...(config.channels || {}), weixin: { ...(config.channels?.weixin || {}), ...patch } };
}

function mergeDeepConfig(current: DeepConfig, patch: DeepConfig): DeepConfig {
  return {
    ...current,
    ...patch,
    planning: patch.planning ? { ...(current.planning || {}), ...patch.planning } : current.planning,
    workspace: patch.workspace ? { ...(current.workspace || {}), ...patch.workspace } : current.workspace,
    shell: patch.shell ? { ...(current.shell || {}), ...patch.shell } : current.shell,
    delegation: patch.delegation ? { ...(current.delegation || {}), ...patch.delegation } : current.delegation,
    context: patch.context ? { ...(current.context || {}), ...patch.context } : current.context,
    output: patch.output ? { ...(current.output || {}), ...patch.output } : current.output,
  };
}

function normalizeConfig(config: AppConfig): AppConfig {
  const next = clone(config);
  next.models = next.models || [];
  next.server = next.server || {};
  next.server.auth = next.server.auth || {};
  next.memory = next.memory || {};
  next.agents = next.agents || {};
  next.agents.items = next.agents.items || [];
  next.channels = next.channels || {};
  next.channels.qq = next.channels.qq || {};
  next.channels.discord = next.channels.discord || {};
  next.channels.weixin = next.channels.weixin || {};
  next.openai_api = next.openai_api || {};
  next.roundtable = next.roundtable || {};
  next.roundtable.members = next.roundtable.members || [];
  next.deep = normalizeDeepConfig(next.deep || {});
  next.tools = next.tools || {};
  next.tools.approval = next.tools.approval || {};
  next.tools.approval.auto_approve = normalizeApprovalStores(next.tools.approval.auto_approve || []);
  next.tools.mcp_servers = next.tools.mcp_servers || [];
  return next;
}

function normalizeConfigForSave(config: AppConfig): AppConfig {
  const next = normalizeConfig(config);
  next.agents = {
    ...(next.agents || {}),
    items: (next.agents?.items || []).flatMap((agent) => {
      if (agent.builtin && agent.id === "coordinator") return [];
      if (agent.builtin) return [{ id: agent.id, enabled: agent.enabled }];
      return [agent];
    }),
  };
  return next;
}

function normalizeDeepConfig(deep: DeepConfig): DeepConfig {
  return {
    instruction: deep.instruction || "",
    max_iterations: deep.max_iterations && deep.max_iterations > 0 ? deep.max_iterations : 20,
    planning: {
      enabled: deep.planning?.enabled ?? true,
    },
    workspace: {
      enabled: deep.workspace?.enabled ?? true,
    },
    shell: {
      enabled: deep.shell?.enabled ?? true,
      streaming: deep.shell?.streaming ?? false,
      timeout: deep.shell?.timeout || "30s",
    },
    delegation: {
      general_agent: deep.delegation?.general_agent ?? true,
      task_tool_description: deep.delegation?.task_tool_description || "",
    },
    context: {
      summary: deep.context?.summary ?? true,
      agents_md: deep.context?.agents_md ?? true,
    },
    output: {
      key: deep.output?.key || "",
    },
    extra_tools: deep.extra_tools || ["doc", "search", "fetch", "ask"],
  };
}

function normalizeApprovalStores(stores: string[]) {
  const allowed = new Set(["all", ...approvalStoreOptions.map((option) => option.value)]);
  const result: string[] = [];
  for (const store of stores) {
    if (!allowed.has(store) || result.includes(store)) continue;
    result.push(store);
  }
  if (result.includes("all")) return ["all"];
  return result;
}

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

function uniqueName(models: ModelConfig[], base: string) {
  const used = new Set(models.map((model) => model.name));
  if (!used.has(base)) return base;
  let index = 2;
  while (used.has(`${base}-${index}`)) index += 1;
  return `${base}-${index}`;
}

function uniqueModelID(models: ModelConfig[], base: string) {
  const used = new Set(models.map((model) => model.id));
  return uniqueID(base, used);
}

function uniqueMemberID(members: TeamMemberConfig[], base: string) {
  const used = new Set(members.map((member) => member.id));
  return uniqueID(base, used);
}

function uniqueAgentID(agents: AgentConfig[], base: string) {
  const used = new Set(agents.map((agent) => agent.id));
  return uniqueID(base, used);
}

function uniqueMCPServerID(servers: MCPServerConfig[], base: string) {
  const used = new Set(servers.map((server) => server.id));
  return uniqueID(base, used);
}

function uniqueID(base: string, used: Set<string | undefined>) {
  if (!used.has(base)) return base;
  let index = 2;
  while (used.has(`${base}-${index}`)) index += 1;
  return `${base}-${index}`;
}
