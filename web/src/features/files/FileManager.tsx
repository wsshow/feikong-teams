import {
  Archive,
  ArrowLeft,
  AlertCircle,
  Code2,
  Download,
  Eye,
  File,
  FilePenLine,
  FileText,
  Folder,
  Image,
  MoreVertical,
  RefreshCcw,
  Save,
  Share2,
  Trash2,
  Upload,
} from "lucide-react";
import { useEffect, useState } from "react";
import type { LucideIcon } from "lucide-react";
import { deleteFile, listFiles, readFileContent, saveFileContent, uploadFile } from "@/api/files";
import { filesActions, appActions } from "@/app/store";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { MarkdownContent } from "@/components/markdown/MarkdownContent";
import { Panel, PanelBody, PanelHeader } from "@/components/ui/panel";
import { formatBytes, formatTime } from "@/lib/format";
import { cn } from "@/lib/cn";
import { highlightCode } from "@/lib/markdown";
import type { FileContent, FileEntry } from "@/types/files";
import { FileShareDialog } from "./FileShareDialog";

type FileViewMode = "preview" | "source" | "edit";

interface FileViewer {
  entry: FileEntry;
  content?: FileContent;
}

export function FileManager() {
  const dispatch = useAppDispatch();
  const path = useAppSelector((state) => state.files.path);
  const entries = useAppSelector((state) => state.files.entries);
  const [uploading, setUploading] = useState(false);
  const [viewer, setViewer] = useState<FileViewer | null>(null);
  const [viewMode, setViewMode] = useState<FileViewMode>("preview");
  const [draft, setDraft] = useState("");
  const [viewerError, setViewerError] = useState("");
  const [saving, setSaving] = useState(false);
  const [shareTarget, setShareTarget] = useState<FileEntry | null>(null);
  const [openActionPath, setOpenActionPath] = useState("");
  const actionEntry = entries.find((entry) => entry.path === openActionPath);

  async function load(nextPath = path) {
    const files = await listFiles(nextPath);
    dispatch(filesActions.setPath(nextPath));
    dispatch(filesActions.setFiles(files || []));
  }

  async function handleUpload(file?: File) {
    if (!file) return;
    setUploading(true);
    try {
      await uploadFile(file, path);
      await load();
    } finally {
      setUploading(false);
    }
  }

  async function openFile(entry: FileEntry, preferredMode?: FileViewMode) {
    setOpenActionPath("");
    setViewerError("");
    setDraft("");
    const kind = filePreviewKind(entry);
    const nextMode = preferredMode || defaultViewMode(entry);
    setViewMode(nextMode);

    if (!requiresTextContent(kind) && nextMode !== "source" && nextMode !== "edit") {
      setViewer({ entry });
      return;
    }

    try {
      const file = await readFileContent(entry.path);
      setViewer({ entry, content: file });
      setDraft(file.content || "");
    } catch (error) {
      setViewer({ entry });
      setViewerError(error instanceof Error ? error.message : String(error));
    }
  }

  function changeViewMode(mode: FileViewMode) {
    if (!viewer) return;
    const kind = filePreviewKind(viewer.entry);
    setViewMode(mode);
    if ((mode === "source" || mode === "edit" || kind === "markdown") && isEditableKind(kind) && !viewer.content) {
      void openFile(viewer.entry, mode);
    }
  }

  async function saveEditor() {
    if (!viewer?.content) return;
    setSaving(true);
    setViewerError("");
    try {
      const saved = await saveFileContent(viewer.content.path, draft);
      setViewer({ ...viewer, content: { ...viewer.content, ...saved, content: draft } });
      dispatch(appActions.showToast("已保存"));
      await load();
    } catch (error) {
      setViewerError(error instanceof Error ? error.message : String(error));
    } finally {
      setSaving(false);
    }
  }

  function openEntry(entry: FileEntry) {
    setOpenActionPath("");
    if (entry.is_dir) {
      setViewer(null);
      void load(entry.path);
      return;
    }
    void openFile(entry);
  }

  function downloadEntry(entry: FileEntry) {
    setOpenActionPath("");
    window.open(`/api/fkteams/files/download?path=${encodeURIComponent(entry.path)}`);
  }

  async function removeEntry(entry: FileEntry) {
    setOpenActionPath("");
    await deleteFile(entry.path);
    await load();
    dispatch(appActions.showToast("已删除"));
  }

  function shareEntry(entry: FileEntry) {
    setOpenActionPath("");
    setShareTarget(entry);
  }

  useEffect(() => {
    void load("");
  }, []);

  return (
    <div className={cn("h-full p-3 sm:p-6", viewer ? "overflow-hidden" : "overflow-auto")}>
      <Panel className={cn("flex min-h-0 flex-col", viewer ? "h-full w-full" : "mx-auto max-w-6xl")}>
        <PanelHeader className="flex flex-wrap items-center justify-between gap-4">
          {viewer ? (
            <>
              <div className="min-w-0">
                <div className="flex min-w-0 items-center gap-2 font-semibold">
                  <FileIcon entry={viewer.entry} />
                  <span className="truncate">{viewer.entry.name || viewer.entry.path}</span>
                </div>
                <div className="mt-0.5 truncate text-sm text-muted-foreground">{viewer.entry.path}</div>
              </div>
              <div className="flex flex-wrap items-center justify-end gap-2">
                <FileModeSwitch entry={viewer.entry} mode={viewMode} onChange={changeViewMode} />
                <Button className="whitespace-nowrap" variant="outline" onClick={() => setViewer(null)}>
                  <ArrowLeft className="h-4 w-4" />
                  返回
                </Button>
                {viewMode === "edit" ? (
                  <Button className="min-w-20 whitespace-nowrap" onClick={() => void saveEditor()} disabled={saving || !viewer.content}>
                    <Save className="h-4 w-4" />
                    {saving ? "保存中" : "保存"}
                  </Button>
                ) : null}
              </div>
            </>
          ) : (
            <>
              <div>
                <div className="font-semibold">文件管理</div>
                <div className="text-sm text-muted-foreground">当前路径：{path || "."}</div>
              </div>
              <div className="grid w-full min-w-0 grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-2 sm:w-auto sm:grid-cols-none sm:flex">
                <Input
                  className="min-w-0"
                  value={path}
                  onChange={(event) => dispatch(filesActions.setPath(event.target.value))}
                  onKeyDown={(event) => {
                    if (event.key === "Enter") void load();
                  }}
                  placeholder="路径"
                />
                <Button className="min-w-20 justify-center whitespace-nowrap px-3 sm:px-4" variant="outline" onClick={() => load()}>
                  <RefreshCcw className="h-4 w-4" />
                  刷新
                </Button>
                <label>
                  <input className="hidden" type="file" onChange={(event) => void handleUpload(event.target.files?.[0])} />
                  <span className="inline-flex h-9 min-w-20 cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-md border border-primary/70 bg-primary px-3 text-sm font-semibold text-primary-foreground shadow-[2px_3px_0_hsl(214_45%_30%/0.16)] transition-colors hover:bg-primary/90 sm:px-4">
                    <Upload className="h-4 w-4" />
                    {uploading ? "上传中" : "上传"}
                  </span>
                </label>
              </div>
            </>
          )}
        </PanelHeader>
        <PanelBody className={cn(viewer && "flex min-h-0 flex-1 flex-col")}>
          {viewerError && !viewer ? <div className="mb-3 rounded-lg border border-destructive/40 bg-destructive/5 px-3 py-2 text-sm text-destructive">{viewerError}</div> : null}
          {viewer ? (
            <div className="flex min-h-0 flex-1 flex-col gap-3">
              {viewerError ? <div className="rounded-lg border border-destructive/40 bg-destructive/5 px-3 py-2 text-sm text-destructive">{viewerError}</div> : null}
              <FileViewerContent viewer={viewer} mode={viewMode} draft={draft} onDraftChange={setDraft} />
            </div>
          ) : (
            <div className="overflow-hidden rounded-md border">
              <table className="w-full text-sm">
                <thead className="bg-muted text-muted-foreground">
                  <tr>
                    <th className="px-3 py-2 text-left">名称</th>
                    <th className="hidden px-3 py-2 text-left sm:table-cell">大小</th>
                    <th className="px-3 py-2 text-left">修改时间</th>
                    <th className="px-3 py-2 text-right">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {path ? (
                    <tr className="border-t">
                      <td className="px-3 py-2" colSpan={4}>
                        <button
                          className="text-primary"
                          onClick={() => {
                            setViewer(null);
                            void load(path.split("/").slice(0, -1).join("/"));
                          }}
                        >
                          返回上级
                        </button>
                      </td>
                    </tr>
                  ) : null}
                  {entries.map((entry) => (
                    <tr key={entry.path} className="group border-t transition-colors hover:bg-card/70">
                      <td className="min-w-0 px-3 py-2">
                        <button className="flex max-w-full min-w-0 items-center gap-2 text-left" onClick={() => openEntry(entry)}>
                          <FileIcon entry={entry} />
                          <span className="min-w-0 truncate">{entry.name}</span>
                        </button>
                      </td>
                      <td className="hidden px-3 py-2 text-muted-foreground sm:table-cell">{entry.is_dir ? "-" : formatBytes(entry.size)}</td>
                      <td className="px-3 py-2 text-muted-foreground">{formatTime(entry.mod_time)}</td>
                      <td className="px-2 py-2 text-right sm:px-3">
                        <div className="hidden justify-end gap-1 sm:flex">
                          <FileActionButtons
                            entry={entry}
                            onEdit={() => void openFile(entry, "edit")}
                            onShare={() => shareEntry(entry)}
                            onDownload={() => downloadEntry(entry)}
                            onDelete={() => void removeEntry(entry)}
                          />
                        </div>
                        <div className="relative flex justify-end sm:hidden">
                          <Button
                            size="icon"
                            variant="ghost"
                            onClick={() => setOpenActionPath(openActionPath === entry.path ? "" : entry.path)}
                            aria-label="更多操作"
                            aria-expanded={openActionPath === entry.path}
                          >
                            <MoreVertical className="h-4 w-4" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </PanelBody>
      </Panel>
      {actionEntry ? (
        <FileActionSheet
          entry={actionEntry}
          onOpen={() => openEntry(actionEntry)}
          onEdit={() => void openFile(actionEntry, "edit")}
          onShare={() => shareEntry(actionEntry)}
          onDownload={() => downloadEntry(actionEntry)}
          onDelete={() => void removeEntry(actionEntry)}
          onClose={() => setOpenActionPath("")}
        />
      ) : null}
      <FileShareDialog file={shareTarget} onClose={() => setShareTarget(null)} />
    </div>
  );
}

function FileModeSwitch({ entry, mode, onChange }: { entry: FileEntry; mode: FileViewMode; onChange: (mode: FileViewMode) => void }) {
  const kind = filePreviewKind(entry);
  const editable = isEditableKind(kind);
  const modes: Array<{ value: FileViewMode; label: string; icon: LucideIcon }> = [];
  if (supportsRenderedPreview(kind)) {
    modes.push({ value: "preview", label: "预览", icon: Eye });
  }
  if (editable) {
    modes.push({ value: "source", label: "源码", icon: Code2 });
    modes.push({ value: "edit", label: "编辑", icon: FilePenLine });
  }
  if (modes.length <= 1) return null;
  return (
    <div className="flex rounded-lg border border-border/75 bg-card/70 p-1">
      {modes.map((item) => {
        const Icon = item.icon;
        return (
          <button
            key={item.value}
            type="button"
            className={cn(
              "inline-flex h-8 items-center gap-1.5 rounded-md px-2.5 text-sm transition-colors",
              mode === item.value ? "bg-primary/10 text-primary" : "text-muted-foreground hover:bg-accent/65 hover:text-foreground",
            )}
            onClick={() => onChange(item.value)}
          >
            <Icon className="h-4 w-4" />
            {item.label}
          </button>
        );
      })}
    </div>
  );
}

function FileViewerContent({
  viewer,
  mode,
  draft,
  onDraftChange,
}: {
  viewer: FileViewer;
  mode: FileViewMode;
  draft: string;
  onDraftChange: (value: string) => void;
}) {
  const kind = filePreviewKind(viewer.entry);
  if (mode === "edit") {
    if (!viewer.content) return <UnsupportedPreview message="这个文件无法作为 UTF-8 文本编辑。" />;
    return (
      <textarea
        className="min-h-0 flex-1 w-full resize-none rounded-lg border border-input bg-card/80 p-4 font-mono text-sm leading-6 text-foreground outline-none transition-colors placeholder:text-muted-foreground focus:border-ring focus:ring-2 focus:ring-ring/30"
        value={draft}
        onChange={(event) => onDraftChange(event.target.value)}
        spellCheck={false}
      />
    );
  }
  if (mode === "source") {
    if (!viewer.content) return <UnsupportedPreview message="这个文件没有可显示的 UTF-8 源码内容。" />;
    return <HighlightedSource content={draft} language={fileLanguage(viewer.entry)} />;
  }
  if (kind === "markdown") {
    if (!viewer.content) return <UnsupportedPreview message="无法读取 Markdown 内容。" />;
    return (
      <div className="chat-scroll min-h-0 flex-1 overflow-auto rounded-lg border border-border/75 bg-background/60 p-5">
        <MarkdownContent className="text-base leading-8" content={draft} />
      </div>
    );
  }
  if (kind === "html" || kind === "pdf") {
    return <iframe className="min-h-0 flex-1 rounded-lg border border-border/75 bg-background" referrerPolicy="no-referrer" src={serveFileURL(viewer.entry.path)} title={viewer.entry.name} sandbox={kind === "html" ? "" : undefined} />;
  }
  if (kind === "image") {
    return (
      <div className="flex min-h-0 flex-1 items-center justify-center overflow-auto rounded-lg border border-border/75 bg-background/60 p-4">
        <img className="max-h-full max-w-full object-contain" src={serveFileURL(viewer.entry.path)} alt={viewer.entry.name} />
      </div>
    );
  }
  if (kind === "audio") {
    return (
      <div className="flex min-h-0 flex-1 items-center justify-center rounded-lg border border-border/75 bg-background/60 p-6">
        <audio className="w-full max-w-3xl" src={serveFileURL(viewer.entry.path)} controls />
      </div>
    );
  }
  if (kind === "video") {
    return (
      <div className="flex min-h-0 flex-1 items-center justify-center overflow-hidden rounded-lg border border-border/75 bg-background/60 p-4">
        <video className="max-h-full max-w-full" src={serveFileURL(viewer.entry.path)} controls />
      </div>
    );
  }
  if (viewer.content) {
    return <HighlightedSource content={viewer.content.content || ""} language={fileLanguage(viewer.entry)} />;
  }
  return <UnsupportedPreview message="当前文件类型暂不支持预览。你仍然可以下载后使用本地应用打开。" />;
}

function HighlightedSource({ content, language }: { content: string; language: string }) {
  return (
    <div className="prose message-prose min-h-0 max-w-none flex-1 overflow-auto rounded-lg border border-border/75 bg-background/60">
      <div className="markdown-code-block m-0 rounded-none border-0">
        <div className="markdown-code-header">
          <span className="markdown-code-language">{language}</span>
        </div>
        <pre className="min-h-full"><code dangerouslySetInnerHTML={{ __html: highlightCode(content, language) }} /></pre>
      </div>
    </div>
  );
}

function UnsupportedPreview({ message }: { message: string }) {
  return (
    <div className="flex min-h-0 flex-1 items-center justify-center rounded-lg border border-dashed border-border bg-background/45 p-8 text-center">
      <div className="max-w-md">
        <AlertCircle className="mx-auto h-9 w-9 text-muted-foreground" />
        <div className="mt-3 font-medium">无法预览</div>
        <div className="mt-2 text-sm leading-6 text-muted-foreground">{message}</div>
      </div>
    </div>
  );
}

function FileIcon({ entry }: { entry: FileEntry }) {
  const Icon = fileIcon(entry);
  return <Icon className="h-4 w-4 shrink-0 text-muted-foreground group-hover:text-foreground" />;
}

function FileActionButtons({
  entry,
  onEdit,
  onShare,
  onDownload,
  onDelete,
}: {
  entry: FileEntry;
  onEdit: () => void;
  onShare: () => void;
  onDownload: () => void;
  onDelete: () => void;
}) {
  const editable = isEditableKind(filePreviewKind(entry));
  return (
    <>
      {!entry.is_dir ? (
        <>
          {editable ? (
            <Button size="icon" variant="ghost" onClick={onEdit} aria-label="编辑">
              <FilePenLine className="h-4 w-4" />
            </Button>
          ) : null}
          <Button size="icon" variant="ghost" onClick={onShare} aria-label="分享文件">
            <Share2 className="h-4 w-4" />
          </Button>
          <Button size="icon" variant="ghost" onClick={onDownload} aria-label="下载">
            <Download className="h-4 w-4" />
          </Button>
        </>
      ) : null}
      <Button size="icon" variant="ghost" onClick={onDelete} aria-label="删除">
        <Trash2 className="h-4 w-4" />
      </Button>
    </>
  );
}

function FileActionSheet({
  entry,
  onOpen,
  onEdit,
  onShare,
  onDownload,
  onDelete,
  onClose,
}: {
  entry: FileEntry;
  onOpen: () => void;
  onEdit: () => void;
  onShare: () => void;
  onDownload: () => void;
  onDelete: () => void;
  onClose: () => void;
}) {
  const editable = isEditableKind(filePreviewKind(entry));
  function run(action: () => void) {
    onClose();
    action();
  }

  return (
    <div className="fixed inset-0 z-50 sm:hidden" role="dialog" aria-modal="true">
      <button className="absolute inset-0 bg-foreground/15 backdrop-blur-[1px]" type="button" aria-label="关闭文件操作菜单" onClick={onClose} />
      <div className="sketch-surface absolute inset-x-3 bottom-3 rounded-2xl bg-card p-2 text-sm shadow-[0_18px_48px_hsl(218_30%_20%/0.2)]">
        <div className="px-3 pb-2 pt-1">
          <div className="truncate text-base font-semibold text-foreground">{entry.name}</div>
          <div className="mt-0.5 truncate text-xs text-muted-foreground">{entry.path}</div>
        </div>
        {entry.is_dir ? (
          <button className="flex h-11 w-full items-center gap-3 rounded-xl px-3 text-left hover:bg-accent/65" type="button" onClick={() => run(onOpen)}>
            <Folder className="h-4 w-4" />
            打开
          </button>
        ) : (
          <>
            {editable ? (
              <button className="flex h-11 w-full items-center gap-3 rounded-xl px-3 text-left hover:bg-accent/65" type="button" onClick={() => run(onEdit)}>
                <FilePenLine className="h-4 w-4" />
                编辑
              </button>
            ) : null}
            <button className="flex h-11 w-full items-center gap-3 rounded-xl px-3 text-left hover:bg-accent/65" type="button" onClick={() => run(onShare)}>
              <Share2 className="h-4 w-4" />
              分享
            </button>
            <button className="flex h-11 w-full items-center gap-3 rounded-xl px-3 text-left hover:bg-accent/65" type="button" onClick={() => run(onDownload)}>
              <Download className="h-4 w-4" />
              下载
            </button>
          </>
        )}
        <div className="my-1 border-t border-border/70" />
        <button className="flex h-11 w-full items-center gap-3 rounded-xl px-3 text-left text-destructive hover:bg-destructive/10" type="button" onClick={() => run(onDelete)}>
          <Trash2 className="h-4 w-4" />
          删除
        </button>
      </div>
    </div>
  );
}

function fileIcon(entry: FileEntry): LucideIcon {
  if (entry.is_dir) return Folder;
  const ext = entry.name.split(".").pop()?.toLowerCase() || "";
  if (["png", "jpg", "jpeg", "gif", "webp", "svg", "bmp"].includes(ext)) return Image;
  if (["md", "txt", "log", "json", "yaml", "yml", "toml", "csv"].includes(ext)) return FileText;
  if (["go", "ts", "tsx", "js", "jsx", "css", "html", "sh", "py", "rs", "java", "c", "cpp", "h"].includes(ext)) return Code2;
  if (["zip", "tar", "gz", "tgz", "rar", "7z"].includes(ext)) return Archive;
  return File;
}

type FilePreviewKind = "markdown" | "html" | "text" | "image" | "audio" | "video" | "pdf" | "unsupported";

function filePreviewKind(entry: FileEntry): FilePreviewKind {
  const ext = fileExtension(entry);
  if (["md", "markdown", "mdown"].includes(ext)) return "markdown";
  if (["html", "htm"].includes(ext)) return "html";
  if (["png", "jpg", "jpeg", "gif", "webp", "svg", "bmp", "ico", "avif"].includes(ext)) return "image";
  if (["mp3", "wav", "ogg", "m4a", "aac", "flac", "weba"].includes(ext)) return "audio";
  if (["mp4", "webm", "mov", "m4v", "ogv"].includes(ext)) return "video";
  if (ext === "pdf") return "pdf";
  if (
    [
      "txt",
      "log",
      "json",
      "jsonl",
      "yaml",
      "yml",
      "toml",
      "csv",
      "tsv",
      "xml",
      "css",
      "scss",
      "less",
      "js",
      "jsx",
      "ts",
      "tsx",
      "go",
      "sh",
      "bash",
      "zsh",
      "py",
      "rs",
      "java",
      "c",
      "cpp",
      "h",
      "hpp",
      "sql",
      "diff",
      "patch",
      "env",
      "ini",
      "conf",
      "dockerfile",
    ].includes(ext) ||
    entry.name.toLowerCase() === "dockerfile"
  ) {
    return "text";
  }
  return "unsupported";
}

function defaultViewMode(entry: FileEntry): FileViewMode {
  const kind = filePreviewKind(entry);
  if (supportsRenderedPreview(kind)) return "preview";
  if (isEditableKind(kind)) return "source";
  return "preview";
}

function supportsRenderedPreview(kind: FilePreviewKind) {
  return ["markdown", "html", "image", "audio", "video", "pdf", "unsupported"].includes(kind);
}

function isEditableKind(kind: FilePreviewKind) {
  return kind === "markdown" || kind === "html" || kind === "text";
}

function requiresTextContent(kind: FilePreviewKind) {
  return kind === "markdown" || kind === "text";
}

function serveFileURL(path: string) {
  return `/api/fkteams/files/serve/${path.replace(/\\/g, "/").split("/").map(encodeURIComponent).join("/")}`;
}

function fileLanguage(entry: FileEntry) {
  const ext = fileExtension(entry);
  const name = entry.name.toLowerCase();
  if (name === "dockerfile") return "dockerfile";
  const aliases: Record<string, string> = {
    htm: "html",
    mdown: "markdown",
    md: "markdown",
    jsonl: "json",
    yml: "yaml",
    bash: "sh",
    zsh: "sh",
    patch: "diff",
    env: "ini",
    conf: "ini",
  };
  return aliases[ext] || ext || "text";
}

function fileExtension(entry: FileEntry) {
  const name = entry.name || entry.path;
  const index = name.lastIndexOf(".");
  if (index < 0 || index === name.length - 1) return "";
  return name.slice(index + 1).toLowerCase();
}
