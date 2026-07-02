import {
  Bot,
  Brain,
  Cable,
  Check,
  ChevronDown,
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
  Trash2,
  Wrench,
  X,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { getConfig, getToolCatalog, saveConfig } from "@/api/config";
import { listProviderModels } from "@/api/providers";
import { configActions, appActions } from "@/app/store";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
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

type ConfigTab = "models" | "server" | "agents" | "roundtable" | "deep" | "memory" | "channels" | "tools" | "other";

const tabs: Array<{ key: ConfigTab; label: string; icon: typeof Bot }> = [
  { key: "models", label: "模型", icon: Bot },
  { key: "server", label: "服务", icon: Server },
  { key: "agents", label: "智能体", icon: Brain },
  { key: "roundtable", label: "圆桌", icon: ListPlus },
  { key: "deep", label: "深度", icon: Layers },
  { key: "memory", label: "记忆", icon: Database },
  { key: "channels", label: "通道", icon: MessageSquare },
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

  async function save() {
    if (!draft) return;
    setSaving(true);
    try {
      await saveConfig(normalizeConfig(draft));
      dispatch(appActions.showToast("配置已保存"));
      await load();
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    } finally {
      setSaving(false);
    }
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

  if (!draft) {
    return (
      <div className="flex h-full items-center justify-center p-6">
        <Panel className="w-full max-w-sm">
          <PanelBody className="py-10 text-center text-muted-foreground">{loading ? "正在加载配置" : "暂无配置"}</PanelBody>
        </Panel>
      </div>
    );
  }

  return (
    <div className="chat-scroll h-full overflow-auto p-6">
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
        {activeTab === "agents" ? <AgentsTab draft={draft} modelIDs={modelIDs} updateDraft={updateDraft} /> : null}
        {activeTab === "roundtable" ? <RoundtableTab draft={draft} modelIDs={modelIDs} updateDraft={updateDraft} /> : null}
        {activeTab === "deep" ? <DeepTab draft={draft} updateDraft={updateDraft} /> : null}
        {activeTab === "memory" ? <MemoryTab draft={draft} updateDraft={updateDraft} /> : null}
        {activeTab === "channels" ? <ChannelsTab draft={draft} updateDraft={updateDraft} /> : null}
        {activeTab === "tools" ? <ToolsTab draft={draft} updateDraft={updateDraft} /> : null}
        {activeTab === "other" ? <OtherTab draft={draft} toolsCount={tools.length} /> : null}
      </div>
    </div>
  );
}

function ModelsTab({ draft, updateDraft }: EditorProps) {
  const models = draft.models || [];
  const [modelLookup, setModelLookup] = useState<Record<number, ModelLookupState>>({});

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
      <PanelHeader className="flex items-center justify-between">
        <SectionTitle icon={Bot} title="模型池" description="模型通过稳定 ID 被智能体、圆桌和自定义配置引用。" />
        <Button
          variant="outline"
          onClick={() =>
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
            })
          }
        >
          <Plus className="h-4 w-4" />
          添加模型
        </Button>
      </PanelHeader>
      <PanelBody className="grid gap-4 xl:grid-cols-2">
        {models.map((model, index) => (
          <ConfigCard
            key={`${model.original_id || model.id || "model"}-${index}`}
            title={model.name || model.id || "未命名模型"}
            aside={model.provider || "provider"}
            onRemove={() =>
              updateDraft((next) => {
                next.models = (next.models || []).filter((_, itemIndex) => itemIndex !== index);
              })
            }
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
              <TextField
                label={model.has_api_key ? "API Key（已配置，留空不修改）" : "API Key"}
                type="password"
                value={model.api_key}
                placeholder={model.has_api_key ? "********" : undefined}
                onChange={(value) => updateModel(updateDraft, index, { api_key: value })}
              />
              <TextField
                label="额外请求头"
                value={model.extra_headers}
                placeholder="X-Key: value, X-Trace: value"
                onChange={(value) => updateModel(updateDraft, index, { extra_headers: value })}
              />
              <ModelUseField uses={model.use_for || []} onChange={(use_for) => updateModel(updateDraft, index, { use_for })} />
            </div>
          </ConfigCard>
        ))}
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

function AgentsTab({ draft, modelIDs, updateDraft }: EditorProps & { modelIDs: string[] }) {
  const agents = draft.agents || {};
  const agentItems = agents.items || [];
  const tools = useAppSelector((state) => state.config.tools);
  const toolOptions = useMemo(() => buildToolOptions(tools, draft.tools?.mcp_servers || []), [draft.tools?.mcp_servers, tools]);
  return (
    <Panel>
      <PanelHeader className="flex items-center justify-between">
        <SectionTitle icon={Brain} title="智能体目录" description="查看和配置全局可调用智能体，内置智能体支持开关和覆盖配置。" />
        <Button
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
      </PanelHeader>
      <PanelBody className="grid gap-4 xl:grid-cols-2">
        {agentItems.map((agent, index) => (
          <ConfigCard
            key={`${agent.id || "agent"}-${index}`}
            title={agent.name || agent.id || "未命名智能体"}
            aside={agent.builtin ? "内置智能体" : agent.model_id || "自定义智能体"}
            onRemove={
              agent.builtin
                ? undefined
                : () =>
                    updateDraft((next) => {
                      next.agents = { ...(next.agents || {}), items: (next.agents?.items || []).filter((_, itemIndex) => itemIndex !== index) };
                    })
            }
          >
            <AgentCatalogEditor
              agent={agent}
              modelIDs={modelIDs}
              toolOptions={toolOptions}
              onChange={(value) =>
                updateDraft((next) => {
                  const items = [...(next.agents?.items || [])];
                  items[index] = value;
                  next.agents = { ...(next.agents || {}), items };
                })
              }
            />
          </ConfigCard>
        ))}
        {agentItems.length === 0 ? (
          <div className="rounded-xl border border-dashed border-border p-8 text-center text-sm text-muted-foreground xl:col-span-2">暂无智能体配置，重新加载配置会从后端获取内置智能体默认信息。</div>
        ) : null}
      </PanelBody>
    </Panel>
  );
}

function RoundtableTab({ draft, modelIDs, updateDraft }: EditorProps & { modelIDs: string[] }) {
  const roundtable = draft.roundtable || {};
  return (
    <Panel>
      <PanelHeader className="flex items-center justify-between">
        <SectionTitle icon={ListPlus} title="圆桌讨论" description="配置 roundtable 模式成员和最大迭代次数。" />
        <Button
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
      </PanelHeader>
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
        <PanelHeader className="flex items-center justify-between">
          <SectionTitle icon={Cable} title="MCP 工具" description="配置 HTTP 或 stdio MCP 服务，启用后可在智能体工具中选择。" />
          <Button
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
        </PanelHeader>
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
  onChange,
}: {
  agent: AgentConfig;
  modelIDs: string[];
  toolOptions: ToolSelectOption[];
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
      </div>
      <ToggleField label="启用智能体" checked={Boolean(agent.enabled)} onChange={(value) => onChange({ ...agent, enabled: value })} />
      <div className="grid gap-3 md:grid-cols-3">
        <TextField
          label="ID"
          value={agent.id}
          onChange={(value) => onChange({ ...agent, id: value })}
          disabled={Boolean(agent.builtin)}
        />
        <TextField label="名称" value={agent.name} onChange={(value) => onChange({ ...agent, name: value })} />
        <ModelSelect label="模型 ID（可选）" value={agent.model_id} modelIDs={["", ...modelIDs]} onChange={(value) => onChange({ ...agent, model_id: value })} />
      </div>
      <TextField label="描述" value={agent.description} onChange={(value) => onChange({ ...agent, description: value })} />
      <ToolSelectField
        tools={agent.tools || []}
        options={toolOptions}
        onChange={(tools) => onChange({ ...agent, tools, ssh: tools.includes("ssh") ? agent.ssh : undefined })}
      />
      {usesSSH ? (
        <div className="grid gap-3 rounded-xl border border-border/75 bg-background/45 p-3 md:grid-cols-3">
          <TextField label="SSH 主机" value={ssh.host} placeholder="ip:port" onChange={(value) => onChange({ ...agent, ssh: { ...(agent.ssh || {}), host: value } })} />
          <TextField label="SSH 用户名" value={ssh.username} onChange={(value) => onChange({ ...agent, ssh: { ...(agent.ssh || {}), username: value } })} />
          <TextField
            label="SSH 密码"
            type="password"
            value={ssh.password}
            onChange={(value) => onChange({ ...agent, ssh: { ...(agent.ssh || {}), password: value } })}
          />
        </div>
      ) : null}
      <Field label="系统提示词">
        <Textarea className="min-h-56 text-sm" value={agent.prompt || ""} onChange={(event) => onChange({ ...agent, prompt: event.target.value })} />
      </Field>
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
}: {
  tools: string[];
  options: ToolSelectOption[];
  onChange: (tools: string[]) => void;
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
        <Button variant="outline" size="sm" onClick={() => setOpen((value) => !value)}>
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
                  <button
                    type="button"
                    className="ml-0.5 rounded-full text-muted-foreground transition-colors hover:text-foreground"
                    aria-label={`移除工具 ${tool}`}
                    onClick={() => removeTool(tool)}
                  >
                    <X className="h-3.5 w-3.5" />
                  </button>
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
    <div className="flex items-start gap-3">
      <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-border bg-card/75">
        <Icon className="h-4 w-4" />
      </div>
      <div>
        <div className="font-semibold">{title}</div>
        <div className="mt-1 text-sm text-muted-foreground">{description}</div>
      </div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block space-y-1.5">
      <span className="text-sm text-muted-foreground">{label}</span>
      {children}
    </label>
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
}: {
  label: string;
  value?: string;
  options: string[];
  onChange: (value: string) => void;
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
          )}
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
}: {
  label: string;
  value?: string;
  modelIDs: string[];
  onChange: (value: string) => void;
}) {
  return <SelectField label={label} value={value} options={modelIDs.length ? modelIDs : [""]} onChange={onChange} />;
}

function ModeField({ value, onChange }: { value?: string; onChange: (value: string) => void }) {
  return <SelectField label="运行模式" value={value} options={["team", "deep", "roundtable", "agent"]} onChange={onChange} />;
}

function ToggleField({ label, checked, onChange }: { label: string; checked: boolean; onChange: (value: boolean) => void }) {
  return (
    <button
      type="button"
      className="flex w-full items-center justify-between gap-3 rounded-lg border border-border/75 bg-card/60 px-3 py-2 text-left transition-colors hover:bg-accent/60"
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
  next.tools.mcp_servers = next.tools.mcp_servers || [];
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
