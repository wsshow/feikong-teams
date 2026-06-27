import {
  Archive,
  ArrowLeft,
  Code2,
  Copy,
  Download,
  File,
  FilePenLine,
  FileText,
  Folder,
  Image,
  RefreshCcw,
  Save,
  Trash2,
  Upload,
} from "lucide-react";
import { useEffect, useState } from "react";
import type { LucideIcon } from "lucide-react";
import { createPreviewLink, deleteFile, listFiles, readFileContent, saveFileContent, uploadFile } from "@/api/files";
import { filesActions, appActions } from "@/app/store";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Panel, PanelBody, PanelHeader } from "@/components/ui/panel";
import { formatBytes, formatTime } from "@/lib/format";
import { cn } from "@/lib/cn";
import type { FileContent, FileEntry } from "@/types/files";

export function FileManager() {
  const dispatch = useAppDispatch();
  const path = useAppSelector((state) => state.files.path);
  const entries = useAppSelector((state) => state.files.entries);
  const [uploading, setUploading] = useState(false);
  const [editor, setEditor] = useState<FileContent | null>(null);
  const [draft, setDraft] = useState("");
  const [editorError, setEditorError] = useState("");
  const [saving, setSaving] = useState(false);
  const [linkingPath, setLinkingPath] = useState("");

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

  async function editFile(filePath: string) {
    setEditorError("");
    try {
      const file = await readFileContent(filePath);
      setEditor(file);
      setDraft(file.content || "");
    } catch (error) {
      setEditorError(error instanceof Error ? error.message : String(error));
    }
  }

  async function saveEditor() {
    if (!editor) return;
    setSaving(true);
    setEditorError("");
    try {
      const saved = await saveFileContent(editor.path, draft);
      setEditor({ ...editor, ...saved, content: draft });
      dispatch(appActions.showToast("已保存"));
      await load();
    } catch (error) {
      setEditorError(error instanceof Error ? error.message : String(error));
    } finally {
      setSaving(false);
    }
  }

  async function copyPreviewLink(filePath: string) {
    setEditorError("");
    setLinkingPath(filePath);
    try {
      const link = await createPreviewLink(filePath);
      const id = link.link_id || link.id;
      if (!id) throw new Error("preview link id is empty");
      const url = `${window.location.origin}/p/${encodeURIComponent(id)}`;
      await navigator.clipboard.writeText(url);
      dispatch(appActions.showToast("预览链接已复制"));
    } catch (error) {
      setEditorError(error instanceof Error ? error.message : String(error));
    } finally {
      setLinkingPath("");
    }
  }

  function openEntry(entry: FileEntry) {
    if (entry.is_dir) {
      setEditor(null);
      void load(entry.path);
      return;
    }
    void editFile(entry.path);
  }

  useEffect(() => {
    void load("");
  }, []);

  return (
    <div className={cn("h-full p-6", editor ? "overflow-hidden" : "overflow-auto")}>
      <Panel className={cn("flex min-h-0 flex-col", editor ? "h-full w-full" : "mx-auto max-w-6xl")}>
        <PanelHeader className="flex flex-wrap items-center justify-between gap-4">
          {editor ? (
            <>
              <div className="min-w-0">
                <div className="flex min-w-0 items-center gap-2 font-semibold">
                  <FileIcon entry={{ name: editor.name || editor.path, path: editor.path }} />
                  <span className="truncate">{editor.name || editor.path}</span>
                </div>
                <div className="mt-0.5 truncate text-sm text-muted-foreground">{editor.path}</div>
              </div>
              <div className="flex items-center gap-2">
                <Button className="whitespace-nowrap" variant="outline" onClick={() => setEditor(null)}>
                  <ArrowLeft className="h-4 w-4" />
                  返回
                </Button>
                <Button className="min-w-20 whitespace-nowrap" onClick={() => void saveEditor()} disabled={saving}>
                  <Save className="h-4 w-4" />
                  {saving ? "保存中" : "保存"}
                </Button>
              </div>
            </>
          ) : (
            <>
              <div>
                <div className="font-semibold">文件管理</div>
                <div className="text-sm text-muted-foreground">当前路径：{path || "."}</div>
              </div>
              <div className="flex min-w-0 items-center gap-2">
                <Input
                  className="w-72"
                  value={path}
                  onChange={(event) => dispatch(filesActions.setPath(event.target.value))}
                  onKeyDown={(event) => {
                    if (event.key === "Enter") void load();
                  }}
                  placeholder="路径"
                />
                <Button className="min-w-20 whitespace-nowrap" variant="outline" onClick={() => load()}>
                  <RefreshCcw className="h-4 w-4" />
                  刷新
                </Button>
                <label>
                  <input className="hidden" type="file" onChange={(event) => void handleUpload(event.target.files?.[0])} />
                  <span className="inline-flex h-9 min-w-20 cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-md border border-primary/70 bg-primary px-4 text-sm font-semibold text-primary-foreground shadow-[2px_3px_0_hsl(214_45%_30%/0.16)] transition-colors hover:bg-primary/90">
                    <Upload className="h-4 w-4" />
                    {uploading ? "上传中" : "上传"}
                  </span>
                </label>
              </div>
            </>
          )}
        </PanelHeader>
        <PanelBody className={cn(editor && "flex min-h-0 flex-1 flex-col")}>
          {editorError && !editor ? <div className="mb-3 rounded-lg border border-destructive/40 bg-destructive/5 px-3 py-2 text-sm text-destructive">{editorError}</div> : null}
          {editor ? (
            <div className="flex min-h-0 flex-1 flex-col gap-3">
              {editorError ? <div className="rounded-lg border border-destructive/40 bg-destructive/5 px-3 py-2 text-sm text-destructive">{editorError}</div> : null}
              <textarea
                className="min-h-0 flex-1 w-full resize-none rounded-lg border border-input bg-card/80 p-4 font-mono text-sm leading-6 text-foreground outline-none transition-colors placeholder:text-muted-foreground focus:border-ring focus:ring-2 focus:ring-ring/30"
                value={draft}
                onChange={(event) => setDraft(event.target.value)}
                spellCheck={false}
              />
            </div>
          ) : (
            <div className="overflow-hidden rounded-md border">
              <table className="w-full text-sm">
                <thead className="bg-muted text-muted-foreground">
                  <tr>
                    <th className="px-3 py-2 text-left">名称</th>
                    <th className="px-3 py-2 text-left">大小</th>
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
                            setEditor(null);
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
                      <td className="px-3 py-2">
                        <button className="flex min-w-0 items-center gap-2 text-left" onClick={() => openEntry(entry)}>
                          <FileIcon entry={entry} />
                          {entry.name}
                        </button>
                      </td>
                      <td className="px-3 py-2 text-muted-foreground">{entry.is_dir ? "-" : formatBytes(entry.size)}</td>
                      <td className="px-3 py-2 text-muted-foreground">{formatTime(entry.mod_time)}</td>
                      <td className="space-x-1 px-3 py-2 text-right">
                        {!entry.is_dir ? (
                          <>
                            <Button size="icon" variant="ghost" onClick={() => void editFile(entry.path)} aria-label="编辑">
                              <FilePenLine className="h-4 w-4" />
                            </Button>
                            <Button
                              size="icon"
                              variant="ghost"
                              onClick={() => void copyPreviewLink(entry.path)}
                              aria-label="生成预览链接"
                              disabled={linkingPath === entry.path}
                            >
                              <Copy className="h-4 w-4" />
                            </Button>
                            <Button size="icon" variant="ghost" onClick={() => window.open(`/api/fkteams/files/download?path=${encodeURIComponent(entry.path)}`)} aria-label="下载">
                              <Download className="h-4 w-4" />
                            </Button>
                          </>
                        ) : null}
                        <Button
                          size="icon"
                          variant="ghost"
                          onClick={() => deleteFile(entry.path).then(() => load()).then(() => dispatch(appActions.showToast("已删除")))}
                          aria-label="删除"
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </PanelBody>
      </Panel>
    </div>
  );
}

function FileIcon({ entry }: { entry: FileEntry }) {
  const Icon = fileIcon(entry);
  return <Icon className="h-4 w-4 shrink-0 text-muted-foreground group-hover:text-foreground" />;
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
