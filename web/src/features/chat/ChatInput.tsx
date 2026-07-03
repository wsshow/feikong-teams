import { useCallback, useEffect, useRef, useState } from "react";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { chatActions, sessionsActions } from "@/app/store";
import { startStream, stopStream } from "@/api/chat";
import { listFiles, searchFiles, uploadFile } from "@/api/files";
import { streamSnapshot, streamStatus, subscribeStream, type StreamSnapshot } from "@/api/stream";
import { readJSON, storageKeys, writeJSON } from "@/lib/storage";
import { cn } from "@/lib/cn";
import { chatPath, pushAppPath } from "@/lib/navigation";
import { loadSessions } from "@/features/sessions/sessionThunks";
import { ChatComposer } from "./ChatComposer";
import { QueuePanel } from "./QueuePanel";
import type { ChatAttachmentDraft } from "@/types/chat";
import type { ContentPartDTO } from "@/types/events";
import type { FileEntry } from "@/types/files";

const maxPastedImageBytes = 12 * 1024 * 1024;
const streamReconnectBaseDelayMs = 400;
const streamReconnectMaxDelayMs = 5000;

export function ChatInput({
  variant = "dock",
  className,
  onReferenceOpenChange,
}: {
  variant?: "dock" | "hero";
  className?: string;
  onReferenceOpenChange?: (open: boolean) => void;
}) {
  const dispatch = useAppDispatch();
  const sessionID = useAppSelector((state) => state.chat.activeSessionID);
  const runningSessionID = useAppSelector((state) => state.chat.runningSessionID);
  const mode = useAppSelector((state) => state.chat.mode);
  const currentAgent = useAppSelector((state) => state.chat.currentAgent);
  const isProcessing = useAppSelector((state) => state.chat.isProcessing);
  const agents = useAppSelector((state) => state.app.agents);
  const [value, setValue] = useState("");
  const [fileSuggestions, setFileSuggestions] = useState<FileEntry[]>([]);
  const [attachments, setAttachments] = useState<ChatAttachmentDraft[]>([]);
  const [referenceLoading, setReferenceLoading] = useState(false);
  const referenceRequestID = useRef(0);
  const fileSuggestionCache = useRef(new Map<string, FileEntry[]>());
  const attachmentsRef = useRef<ChatAttachmentDraft[]>([]);
  const dockRef = useRef<HTMLDivElement | null>(null);
  const activeSessionRef = useRef(sessionID);
  const streamSubscriptionsRef = useRef(new Set<string>());

  useEffect(() => {
    activeSessionRef.current = sessionID;
  }, [sessionID]);

  useEffect(() => {
    if (variant !== "dock") return;
    if (!sessionID || runningSessionID !== sessionID || !isProcessing) return;
    ensureStreamSubscription(sessionID);
  }, [variant, sessionID, runningSessionID, isProcessing]);

  useEffect(() => {
    attachmentsRef.current = attachments;
  }, [attachments]);

  useEffect(() => () => {
    for (const attachment of attachmentsRef.current) revokeAttachmentPreview(attachment);
  }, []);

  useEffect(() => {
    if (variant !== "dock") return;
    const dock = dockRef.current;
    if (!dock) return;
    const updateHeight = () => {
      document.documentElement.style.setProperty("--chat-dock-height", `${dock.offsetHeight}px`);
    };
    const observer = new ResizeObserver(updateHeight);

    updateHeight();
    observer.observe(dock);
    window.addEventListener("resize", updateHeight);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", updateHeight);
      document.documentElement.style.removeProperty("--chat-dock-height");
    };
  }, [variant]);

  async function submit() {
    const message = value.trim();
    const readyAttachments = attachments.filter((attachment) => attachment.status === "ready");
    if (!message && readyAttachments.length === 0) return;
    if (attachments.some((attachment) => attachment.status === "uploading")) {
      dispatch(chatActions.setError("附件仍在处理中，请稍后发送"));
      return;
    }
    if (attachments.some((attachment) => attachment.status === "error")) {
      dispatch(chatActions.setError("请先移除上传失败的附件"));
      return;
    }
    const contents = readyAttachments.length ? buildContentParts(message, readyAttachments) : undefined;
    const displayText = message || attachmentSummary(readyAttachments);
    const targetSessionID = sessionID;
    const queueing = Boolean(isProcessing && runningSessionID === sessionID && targetSessionID);
    setValue("");
    clearAttachments();
    dispatch(chatActions.setError(undefined));
    if (!queueing) {
      dispatch(chatActions.appendUserMessage({ id: `user-${Date.now()}`, content: displayText, sessionID: targetSessionID, contentParts: contents, createdAt: new Date().toISOString() }));
      dispatch(chatActions.setProcessing(true));
    }
    try {
      if (queueing && targetSessionID) {
        const result = await startStream({
          session_id: targetSessionID,
          message,
          contents,
          mode,
          agent_name: currentAgent || undefined,
        });
        if (Array.isArray(result.queue)) dispatch(chatActions.setQueue(result.queue));
        ensureStreamSubscription(targetSessionID);
        return;
      }
      const result = await startStream({
        session_id: targetSessionID || undefined,
        message,
        contents,
        mode,
        agent_name: currentAgent || undefined,
      });
      if (result.status === "queued") {
        if (Array.isArray(result.queue)) dispatch(chatActions.setQueue(result.queue));
        dispatch(chatActions.setRunningSession(result.session_id));
        ensureStreamSubscription(result.session_id);
        return;
      }
      const now = new Date().toISOString();
      dispatch(sessionsActions.upsertSession({
        session_id: result.session_id,
        title: displayText,
        status: "processing",
        active_task: true,
        mod_time: now,
        updated_at: now,
      }));
      dispatch(chatActions.setRunningSession(result.session_id));
      dispatch(chatActions.setActiveSession(result.session_id));
      pushAppPath(chatPath(result.session_id));
      dispatch(loadSessions());
      resetOffset(result.session_id);
      ensureStreamSubscription(result.session_id, 0);
    } catch (error) {
      dispatch(chatActions.setError(error instanceof Error ? error.message : String(error)));
      if (!queueing) dispatch(chatActions.setProcessing(false));
    }
  }

  function ensureStreamSubscription(id: string, initialOffset?: number) {
    if (streamSubscriptionsRef.current.has(id)) return;
    streamSubscriptionsRef.current.add(id);
    void subscribe(id, initialOffset).finally(() => {
      streamSubscriptionsRef.current.delete(id);
    });
  }

  async function subscribe(id: string, initialOffset?: number) {
    const offsets = readJSON<Record<string, number>>(storageKeys.streamOffsets, {});
    let retryCount = 0;
    let fallbackOffset = initialOffset;
    for (;;) {
      if (activeSessionRef.current !== id && fallbackOffset === undefined) return;
      const offset = await resolveSubscribeOffset(id, fallbackOffset, offsets);
      if (offset === undefined) return;
      try {
        dispatch(chatActions.setConnectionState("connecting"));
        await subscribeStream(id, offset, (event) => {
          retryCount = 0;
          fallbackOffset = undefined;
          dispatch(chatActions.setConnectionState("connected"));
          dispatch(chatActions.receiveEvent(event));
          if (event.type === "processing_end" || event.type === "cancelled") {
            dispatch(loadSessions());
          }
          if (event.stream_event_id !== undefined) {
            offsets[id] = Number(event.stream_event_id) + 1;
            writeJSON(storageKeys.streamOffsets, offsets);
          }
        });
        await handleSubscribeClosed(id);
        return;
      } catch (error) {
        const shouldReconnect = await handleSubscribeError(id, error);
        if (!shouldReconnect) return;
        retryCount += 1;
        await sleep(streamReconnectDelay(retryCount));
      }
    }
  }

  async function resolveSubscribeOffset(id: string, fallbackOffset: number | undefined, offsets: Record<string, number>) {
    const storedOffset = offsets[id];
    if (storedOffset > 0) return replayStreamSnapshot(id, storedOffset, offsets);
    if (fallbackOffset !== undefined) return fallbackOffset;
    return loadTailStreamSnapshot(id, offsets);
  }

  async function handleSubscribeClosed(id: string) {
    const status = await streamStatus(id).catch(() => undefined);
    if (isTerminalStreamStatus(status?.status)) {
      dispatch(loadSessions());
      if (activeSessionRef.current === id) dispatch(chatActions.setProcessing(false));
    }
    dispatch(chatActions.setConnectionState("connected"));
  }

  async function replayStreamSnapshot(id: string, offset: number, offsets: Record<string, number>) {
    let nextOffset = offset;
    for (;;) {
      const snapshot = await streamSnapshot(id, { offset: nextOffset, limit: 1000 }).catch(() => undefined);
      if (!snapshot) return nextOffset;
      const appliedOffset = applyStreamSnapshot(id, snapshot, offsets);
      if (appliedOffset === undefined) return undefined;
      if (!snapshot.more_available || appliedOffset <= nextOffset) return appliedOffset;
      nextOffset = appliedOffset;
    }
  }

  async function loadTailStreamSnapshot(id: string, offsets: Record<string, number>) {
    const snapshot = await streamSnapshot(id).catch(() => undefined);
    if (!snapshot) return undefined;
    return applyStreamSnapshot(id, snapshot, offsets);
  }

  function applyStreamSnapshot(id: string, snapshot: StreamSnapshot, offsets: Record<string, number>) {
    const isActiveSession = activeSessionRef.current === id;
    if (isActiveSession && Array.isArray(snapshot.queue)) dispatch(chatActions.setQueue(snapshot.queue));
    for (const event of snapshot.events || []) {
      dispatch(chatActions.receiveEvent(event));
      if (event.stream_event_id !== undefined) {
        offsets[id] = Number(event.stream_event_id) + 1;
      }
    }
    const nextOffset = Math.max(Number(snapshot.next_offset || 0), offsets[id] || 0);
    offsets[id] = nextOffset;
    writeJSON(storageKeys.streamOffsets, offsets);
    if (isTerminalStreamStatus(snapshot.status)) {
      dispatch(loadSessions());
      if (isActiveSession) dispatch(chatActions.setProcessing(false));
      return undefined;
    }
    return nextOffset;
  }

  function isTerminalStreamStatus(status?: string) {
    return status === "completed" || status === "cancelled" || status === "failed" || status === "error";
  }

  async function handleSubscribeError(id: string, _error: unknown) {
    dispatch(chatActions.setConnectionState("connecting"));
    const status = await streamStatus(id).catch(() => undefined);
    if (isTerminalStreamStatus(status?.status)) {
      dispatch(loadSessions());
      if (activeSessionRef.current === id) dispatch(chatActions.setProcessing(false));
      dispatch(chatActions.setConnectionState("connected"));
      return false;
    }
    if (activeSessionRef.current !== id) {
      dispatch(loadSessions());
      dispatch(chatActions.setConnectionState("connected"));
      return false;
    }
    if (status?.status === "processing" || status === undefined) return true;
    dispatch(loadSessions());
    dispatch(chatActions.setConnectionState("connected"));
    return false;
  }

  async function stop() {
    const id = runningSessionID === sessionID ? sessionID : "";
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

  async function addAttachments(files: File[]) {
    if (!files.length) return;
    const uploadDir = `chat-attachments/${Date.now().toString(36)}`;
    for (const file of files) {
      const id = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
      const isImage = file.type.startsWith("image/");
      const draft: ChatAttachmentDraft = {
        id,
        kind: isImage ? "image" : "file",
        name: file.name || (isImage ? "pasted-image.png" : "attachment"),
        size: file.size,
        mimeType: file.type || "application/octet-stream",
        status: "uploading",
        previewURL: isImage ? URL.createObjectURL(file) : undefined,
      };
      setAttachments((current) => [...current, draft]);
      try {
        if (isImage) {
          if (file.size > maxPastedImageBytes) {
            throw new Error("图片过大，无法直接粘贴发送");
          }
          const dataURL = await readFileAsDataURL(file);
          updateAttachment(id, {
            status: "ready",
            base64Data: dataURL.slice(dataURL.indexOf(",") + 1),
            mimeType: file.type || mimeTypeFromDataURL(dataURL) || "image/png",
          });
          continue;
        }
        const uploaded = await uploadFile(file, uploadDir);
        const path = uploaded[0]?.path;
        if (!path) throw new Error("文件上传失败");
        updateAttachment(id, { status: "ready", path });
      } catch (error) {
        updateAttachment(id, { status: "error", error: error instanceof Error ? error.message : String(error) });
      }
    }
  }

  function updateAttachment(id: string, patch: Partial<ChatAttachmentDraft>) {
    setAttachments((current) => current.map((attachment) => (
      attachment.id === id ? { ...attachment, ...patch } : attachment
    )));
  }

  function removeAttachment(id: string) {
    setAttachments((current) => {
      const next: ChatAttachmentDraft[] = [];
      for (const attachment of current) {
        if (attachment.id === id) revokeAttachmentPreview(attachment);
        else next.push(attachment);
      }
      return next;
    });
  }

  function clearAttachments() {
    setAttachments((current) => {
      for (const attachment of current) revokeAttachmentPreview(attachment);
      return [];
    });
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
        attachments={attachments}
        referenceLoading={referenceLoading}
        variant="hero"
        onValueChange={setValue}
        onModeChange={changeMode}
        onReferenceQuery={queryReferences}
        onReferenceOpenChange={onReferenceOpenChange}
        onFilesAdded={(files) => void addAttachments(files)}
        onRemoveAttachment={removeAttachment}
        onAgentChange={changeAgent}
        onSubmit={() => void submit()}
        onStop={() => void stop()}
      />
    );
  }

  return (
    <div
      ref={dockRef}
      className={cn(
        "fixed inset-x-0 bottom-[var(--app-keyboard-inset-bottom,0px)] z-30 bg-transparent px-3 pb-3 pt-2 md:static md:z-auto md:px-6 md:pb-5",
        className,
      )}
    >
      <div className="mx-auto max-w-4xl">
        <QueuePanel onEditMessage={setValue} />
        <ChatComposer
          className="relative z-10 shadow-[0_12px_32px_hsl(218_30%_25%/0.12)]"
          value={value}
          mode={mode}
          processing={isProcessing}
          agents={agents}
          selectedAgent={currentAgent}
          fileSuggestions={fileSuggestions}
          attachments={attachments}
          referenceLoading={referenceLoading}
          variant="dock"
          onValueChange={setValue}
          onModeChange={changeMode}
          onReferenceQuery={queryReferences}
          onReferenceOpenChange={onReferenceOpenChange}
          onFilesAdded={(files) => void addAttachments(files)}
          onRemoveAttachment={removeAttachment}
          onAgentChange={changeAgent}
          onSubmit={() => void submit()}
          onStop={() => void stop()}
        />
      </div>
    </div>
  );
}

function resetOffset(sessionID: string) {
  const offsets = readJSON<Record<string, number>>(storageKeys.streamOffsets, {});
  delete offsets[sessionID];
  writeJSON(storageKeys.streamOffsets, offsets);
}

function streamReconnectDelay(retryCount: number) {
  const delay = streamReconnectBaseDelayMs * Math.max(1, 2 ** Math.min(retryCount - 1, 4));
  return Math.min(delay, streamReconnectMaxDelayMs);
}

function sleep(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
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

function buildContentParts(message: string, attachments: ChatAttachmentDraft[]): ContentPartDTO[] | undefined {
  if (!message && attachments.length === 0) return undefined;
  const parts: ContentPartDTO[] = [];
  if (message) parts.push({ type: "text", text: message });
  for (const attachment of attachments) {
    if (attachment.kind === "image" && attachment.base64Data) {
      parts.push({
        type: "image_base64",
        name: attachment.name,
        base64_data: attachment.base64Data,
        mime_type: attachment.mimeType || "image/png",
        detail: "auto",
      });
      continue;
    }
    if (attachment.kind === "file" && attachment.path) {
      parts.push({
        type: "file_url",
        name: attachment.name,
        url: attachment.path,
      });
    }
  }
  return parts;
}

function attachmentSummary(attachments: ChatAttachmentDraft[]) {
  const imageCount = attachments.filter((attachment) => attachment.kind === "image").length;
  const fileCount = attachments.length - imageCount;
  const labels: string[] = [];
  if (imageCount) labels.push(`${imageCount} 张图片`);
  if (fileCount) labels.push(`${fileCount} 个文件`);
  return labels.length ? `发送了${labels.join("、")}` : "发送了附件";
}

function readFileAsDataURL(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result || ""));
    reader.onerror = () => reject(reader.error || new Error("读取文件失败"));
    reader.readAsDataURL(file);
  });
}

function mimeTypeFromDataURL(dataURL: string) {
  const match = /^data:([^;,]+)/.exec(dataURL);
  return match?.[1];
}

function revokeAttachmentPreview(attachment: ChatAttachmentDraft) {
  if (attachment.previewURL?.startsWith("blob:")) URL.revokeObjectURL(attachment.previewURL);
}
