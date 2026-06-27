import {
  Bot,
  Brain,
  Cable,
  Database,
  KeyRound,
  ListPlus,
  MessageSquare,
  Plus,
  RefreshCcw,
  Save,
  Server,
  Settings2,
  Trash2,
  Wrench,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { getConfig, getToolCatalog, saveConfig } from "@/api/config";
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
  ChannelDiscordConfig,
  ChannelQQConfig,
  ChannelWeixinConfig,
  CustomAgentConfig,
  MCPServerConfig,
  ModelConfig,
  ServerAuthConfig,
  SSHVisitorConfig,
  TeamMemberConfig,
} from "@/types/config";

type ConfigTab = "models" | "server" | "agents" | "memory" | "channels" | "custom" | "tools" | "other";

const tabs: Array<{ key: ConfigTab; label: string; icon: typeof Bot }> = [
  { key: "models", label: "模型", icon: Bot },
  { key: "server", label: "服务", icon: Server },
  { key: "agents", label: "智能体", icon: Brain },
  { key: "memory", label: "记忆", icon: Database },
  { key: "channels", label: "通道", icon: MessageSquare },
  { key: "custom", label: "自定义", icon: Settings2 },
  { key: "tools", label: "工具", icon: Wrench },
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
  const modelNames = useMemo(() => (draft?.models || []).map((model) => model.name).filter(Boolean), [draft?.models]);

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
        {activeTab === "agents" ? <AgentsTab draft={draft} modelNames={modelNames} updateDraft={updateDraft} /> : null}
        {activeTab === "memory" ? <MemoryTab draft={draft} updateDraft={updateDraft} /> : null}
        {activeTab === "channels" ? <ChannelsTab draft={draft} updateDraft={updateDraft} /> : null}
        {activeTab === "custom" ? <CustomTab draft={draft} modelNames={modelNames} updateDraft={updateDraft} /> : null}
        {activeTab === "tools" ? <ToolsTab /> : null}
        {activeTab === "other" ? <OtherTab draft={draft} toolsCount={tools.length} /> : null}
      </div>
    </div>
  );
}

function ModelsTab({ draft, updateDraft }: EditorProps) {
  const models = draft.models || [];
  return (
    <Panel>
      <PanelHeader className="flex items-center justify-between">
        <SectionTitle icon={Bot} title="模型池" description="模型通过名称被智能体、圆桌和自定义配置引用。" />
        <Button
          variant="outline"
          onClick={() =>
            updateDraft((next) => {
              next.models = [
                ...(next.models || []),
                {
                  name: uniqueName(next.models || [], "model"),
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
            key={`${model.original_name || model.name || "model"}-${index}`}
            title={model.name || "未命名模型"}
            aside={model.provider || "provider"}
            onRemove={() =>
              updateDraft((next) => {
                next.models = (next.models || []).filter((_, itemIndex) => itemIndex !== index);
              })
            }
          >
            <div className="grid gap-3 md:grid-cols-2">
              <TextField label="名称" value={model.name} onChange={(value) => updateModel(updateDraft, index, { name: value })} />
              <SelectField
                label="提供商"
                value={model.provider}
                options={["openai", "deepseek", "claude", "ollama", "ark", "gemini", "qwen", "openrouter", "copilot"]}
                onChange={(value) => updateModel(updateDraft, index, { provider: value })}
              />
              <TextField label="模型" value={model.model} onChange={(value) => updateModel(updateDraft, index, { model: value })} />
              <TextField label="Base URL" value={model.base_url} onChange={(value) => updateModel(updateDraft, index, { base_url: value })} />
              <TextField
                label={model.has_api_key ? "API Key（已配置，留空不修改）" : "API Key"}
                type="password"
                value={model.api_key}
                onChange={(value) => updateModel(updateDraft, index, { api_key: value })}
              />
              <TextField
                label="额外请求头"
                value={model.extra_headers}
                placeholder="X-Key: value, X-Trace: value"
                onChange={(value) => updateModel(updateDraft, index, { extra_headers: value })}
              />
            </div>
          </ConfigCard>
        ))}
        {!models.length ? <EmptyState title="暂无模型配置" description="添加 default 模型后即可开始使用。" /> : null}
      </PanelBody>
    </Panel>
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

function AgentsTab({ draft, modelNames, updateDraft }: EditorProps & { modelNames: string[] }) {
  const agents = draft.agents || {};
  const ssh = agents.ssh_visitor || {};
  const roundtable = draft.roundtable || {};
  return (
    <div className="grid gap-4 xl:grid-cols-[420px_1fr]">
      <Panel>
        <PanelHeader>
          <SectionTitle icon={Brain} title="内置智能体" description="控制内置成员是否参与团队能力。" />
        </PanelHeader>
        <PanelBody className="space-y-4">
          <ToggleField label="Researcher" checked={Boolean(agents.researcher)} onChange={(value) => updateDraft((next) => setAgents(next, { researcher: value }))} />
          <ToggleField label="Assistant" checked={Boolean(agents.assistant)} onChange={(value) => updateDraft((next) => setAgents(next, { assistant: value }))} />
          <ToggleField label="Analyst" checked={Boolean(agents.analyst)} onChange={(value) => updateDraft((next) => setAgents(next, { analyst: value }))} />
          <div className="border-t border-border/70 pt-4">
            <ToggleField
              label="SSH Visitor"
              checked={Boolean(ssh.enabled)}
              onChange={(value) => updateDraft((next) => setSSHVisitor(next, { enabled: value }))}
            />
            <div className="mt-4 grid gap-3">
              <TextField label="主机" value={ssh.host} onChange={(value) => updateDraft((next) => setSSHVisitor(next, { host: value }))} />
              <TextField label="用户名" value={ssh.username} onChange={(value) => updateDraft((next) => setSSHVisitor(next, { username: value }))} />
              <TextField label="密码" type="password" value={ssh.password} onChange={(value) => updateDraft((next) => setSSHVisitor(next, { password: value }))} />
            </div>
          </div>
        </PanelBody>
      </Panel>
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
                  members: [...members, { index: members.length, name: "", desc: "", model: modelNames[0] || "default" }],
                };
              })
            }
          >
            <Plus className="h-4 w-4" />
            添加成员
          </Button>
        </PanelHeader>
        <PanelBody className="space-y-4">
          <NumberField
            label="最大迭代次数"
            value={roundtable.max_iterations}
            onChange={(value) =>
              updateDraft((next) => {
                next.roundtable = { ...(next.roundtable || {}), max_iterations: value };
              })
            }
          />
          {(roundtable.members || []).map((member, index) => (
            <RoundtableMemberEditor key={index} member={member} index={index} modelNames={modelNames} updateDraft={updateDraft} />
          ))}
        </PanelBody>
      </Panel>
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
      </ChannelCard>
      <ChannelCard title="Discord" description="Discord Bot 通道">
        <ToggleField label="启用" checked={Boolean(discord.enabled)} onChange={(value) => updateDraft((next) => setDiscord(next, { enabled: value }))} />
        <TextField label="Token" type="password" value={discord.token} onChange={(value) => updateDraft((next) => setDiscord(next, { token: value }))} />
        <TextField label="允许用户" value={discord.allow_from} placeholder="多个 ID 用逗号分隔" onChange={(value) => updateDraft((next) => setDiscord(next, { allow_from: value }))} />
        <ModeField value={discord.mode} onChange={(value) => updateDraft((next) => setDiscord(next, { mode: value }))} />
      </ChannelCard>
      <ChannelCard title="微信" description="iLinkAI 微信通道">
        <ToggleField label="启用" checked={Boolean(weixin.enabled)} onChange={(value) => updateDraft((next) => setWeixin(next, { enabled: value }))} />
        <TextField label="Base URL" value={weixin.base_url} onChange={(value) => updateDraft((next) => setWeixin(next, { base_url: value }))} />
        <TextField label="凭证路径" value={weixin.cred_path} onChange={(value) => updateDraft((next) => setWeixin(next, { cred_path: value }))} />
        <SelectField label="日志级别" value={weixin.log_level} options={["debug", "info", "warn", "error", "silent"]} onChange={(value) => updateDraft((next) => setWeixin(next, { log_level: value }))} />
        <TextField label="允许用户" value={weixin.allow_from} placeholder="多个 ID 用逗号分隔" onChange={(value) => updateDraft((next) => setWeixin(next, { allow_from: value }))} />
        <ModeField value={weixin.mode} onChange={(value) => updateDraft((next) => setWeixin(next, { mode: value }))} />
      </ChannelCard>
    </div>
  );
}

function CustomTab({ draft, modelNames, updateDraft }: EditorProps & { modelNames: string[] }) {
  const custom = draft.custom || {};
  const moderator = custom.moderator || {};
  return (
    <div className="space-y-4">
      <Panel>
        <PanelHeader>
          <SectionTitle icon={Bot} title="主持人" description="自定义模式的协调者配置。" />
        </PanelHeader>
        <PanelBody>
          <CustomAgentEditor
            agent={moderator}
            modelNames={modelNames}
            onChange={(value) =>
              updateDraft((next) => {
                next.custom = { ...(next.custom || {}), moderator: value };
              })
            }
          />
        </PanelBody>
      </Panel>

      <Panel>
        <PanelHeader className="flex items-center justify-between">
          <SectionTitle icon={Brain} title="自定义智能体" description="配置 custom 模式下的成员、提示词和工具。" />
          <Button
            variant="outline"
            onClick={() =>
              updateDraft((next) => {
                next.custom = {
                  ...(next.custom || {}),
                  agents: [...(next.custom?.agents || []), { name: "", desc: "", system_prompt: "", model: modelNames[0] || "default", tools: [] }],
                };
              })
            }
          >
            <Plus className="h-4 w-4" />
            添加智能体
          </Button>
        </PanelHeader>
        <PanelBody className="grid gap-4 xl:grid-cols-2">
          {(custom.agents || []).map((agent, index) => (
            <ConfigCard
              key={index}
              title={agent.name || "未命名智能体"}
              aside={agent.model || "model"}
              onRemove={() =>
                updateDraft((next) => {
                  next.custom = {
                    ...(next.custom || {}),
                    agents: (next.custom?.agents || []).filter((_, itemIndex) => itemIndex !== index),
                  };
                })
              }
            >
              <CustomAgentEditor
                agent={agent}
                modelNames={modelNames}
                onChange={(value) =>
                  updateDraft((next) => {
                    const agents = [...(next.custom?.agents || [])];
                    agents[index] = value;
                    next.custom = { ...(next.custom || {}), agents };
                  })
                }
              />
            </ConfigCard>
          ))}
        </PanelBody>
      </Panel>

      <Panel>
        <PanelHeader className="flex items-center justify-between">
          <SectionTitle icon={Cable} title="MCP 服务" description="配置 HTTP 或 stdio MCP 服务。" />
          <Button
            variant="outline"
            onClick={() =>
              updateDraft((next) => {
                next.custom = {
                  ...(next.custom || {}),
                  mcp_servers: [
                    ...(next.custom?.mcp_servers || []),
                    { name: "", desc: "", enabled: false, timeout: 30, transport_type: "http", url: "" },
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
          {(custom.mcp_servers || []).map((server, index) => (
            <MCPServerEditor key={index} server={server} index={index} updateDraft={updateDraft} />
          ))}
        </PanelBody>
      </Panel>
    </div>
  );
}

function ToolsTab() {
  const tools = useAppSelector((state) => state.config.tools);
  return (
    <Panel>
      <PanelHeader>
        <SectionTitle icon={Wrench} title="工具目录" description={`${tools.length} 个工具组，可用于自定义智能体工具配置。`} />
      </PanelHeader>
      <PanelBody className="grid gap-3 xl:grid-cols-2">
        {tools.map((tool) => (
          <div key={tool.name} className="rounded-xl border border-border/75 bg-card/65 p-4">
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
        ))}
      </PanelBody>
    </Panel>
  );
}

function OtherTab({ draft, toolsCount }: { draft: AppConfig; toolsCount: number }) {
  const known = new Set(["models", "server", "agents", "custom", "channels", "memory", "openai_api", "roundtable"]);
  const unknownKeys = Object.keys(draft).filter((key) => !known.has(key));
  return (
    <Panel>
      <PanelHeader>
        <SectionTitle icon={Settings2} title="其他信息" description="当前页面会保留未知顶层配置，但不提供原始文本编辑入口。" />
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
  modelNames,
  updateDraft,
}: {
  member: TeamMemberConfig;
  index: number;
  modelNames: string[];
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
      aside={member.model || "model"}
      onRemove={() =>
        updateDraft((next) => {
          next.roundtable = { ...(next.roundtable || {}), members: (next.roundtable?.members || []).filter((_, itemIndex) => itemIndex !== index) };
        })
      }
    >
      <div className="grid gap-3 md:grid-cols-[100px_1fr_1fr]">
        <NumberField label="序号" value={member.index ?? index} onChange={(value) => update({ index: value })} />
        <TextField label="名称" value={member.name} onChange={(value) => update({ name: value })} />
        <ModelSelect label="模型" value={member.model} modelNames={modelNames} onChange={(value) => update({ model: value })} />
      </div>
      <TextField label="描述" value={member.desc} onChange={(value) => update({ desc: value })} />
    </ConfigCard>
  );
}

function CustomAgentEditor({
  agent,
  modelNames,
  onChange,
}: {
  agent: CustomAgentConfig;
  modelNames: string[];
  onChange: (value: CustomAgentConfig) => void;
}) {
  return (
    <div className="grid gap-3">
      <div className="grid gap-3 md:grid-cols-2">
        <TextField label="名称" value={agent.name} onChange={(value) => onChange({ ...agent, name: value })} />
        <ModelSelect label="模型" value={agent.model} modelNames={modelNames} onChange={(value) => onChange({ ...agent, model: value })} />
      </div>
      <TextField label="描述" value={agent.desc} onChange={(value) => onChange({ ...agent, desc: value })} />
      <Field label="系统提示词">
        <Textarea
          className="min-h-32 text-sm"
          value={agent.system_prompt || ""}
          onChange={(event) => onChange({ ...agent, system_prompt: event.target.value })}
        />
      </Field>
      <StringListField label="工具" values={agent.tools || []} placeholder="command 或 mcp-服务名称" onChange={(values) => onChange({ ...agent, tools: values })} />
    </div>
  );
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
      const servers = [...(next.custom?.mcp_servers || [])];
      servers[index] = { ...servers[index], ...patch };
      next.custom = { ...(next.custom || {}), mcp_servers: servers };
    });
  }

  return (
    <ConfigCard
      title={server.name || "未命名 MCP"}
      aside={server.transport_type || "transport"}
      onRemove={() =>
        updateDraft((next) => {
          next.custom = { ...(next.custom || {}), mcp_servers: (next.custom?.mcp_servers || []).filter((_, itemIndex) => itemIndex !== index) };
        })
      }
    >
      <div className="grid gap-3 md:grid-cols-2">
        <TextField label="名称" value={server.name} onChange={(value) => update({ name: value })} />
        <SelectField label="传输" value={server.transport_type} options={["http", "stdio"]} onChange={(value) => update({ transport_type: value })} />
        <ToggleField label="启用" checked={Boolean(server.enabled)} onChange={(value) => update({ enabled: value })} />
        <NumberField label="超时秒数" value={server.timeout} onChange={(value) => update({ timeout: value })} />
      </div>
      <TextField label="描述" value={server.desc} onChange={(value) => update({ desc: value })} />
      {server.transport_type === "stdio" ? (
        <>
          <TextField label="命令" value={server.command} onChange={(value) => update({ command: value })} />
          <StringListField label="参数" values={server.args || []} placeholder="run" onChange={(values) => update({ args: values })} />
          <StringListField label="环境变量" values={server.env_vars || []} placeholder="KEY=value" onChange={(values) => update({ env_vars: values })} />
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
}: {
  label: string;
  value?: string;
  onChange: (value: string) => void;
  placeholder?: string;
  type?: "text" | "password";
}) {
  return (
    <Field label={label}>
      <Input type={type} value={value || ""} placeholder={placeholder} onChange={(event) => onChange(event.target.value)} />
    </Field>
  );
}

function NumberField({ label, value, onChange }: { label: string; value?: number; onChange: (value: number) => void }) {
  return (
    <Field label={label}>
      <Input type="number" value={value ?? 0} onChange={(event) => onChange(Number(event.target.value || 0))} />
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
  return (
    <Field label={label}>
      <select
        className="sketch-inset flex h-9 w-full rounded-md px-3 py-1 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        value={value || options[0] || ""}
        onChange={(event) => onChange(event.target.value)}
      >
        {options.map((option) => (
          <option key={option} value={option}>
            {option}
          </option>
        ))}
      </select>
    </Field>
  );
}

function ModelSelect({
  label,
  value,
  modelNames,
  onChange,
}: {
  label: string;
  value?: string;
  modelNames: string[];
  onChange: (value: string) => void;
}) {
  return <SelectField label={label} value={value} options={modelNames.length ? modelNames : ["default"]} onChange={onChange} />;
}

function ModeField({ value, onChange }: { value?: string; onChange: (value: string) => void }) {
  return <SelectField label="运行模式" value={value} options={["team", "deep", "roundtable", "custom"]} onChange={onChange} />;
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
    models[index] = { ...models[index], ...patch, original_name: models[index]?.original_name || models[index]?.name };
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

function setSSHVisitor(config: AppConfig, patch: Partial<SSHVisitorConfig>) {
  config.agents = { ...(config.agents || {}), ssh_visitor: { ...(config.agents?.ssh_visitor || {}), ...patch } };
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

function normalizeConfig(config: AppConfig): AppConfig {
  const next = clone(config);
  next.models = next.models || [];
  next.server = next.server || {};
  next.server.auth = next.server.auth || {};
  next.memory = next.memory || {};
  next.agents = next.agents || {};
  next.agents.ssh_visitor = next.agents.ssh_visitor || {};
  next.channels = next.channels || {};
  next.channels.qq = next.channels.qq || {};
  next.channels.discord = next.channels.discord || {};
  next.channels.weixin = next.channels.weixin || {};
  next.openai_api = next.openai_api || {};
  next.roundtable = next.roundtable || {};
  next.roundtable.members = next.roundtable.members || [];
  next.custom = next.custom || {};
  next.custom.moderator = next.custom.moderator || {};
  next.custom.agents = next.custom.agents || [];
  next.custom.mcp_servers = next.custom.mcp_servers || [];
  return next;
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
