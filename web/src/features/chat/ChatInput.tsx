import { useCallback, useEffect, useRef, useState } from "react";
import { Eraser, Pencil } from "lucide-react";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { appActions, chatActions } from "@/app/store";
import { Dialog } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { clearAssistant, renameAssistant } from "@/features/sessions/sessionThunks";
import { stopStream } from "@/api/chat";
import { listFiles, searchFiles, uploadFile } from "@/api/files";
import { cn } from "@/lib/cn";
import { assistantAgentID, assistantDisplayName, assistantSessionID } from "@/lib/assistant";
import { ChatComposer } from "./ChatComposer";
import { QueuePanel } from "./QueuePanel";
import { newChatID, sendChatMessage } from "./sendMessage";
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
  const runningTask = useAppSelector((state) => (sessionID ? state.chat.runningTasks[sessionID] : undefined));
  const mode = useAppSelector((state) => sessionID === assistantSessionID ? "team" : state.chat.mode);
  const currentAgent = useAppSelector((state) => sessionID === assistantSessionID ? assistantAgentID : state.chat.currentAgent);
  const assistantName = useAppSelector((state) => assistantDisplayName(state.sessions.items));
  const isProcessing = runningTask?.phase === "processing";
  const agents = useAppSelector((state) => state.app.agents);
  const [value, setValue] = useState("");
  const [fileSuggestions, setFileSuggestions] = useState<FileEntry[]>([]);
  const [attachments, setAttachments] = useState<ChatAttachmentDraft[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [referenceLoading, setReferenceLoading] = useState(false);
  const [commandDialog, setCommandDialog] = useState<"clear" | "rename">();
  const [assistantNameDraft, setAssistantNameDraft] = useState("");
  const [commandBusy, setCommandBusy] = useState(false);
  const [commandError, setCommandError] = useState("");
  const commandBusyRef = useRef(false);
  const referenceRequestID = useRef(0);
  const fileSuggestionCache = useRef(new Map<string, FileEntry[]>());
  const attachmentsRef = useRef<ChatAttachmentDraft[]>([]);
  const submittingRef = useRef(false);
  const dockRef = useRef<HTMLDivElement | null>(null);
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
    if (submittingRef.current) return;
    const message = value.trim();
    if (sessionID === assistantSessionID && (message === "/clear" || message === "/rename")) {
      selectCommand(message.slice(1));
      return;
    }
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
    const newSession = !sessionID;
    const targetSessionID = sessionID || newChatID();
    const submittedValue = value;
    const submittedAttachmentIDs = new Set(readyAttachments.map((attachment) => attachment.id));
    submittingRef.current = true;
    setSubmitting(true);
    setValue("");
    try {
      await dispatch(sendChatMessage({ sessionID: targetSessionID, message, displayText, contents, mode, agent: currentAgent, newSession }));
      clearSubmittedAttachments(submittedAttachmentIDs);
    } catch (error) {
      setValue((current) => restoreSubmittedValue(submittedValue, current));
      dispatch(chatActions.setError(error instanceof Error ? error.message : String(error)));
    } finally {
      submittingRef.current = false;
      setSubmitting(false);
    }
  }

  function selectCommand(id: string) {
    if (id !== "clear" && id !== "rename") return;
    if (commandBusyRef.current || submittingRef.current) return;
    if (id === "clear" && runningTask) return;
    setValue("");
    setAssistantNameDraft(assistantName);
    setCommandError("");
    setCommandDialog(id);
  }

  async function confirmCommand() {
    if (!commandDialog || commandBusyRef.current) return;
    const name = assistantNameDraft.trim();
    if (commandDialog === "rename" && (!name || Array.from(name).length > 40 || /\p{Cc}/u.test(name))) {
      setCommandError("助手名称需为 1–40 个字符，且不能包含控制字符");
      return;
    }
    if (commandDialog === "clear" && runningTask) {
      setCommandError("助手正在运行，请在任务结束后清空上下文");
      return;
    }
    commandBusyRef.current = true;
    setCommandBusy(true);
    setCommandError("");
    try {
      if (commandDialog === "clear") await dispatch(clearAssistant()).unwrap();
      else await dispatch(renameAssistant(name)).unwrap();
      dispatch(appActions.showToast(commandDialog === "clear" ? "助手上下文已清空" : "助手名称已更新"));
      setCommandDialog(undefined);
    } catch (error) {
      setCommandError(typeof error === "object" && error && "message" in error ? String(error.message) : "操作失败，请重试");
    } finally {
      commandBusyRef.current = false;
      setCommandBusy(false);
    }
  }

  async function stop() {
    const id = runningTask?.phase === "processing" ? sessionID : "";
    if (!id) return;
    try {
      await stopStream(id);
    } catch (error) {
      dispatch(chatActions.setError(error instanceof Error ? error.message : String(error)));
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

  function clearSubmittedAttachments(submittedIDs: Set<string>) {
    setAttachments((current) => {
      const next: ChatAttachmentDraft[] = [];
      for (const attachment of current) {
        if (submittedIDs.has(attachment.id)) revokeAttachmentPreview(attachment);
        else next.push(attachment);
      }
      return next;
    });
  }

  const composer = (
    <ChatComposer
      className={variant === "hero" ? className : "relative z-10 shadow-raised"}
      value={value}
      mode={mode}
      processing={isProcessing}
      submitting={submitting}
      agents={agents}
      selectedAgent={currentAgent}
      lockedTargetName={sessionID === assistantSessionID ? assistantName : undefined}
      commands={sessionID === assistantSessionID ? [
        { id: "clear", label: "清空助手上下文", description: runningTask ? "任务结束后可清空上下文" : "清空聊天记录，开始新的对话", icon: Eraser, disabled: Boolean(runningTask) },
        { id: "rename", label: "重命名助手", description: "为助手换一个称呼", icon: Pencil },
      ] : undefined}
      onCommandSelect={selectCommand}
      fileSuggestions={fileSuggestions}
      attachments={attachments}
      referenceLoading={referenceLoading}
      variant={variant}
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

  return (
    <>
      {variant === "hero" ? composer : (
        <div
          ref={dockRef}
          className={cn(
            "fixed inset-x-0 bottom-[var(--app-keyboard-inset-bottom,0px)] z-30 bg-transparent px-3 pb-3 pt-2 md:static md:z-auto md:px-6 md:pb-5",
            className,
          )}
        >
          <div className="mx-auto max-w-4xl">
            <QueuePanel onEditMessage={setValue} />
            {composer}
          </div>
        </div>
      )}
      <Dialog
        open={Boolean(commandDialog)}
        title={commandDialog === "clear" ? "清空助手上下文" : "重命名助手"}
        className="max-w-md"
        closeDisabled={commandBusy}
        onOpenChange={(open) => { if (!open) setCommandDialog(undefined); }}
        footer={
          <div className="flex justify-end gap-2">
            <Button variant="outline" disabled={commandBusy} onClick={() => setCommandDialog(undefined)} autoFocus={commandDialog === "clear"}>
              取消
            </Button>
            <Button
              variant={commandDialog === "clear" ? "destructive" : "default"}
              disabled={commandBusy || (commandDialog === "rename" && !assistantNameDraft.trim())}
              onClick={() => void confirmCommand()}
            >
              {commandBusy ? "处理中…" : commandDialog === "clear" ? "确认清空" : "保存"}
            </Button>
          </div>
        }
      >
        {commandDialog === "clear" ? (
          <p className="text-sm leading-6 text-muted-foreground">将清空当前助手的聊天记录和待处理队列，保留助手名称与长期记忆。此操作无法撤销。</p>
        ) : (
          <form onSubmit={(event) => { event.preventDefault(); void confirmCommand(); }}>
            <label className="block space-y-2 text-sm font-medium">
              <span>助手名称</span>
              <Input
                autoFocus
                disabled={commandBusy}
                value={assistantNameDraft}
                placeholder="输入助手名称"
                onChange={(event) => setAssistantNameDraft(event.target.value)}
                aria-invalid={Boolean(commandError)}
              />
            </label>
            <p className="mt-2 text-xs text-muted-foreground">1–40 个字符</p>
          </form>
        )}
        {commandError ? <p role="alert" className="mt-3 text-sm text-destructive">{commandError}</p> : null}
      </Dialog>
    </>
  );
}

function restoreSubmittedValue(submitted: string, current: string) {
  if (!current) return submitted;
  if (!submitted) return current;
  return `${submitted.trimEnd()}\n${current}`;
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
