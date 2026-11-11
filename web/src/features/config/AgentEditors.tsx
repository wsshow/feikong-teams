import { Bot, Brain, Check, ChevronDown, Copy, Layers, ListPlus, Plus, RefreshCcw, Sparkles, Trash2, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { generateAgentDrafts } from "@/api/ai";
import { isAbortError } from "@/api/client";
import { listProviderModels } from "@/api/providers";
import { appActions } from "@/app/store";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { ConfirmDialog } from "@/components/ui/action-dialog";
import { LoadingSurface } from "@/components/ui/loading-surface";
import { Panel, PanelBody, PanelHeader } from "@/components/ui/panel";
import { cn } from "@/lib/cn";
import type { AgentConfig, DeepConfig, ModelConfig, TeamMemberConfig } from "@/types/config";
import { type EditorProps, type ToolSelectOption, buildToolOptions, updateModel, mergeDeepConfig, normalizeConfig, uniqueName, uniqueModelID, uniqueMemberID, uniqueAgentID } from "./configModel";
import { AgentSummaryMetric, AITextField, AIRewriteButton, ToolSelectField, ConfigCard, SectionTitle, SectionHeader, Field, TextField, ModelAPIKeyField, NumberField, SelectField, ModelUseField, ModelSelect, ToggleField, EmptyState } from "./ConfigFields";

export function ModelsTab({ draft, updateDraft }: EditorProps) {
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

export function AgentsTab({ draft, modelIDs, updateDraft, autoSaveDraft, saving }: EditorProps & { modelIDs: string[] }) {
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



export function RoundtableTab({ draft, modelIDs, updateDraft }: EditorProps & { modelIDs: string[] }) {
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
