import { Bot, Brain, ChevronDown, Copy, ListPlus, Plus, RefreshCcw, Sparkles } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { generateAgentDrafts } from "@/api/ai";
import { isAbortError } from "@/api/client";
import { listProviderModels } from "@/api/providers";
import { useAppSelector } from "@/app/hooks";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { LoadingSurface } from "@/components/ui/loading-surface";
import { Dialog } from "@/components/ui/dialog";
import { Panel, PanelBody } from "@/components/ui/panel";
import { cn } from "@/lib/cn";
import type { AgentConfig, DeepConfig, ModelConfig, TeamMemberConfig } from "@/types/config";
import { type EditorProps, type ToolSelectOption, buildToolOptions, updateModel, mergeDeepConfig, normalizeConfig, uniqueName, uniqueModelID, uniqueMemberID, uniqueAgentID } from "./configModel";
import { AITextField, AIRewriteButton, ToolSelectField, SectionHeader, Field, TextField, ModelAPIKeyField, NumberField, SelectField, ModelUseField, ModelSelect, ToggleField, EmptyState } from "./ConfigFields";

import { ConfigEditCard, useConfigEditing } from "./ConfigEditor";

export function ModelsTab({ draft, updateDraft }: EditorProps) {
  const models = draft.models || [];
  const [modelLookup, setModelLookup] = useState<Record<number, ModelLookupState>>({});
  const editing = useConfigEditing();

  function removeModel(index: number) {
    editing.remove(models[index].name || models[index].id || "模型", (next) => {
      next.models = (next.models || []).filter((_, itemIndex) => itemIndex !== index);
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
            editing.begin(`model-${models.length}`, (next) => {
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
          }}
        >
          <Plus className="h-4 w-4" />
          添加模型
        </Button>
      </SectionHeader>
      <PanelBody className="grid gap-4 xl:grid-cols-2">
        {models.map((model, index) => {
          return (
            <ModelConfigCard
              key={index}
              model={model}
              editorKey={`model-${index}`}
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
                <div className="space-y-2 md:col-span-2">
                  <SelectField
                    label="视觉输入"
                    value={visionInputSummary(model.supports_vision)}
                    options={["未设置（沿用供应商行为）", "支持", "不支持"]}
                    onChange={(value) => updateModel(updateDraft, index, { supports_vision: value === "支持" ? true : value === "不支持" ? false : undefined })}
                  />
                  <p className="text-xs leading-5 text-muted-foreground">按实际模型能力选择；支持时发送图片，不支持时在请求前拦截。未设置时保留现有行为。</p>
                </div>
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

function ModelConfigCard({ model, editorKey, onRemove, children }: {
  model: ModelConfig;
  editorKey: string;
  onRemove: () => void;
  children: React.ReactNode;
}) {
  return <ConfigEditCard editorKey={editorKey} title={model.name || model.id || "未命名模型"}
    description={model.model || "尚未指定供应商模型"} badges={[model.provider || "未设置提供商"]}
    summary={[
      { label: "ID", value: model.id || "未设置" },
      { label: "用途", value: modelUsesSummary(model.use_for || []) },
      { label: "视觉输入", value: visionInputSummary(model.supports_vision) },
      { label: "密钥", value: model.has_api_key || model.api_key ? "已配置" : "未配置" },
    ]} onRemove={onRemove}>{children}</ConfigEditCard>;
}

function visionInputSummary(supportsVision?: boolean) {
  return supportsVision === undefined ? "未设置（沿用供应商行为）" : supportsVision ? "支持" : "不支持";
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
          <div className="sketch-surface absolute left-0 right-0 top-[calc(100%+0.4rem)] z-40 max-h-56 space-y-1 overflow-y-auto rounded-xl bg-card p-2 text-sm shadow-popover">
            {visibleModels.length ? visibleModels.map((name) => (
              <button
                key={name}
                type="button"
                className={cn(
                  "flex w-full items-center rounded-lg px-3 py-2 text-left transition-colors hover:bg-accent/70",
                  model.model === name && "bg-accent text-primary",
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

export function AgentsTab({ draft, modelIDs, updateDraft, saving }: EditorProps & { modelIDs: string[] }) {
  const editing = useConfigEditing();
  const agents = draft.agents || {};
  const agentItems = agents.items || [];
  const tools = useAppSelector((state) => state.config.tools);
  const toolOptions = useMemo(() => buildToolOptions(tools, draft.tools?.mcp_servers || []), [draft.tools?.mcp_servers, tools]);
  const [assistantOpen, setAssistantOpen] = useState(false);

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
    await editing.apply((next) => { next.agents = nextDraft.agents; });
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
    editing.begin(`agent-${agentItems.length}`, (next) => {
      const items = next.agents?.items || [];
      next.agents = { ...(next.agents || {}), items: [...items, copy] };
    });
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
              editing.begin(`agent-${agentItems.length}`, (next) => {
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
      <PanelBody className="grid items-start gap-4 xl:grid-cols-2">
        {agentItems.map((agent, index) => {
          const agentKey = `agent-${index}`;
          return (
            <AgentConfigCard
              key={agentKey}
              agent={agent}
              editorKey={agentKey}
              onDuplicate={agent.builtin ? () => duplicateBuiltinAgent(agent) : undefined}
              onRemove={
                agent.builtin
                  ? undefined
                  : () => editing.remove(agent.name || agent.id || "智能体", (next) => {
                      next.agents = { ...(next.agents || {}), items: (next.agents?.items || []).filter((_, itemIndex) => itemIndex !== index) };
                    })
              }
            >
              <AgentCatalogEditor
                agent={agent}
                modelIDs={modelIDs}
                toolOptions={toolOptions}
                readOnly={Boolean(agent.builtin)}
                enabledReadOnly={Boolean(agent.builtin && (agent.id === "coordinator" || agent.id === "workspace_assistant"))}
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

function AgentConfigCard({ agent, editorKey, onDuplicate, onRemove, children }: {
  agent: AgentConfig;
  editorKey: string;
  onDuplicate?: () => void;
  onRemove?: () => void;
  children: React.ReactNode;
}) {
  return <ConfigEditCard editorKey={editorKey} title={agent.name || agent.id || "未命名智能体"}
    description={agent.description || "暂无描述"} badges={[agent.builtin ? "内置" : "自定义", agent.enabled ? "已启用" : "已关闭"]}
    summary={[
      { label: "ID", value: agent.id || "未设置" },
      { label: "模型", value: agent.model_id || "默认" },
      { label: "工具", value: `${agent.tools?.length || 0} 项` },
    ]} onRemove={onRemove}
    actions={onDuplicate ? <Button size="icon" variant="ghost" onClick={onDuplicate} aria-label="复制为自定义"><Copy className="h-4 w-4" /></Button> : undefined}
  >{children}</ConfigEditCard>;
}

export function RoundtableTab({ draft, modelIDs, updateDraft }: EditorProps & { modelIDs: string[] }) {
  const roundtable = draft.roundtable || {};
  const editing = useConfigEditing();
  return (
    <Panel>
      <SectionHeader icon={ListPlus} title="圆桌讨论" description="配置 roundtable 模式成员和最大迭代次数。">
        <Button
          className="w-full sm:w-auto"
          variant="outline"
          onClick={() =>
            editing.begin(`member-${roundtable.members?.length || 0}`, (next) => {
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
        <ConfigEditCard editorKey="roundtable-settings" title="讨论设置" description="设置圆桌讨论的执行轮数。" summary={[{ label: "最大迭代", value: `${roundtable.max_iterations || 0} 次` }]}>
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
        </ConfigEditCard>
        {(roundtable.members || []).map((member, index) => (
          <RoundtableMemberEditor key={index} member={member} index={index} modelIDs={modelIDs} updateDraft={updateDraft} />
        ))}
      </PanelBody>
    </Panel>
  );
}

export function DeepTab({ draft, updateDraft }: EditorProps) {
  const deep = draft.deep || {};
  const tools = useAppSelector((state) => state.config.tools);
  const toolOptions = useMemo(() => buildToolOptions(tools, draft.tools?.mcp_servers || []), [draft.tools?.mcp_servers, tools]);

  function update(patch: DeepConfig) {
    updateDraft((next) => {
      next.deep = mergeDeepConfig(next.deep || {}, patch);
    });
  }

  return (
    <div className="grid items-start gap-4 md:grid-cols-2">
      <ConfigEditCard editorKey="deep-agent" title="深度智能体" description="设置提示词、工具和最大迭代次数。"
        summary={[{ label: "最大迭代", value: `${deep.max_iterations || 0} 次` }, { label: "工具", value: `${deep.extra_tools?.length || 0} 项` }, { label: "提示词", value: deep.instruction ? "自定义" : "内置" }]}>
        <div className="space-y-4">
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
        </div>
      </ConfigEditCard>
      <ConfigEditCard editorKey="deep-capabilities" title="能力配置" description="选择深度模式可直接使用的核心能力。"
        summary={[{ label: "计划清单", value: deep.planning?.enabled ? "开启" : "关闭" }, { label: "工作区", value: deep.workspace?.enabled ? "开启" : "关闭" }, { label: "Shell", value: deep.shell?.enabled ? "开启" : "关闭" }]}>
          <div className="space-y-3">
            <ToggleField label="计划清单" checked={Boolean(deep.planning?.enabled)} onChange={(value) => update({ planning: { enabled: value } })} />
            <ToggleField label="工作区文件" checked={Boolean(deep.workspace?.enabled)} onChange={(value) => update({ workspace: { enabled: value } })} />
            <ToggleField label="Shell 命令" checked={Boolean(deep.shell?.enabled)} onChange={(value) => update({ shell: { enabled: value } })} />
          </div>
      </ConfigEditCard>
    </div>
  );
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
  const editing = useConfigEditing();
  function update(patch: Partial<TeamMemberConfig>) {
    updateDraft((next) => {
      const members = [...(next.roundtable?.members || [])];
      members[index] = { ...members[index], ...patch };
      next.roundtable = { ...(next.roundtable || {}), members };
    });
  }

  return (
    <ConfigEditCard
      editorKey={`member-${index}`}
      description={member.description || "设置成员的模型、提示词和职责。"}
      title={member.name || `成员 ${index + 1}`}
      summary={[{ label: "模型", value: member.model_id || "默认" }, { label: "ID", value: member.id || "未设置" }]}
      onRemove={() =>
        editing.remove(member.name || member.id || "圆桌成员", (next) => {
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
    </ConfigEditCard>
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
        {agent.builtin && (agent.id === "coordinator" || agent.id === "workspace_assistant") ? <Badge>核心入口</Badge> : null}
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
    <Dialog open title="AI 创建智能体" closeDisabled={loading || applying} onOpenChange={onClose}
      overlay={loading ? <div className="flex flex-col items-center gap-3"><LoadingSurface label="正在生成智能体草稿" /><Button variant="outline" onClick={cancelGenerate}>取消生成</Button></div> : undefined}
      footer={<div className="flex flex-wrap justify-end gap-2">
        <Button variant="outline" disabled={loading || applying} onClick={onClose}>取消</Button>
        <Button variant="outline" disabled={!instruction.trim() || loading || applying} onClick={() => void generate()}><Sparkles className="h-4 w-4" />{loading ? "生成中" : "生成草稿"}</Button>
        <Button disabled={!selectedDrafts.length || loading || applying} onClick={async () => {
          setError("");
          try { await onApply(selectedDrafts); }
          catch (err) { setError(err instanceof Error ? err.message : String(err)); }
        }}><Plus className="h-4 w-4" />{applying ? "保存中" : "确认并保存"}</Button>
      </div>}>
      <p className="mb-4 text-sm text-muted-foreground">描述角色、职责和工具偏好，选择生成的智能体后确认保存。</p>
        <div className="space-y-4">
          <div className="grid gap-4">
            <Field label="创建要求">
              <Textarea
                className="min-h-28 text-sm"
                disabled={loading || applying}
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
                      <input className="mt-1 h-4 w-4 shrink-0 accent-primary" type="checkbox" disabled={loading || applying} checked={checked} onChange={() => toggle(id)} />
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

    </Dialog>
  );
}
