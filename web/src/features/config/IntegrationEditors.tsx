import { Cable, FileCode2, Play, Plus, Sparkles, Wrench, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { generateJavaScriptDraft } from "@/api/ai";
import { testJavaScriptTool } from "@/api/javascript";
import { useAppSelector } from "@/app/hooks";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { Panel, PanelBody, PanelHeader } from "@/components/ui/panel";
import { cn } from "@/lib/cn";
import { splitToolCatalog } from "@/features/config/toolCatalog";
import type { JavaScriptHookConfig, JavaScriptToolConfig, MCPServerConfig, ToolInfo } from "@/types/config";
import { uniqueJavaScriptID, type EditorProps, uniqueMCPServerID } from "./configModel";
import { ExtensionConfigCard, ConfigCard, SectionTitle, SectionHeader, Field, TextField, NumberField, SelectField, ToggleField, StringListField, KeyValueMapField, EmptyState } from "./ConfigFields";

export function ToolsTab({ draft, updateDraft }: EditorProps) {
  const tools = useAppSelector((state) => state.config.tools);
  const { builtin: builtinTools, custom: customTools, mcp: mcpTools } = splitToolCatalog(tools);
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
      {customTools.length ? (
        <Panel>
          <PanelHeader>
            <SectionTitle icon={FileCode2} title="自定义工具" description={`${customTools.length} 个用户创建的工具组，可在“扩展”页中编辑。`} />
          </PanelHeader>
          <PanelBody className="grid gap-3 xl:grid-cols-2">
            {customTools.map((tool) => (
              <ToolInfoCard key={tool.name} tool={tool} />
            ))}
          </PanelBody>
        </Panel>
      ) : null}
      <Panel>
        <SectionHeader icon={Cable} title="MCP 服务" description="连接 HTTP 或 stdio MCP 服务，启用后可在智能体工具中选择其能力。">
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
            添加 MCP 服务
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
            <SectionTitle icon={Cable} title="已连接的 MCP 工具" description={`${mcpTools.length} 个来自已启用 MCP 服务的工具组。`} />
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

const hookPointOptions = [
  "before_run",
  "after_run",
  "on_event",
  "before_tool_call",
  "after_tool_call",
  "before_model_request",
  "after_model_response",
];

const hookPointLabels: Record<string, string> = {
  before_run: "任务开始前",
  after_run: "任务结束后",
  on_event: "事件发送时",
  before_tool_call: "工具调用前",
  after_tool_call: "工具调用后",
  before_model_request: "模型请求前",
  after_model_response: "模型响应后",
};

function toolParameterCount(tool: JavaScriptToolConfig) {
  const properties = tool.parameters?.properties;
  if (!properties || typeof properties !== "object" || Array.isArray(properties)) return 0;
  return Object.keys(properties).length;
}

function flowRuleSummary(points: string[]) {
  if (!points.length) return "尚未选择触发环节";
  return points.map((point) => hookPointLabels[point] || point).join("、");
}

function errorPolicyLabel(policy: string) {
  return ({ fail: "中止任务", warn: "记录警告", ignore: "忽略错误" } as Record<string, string>)[policy] || policy;
}

type JavaScriptDialogState = { kind: "tool" | "hook"; index?: number };

export function JavaScriptTab({ draft, updateDraft }: EditorProps) {
  const tools = draft.javascript?.tools || [];
  const hooks = draft.javascript?.hooks || [];
  const [dialog, setDialog] = useState<JavaScriptDialogState | null>(null);
  const [expandedToolIndex, setExpandedToolIndex] = useState<number | null>(null);
  const [expandedHookIndex, setExpandedHookIndex] = useState<number | null>(null);

  function addTool() {
    updateDraft((next) => {
      const items = next.javascript?.tools || [];
      next.javascript = {
        ...(next.javascript || {}),
        tools: [
          ...items,
          {
            id: uniqueJavaScriptID(items.map((item) => item.id), "custom_tool"),
            name: "自定义工具",
            description: "",
            enabled: false,
            timeout_ms: 200,
            read_only: true,
            parameters: { type: "object", properties: {} },
            source: "function execute(input, context) {\n  return input;\n}",
          },
        ],
      };
    });
    setExpandedToolIndex(tools.length);
  }

  function addHook() {
    updateDraft((next) => {
      const items = next.javascript?.hooks || [];
      next.javascript = {
        ...(next.javascript || {}),
        hooks: [
          ...items,
          {
            id: uniqueJavaScriptID(items.map((item) => item.id), "custom_hook"),
            name: "自定义流程规则",
            enabled: false,
            hook_points: ["before_tool_call"],
            timeout_ms: 200,
            error_policy: "fail",
            priority: 100,
            source: "function handle(hook) {\n  return { action: \"continue\" };\n}",
          },
        ],
      };
    });
    setExpandedHookIndex(hooks.length);
  }

  function applyAIDraft(value: JavaScriptToolConfig | JavaScriptHookConfig) {
    if (!dialog) return;
    const targetIndex = dialog.index ?? (dialog.kind === "tool" ? tools.length : hooks.length);
    updateDraft((next) => {
      if (dialog.kind === "tool") {
        const items = [...(next.javascript?.tools || [])];
        if (dialog.index === undefined) items.push(value as JavaScriptToolConfig);
        else items[dialog.index] = value as JavaScriptToolConfig;
        next.javascript = { ...(next.javascript || {}), tools: items };
        return;
      }
      const items = [...(next.javascript?.hooks || [])];
      if (dialog.index === undefined) items.push(value as JavaScriptHookConfig);
      else items[dialog.index] = value as JavaScriptHookConfig;
      next.javascript = { ...(next.javascript || {}), hooks: items };
    });
    if (dialog.kind === "tool") setExpandedToolIndex(targetIndex);
    else setExpandedHookIndex(targetIndex);
    setDialog(null);
  }

  const current = dialog?.kind === "tool" ? (dialog.index === undefined ? undefined : tools[dialog.index]) : dialog?.index === undefined ? undefined : hooks[dialog.index];
  const existingIDs = (dialog?.kind === "tool" ? tools : hooks)
    .filter((_, index) => index !== dialog?.index)
    .map((item) => item.id || "")
    .filter(Boolean);

  return (
    <div className="space-y-4">
      <Panel>
        <SectionHeader icon={FileCode2} title="自定义工具" description="创建智能体可直接调用的新工具；工作区、命令、网络、状态和通知能力会自动接入。">
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" onClick={() => setDialog({ kind: "tool" })}>
              <Sparkles className="h-4 w-4" />
              AI 创建工具
            </Button>
            <Button onClick={addTool}>
              <Plus className="h-4 w-4" />
              手动添加
            </Button>
          </div>
        </SectionHeader>
        <PanelBody className="grid gap-4 xl:grid-cols-2">
          {tools.map((tool, index) => {
            const expanded = expandedToolIndex === index;
            return (
              <JavaScriptToolEditor
                key={index}
                tool={tool}
                expanded={expanded}
                onToggle={() => setExpandedToolIndex(expanded ? null : index)}
                onAI={() => setDialog({ kind: "tool", index })}
                onChange={(value) =>
                  updateDraft((next) => {
                    const items = [...(next.javascript?.tools || [])];
                    items[index] = value;
                    next.javascript = { ...(next.javascript || {}), tools: items };
                  })
                }
                onRemove={() => {
                  updateDraft((next) => {
                    next.javascript = { ...(next.javascript || {}), tools: (next.javascript?.tools || []).filter((_, itemIndex) => itemIndex !== index) };
                  });
                  setExpandedToolIndex((current) => current === index ? null : current !== null && current > index ? current - 1 : current);
                }}
              />
            );
          })}
          {tools.length === 0 ? <EmptyState title="暂无自定义工具" description="手动添加，或描述需求让 AI 创建完整工具。" /> : null}
        </PanelBody>
      </Panel>

      <Panel>
        <SectionHeader icon={Cable} title="流程规则" description="在任务运行、事件、工具调用和模型请求等环节观察、改写、跳过或拒绝流程。">
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" onClick={() => setDialog({ kind: "hook" })}>
              <Sparkles className="h-4 w-4" />
              AI 创建规则
            </Button>
            <Button onClick={addHook}>
              <Plus className="h-4 w-4" />
              手动添加
            </Button>
          </div>
        </SectionHeader>
        <PanelBody className="grid gap-4 xl:grid-cols-2">
          {hooks.map((hook, index) => {
            const expanded = expandedHookIndex === index;
            return (
              <JavaScriptHookEditor
                key={index}
                hook={hook}
                expanded={expanded}
                onToggle={() => setExpandedHookIndex(expanded ? null : index)}
                onAI={() => setDialog({ kind: "hook", index })}
                onChange={(value) =>
                  updateDraft((next) => {
                    const items = [...(next.javascript?.hooks || [])];
                    items[index] = value;
                    next.javascript = { ...(next.javascript || {}), hooks: items };
                  })
                }
                onRemove={() => {
                  updateDraft((next) => {
                    next.javascript = { ...(next.javascript || {}), hooks: (next.javascript?.hooks || []).filter((_, itemIndex) => itemIndex !== index) };
                  });
                  setExpandedHookIndex((current) => current === index ? null : current !== null && current > index ? current - 1 : current);
                }}
              />
            );
          })}
          {hooks.length === 0 ? <EmptyState title="暂无流程规则" description="添加规则后可按需控制智能体执行流程。" /> : null}
        </PanelBody>
      </Panel>

      {dialog ? (
        <JavaScriptAIDialog
          kind={dialog.kind}
          current={current}
          existingIDs={existingIDs}
          onApply={applyAIDraft}
          onClose={() => setDialog(null)}
        />
      ) : null}
    </div>
  );
}

function JavaScriptToolEditor({
  tool,
  expanded,
  onToggle,
  onChange,
  onRemove,
  onAI,
}: {
  tool: JavaScriptToolConfig;
  expanded: boolean;
  onToggle: () => void;
  onChange: (value: JavaScriptToolConfig) => void;
  onRemove: () => void;
  onAI: () => void;
}) {
  const update = (patch: Partial<JavaScriptToolConfig>) => onChange({ ...tool, ...patch });
  const [testInput, setTestInput] = useState("{}");
  const [testResult, setTestResult] = useState("");
  const [testNotices, setTestNotices] = useState<Array<{ level: string; message: string }>>([]);
  const [testError, setTestError] = useState("");
  const [testing, setTesting] = useState(false);
  const [testRan, setTestRan] = useState(false);

  async function runTest() {
    if (testing) return;
    setTestError("");
    let input: Record<string, unknown>;
    try {
      const parsed = JSON.parse(testInput) as unknown;
      if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") throw new Error("测试输入必须是 JSON 对象");
      input = parsed as Record<string, unknown>;
    } catch (error) {
      setTestError(error instanceof Error ? error.message : String(error));
      return;
    }
    setTesting(true);
    setTestRan(false);
    try {
      const response = await testJavaScriptTool(tool, input);
      setTestResult(response.raw);
      setTestNotices(response.notices || []);
      setTestRan(true);
    } catch (error) {
      setTestResult("");
      setTestNotices([]);
      setTestError(error instanceof Error ? error.message : String(error));
    } finally {
      setTesting(false);
    }
  }

  return (
    <ExtensionConfigCard
      title={tool.name || tool.id || "未命名工具"}
      id={tool.id}
      description={tool.description}
      expanded={expanded}
      badges={[tool.enabled ? "已启用" : "已关闭", tool.read_only ?? true ? "只读" : "可修改"]}
      metrics={[
        { label: "状态", value: tool.enabled ? "启用" : "关闭" },
        { label: "宿主能力", value: "自动接入" },
        { label: "参数", value: `${toolParameterCount(tool)} 项` },
        { label: "超时", value: `${tool.timeout_ms ?? 200} ms` },
      ]}
      onToggle={onToggle}
      onAI={onAI}
      onRemove={onRemove}
    >
      <ToggleField label="启用工具" checked={Boolean(tool.enabled)} onChange={(enabled) => update({ enabled })} />
      <div className="grid gap-3 md:grid-cols-2">
        <TextField label="工具 ID" value={tool.id} onChange={(id) => update({ id })} placeholder="lower_snake_case" />
        <TextField label="展示名称" value={tool.name} onChange={(name) => update({ name })} />
      </div>
      <TextField label="工具描述" value={tool.description} onChange={(description) => update({ description })} placeholder="说明用途、调用时机和返回内容" />
      <div className="grid gap-3 md:grid-cols-2">
        <NumberField label="超时（毫秒）" value={tool.timeout_ms ?? 200} min={10} max={120000} onChange={(timeout_ms) => update({ timeout_ms })} />
        <ToggleField label="只读工具" checked={tool.read_only ?? true} onChange={(read_only) => update({ read_only })} />
      </div>
      <div className="rounded-xl border border-border/75 bg-background/45 px-3 py-2 text-xs leading-5 text-muted-foreground">
        可直接调用工作区、命令、网络、状态和通知函数；危险操作仍受审批和工作区边界保护。
      </div>
      <JSONSchemaField value={tool.parameters} onChange={(parameters) => update({ parameters })} />
      <Field label="执行代码（JavaScript，必须定义 execute(input, context)）">
        <Textarea className="min-h-64 font-mono text-xs leading-5" value={tool.source || ""} spellCheck={false} onChange={(event) => update({ source: event.target.value })} />
      </Field>
      <Field
        label="试运行输入"
        action={
          <Button size="sm" variant="outline" disabled={testing} onClick={() => void runTest()}>
            <Play className="h-3.5 w-3.5" />
            {testing ? "执行中" : "试运行"}
          </Button>
        }
      >
        <Textarea className="min-h-28 font-mono text-xs leading-5" value={testInput} spellCheck={false} onChange={(event) => setTestInput(event.target.value)} />
        <div className="text-xs leading-5 text-amber-700">试运行会真实调用宿主能力并可能产生副作用；工具审批和工作区边界仍然生效。</div>
        {testError ? <div className="rounded-lg border border-destructive/40 bg-destructive/5 px-3 py-2 text-xs text-destructive">{testError}</div> : null}
        {testRan ? <Textarea className="min-h-28 font-mono text-xs leading-5" value={testResult} placeholder="工具返回空结果" readOnly /> : null}
        {testNotices.length > 0 ? (
          <div className="space-y-1 rounded-lg border border-border/70 bg-card/45 px-3 py-2">
            {testNotices.map((notice, index) => (
              <div key={`${notice.level}-${index}`} className="text-xs"><Badge>{notice.level}</Badge> <span className="ml-1">{notice.message}</span></div>
            ))}
          </div>
        ) : null}
      </Field>
    </ExtensionConfigCard>
  );
}

function JavaScriptHookEditor({
  hook,
  expanded,
  onToggle,
  onChange,
  onRemove,
  onAI,
}: {
  hook: JavaScriptHookConfig;
  expanded: boolean;
  onToggle: () => void;
  onChange: (value: JavaScriptHookConfig) => void;
  onRemove: () => void;
  onAI: () => void;
}) {
  const update = (patch: Partial<JavaScriptHookConfig>) => onChange({ ...hook, ...patch });
  return (
    <ExtensionConfigCard
      title={hook.name || hook.id || "未命名流程规则"}
      id={hook.id}
      description={flowRuleSummary(hook.hook_points || [])}
      expanded={expanded}
      badges={[hook.enabled ? "已启用" : "已关闭", "流程规则"]}
      metrics={[
        { label: "状态", value: hook.enabled ? "启用" : "关闭" },
        { label: "触发环节", value: `${hook.hook_points?.length || 0} 项` },
        { label: "优先级", value: `${hook.priority ?? 100}` },
        { label: "异常处理", value: errorPolicyLabel(hook.error_policy || "fail") },
      ]}
      onToggle={onToggle}
      onAI={onAI}
      onRemove={onRemove}
    >
      <ToggleField label="启用规则" checked={Boolean(hook.enabled)} onChange={(enabled) => update({ enabled })} />
      <div className="grid gap-3 md:grid-cols-2">
        <TextField label="规则 ID" value={hook.id} onChange={(id) => update({ id })} placeholder="lower_snake_case" />
        <TextField label="展示名称" value={hook.name} onChange={(name) => update({ name })} />
      </div>
      <HookPointField values={hook.hook_points || []} onChange={(hook_points) => update({ hook_points })} />
      <div className="grid gap-3 md:grid-cols-3">
        <NumberField label="超时（毫秒）" value={hook.timeout_ms ?? 200} min={10} max={5000} onChange={(timeout_ms) => update({ timeout_ms })} />
        <NumberField label="优先级" value={hook.priority ?? 100} onChange={(priority) => update({ priority })} />
        <SelectField label="异常处理" value={hook.error_policy || "fail"} options={["fail", "warn", "ignore"]} optionLabels={{ fail: "中止任务", warn: "记录警告", ignore: "忽略错误" }} onChange={(error_policy) => update({ error_policy: error_policy as JavaScriptHookConfig["error_policy"] })} />
      </div>
      <Field label="规则代码（JavaScript，必须定义 handle(hook)）">
        <Textarea className="min-h-64 font-mono text-xs leading-5" value={hook.source || ""} spellCheck={false} onChange={(event) => update({ source: event.target.value })} />
      </Field>
    </ExtensionConfigCard>
  );
}

function HookPointField({ values, onChange }: { values: string[]; onChange: (values: string[]) => void }) {
  const selected = new Set(values);
  return (
    <Field label="触发环节">
      <div className="flex flex-wrap gap-2 rounded-xl border border-border/75 bg-background/45 p-3">
        {hookPointOptions.map((point) => (
          <button
            key={point}
            type="button"
            className={cn("rounded-lg border px-2.5 py-1.5 text-xs", selected.has(point) ? "border-primary/50 bg-primary/10 text-primary" : "border-border text-muted-foreground")}
            onClick={() => onChange(selected.has(point) ? values.filter((item) => item !== point) : [...values, point])}
          >
            {hookPointLabels[point] || point}
          </button>
        ))}
      </div>
    </Field>
  );
}

function JSONSchemaField({ value, onChange }: { value?: Record<string, unknown>; onChange: (value: Record<string, unknown>) => void }) {
  const serialized = JSON.stringify(value || { type: "object", properties: {} }, null, 2);
  const [text, setText] = useState(serialized);
  const [error, setError] = useState("");
  const committed = useRef(serialized);

  useEffect(() => {
    if (serialized === committed.current) return;
    committed.current = serialized;
    setText(serialized);
    setError("");
  }, [serialized]);

  function update(raw: string) {
    setText(raw);
    try {
      const parsed = JSON.parse(raw) as Record<string, unknown>;
      if (!parsed || Array.isArray(parsed) || parsed.type !== "object") throw new Error("根 type 必须是 object");
      const normalized = JSON.stringify(parsed, null, 2);
      committed.current = normalized;
      setError("");
      onChange(parsed);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  return (
    <Field label="参数 JSON Schema">
      <Textarea className="min-h-44 font-mono text-xs leading-5" value={text} spellCheck={false} onChange={(event) => update(event.target.value)} />
      {error ? <div className="text-xs text-destructive">Schema 尚未生效：{error}</div> : null}
    </Field>
  );
}

function JavaScriptAIDialog({
  kind,
  current,
  existingIDs,
  onApply,
  onClose,
}: {
  kind: "tool" | "hook";
  current?: JavaScriptToolConfig | JavaScriptHookConfig;
  existingIDs: string[];
  onApply: (value: JavaScriptToolConfig | JavaScriptHookConfig) => void;
  onClose: () => void;
}) {
  const [instruction, setInstruction] = useState("");
  const [result, setResult] = useState<JavaScriptToolConfig | JavaScriptHookConfig>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function generate() {
    if (!instruction.trim() || loading) return;
    setLoading(true);
    setError("");
    try {
      const response = await generateJavaScriptDraft({
        kind,
        instruction: instruction.trim(),
        existing_ids: existingIDs,
        current_tool: kind === "tool" ? (current as JavaScriptToolConfig | undefined) : undefined,
        current_hook: kind === "hook" ? (current as JavaScriptHookConfig | undefined) : undefined,
      });
      const value = kind === "tool" ? response.tool : response.hook;
      if (!value) throw new Error(`AI 未返回有效${kind === "tool" ? "工具" : "流程规则"}`);
      setResult(value);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-foreground/15 p-3 backdrop-blur-[1px] sm:p-6" role="dialog" aria-modal="true">
      <div className="sketch-surface flex max-h-[calc(100dvh-1.5rem)] w-full max-w-3xl flex-col overflow-hidden rounded-2xl bg-card/95 shadow-[0_18px_48px_hsl(218_30%_20%/0.18)]">
        <div className="flex items-start justify-between gap-3 border-b border-border/70 p-4">
          <div>
            <div className="flex items-center gap-2 font-semibold">
              <Sparkles className="h-4 w-4 text-primary" />
              AI {current ? "修改" : "创建"}{kind === "tool" ? "自定义工具" : "流程规则"}
            </div>
            <div className="mt-1 text-sm leading-6 text-muted-foreground">生成结果会先经过代码校验，确认应用后仍需保存配置才会生效。</div>
          </div>
          <Button size="icon" variant="ghost" onClick={onClose} aria-label="关闭">
            <X className="h-4 w-4" />
          </Button>
        </div>
        <div className="chat-scroll min-h-0 flex-1 space-y-4 overflow-auto p-4">
          <Field label="扩展需求">
            <Textarea
              className="min-h-28 text-sm"
              value={instruction}
              placeholder={kind === "tool" ? "例如：读取指定工作区文件，提取 Markdown 表格并把累计处理次数持久化。" : "例如：调用 execute 工具前拒绝包含 sudo 的命令。"}
              onChange={(event) => setInstruction(event.target.value)}
            />
          </Field>
          {error ? <div className="rounded-lg border border-destructive/40 bg-destructive/5 px-3 py-2 text-sm text-destructive">{error}</div> : null}
          {result ? (
            <Field label="已通过校验的草稿">
              <Textarea className="min-h-72 font-mono text-xs leading-5" value={JSON.stringify(result, null, 2)} readOnly />
            </Field>
          ) : null}
        </div>
        <div className="flex flex-col gap-2 border-t border-border/70 p-4 sm:flex-row sm:justify-end">
          <Button variant="outline" onClick={onClose}>取消</Button>
          <Button variant="outline" disabled={!instruction.trim() || loading} onClick={() => void generate()}>
            <Sparkles className="h-4 w-4" />
            {loading ? "生成中" : "生成草稿"}
          </Button>
          <Button disabled={!result} onClick={() => result && onApply(result)}>应用草稿</Button>
        </div>
      </div>
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
