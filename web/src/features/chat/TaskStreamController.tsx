import { useEffect, useRef } from "react";
import { APIError, isAbortError } from "@/api/client";
import { streamSnapshot, streamStatus, subscribeStream, type StreamSnapshot } from "@/api/stream";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { chatActions, sessionsActions, type AppDispatch } from "@/app/store";
import type { ChatEvent } from "@/types/events";
import { clearStreamOffset, readStreamOffset, writeStreamOffset } from "./streamOffsets";

const streamReconnectBaseDelayMs = 400;
const streamReconnectMaxDelayMs = 5000;

interface ActiveSubscription {
  sessionID: string;
  controller: AbortController;
}

export function TaskStreamController() {
  const dispatch = useAppDispatch();
  const activeSessionID = useAppSelector((state) => state.chat.activeSessionID);
  const viewSessionID = useAppSelector((state) => state.chat.viewSessionID);
  const runningSessionID = useAppSelector((state) => state.chat.runningSessionID);
  const initialOffset = useAppSelector((state) => state.chat.streamInitialOffset);
  const isProcessing = useAppSelector((state) => state.chat.isProcessing);
  const authExpired = useAppSelector((state) => state.app.authExpired);
  const consumableSessionRef = useRef("");
  const subscriptionRef = useRef<ActiveSubscription | undefined>(undefined);

  const consumableSessionID = activeSessionID === runningSessionID && viewSessionID === runningSessionID
    ? runningSessionID
    : "";
  consumableSessionRef.current = consumableSessionID;

  useEffect(() => {
    const current = subscriptionRef.current;
    if (authExpired || !consumableSessionID || !isProcessing) {
      if (current) {
        current.controller.abort();
        subscriptionRef.current = undefined;
      }
      dispatch(chatActions.setConnectionState(authExpired ? "disconnected" : "connected"));
      return;
    }
    if (current?.sessionID === consumableSessionID) return;
    current?.controller.abort();

    const controller = new AbortController();
    const subscription = { sessionID: consumableSessionID, controller };
    subscriptionRef.current = subscription;
    dispatch(chatActions.consumeStreamInitialOffset());
    void followTaskStream(
      consumableSessionID,
      initialOffset,
      controller.signal,
      dispatch,
      () => consumableSessionRef.current === consumableSessionID,
    ).finally(() => {
      if (subscriptionRef.current === subscription) subscriptionRef.current = undefined;
    });
  }, [authExpired, consumableSessionID, dispatch, isProcessing]);

  useEffect(() => () => {
    subscriptionRef.current?.controller.abort();
    subscriptionRef.current = undefined;
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
  let fallbackOffset = initialOffset;
  for (;;) {
    if (signal.aborted) return;
    try {
      const offset = await resolveSubscribeOffset(sessionID, fallbackOffset, dispatch, canConsume);
      if (offset === undefined || signal.aborted) return;
      dispatch(chatActions.setConnectionState("connecting"));
      const result = await subscribeStream(sessionID, offset, (event) => {
        if (!canConsume()) return;
        retryCount = 0;
        fallbackOffset = undefined;
        if (event.stream_event_id !== undefined) {
          writeStreamOffset(sessionID, Number(event.stream_event_id) + 1);
        }
        dispatch(chatActions.setConnectionState("connected"));
        applyStreamEvent(sessionID, event, dispatch);
      }, signal);
      if (result === "done") {
        dispatch(chatActions.setConnectionState("connected"));
        return;
      }
    } catch (error) {
      if (signal.aborted || isAbortError(error) || isAuthenticationError(error)) return;
    }

    if (!await shouldReconnect(sessionID, dispatch)) return;
    retryCount += 1;
    await sleep(streamReconnectDelay(retryCount), signal);
  }
}

async function resolveSubscribeOffset(
  sessionID: string,
  fallbackOffset: number | undefined,
  dispatch: AppDispatch,
  canConsume: () => boolean,
) {
  if (!canConsume()) return undefined;
  if (fallbackOffset !== undefined) return fallbackOffset;
  const storedOffset = readStreamOffset(sessionID);
  if (storedOffset !== undefined) return storedOffset;
  return replayStreamSnapshot(sessionID, dispatch, canConsume);
}

async function replayStreamSnapshot(
  sessionID: string,
  dispatch: AppDispatch,
  canConsume: () => boolean,
) {
  let nextOffset = 0;
  for (;;) {
    if (!canConsume()) return undefined;
    const snapshot = await streamSnapshot(sessionID, { offset: nextOffset, limit: 1000 });
    const appliedOffset = applyStreamSnapshot(sessionID, snapshot, dispatch, canConsume);
    if (appliedOffset === undefined) return undefined;
    if (!snapshot.more_available || appliedOffset <= nextOffset) return appliedOffset;
    nextOffset = appliedOffset;
  }
}

function applyStreamSnapshot(
  sessionID: string,
  snapshot: StreamSnapshot,
  dispatch: AppDispatch,
  canConsume: () => boolean,
) {
  if (!canConsume()) return undefined;
  if (Array.isArray(snapshot.queue)) {
    dispatch(chatActions.setQueue(snapshot.queue));
  }
  for (const event of snapshot.events || []) {
    if (!canConsume()) return undefined;
    if (event.stream_event_id !== undefined) {
      writeStreamOffset(sessionID, Number(event.stream_event_id) + 1);
    }
    applyStreamEvent(sessionID, event, dispatch);
  }
  const nextOffset = Math.max(Number(snapshot.next_offset || 0), readStreamOffset(sessionID) || 0);
  writeStreamOffset(sessionID, nextOffset);
  if (isTerminalStreamStatus(snapshot.status) && !snapshot.more_available) {
    markSessionFinished(sessionID, snapshot.status, snapshot.finished_at, dispatch);
    return undefined;
  }
  return nextOffset;
}

function applyStreamEvent(sessionID: string, event: ChatEvent, dispatch: AppDispatch) {
  dispatch(chatActions.receiveEvent(event));
  const status = terminalEventStatus(event);
  if (status) markSessionFinished(sessionID, status, event.created_at, dispatch);
}

async function shouldReconnect(sessionID: string, dispatch: AppDispatch) {
  dispatch(chatActions.setConnectionState("connecting"));
  try {
    const status = await streamStatus(sessionID);
    if (status.status === "processing") return true;
    if (isTerminalStreamStatus(status.status)) {
      markSessionFinished(sessionID, status.status, status.finished_at, dispatch);
    } else {
      clearStreamOffset(sessionID);
      dispatch(chatActions.finishRunningSession(sessionID));
      dispatch(sessionsActions.updateSessionRuntime({ sessionID, status: status.status, activeTask: false }));
    }
    dispatch(chatActions.setConnectionState("connected"));
    return false;
  } catch (error) {
    if (isAuthenticationError(error)) return false;
    if (error instanceof APIError && error.status === 404) {
      clearStreamOffset(sessionID);
      dispatch(chatActions.finishRunningSession(sessionID));
      dispatch(sessionsActions.updateSessionRuntime({ sessionID, activeTask: false }));
      dispatch(chatActions.setConnectionState("connected"));
      return false;
    }
    return true;
  }
}

function markSessionFinished(sessionID: string, status: string, updatedAt: string | undefined, dispatch: AppDispatch) {
  const timestamp = updatedAt || new Date().toISOString();
  clearStreamOffset(sessionID);
  dispatch(sessionsActions.updateSessionRuntime({ sessionID, status, activeTask: false, updatedAt: timestamp }));
  dispatch(chatActions.finishRunningSession(sessionID));
}

function terminalEventStatus(event: ChatEvent) {
  if (event.type === "processing_end") return "completed";
  if (event.type === "cancelled") return "cancelled";
  if (event.type === "error") return "error";
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
