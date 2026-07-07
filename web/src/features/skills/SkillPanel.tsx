import {
  Box,
  Download,
  ExternalLink,
  FilePlus,
  FileText,
  Folder,
  FolderPlus,
  PackageCheck,
  Plus,
  RefreshCcw,
  Save,
  Search,
  Sparkles,
  Star,
  Trash2,
  Wand2,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { generateSkillDraft } from "@/api/ai";
import {
  createSkill,
  createSkillFile,
  deleteSkillFile,
  installSkill,
  listSkillFiles,
  listSkills,
  readSkillFile,
  removeSkill,
  saveSkillFile,
  searchSkills,
} from "@/api/skills";
import { skillsActions, appActions } from "@/app/store";
import { useAppDispatch, useAppSelector } from "@/app/hooks";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { Dialog } from "@/components/ui/dialog";
import { LoadingSurface } from "@/components/ui/loading-surface";
import { Panel, PanelBody, PanelHeader } from "@/components/ui/panel";
import { cn } from "@/lib/cn";
import type { SkillCreateRequest, SkillFileEntry, SkillInfo } from "@/types/skills";

type SkillView = "installed" | "market";

export function SkillPanel() {
  const dispatch = useAppDispatch();
  const local = useAppSelector((state) => state.skills.local);
  const results = useAppSelector((state) => state.skills.results);
  const [keyword, setKeyword] = useState("");
  const [view, setView] = useState<SkillView>("installed");
  const [selectedSlug, setSelectedSlug] = useState("");
  const [filePath, setFilePath] = useState("");
  const [files, setFiles] = useState<SkillFileEntry[]>([]);
  const [content, setContent] = useState("");
  const [editorContent, setEditorContent] = useState("");
  const [activeFile, setActiveFile] = useState("");
  const [loadingLocal, setLoadingLocal] = useState(false);
  const [searching, setSearching] = useState(false);
  const [busySlug, setBusySlug] = useState("");
  const [savingFile, setSavingFile] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [creatingSkill, setCreatingSkill] = useState(false);
  const [generatingSkill, setGeneratingSkill] = useState(false);
  const [aiInstruction, setAiInstruction] = useState("");
  const [skillDraft, setSkillDraft] = useState<SkillCreateRequest>(() => emptySkillDraft());
  const installedSlugs = useMemo(() => new Set(local.map((skill) => skill.slug)), [local]);
  const selectedSkill =
    local.find((skill) => skill.slug === selectedSlug) || results.find((skill) => skill.slug === selectedSlug);

  async function loadLocal() {
    setLoadingLocal(true);
    try {
      const result = await listSkills();
      dispatch(skillsActions.setLocalSkills(result.skills || []));
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    } finally {
      setLoadingLocal(false);
    }
  }

  async function search() {
    const query = keyword.trim();
    if (!query) return;
    setSearching(true);
    setView("market");
    try {
      const result = await searchSkills(query);
      dispatch(skillsActions.setSkillResults(result.skills || []));
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    } finally {
      setSearching(false);
    }
  }

  async function select(skill: SkillInfo) {
    setSelectedSlug(skill.slug);
    setFilePath("");
    setActiveFile("");
    setContent("");
    setEditorContent("");
    if (!installedSlugs.has(skill.slug)) {
      setFiles([]);
      return;
    }
    await openDirectory(skill.slug, "");
  }

  async function openDirectory(slug = selectedSlug, path = "") {
    if (!slug) return;
    setFilePath(path);
    setActiveFile("");
    setContent("");
    setEditorContent("");
    try {
      const result = await listSkillFiles(slug, path);
      setFiles(result.files || []);
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    }
  }

  async function refreshDirectory(slug = selectedSlug, path = filePath) {
    if (!slug) return;
    try {
      const result = await listSkillFiles(slug, path);
      setFiles(result.files || []);
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    }
  }

  async function openFile(path: string) {
    if (!selectedSlug) return;
    setActiveFile(path);
    try {
      const result = await readSkillFile(selectedSlug, path);
      setContent(result.content || "");
      setEditorContent(result.content || "");
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    }
  }

  async function createCustomSkill() {
    const draft = {
      ...skillDraft,
      slug: skillDraft.slug.trim(),
      name: skillDraft.name.trim(),
      description: skillDraft.description.trim(),
      content: skillDraft.content.trim(),
    };
    if (!draft.slug || !draft.name || !draft.content) {
      dispatch(appActions.showToast("技能标识、名称和内容不能为空"));
      return;
    }
    setCreatingSkill(true);
    try {
      const result = await createSkill(draft);
      await loadLocal();
      dispatch(appActions.showToast("技能已创建"));
      setCreateOpen(false);
      setSkillDraft(emptySkillDraft());
      setAiInstruction("");
      setView("installed");
      const created = result.skill || { slug: draft.slug, name: draft.name, description: draft.description };
      setSelectedSlug(created.slug);
      await openDirectory(created.slug, "");
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    } finally {
      setCreatingSkill(false);
    }
  }

  async function generateDraft() {
    const instruction = aiInstruction.trim();
    if (!instruction) {
      dispatch(appActions.showToast("请输入希望创建的技能说明"));
      return;
    }
    setGeneratingSkill(true);
    try {
      const result = await generateSkillDraft({ instruction, existing_skills: local.map((skill) => skill.slug) });
      setSkillDraft(result.skill);
      dispatch(appActions.showToast("AI 草稿已生成"));
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    } finally {
      setGeneratingSkill(false);
    }
  }

  async function generateAndCreateSkill() {
    const instruction = aiInstruction.trim();
    if (!instruction) {
      dispatch(appActions.showToast("请输入希望创建的技能说明"));
      return;
    }
    setGeneratingSkill(true);
    setCreatingSkill(true);
    try {
      const draftResult = await generateSkillDraft({ instruction, existing_skills: local.map((skill) => skill.slug) });
      const result = await createSkill(draftResult.skill);
      await loadLocal();
      dispatch(appActions.showToast("AI 技能已创建"));
      setCreateOpen(false);
      setSkillDraft(emptySkillDraft());
      setAiInstruction("");
      setView("installed");
      const created = result.skill || {
        slug: draftResult.skill.slug,
        name: draftResult.skill.name,
        description: draftResult.skill.description,
      };
      setSelectedSlug(created.slug);
      await openDirectory(created.slug, "");
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    } finally {
      setGeneratingSkill(false);
      setCreatingSkill(false);
    }
  }

  async function saveActiveFile() {
    if (!selectedSlug || !activeFile) return;
    setSavingFile(true);
    try {
      await saveSkillFile(selectedSlug, activeFile, editorContent);
      setContent(editorContent);
      await refreshDirectory(selectedSlug, filePath);
      if (activeFile === "SKILL.md") {
        await loadLocal();
      }
      dispatch(appActions.showToast("文件已保存"));
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    } finally {
      setSavingFile(false);
    }
  }

  async function createFileEntry(isDir: boolean) {
    if (!selectedSlug) return;
    const name = window.prompt(isDir ? "输入目录名" : "输入文件名");
    const trimmed = name?.trim();
    if (!trimmed) return;
    const path = filePath ? `${filePath}/${trimmed}` : trimmed;
    try {
      await createSkillFile(selectedSlug, path, "", isDir);
      await openDirectory(selectedSlug, filePath);
      dispatch(appActions.showToast(isDir ? "目录已创建" : "文件已创建"));
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    }
  }

  async function deleteActiveFile() {
    if (!selectedSlug || !activeFile) return;
    if (!window.confirm("确定删除这个文件或目录吗？")) return;
    try {
      await deleteSkillFile(selectedSlug, activeFile);
      setActiveFile("");
      setContent("");
      setEditorContent("");
      await openDirectory(selectedSlug, filePath);
      dispatch(appActions.showToast("文件已删除"));
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    }
  }

  async function install(slug: string) {
    setBusySlug(slug);
    try {
      await installSkill(slug);
      await loadLocal();
      dispatch(appActions.showToast("技能已安装"));
      setView("installed");
      const skill = results.find((item) => item.slug === slug) || { slug };
      await select(skill);
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    } finally {
      setBusySlug("");
    }
  }

  async function remove(slug: string) {
    if (!window.confirm("确定删除这个技能吗？")) return;
    setBusySlug(slug);
    try {
      await removeSkill(slug);
      if (selectedSlug === slug) {
        setSelectedSlug("");
        setFiles([]);
        setContent("");
        setEditorContent("");
      }
      await loadLocal();
      dispatch(appActions.showToast("技能已删除"));
    } catch (error) {
      dispatch(appActions.showToast(error instanceof Error ? error.message : String(error)));
    } finally {
      setBusySlug("");
    }
  }

  useEffect(() => {
    void loadLocal();
  }, []);

  return (
    <div className="chat-scroll h-full overflow-auto p-3 sm:p-6">
      <div className="mx-auto flex max-w-7xl flex-col gap-4">
        <Panel>
          <PanelHeader className="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
            <div className="min-w-0">
              <div className="flex items-center gap-3">
                <Sparkles className="h-5 w-5 text-primary" />
                <h2 className="text-xl font-semibold">技能</h2>
              </div>
              <div className="mt-1 text-sm text-muted-foreground">管理本地技能，搜索市场技能，并直接查看技能文件内容。</div>
            </div>
            <div className="grid w-full min-w-0 grid-cols-1 gap-2 sm:grid-cols-[minmax(0,1fr)_auto_auto_auto] xl:w-[660px]">
              <Input
                className="min-w-0"
                value={keyword}
                onChange={(event) => setKeyword(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") void search();
                }}
                placeholder="搜索技能市场"
              />
              <Button className="min-w-20 justify-center whitespace-nowrap" onClick={() => void search()} disabled={searching || !keyword.trim()}>
                <Search className="h-4 w-4" />
                搜索
              </Button>
              <Button className="min-w-20 justify-center whitespace-nowrap" variant="outline" onClick={() => void loadLocal()} disabled={loadingLocal}>
                <RefreshCcw className="h-4 w-4" />
                刷新
              </Button>
              <Button className="min-w-24 justify-center whitespace-nowrap" onClick={() => setCreateOpen(true)}>
                <Plus className="h-4 w-4" />
                新建技能
              </Button>
            </div>
          </PanelHeader>
          <PanelBody className="grid gap-3 border-t border-border/70 md:grid-cols-3">
            <MetricCard icon={PackageCheck} label="已安装" value={local.length} />
            <MetricCard icon={Search} label="搜索结果" value={results.length} />
            <MetricCard icon={Box} label="当前选择" value={selectedSkill ? 1 : 0} detail={selectedSkill?.name || selectedSkill?.slug || "未选择"} />
          </PanelBody>
        </Panel>

        <Panel>
          <PanelHeader className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
            <div className="flex gap-2">
              <ViewButton active={view === "installed"} onClick={() => setView("installed")} label="已安装" count={local.length} />
              <ViewButton active={view === "market"} onClick={() => setView("market")} label="技能市场" count={results.length} />
            </div>
            <div className="text-sm text-muted-foreground">
              {view === "installed" ? "点击技能卡片查看文件与说明" : "搜索后可直接安装，已安装技能会标记状态"}
            </div>
          </PanelHeader>
          <PanelBody>
            {view === "installed" ? (
              <SkillGrid
                emptyTitle={loadingLocal ? "正在加载本地技能" : "暂无本地技能"}
                emptyDescription="从技能市场搜索并安装后会显示在这里。"
                skills={local}
                selectedSlug={selectedSlug}
                installedSlugs={installedSlugs}
                busySlug={busySlug}
                onSelect={select}
                onInstall={install}
                onRemove={remove}
              />
            ) : (
              <SkillGrid
                emptyTitle={searching ? "正在搜索技能" : "暂无搜索结果"}
                emptyDescription="输入关键词后搜索技能市场。"
                skills={results}
                selectedSlug={selectedSlug}
                installedSlugs={installedSlugs}
                busySlug={busySlug}
                onSelect={select}
                onInstall={install}
                onRemove={remove}
              />
            )}
          </PanelBody>
        </Panel>

        <SkillDetail
          skill={selectedSkill}
          installed={Boolean(selectedSkill && installedSlugs.has(selectedSkill.slug))}
          files={files}
          filePath={filePath}
          activeFile={activeFile}
          content={content}
          editorContent={editorContent}
          busy={Boolean(selectedSkill && busySlug === selectedSkill.slug)}
          onInstall={(slug) => void install(slug)}
          onRemove={(slug) => void remove(slug)}
          onOpenDirectory={(path) => void openDirectory(selectedSkill?.slug, path)}
          onOpenFile={(path) => void openFile(path)}
          onEditorChange={setEditorContent}
          onSaveFile={() => void saveActiveFile()}
          onCreateFile={() => void createFileEntry(false)}
          onCreateDirectory={() => void createFileEntry(true)}
          onDeleteFile={() => void deleteActiveFile()}
          savingFile={savingFile}
        />
        <CreateSkillDialog
          open={createOpen}
          draft={skillDraft}
          aiInstruction={aiInstruction}
          creating={creatingSkill}
          generating={generatingSkill}
          onOpenChange={setCreateOpen}
          onDraftChange={setSkillDraft}
          onInstructionChange={setAiInstruction}
          onGenerate={() => void generateDraft()}
          onGenerateCreate={() => void generateAndCreateSkill()}
          onCreate={() => void createCustomSkill()}
        />
      </div>
    </div>
  );
}

function SkillGrid({
  skills,
  selectedSlug,
  installedSlugs,
  busySlug,
  emptyTitle,
  emptyDescription,
  onSelect,
  onInstall,
  onRemove,
}: {
  skills: SkillInfo[];
  selectedSlug: string;
  installedSlugs: Set<string>;
  busySlug: string;
  emptyTitle: string;
  emptyDescription: string;
  onSelect: (skill: SkillInfo) => Promise<void>;
  onInstall: (slug: string) => Promise<void>;
  onRemove: (slug: string) => Promise<void>;
}) {
  if (!skills.length) {
    return <EmptyState title={emptyTitle} description={emptyDescription} />;
  }

  return (
    <div className="grid gap-3 md:grid-cols-2 2xl:grid-cols-3">
      {skills.map((skill) => {
        const installed = installedSlugs.has(skill.slug);
        const selected = selectedSlug === skill.slug;
        return (
          <div
            key={skill.slug}
            role="button"
            tabIndex={0}
            className={cn(
              "group flex min-h-44 cursor-pointer flex-col rounded-xl border bg-card/65 p-4 text-left transition-[background,border-color,box-shadow] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
              selected ? "border-primary/60 bg-primary/5 shadow-[2px_3px_0_hsl(214_45%_30%/0.12)]" : "border-border/75 hover:bg-accent/45",
            )}
            onClick={() => void onSelect(skill)}
            onKeyDown={(event) => {
              if (event.key !== "Enter" && event.key !== " ") return;
              event.preventDefault();
              void onSelect(skill);
            }}
          >
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="truncate text-base font-semibold">{skill.name || skill.slug}</div>
                <div className="mt-1 truncate text-xs text-muted-foreground">{skill.slug}</div>
              </div>
              <div className="flex shrink-0 flex-wrap justify-end gap-1">
                {installed ? <Badge>已安装</Badge> : null}
                {skill.version ? <Badge>{skill.version}</Badge> : null}
              </div>
            </div>
            <div className="mt-3 line-clamp-3 flex-1 text-sm leading-6 text-muted-foreground">
              {skill.description_zh || skill.description || "暂无描述"}
            </div>
            <div className="mt-4 flex items-center justify-between gap-3">
              <SkillMeta skill={skill} />
              <div className="flex shrink-0 gap-1 opacity-100 md:opacity-0 md:transition-opacity md:group-hover:opacity-100">
                {installed ? (
                  <Button
                    className="whitespace-nowrap"
                    size="sm"
                    variant="ghost"
                    disabled={busySlug === skill.slug}
                    onClick={(event) => {
                      event.stopPropagation();
                      void onRemove(skill.slug);
                    }}
                  >
                    <Trash2 className="h-4 w-4" />
                    删除
                  </Button>
                ) : (
                  <Button
                    className="whitespace-nowrap"
                    size="sm"
                    disabled={busySlug === skill.slug}
                    onClick={(event) => {
                      event.stopPropagation();
                      void onInstall(skill.slug);
                    }}
                  >
                    <Download className="h-4 w-4" />
                    安装
                  </Button>
                )}
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
}

function SkillDetail({
  skill,
  installed,
  files,
  filePath,
  activeFile,
  content,
  editorContent,
  busy,
  savingFile,
  onInstall,
  onRemove,
  onOpenDirectory,
  onOpenFile,
  onEditorChange,
  onSaveFile,
  onCreateFile,
  onCreateDirectory,
  onDeleteFile,
}: {
  skill?: SkillInfo;
  installed: boolean;
  files: SkillFileEntry[];
  filePath: string;
  activeFile: string;
  content: string;
  editorContent: string;
  busy: boolean;
  savingFile: boolean;
  onInstall: (slug: string) => void;
  onRemove: (slug: string) => void;
  onOpenDirectory: (path: string) => void;
  onOpenFile: (path: string) => void;
  onEditorChange: (value: string) => void;
  onSaveFile: () => void;
  onCreateFile: () => void;
  onCreateDirectory: () => void;
  onDeleteFile: () => void;
}) {
  if (!skill) {
    return (
      <Panel>
        <PanelBody>
          <EmptyState title="选择一个技能" description="技能详情、文件列表和内容预览会在这里展示。" />
        </PanelBody>
      </Panel>
    );
  }

  return (
    <Panel>
      <PanelHeader className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-lg font-semibold">{skill.name || skill.slug}</h3>
            {installed ? <Badge>已安装</Badge> : <Badge>未安装</Badge>}
            {skill.version ? <Badge>{skill.version}</Badge> : null}
          </div>
          <div className="mt-1 text-sm text-muted-foreground">{skill.description_zh || skill.description || "暂无描述"}</div>
          <div className="mt-3 flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
            <span>{skill.slug}</span>
            {skill.owner ? <span>{skill.owner}</span> : null}
            {skill.homepage ? (
              <a className="inline-flex items-center gap-1 hover:text-foreground" href={skill.homepage} target="_blank" rel="noreferrer">
                主页
                <ExternalLink className="h-3.5 w-3.5" />
              </a>
            ) : null}
          </div>
        </div>
        <div className="flex shrink-0 gap-2">
          {installed ? (
            <Button className="min-w-20 whitespace-nowrap" variant="outline" disabled={busy} onClick={() => onRemove(skill.slug)}>
              <Trash2 className="h-4 w-4" />
              删除
            </Button>
          ) : (
            <Button className="min-w-20 whitespace-nowrap" disabled={busy} onClick={() => onInstall(skill.slug)}>
              <Download className="h-4 w-4" />
              安装
            </Button>
          )}
        </div>
      </PanelHeader>
      <PanelBody className="space-y-4 border-t border-border/70">
        {installed ? (
          <>
            <div className="space-y-2">
              <div className="flex flex-wrap items-center gap-2">
                <button
                  className={cn(
                    "rounded-full border px-3 py-1 text-sm",
                    filePath ? "border-border bg-card/70 hover:bg-accent/60" : "border-primary/50 bg-primary/10 text-primary",
                  )}
                  onClick={() => onOpenDirectory("")}
                >
                  根目录
                </button>
                {filePath ? <span className="text-sm text-muted-foreground">/ {filePath}</span> : null}
                <div className="ml-auto flex flex-wrap gap-2">
                  <Button size="sm" variant="outline" onClick={onCreateFile}>
                    <FilePlus className="h-4 w-4" />
                    文件
                  </Button>
                  <Button size="sm" variant="outline" onClick={onCreateDirectory}>
                    <FolderPlus className="h-4 w-4" />
                    目录
                  </Button>
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                {files.map((file) => {
                  const Icon = file.is_dir ? Folder : FileText;
                  return (
                    <button
                      key={file.path}
                      className={cn(
                        "inline-flex h-9 items-center gap-2 rounded-lg border px-3 text-sm transition-colors hover:bg-accent/60",
                        activeFile === file.path ? "border-primary/50 bg-primary/10 text-primary" : "border-border/75 bg-card/70",
                      )}
                      onClick={() => (file.is_dir ? onOpenDirectory(file.path) : onOpenFile(file.path))}
                    >
                      <Icon className="h-4 w-4" />
                      <span>{file.name}</span>
                      {file.size !== undefined && !file.is_dir ? <span className="text-xs text-muted-foreground">{formatSize(file.size)}</span> : null}
                    </button>
                  );
                })}
                {!files.length ? <div className="text-sm text-muted-foreground">暂无文件</div> : null}
              </div>
            </div>
            <div className="rounded-xl border border-border/75 bg-card/65">
              <div className="flex min-h-11 flex-wrap items-center justify-between gap-2 border-b border-border/70 px-4 py-2">
                <div className="truncate text-sm font-medium">{activeFile || "文件预览"}</div>
                <div className="flex flex-wrap items-center gap-2">
                  {content ? <Badge>{content.length} 字符</Badge> : null}
                  {activeFile ? (
                    <>
                      <Button size="sm" variant="outline" disabled={savingFile} onClick={onSaveFile}>
                        <Save className="h-4 w-4" />
                        保存
                      </Button>
                      {activeFile !== "SKILL.md" ? (
                        <Button size="sm" variant="ghost" onClick={onDeleteFile}>
                          <Trash2 className="h-4 w-4" />
                          删除
                        </Button>
                      ) : null}
                    </>
                  ) : null}
                </div>
              </div>
              {activeFile ? (
                <Textarea
                  className="chat-scroll min-h-80 rounded-none border-0 bg-transparent p-5 font-mono text-sm leading-7 shadow-none focus-visible:ring-0"
                  value={editorContent}
                  onChange={(event) => onEditorChange(event.target.value)}
                  spellCheck={false}
                />
              ) : (
                <pre className="chat-scroll max-h-[46vh] min-h-64 overflow-auto whitespace-pre-wrap p-5 text-sm leading-7">
                  选择文件查看内容
                </pre>
              )}
            </div>
          </>
        ) : (
          <EmptyState title="技能尚未安装" description="安装后可查看本地文件和技能说明。" />
        )}
      </PanelBody>
    </Panel>
  );
}

function CreateSkillDialog({
  open,
  draft,
  aiInstruction,
  creating,
  generating,
  onOpenChange,
  onDraftChange,
  onInstructionChange,
  onGenerate,
  onGenerateCreate,
  onCreate,
}: {
  open: boolean;
  draft: SkillCreateRequest;
  aiInstruction: string;
  creating: boolean;
  generating: boolean;
  onOpenChange: (open: boolean) => void;
  onDraftChange: (draft: SkillCreateRequest) => void;
  onInstructionChange: (value: string) => void;
  onGenerate: () => void;
  onGenerateCreate: () => void;
  onCreate: () => void;
}) {
  const busy = generating || creating;

  function update<K extends keyof SkillCreateRequest>(key: K, value: SkillCreateRequest[K]) {
    const next = { ...draft, [key]: value };
    if (key === "name" && (!draft.slug || draft.slug === slugifySkill(draft.name))) {
      next.slug = slugifySkill(String(value));
    }
    if ((key === "name" || key === "description") && draft.content === defaultSkillContent(draft.name, draft.description)) {
      next.content = defaultSkillContent(next.name, next.description);
    }
    onDraftChange(next);
  }

  return (
    <Dialog
      open={open}
      title="新建技能"
      closeDisabled={busy}
      overlay={busy ? <LoadingSurface label={generating && creating ? "正在生成并创建技能" : generating ? "正在生成技能草稿" : "正在创建技能"} /> : undefined}
      onOpenChange={(next) => {
        if (busy && !next) return;
        onOpenChange(next);
      }}
    >
      <div className="space-y-5">
        <div className="grid gap-3 rounded-lg border border-border/75 bg-card/60 p-3">
          <div className="text-sm font-medium">AI 创建</div>
          <Textarea
            className="min-h-24"
            disabled={busy}
            value={aiInstruction}
            onChange={(event) => onInstructionChange(event.target.value)}
            placeholder="描述你想创建的技能，例如：帮我创建一个用于代码评审的技能，关注安全、测试和可维护性。"
          />
          <div className="flex flex-wrap justify-end gap-2">
            <Button variant="outline" onClick={onGenerate} disabled={generating || !aiInstruction.trim()}>
              <Wand2 className="h-4 w-4" />
              {generating ? "生成中" : "生成草稿"}
            </Button>
            <Button onClick={onGenerateCreate} disabled={generating || creating || !aiInstruction.trim()}>
              <Wand2 className="h-4 w-4" />
              生成并创建
            </Button>
          </div>
        </div>

        <div className="grid gap-3 md:grid-cols-2">
          <label className="space-y-1 text-sm font-medium">
            <span>技能标识</span>
            <Input disabled={busy} value={draft.slug} onChange={(event) => update("slug", event.target.value)} placeholder="my_skill" />
          </label>
          <label className="space-y-1 text-sm font-medium">
            <span>技能名称</span>
            <Input disabled={busy} value={draft.name} onChange={(event) => update("name", event.target.value)} placeholder="我的技能" />
          </label>
        </div>
        <label className="space-y-1 text-sm font-medium">
          <span>描述</span>
          <Input disabled={busy} value={draft.description} onChange={(event) => update("description", event.target.value)} placeholder="一句话说明技能用途" />
        </label>
        <label className="space-y-1 text-sm font-medium">
          <span>SKILL.md</span>
          <Textarea
            className="min-h-80 font-mono text-sm"
            disabled={busy}
            value={draft.content}
            onChange={(event) => update("content", event.target.value)}
            spellCheck={false}
          />
        </label>
        <div className="flex justify-end gap-2">
          <Button variant="outline" disabled={busy} onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={onCreate} disabled={creating || !draft.slug.trim() || !draft.name.trim() || !draft.content.trim()}>
            <Plus className="h-4 w-4" />
            {creating ? "创建中" : "创建技能"}
          </Button>
        </div>
      </div>
    </Dialog>
  );
}

function ViewButton({ active, label, count, onClick }: { active: boolean; label: string; count: number; onClick: () => void }) {
  return (
    <button
      className={cn(
        "inline-flex h-10 items-center gap-2 rounded-lg border px-3 text-sm transition-colors",
        active ? "border-primary/50 bg-primary/10 text-primary" : "border-transparent text-muted-foreground hover:border-border hover:bg-card",
      )}
      onClick={onClick}
    >
      {label}
      <span className="rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground">{count}</span>
    </button>
  );
}

function MetricCard({ icon: Icon, label, value, detail }: { icon: typeof PackageCheck; label: string; value: number; detail?: string }) {
  return (
    <div className="rounded-xl border border-border/75 bg-card/65 p-4">
      <div className="flex items-center justify-between gap-3">
        <div className="text-sm text-muted-foreground">{label}</div>
        <Icon className="h-4 w-4 text-muted-foreground" />
      </div>
      <div className="mt-2 text-3xl font-semibold">{value}</div>
      {detail ? <div className="mt-1 truncate text-xs text-muted-foreground">{detail}</div> : null}
    </div>
  );
}

function SkillMeta({ skill }: { skill: SkillInfo }) {
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2 text-xs text-muted-foreground">
      {skill.stars !== undefined ? (
        <span className="inline-flex items-center gap-1">
          <Star className="h-3.5 w-3.5" />
          {skill.stars}
        </span>
      ) : null}
      {skill.downloads !== undefined ? (
        <span className="inline-flex items-center gap-1">
          <Download className="h-3.5 w-3.5" />
          {skill.downloads}
        </span>
      ) : null}
      {skill.owner ? <span className="truncate">{skill.owner}</span> : null}
    </div>
  );
}

function EmptyState({ title, description }: { title: string; description: string }) {
  return (
    <div className="rounded-xl border border-dashed border-border p-10 text-center">
      <div className="font-medium">{title}</div>
      <div className="mt-1 text-sm text-muted-foreground">{description}</div>
    </div>
  );
}

function formatSize(value: number) {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  return `${(value / 1024 / 1024).toFixed(1)} MB`;
}

function emptySkillDraft(): SkillCreateRequest {
  return {
    slug: "",
    name: "",
    description: "",
    content: defaultSkillContent("", ""),
  };
}

function defaultSkillContent(name: string, description: string) {
  const safeName = name.trim() || "我的技能";
  const safeDescription = description.trim() || "描述这个技能的用途。";
  return `---
name: ${yamlScalar(safeName)}
description: ${yamlScalar(safeDescription)}
---

# ${safeName}

${safeDescription}

## Use when

- Describe when this skill should be used.

## Instructions

- Add the reusable workflow, constraints, and examples here.
`;
}

function yamlScalar(value: string) {
  return JSON.stringify(value);
}

function slugifySkill(value: string) {
  const slug = value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9_-]+/g, "_")
    .replace(/^_+|_+$/g, "")
    .slice(0, 64);
  return slug || "";
}
