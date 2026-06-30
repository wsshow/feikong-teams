import { useCallback, useRef, useState } from "react";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { chatActions } from "@/app/store";
import { sendSteering, startStream, stopStream } from "@/api/chat";
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
  const fileSuggestionCache = useRef(new Map<string, FileEntry[]>());

  async function submit() {
    const message = value.trim();
    if (!message) return;
    const targetSessionID = runningSessionID || sessionID;
    const queueing = Boolean(isProcessing && targetSessionID);
    setValue("");
    dispatch(chatActions.setError(undefined));
    if (!queueing) {
      dispatch(chatActions.appendUserMessage({ id: `user-${Date.now()}`, content: message, createdAt: new Date().toISOString() }));
      dispatch(chatActions.setProcessing(true));
    }
    try {
      if (queueing && targetSessionID) {
        await sendSteering(targetSessionID, message);
        return;
      }
      const result = await startStream({
        session_id: targetSessionID || undefined,
        message,
        mode,
        agent_name: currentAgent || undefined,
      });
      if (result.status === "queued") return;
      dispatch(chatActions.setActiveSession(result.session_id));
      dispatch(chatActions.setRunningSession(result.session_id));
      pushAppPath(chatPath(result.session_id));
      dispatch(loadSessions());
      resetOffset(result.session_id);
      void subscribe(result.session_id, 0);
    } catch (error) {
      dispatch(chatActions.setError(error instanceof Error ? error.message : String(error)));
      if (!queueing) dispatch(chatActions.setProcessing(false));
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
    const keyword = query.trim();
    const cached = fileSuggestionCache.current.get(keyword);
    if (cached) {
      setFileSuggestions(cached);
      setReferenceLoading(false);
      return;
    }
    const requestID = referenceRequestID.current + 1;
    referenceRequestID.current = requestID;
    setReferenceLoading(true);
    dispatch(chatActions.setError(undefined));
    try {
      const files = await fileReferenceSuggestions(keyword);
      if (referenceRequestID.current === requestID) {
        fileSuggestionCache.current.set(keyword, files || []);
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

async function fileReferenceSuggestions(query: string) {
  const normalized = query.replace(/\\/g, "/").replace(/^\/+/, "");
  if (!normalized) return listFiles("");

  const slashIndex = normalized.lastIndexOf("/");
  if (slashIndex < 0) return searchFiles(normalized);

  const parent = normalized.slice(0, slashIndex).replace(/\/+$/, "");
  const leaf = normalized.slice(slashIndex + 1).toLowerCase();
  const listed = await listFiles(parent).catch(() => [] as FileEntry[]);
  const filtered = leaf
    ? listed.filter((file) => file.name.toLowerCase().includes(leaf) || file.path.toLowerCase().includes(normalized.toLowerCase()))
    : listed;

  if (leaf) {
    const searched = await searchFiles(normalized).catch(() => []);
    return mergeFileSuggestions([...filtered, ...searched]);
  }
  return filtered;
}

function mergeFileSuggestions(files: FileEntry[]) {
  const seen = new Set<string>();
  const result: FileEntry[] = [];
  for (const file of files) {
    if (!file.path || seen.has(file.path)) continue;
    seen.add(file.path);
    result.push(file);
  }
  return result;
}
