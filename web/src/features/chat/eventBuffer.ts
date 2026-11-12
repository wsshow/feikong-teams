import type { ChatEvent } from "@/types/events";

export const maxFallbackEventKeys = 2048;
const fallbackEventKeyPruneCount = 512;

export interface ChatEventDedupState {
  seenEventKeys: Record<string, true>;
  seenEventKeyOrder: string[];
  seenStreamEventID?: number;
}

export function stableChatEventIdentity(event: ChatEvent) {
  if (event.event_id) return `event:${event.event_id}`;
  if (event.run_id && event.sequence !== undefined) return `run:${event.run_id}:${event.sequence}`;
  return undefined;
}

// rememberChatEvent 使用流序号水位线去重，避免为每个增量事件长期保存字符串键。
export function rememberChatEvent(state: ChatEventDedupState, event: ChatEvent) {
  const identity = stableChatEventIdentity(event);
  const streamEventID = event.stream_event_id;
  if (typeof streamEventID === "number" && Number.isSafeInteger(streamEventID) && streamEventID >= 0) {
    if (identity && state.seenEventKeys[identity]) return false;
    if (state.seenStreamEventID !== undefined && streamEventID <= state.seenStreamEventID) return false;
    state.seenStreamEventID = streamEventID;
    return true;
  }
  if (!identity) return true;
  if (state.seenEventKeys[identity]) return false;
  state.seenEventKeys[identity] = true;
  state.seenEventKeyOrder.push(identity);
  if (state.seenEventKeyOrder.length > maxFallbackEventKeys) {
    const expired = state.seenEventKeyOrder.splice(0, fallbackEventKeyPruneCount);
    for (const key of expired) delete state.seenEventKeys[key];
  }
  return true;
}
