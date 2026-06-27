export interface ModelConfig {
  name: string;
  provider: string;
  model: string;
  api_key?: string;
  base_url?: string;
  extra_headers?: string;
  has_api_key?: boolean;
  original_name?: string;
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

export interface SSHVisitorConfig {
  enabled?: boolean;
  host?: string;
  username?: string;
  password?: string;
}

export interface AgentsConfig {
  researcher?: boolean;
  assistant?: boolean;
  analyst?: boolean;
  ssh_visitor?: SSHVisitorConfig;
}

export interface ChannelQQConfig {
  enabled?: boolean;
  app_id?: string;
  app_secret?: string;
  sandbox?: boolean;
  mode?: string;
}

export interface ChannelDiscordConfig {
  enabled?: boolean;
  token?: string;
  allow_from?: string;
  mode?: string;
}

export interface ChannelWeixinConfig {
  enabled?: boolean;
  base_url?: string;
  cred_path?: string;
  log_level?: string;
  allow_from?: string;
  mode?: string;
}

export interface ChannelsConfig {
  qq?: ChannelQQConfig;
  discord?: ChannelDiscordConfig;
  weixin?: ChannelWeixinConfig;
}

export interface TeamMemberConfig {
  index?: number;
  name?: string;
  desc?: string;
  model?: string;
}

export interface RoundtableConfig {
  members?: TeamMemberConfig[];
  max_iterations?: number;
}

export interface CustomAgentConfig {
  name?: string;
  desc?: string;
  system_prompt?: string;
  model?: string;
  tools?: string[];
}

export interface MCPServerConfig {
  name?: string;
  desc?: string;
  enabled?: boolean;
  timeout?: number;
  url?: string;
  command?: string;
  env_vars?: string[];
  args?: string[];
  transport_type?: string;
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
