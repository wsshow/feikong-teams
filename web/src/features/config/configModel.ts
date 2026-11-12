import { isMCPToolGroup } from "@/features/config/toolCatalog";
import type { AppConfig, AgentConfig, ChannelDiscordConfig, ChannelQQConfig, ChannelWeixinConfig, DeepConfig, MCPServerConfig, ModelConfig, ServerAuthConfig, TeamMemberConfig, ToolInfo } from "@/types/config";

export const knownTopLevelConfigKeys = new Set([
  "models",
  "server",
  "agents",
  "tools",
  "channels",
  "memory",
  "openai_api",
  "roundtable",
  "deep",
  "javascript",
]);

export const approvalStoreOptions = [
  { value: "command", label: "命令执行" },
  { value: "file", label: "外部文件" },
  { value: "git", label: "Git 操作" },
  { value: "dispatch", label: "任务分发" },
];

export function uniqueJavaScriptID(values: Array<string | undefined>, base: string) {
  const used = new Set(values.filter(Boolean));
  if (!used.has(base)) return base;
  let suffix = 2;
  while (used.has(`${base}_${suffix}`)) suffix += 1;
  return `${base}_${suffix}`;
}

export interface EditorProps {
  draft: AppConfig;
  updateDraft: (mutator: (next: AppConfig) => void) => void;
  autoSaveDraft?: (next: AppConfig) => Promise<void>;
  saving?: boolean;
}

export interface ToolSelectOption {
  name: string;
  label: string;
  description?: string;
  category?: string;
  source: "builtin" | "mcp" | "custom";
  readOnly?: boolean;
  destructive?: boolean;
  enabled?: boolean;
}

export function buildToolOptions(tools: ToolInfo[], mcpServers: MCPServerConfig[]): ToolSelectOption[] {
  const options = new Map<string, ToolSelectOption>();
  for (const tool of tools) {
    const name = tool.name.trim();
    if (!name) continue;
    options.set(name, {
      name,
      label: tool.display_name || name,
      description: tool.description || tool.included_tools?.join(", "),
      category: tool.category,
      source: isMCPToolGroup(tool) ? "mcp" : tool.builtin === false ? "custom" : "builtin",
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

export function uniqueToolNames(values: string[]) {
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

export const modelUseOptions = [
  { value: "chat", label: "对话" },
  { value: "agent", label: "智能体" },
  { value: "title", label: "标题" },
  { value: "summary", label: "摘要" },
];

export function updateModel(updateDraft: EditorProps["updateDraft"], index: number, patch: Partial<ModelConfig>) {
  updateDraft((next) => {
    const models = [...(next.models || [])];
    models[index] = { ...models[index], ...patch, original_id: models[index]?.original_id || models[index]?.id };
    next.models = models;
  });
}

export function setServer(config: AppConfig, patch: AppConfig["server"]) {
  config.server = { ...(config.server || {}), ...patch };
}

export function setAuth(config: AppConfig, patch: Partial<ServerAuthConfig>) {
  config.server = { ...(config.server || {}), auth: { ...(config.server?.auth || {}), ...patch } };
}

function setAgents(config: AppConfig, patch: AppConfig["agents"]) {
  config.agents = { ...(config.agents || {}), ...patch };
}

export function setQQ(config: AppConfig, patch: Partial<ChannelQQConfig>) {
  config.channels = { ...(config.channels || {}), qq: { ...(config.channels?.qq || {}), ...patch } };
}

export function setDiscord(config: AppConfig, patch: Partial<ChannelDiscordConfig>) {
  config.channels = { ...(config.channels || {}), discord: { ...(config.channels?.discord || {}), ...patch } };
}

export function setWeixin(config: AppConfig, patch: Partial<ChannelWeixinConfig>) {
  config.channels = { ...(config.channels || {}), weixin: { ...(config.channels?.weixin || {}), ...patch } };
}

export function mergeDeepConfig(current: DeepConfig, patch: DeepConfig): DeepConfig {
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

export function normalizeConfig(config: AppConfig): AppConfig {
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
  next.javascript = next.javascript || {};
  next.javascript.tools = next.javascript.tools || [];
  next.javascript.hooks = next.javascript.hooks || [];
  return next;
}

export function normalizeConfigForSave(config: AppConfig): AppConfig {
  const next = normalizeConfig(config);
  next.agents = {
    ...(next.agents || {}),
    items: (next.agents?.items || []).flatMap((agent) => {
      if (agent.builtin && (agent.id === "coordinator" || agent.id === "workspace_assistant")) return [];
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

export function normalizeApprovalStores(stores: string[]) {
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

export function uniqueName(models: ModelConfig[], base: string) {
  const used = new Set(models.map((model) => model.name));
  if (!used.has(base)) return base;
  let index = 2;
  while (used.has(`${base}-${index}`)) index += 1;
  return `${base}-${index}`;
}

export function uniqueModelID(models: ModelConfig[], base: string) {
  const used = new Set(models.map((model) => model.id));
  return uniqueID(base, used);
}

export function uniqueMemberID(members: TeamMemberConfig[], base: string) {
  const used = new Set(members.map((member) => member.id));
  return uniqueID(base, used);
}

export function uniqueAgentID(agents: AgentConfig[], base: string) {
  const used = new Set(agents.map((agent) => agent.id));
  return uniqueID(base, used);
}

export function uniqueMCPServerID(servers: MCPServerConfig[], base: string) {
  const used = new Set(servers.map((server) => server.id));
  return uniqueID(base, used);
}

function uniqueID(base: string, used: Set<string | undefined>) {
  if (!used.has(base)) return base;
  let index = 2;
  while (used.has(`${base}-${index}`)) index += 1;
  return `${base}-${index}`;
}
