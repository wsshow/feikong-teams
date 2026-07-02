export interface ModelConfig {
  id: string;
  name: string;
  use_for?: string[];
  provider: string;
  model: string;
  api_key?: string;
  base_url?: string;
  extra_headers?: string;
  has_api_key?: boolean;
  original_id?: string;
}

export interface MemoryConfig {
  enabled?: boolean;
}

export interface ServerAuthConfig {
  enabled?: boolean;
  username?: string;
  password?: string;
  secret?: string;
}

export interface ServerConfig {
  host?: string;
  port?: number;
  log_level?: string;
  allow_origins?: string[];
  auth?: ServerAuthConfig;
}

export interface AgentSSHConfig {
  host?: string;
  username?: string;
  password?: string;
}

export interface AgentConfig {
  id?: string;
  name?: string;
  description?: string;
  prompt?: string;
  model_id?: string;
  tools?: string[];
  ssh?: AgentSSHConfig;
  enabled?: boolean;
  builtin?: boolean;
  team_member?: boolean;
}

export interface AgentsConfig {
  items?: AgentConfig[];
}

export interface ChannelQQConfig {
  enabled?: boolean;
  app_id?: string;
  app_secret?: string;
  sandbox?: boolean;
  mode?: string;
  agent_id?: string;
}

export interface ChannelDiscordConfig {
  enabled?: boolean;
  token?: string;
  allow_from?: string;
  mode?: string;
  agent_id?: string;
}

export interface ChannelWeixinConfig {
  enabled?: boolean;
  base_url?: string;
  cred_path?: string;
  log_level?: string;
  allow_from?: string;
  mode?: string;
  agent_id?: string;
}

export interface ChannelsConfig {
  qq?: ChannelQQConfig;
  discord?: ChannelDiscordConfig;
  weixin?: ChannelWeixinConfig;
}

export interface TeamMemberConfig {
  id?: string;
  name?: string;
  description?: string;
  model_id?: string;
  prompt?: string;
}

export interface RoundtableConfig {
  members?: TeamMemberConfig[];
  max_iterations?: number;
}

export type CustomAgentConfig = AgentConfig;

export interface MCPServerConfig {
  id?: string;
  name?: string;
  description?: string;
  enabled?: boolean;
  timeout?: string;
  url?: string;
  command?: string;
  env?: Record<string, string>;
  args?: string[];
  transport?: string;
}

export interface CustomConfig {
  moderator?: CustomAgentConfig;
  agents?: CustomAgentConfig[];
  mcp_servers?: MCPServerConfig[];
}

export interface OpenAIAPIConfig {
  api_keys?: string[];
}

export interface ToolInfo {
  name: string;
  display_name?: string;
  description?: string;
  category?: string;
  builtin?: boolean;
  read_only?: boolean;
  destructive?: boolean;
  included_tools?: string[];
}

export interface AppConfig {
  models?: ModelConfig[];
  server?: ServerConfig;
  agents?: AgentsConfig;
  custom?: CustomConfig;
  channels?: ChannelsConfig;
  memory?: MemoryConfig;
  openai_api?: OpenAIAPIConfig;
  roundtable?: RoundtableConfig;
  [key: string]: unknown;
}
