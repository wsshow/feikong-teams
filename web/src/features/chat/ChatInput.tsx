import { useCallback, useEffect, useRef, useState } from "react";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { chatActions, sessionsActions } from "@/app/store";
import { startStream, stopStream } from "@/api/chat";
import { listFiles, searchFiles, uploadFile } from "@/api/files";
import { subscribeStream } from "@/api/stream";
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
  const streamSubscriptionRef = useRef("");

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
    const targetSessionID = runningSessionID || sessionID;
    const queueing = Boolean(isProcessing && targetSessionID);
    setValue("");
    clearAttachments();
    dispatch(chatActions.setError(undefined));
    if (!queueing) {
      dispatch(chatActions.appendUserMessage({ id: `user-${Date.now()}`, content: displayText, contentParts: contents, createdAt: new Date().toISOString() }));
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
      if (result.status === "queued") return;
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
    if (streamSubscriptionRef.current === id) return;
    streamSubscriptionRef.current = id;
    void subscribe(id, initialOffset).finally(() => {
      if (streamSubscriptionRef.current === id) streamSubscriptionRef.current = "";
    });
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
