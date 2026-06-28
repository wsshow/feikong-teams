import { useCallback, useRef, useState } from "react";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { chatActions } from "@/app/store";
import { startStream, stopStream } from "@/api/chat";
import { listFiles, searchFiles } from "@/api/files";
import { subscribeStream } from "@/api/stream";
import { readJSON, storageKeys, writeJSON } from "@/lib/storage";
import { cn } from "@/lib/cn";
import { chatPath, pushAppPath } from "@/lib/navigation";
import { loadSessions } from "@/features/sessions/sessionThunks";
import { ChatComposer } from "./ChatComposer";
import type { FileEntry } from "@/types/files";

export function ChatInput({ variant = "dock", className }: { variant?: "dock" | "hero"; className?: string }) {
  const dispatch = useAppDispatch();
  const sessionID = useAppSelector((state) => state.chat.activeSessionID);
  const runningSessionID = useAppSelector((state) => state.chat.runningSessionID);
  const mode = useAppSelector((state) => state.chat.mode);
  const currentAgent = useAppSelector((state) => state.chat.currentAgent);
  const isProcessing = useAppSelector((state) => state.chat.isProcessing);
  const agents = useAppSelector((state) => state.app.agents);
  const [value, setValue] = useState("");
  const [fileSuggestions, setFileSuggestions] = useState<FileEntry[]>([]);
  const [referenceLoading, setReferenceLoading] = useState(false);
  const referenceRequestID = useRef(0);

  async function submit() {
    const message = value.trim();
    if (!message || isProcessing) return;
    setValue("");
    dispatch(chatActions.setError(undefined));
    dispatch(chatActions.appendUserMessage({ id: `user-${Date.now()}`, content: message, createdAt: new Date().toISOString() }));
    dispatch(chatActions.setProcessing(true));
    try {
      const result = await startStream({
        session_id: sessionID || undefined,
        message,
        mode,
        agent_name: currentAgent || undefined,
      });
      dispatch(chatActions.setActiveSession(result.session_id));
      dispatch(chatActions.setRunningSession(result.session_id));
      pushAppPath(chatPath(result.session_id));
      dispatch(loadSessions());
      resetOffset(result.session_id);
      void subscribe(result.session_id, 0);
    } catch (error) {
      dispatch(chatActions.setError(error instanceof Error ? error.message : String(error)));
      dispatch(chatActions.setProcessing(false));
    }
  }

  async function subscribe(id: string, initialOffset?: number) {
    const offsets = readJSON<Record<string, number>>(storageKeys.streamOffsets, {});
    const offset = initialOffset ?? offsets[id] ?? 0;
    await subscribeStream(id, offset, (event) => {
      dispatch(chatActions.receiveEvent(event));
      if (event.type === "processing_end" || event.type === "cancelled") {
        dispatch(loadSessions());
      }
      if (event.stream_event_id !== undefined) {
        offsets[id] = Number(event.stream_event_id) + 1;
        writeJSON(storageKeys.streamOffsets, offsets);
      }
    }).catch((error) => {
      dispatch(chatActions.setError(error instanceof Error ? error.message : String(error)));
      dispatch(chatActions.setProcessing(false));
    });
  }

  async function stop() {
    const id = runningSessionID || sessionID;
    if (!id) return;
    try {
      await stopStream(id);
    } catch (error) {
      dispatch(chatActions.setError(error instanceof Error ? error.message : String(error)));
    } finally {
      dispatch(chatActions.setProcessing(false));
    }
  }

  function changeMode(nextMode: string) {
    dispatch(chatActions.setMode(nextMode));
    dispatch(chatActions.setCurrentAgent(""));
  }

  const queryReferences = useCallback(async (query: string) => {
    const requestID = referenceRequestID.current + 1;
    referenceRequestID.current = requestID;
    setReferenceLoading(true);
    dispatch(chatActions.setError(undefined));
    try {
      const keyword = query.trim();
      const files = keyword ? await searchFiles(keyword) : await listFiles("");
      if (referenceRequestID.current === requestID) {
        setFileSuggestions(files || []);
      }
    } catch {
      if (referenceRequestID.current === requestID) {
        setFileSuggestions([]);
      }
    } finally {
      if (referenceRequestID.current === requestID) setReferenceLoading(false);
    }
  }, [dispatch]);

  function changeAgent(agent: string) {
    dispatch(chatActions.setCurrentAgent(agent));
  }

  if (variant === "hero") {
    return (
      <ChatComposer
        className={className}
        value={value}
        mode={mode}
        processing={isProcessing}
        agents={agents}
        selectedAgent={currentAgent}
        fileSuggestions={fileSuggestions}
        referenceLoading={referenceLoading}
        variant="hero"
        onValueChange={setValue}
        onModeChange={changeMode}
        onReferenceQuery={queryReferences}
        onAgentChange={changeAgent}
        onSubmit={() => void submit()}
        onStop={() => void stop()}
      />
    );
  }

  return (
    <div className={cn("bg-transparent px-6 pb-5 pt-2", className)}>
      <ChatComposer
        className="mx-auto max-w-4xl shadow-[0_12px_32px_hsl(218_30%_25%/0.12)]"
        value={value}
        mode={mode}
        processing={isProcessing}
        agents={agents}
        selectedAgent={currentAgent}
        fileSuggestions={fileSuggestions}
        referenceLoading={referenceLoading}
        variant="dock"
        onValueChange={setValue}
        onModeChange={changeMode}
        onReferenceQuery={queryReferences}
        onAgentChange={changeAgent}
        onSubmit={() => void submit()}
        onStop={() => void stop()}
      />
    </div>
  );
}

function resetOffset(sessionID: string) {
  const offsets = readJSON<Record<string, number>>(storageKeys.streamOffsets, {});
  delete offsets[sessionID];
  writeJSON(storageKeys.streamOffsets, offsets);
}
