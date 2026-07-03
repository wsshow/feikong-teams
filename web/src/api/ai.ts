import { post } from "@/api/client";
import type { AgentConfig } from "@/types/config";

export interface AgentDraftRequest {
  instruction: string;
  existing_agents?: string[];
  available_tools?: string[];
  available_models?: string[];
  default_model_id?: string;
}

export interface AgentDraftResponse {
  agents: AgentConfig[];
}

export interface RewriteTextRequest {
  scenario: string;
  instruction: string;
  text: string;
  context?: Record<string, unknown>;
}

export interface RewriteTextResponse {
  text: string;
}

export function generateAgentDrafts(body: AgentDraftRequest) {
  return post<AgentDraftResponse>("/api/fkteams/ai/agents/draft", body);
}

export function rewriteText(body: RewriteTextRequest) {
  return post<RewriteTextResponse>("/api/fkteams/ai/text/rewrite", body);
}
