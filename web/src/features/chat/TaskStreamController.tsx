import { useEffect, useRef } from "react";
import { APIError, isAbortError } from "@/api/client";
import { streamSnapshot, streamStatus, subscribeStream, type StreamSnapshot } from "@/api/stream";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { chatActions, sessionsActions, type AppDispatch } from "@/app/store";
import type { ChatEvent } from "@/types/events";
import { EventBatcher } from "./eventBatcher";
import { assistantSessionID } from "@/lib/assistant";
import { refreshAssistant } from "@/features/sessions/sessionThunks";

const streamReconnectBaseDelayMs = 400;
const streamReconnectMaxDelayMs = 5000;

interface ActiveSubscription {
  sessionID: string;
  mode: "foreground" | "background";
  controller: AbortController;
}

export function TaskStreamController() {
  const dispatch = useAppDispatch();
  const activeSessionID = useAppSelector((state) => state.chat.activeSessionID);
  const viewSessionID = useAppSelector((state) => state.chat.viewSessionID);
  const runningTasks = useAppSelector((state) => state.chat.runningTasks);
  const authExpired = useAppSelector((state) => state.app.authExpired);
  const activeSessionRef = useRef(activeSessionID);
  const viewSessionRef = useRef(viewSessionID);
  const subscriptionsRef = useRef(new Map<string, ActiveSubscription>());
  activeSessionRef.current = activeSessionID;
  viewSessionRef.current = viewSessionID;
  const taskKey = Object.entries(runningTasks)
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([sessionID, task]) => `${sessionID}:${task.phase}:${task.initialOffset ?? ""}`)
    .join("|");

  useEffect(() => {
    const subscriptions = subscriptionsRef.current;
    const desired = new Map<string, "foreground" | "background">();
    if (!authExpired) {
      for (const [sessionID, task] of Object.entries(runningTasks)) {
        if (task.phase !== "processing") continue;
        desired.set(
          sessionID,
          sessionID === activeSessionID && sessionID === viewSessionID ? "foreground" : "background",
        );
      }
    }

    for (const [sessionID, subscription] of subscriptions) {
      if (desired.get(sessionID) === subscription.mode) continue;
      subscription.controller.abort();
      subscriptions.delete(sessionID);
    }

    for (const [sessionID, mode] of desired) {
      if (subscriptions.has(sessionID)) continue;
      const controller = new AbortController();
      const subscription: ActiveSubscription = { sessionID, mode, controller };
      subscriptions.set(sessionID, subscription);
      const task = runningTasks[sessionID];
      const work = mode === "foreground"
        ? followTaskStream(
          sessionID,
          task.initialOffset,
          controller.signal,
          dispatch,
          () => activeSessionRef.current === sessionID && viewSessionRef.current === sessionID,
        )
        : monitorTaskStream(sessionID, controller.signal, dispatch);
      if (mode === "foreground") dispatch(chatActions.consumeStreamInitialOffset(sessionID));
      void work.finally(() => {
        if (subscriptions.get(sessionID) === subscription) subscriptions.delete(sessionID);
      });
    }
  }, [activeSessionID, authExpired, dispatch, taskKey, viewSessionID]);

  useEffect(() => () => {
    for (const subscription of subscriptionsRef.current.values()) subscription.controller.abort();
    subscriptionsRef.current.clear();
  }, []);

  return null;
}

async function followTaskStream(
  sessionID: string,
  initialOffset: number | undefined,
  signal: AbortSignal,
  dispatch: AppDispatch,
  canConsume: () => boolean,
) {
  let retryCount = 0;
  let nextOffset = initialOffset;
  let terminal = false;
  const eventBatcher = new EventBatcher<ChatEvent>((events) => {
    if (signal.aborted || !canConsume()) return;
    terminal = applyStreamEvents(sessionID, events, dispatch) || terminal;
  });
  try {
    for (;;) {
      if (signal.aborted) return;
      try {
        const offset = nextOffset ?? await replayStreamSnapshot(sessionID, signal, dispatch, canConsume);
        nextOffset = undefined;
        if (offset === undefined || signal.aborted) return;
        dispatch(chatActions.setTaskConnectionState({ sessionID, connectionState: "connecting" }));
        let connected = false;
        const result = await subscribeStream(sessionID, offset, (events) => {
          if (signal.aborted || !canConsume()) return;
          retryCount = 0;
          if (!connected) {
            connected = true;
            dispatch(chatActions.setTaskConnectionState({ sessionID, connectionState: "connected" }));
          }
          eventBatcher.push(events);
        }, signal);
        eventBatcher.flush();
        if (result === "done" && !signal.aborted) {
          dispatch(chatActions.setTaskConnectionState({ sessionID, connectionState: "connected" }));
          if (terminal) return;
        }
      } catch (error) {
        eventBatcher.flush();
        if (signal.aborted || isAbortError(error) || isAuthenticationError(error)) return;
        if (error instanceof APIError && error.status === 404) {
          finishFromStatus(sessionID, "idle", undefined, dispatch);
          return;
        }
      }

      if (signal.aborted || !canConsume()) return;
      retryCount += 1;
      await sleep(streamReconnectDelay(retryCount), signal);
    }
  } finally {
    if (signal.aborted || !canConsume()) eventBatcher.discard();
    else eventBatcher.flush();
  }
}

async function monitorTaskStream(sessionID: string, signal: AbortSignal, dispatch: AppDispatch) {
  let retryCount = 0;
  for (;;) {
    if (signal.aborted) return;
    try {
      dispatch(chatActions.setTaskConnectionState({ sessionID, connectionState: "connecting" }));
      const status = await streamStatus(sessionID, signal);
      if (signal.aborted) return;
      if (status.status !== "processing" || status.has_task === false) {
        finishFromStatus(sessionID, status.status, status.finished_at || status.updated_at, dispatch);
        return;
      }
      dispatch(chatActions.setTaskConnectionState({ sessionID, connectionState: "connected" }));
      let terminal = false;
      await subscribeStream(sessionID, Number(status.event_count || 0), (events) => {
        if (signal.aborted) return;
        refreshRenamedAssistant(sessionID, events, dispatch);
        const event = [...events].reverse().find((item) => terminalEventStatus(item));
        if (!event) return;
        terminal = true;
        markSessionFinished(sessionID, terminalEventStatus(event) || "completed", event.created_at, dispatch);
      }, signal);
      if (terminal || signal.aborted) return;
    } catch (error) {
      if (signal.aborted || isAbortError(error) || isAuthenticationError(error)) return;
    }

    if (!await shouldReconnect(sessionID, signal, dispatch)) return;
    retryCount += 1;
    await sleep(streamReconnectDelay(retryCount), signal);
  }
}

async function replayStreamSnapshot(
  sessionID: string,
  signal: AbortSignal,
  dispatch: AppDispatch,
  canConsume: () => boolean,
) {
  if (signal.aborted || !canConsume()) return undefined;
  const snapshot = await streamSnapshot(sessionID, { signal });
  if (signal.aborted || !canConsume()) return undefined;
  return applyStreamSnapshot(sessionID, snapshot, dispatch, canConsume);
}

function applyStreamSnapshot(
  sessionID: string,
  snapshot: StreamSnapshot,
  dispatch: AppDispatch,
  canConsume: () => boolean,
) {
  if (!canConsume()) return undefined;
  if (snapshot.mode) dispatch(chatActions.setMode(snapshot.mode));
  dispatch(chatActions.setCurrentAgent(snapshot.current_agent || ""));
  if (Array.isArray(snapshot.queue)) {
    dispatch(chatActions.setQueue(snapshot.queue));
  }
  const nextOffset = snapshot.next_offset;
  dispatch(chatActions.setPresentationSnapshot({ sessionID, view: snapshot.view, nextOffset }));
  if (snapshot.has_task === false || isTerminalStreamStatus(snapshot.status)) {
    markSessionFinished(sessionID, snapshot.status, snapshot.finished_at, dispatch);
    return undefined;
  }
  return nextOffset;
}

function applyStreamEvents(sessionID: string, events: ChatEvent[], dispatch: AppDispatch) {
  if (events.length === 0) return false;
  refreshRenamedAssistant(sessionID, events, dispatch);
  dispatch(chatActions.receiveEvents(events));
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index];
    const status = terminalEventStatus(event);
    if (!status) continue;
    markSessionFinished(sessionID, status, event.created_at, dispatch);
    return true;
  }
  return false;
}

async function shouldReconnect(sessionID: string, signal: AbortSignal, dispatch: AppDispatch) {
  if (signal.aborted) return false;
  dispatch(chatActions.setTaskConnectionState({ sessionID, connectionState: "connecting" }));
  try {
    const status = await streamStatus(sessionID, signal);
    if (signal.aborted) return false;
    if (status.status === "processing" && status.has_task !== false) return true;
    if (isTerminalStreamStatus(status.status)) {
      markSessionFinished(sessionID, status.status, status.finished_at, dispatch);
    } else {
      dispatch(chatActions.finishRunningSession(sessionID));
      dispatch(sessionsActions.updateSessionRuntime({ sessionID, status: status.status, activeTask: false }));
    }
    dispatch(chatActions.setTaskConnectionState({ sessionID, connectionState: "connected" }));
    return false;
  } catch (error) {
    if (signal.aborted || isAbortError(error)) return false;
    if (isAuthenticationError(error)) return false;
    if (error instanceof APIError && error.status === 404) {
      dispatch(chatActions.finishRunningSession(sessionID));
      dispatch(sessionsActions.updateSessionRuntime({ sessionID, activeTask: false }));
      dispatch(chatActions.setTaskConnectionState({ sessionID, connectionState: "connected" }));
      return false;
    }
    return true;
  }
}

function finishFromStatus(sessionID: string, status: string, updatedAt: string | undefined, dispatch: AppDispatch) {
  if (isTerminalStreamStatus(status)) {
    markSessionFinished(sessionID, status, updatedAt, dispatch);
    return;
  }
  if (sessionID === assistantSessionID) void dispatch(refreshAssistant());
  dispatch(chatActions.finishRunningSession(sessionID));
  dispatch(sessionsActions.updateSessionRuntime({ sessionID, status, activeTask: false, updatedAt }));
}

function markSessionFinished(sessionID: string, status: string, updatedAt: string | undefined, dispatch: AppDispatch) {
  if (sessionID === assistantSessionID) void dispatch(refreshAssistant());
  const timestamp = updatedAt || new Date().toISOString();
  dispatch(sessionsActions.updateSessionRuntime({ sessionID, status, activeTask: false, updatedAt: timestamp }));
  dispatch(chatActions.finishRunningSession(sessionID));
}

function refreshRenamedAssistant(sessionID: string, events: ChatEvent[], dispatch: AppDispatch) {
  if (sessionID === assistantSessionID && events.some((event) => event.type === "tool_call_completed" && event.tool_name === "assistant_rename")) {
    void dispatch(refreshAssistant());
  }
}

function terminalEventStatus(event: ChatEvent) {
  if (event.type === "processing_end") return event.status;
  return undefined;
}

function isTerminalStreamStatus(status?: string) {
  return status === "completed" || status === "cancelled" || status === "failed" || status === "error";
}

function streamReconnectDelay(retryCount: number) {
  const delay = streamReconnectBaseDelayMs * Math.max(1, 2 ** Math.min(retryCount - 1, 4));
  return Math.min(delay, streamReconnectMaxDelayMs);
}

function sleep(ms: number, signal: AbortSignal) {
  return new Promise<void>((resolve) => {
    if (signal.aborted) {
      resolve();
      return;
    }
    const timer = window.setTimeout(done, ms);
    signal.addEventListener("abort", done, { once: true });

    function done() {
      window.clearTimeout(timer);
      signal.removeEventListener("abort", done);
      resolve();
    }
  });
}

function isAuthenticationError(error: unknown) {
  return error instanceof APIError && error.status === 401;
}
