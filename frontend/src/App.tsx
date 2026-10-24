import SyncSettings, { useSyncClient, type SyncReport, type SyncMarker } from "./SyncSettings";
import { draftCache } from "./draft-cache";
import { localSettings } from "./local-settings";
import { sourceMatches, nextSourceMatch, type LocateSource } from "./source-location";
import AIProgress, { type GenerationProgress } from "./AIProgress";
import TextDiff, { docText } from "./TextDiff";
import LoreImportDialog from "./LoreImportDialog";
import ImageLibrary from "./ImageLibrary";
import { parseSceneMap, sceneMapSVG } from "./scene-map";
import AIRunHistory from "./AIRunHistory";
import AIContent from "./AIContent";
import ReviewResult from "./ReviewResult";
import { replaceReviewParagraph,replaceReviewParagraphs,parseReviewResult,reviewRetryInstruction,type ReviewSuggestion } from "./review-replacements";
import ComfySettings from "./ComfySettings";
import ComfyImagePanel from "./ComfyImagePanel";
import RevisionCandidate from "./RevisionCandidate";
import { revisionNotes, type RevisionNoteEntry } from "./revision-notes";
import { revisionTasks, revisionInstruction, parseRevisionReplacements, applyRevisionReplacements, revisionHasChange, canAcceptRevision, remainingRevisionTasks } from "./revision-ai";
import { closeHistory } from "@tiptap/pm/history";
import SceneMapPreview from "./SceneMapPreview";
import { parseCSV } from "./lore-import";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import type { Editor } from "@tiptap/react";
import {
  BookOpen,
  Feather,
  Layers,
  GitBranch,
  Users,
  Package,
  MapPin,
  Sparkles,
  Search,
  Settings,
  Plus,
  ChevronRight,
  ChevronDown,
  Check,
  CheckCircle2,
  AlertCircle,
  Loader2,
  Download,
  Upload,
  Library,
  ArrowLeft,
  X,
  FileText,
  History,
  Sun,
  Moon,
  PanelRightClose,
  PanelRightOpen,
  Trash2,
  ArrowUp,
  ArrowDown,
  Link,
  StopCircle,
  Eye,
  RefreshCw,
  ShieldCheck,
  Copy,
  Pencil,
  Images,
} from "lucide-react";
import WritingEditor, { type Selection } from "./Editor";
import {
  api,
  apiURL,
  apiHeaders,
  doc,
  uid,
  words,
  boundEntities,
  resolveVersion,
  blankVersion,
  type Project,
  type ProjectInfo,
  type Chapter,
  type Entity,
  type Version,
  type ContextPackage,
  type Config,
  type Detection,
  type Review,
  type SearchResult,
  type AIRun,
  type Doc,
  type ProjectImage,
} from "./types";

type Page =
  | "write"
  | "outline"
  | "detail"
  | "story"
  | "character"
  | "item"
  | "location"
  | "inspiration"
  | "settings"
  | "images";
type FieldSpec = {
  key: string;
  label: string;
  value?: string;
  area?: boolean;
  placeholder?: string;
};
type Dialog = {
  title: string;
  description?: string;
  fields: FieldSpec[];
  submit: (v: Record<string, string>) => void | Promise<void>;
  button?: string;
};
type Candidate = {
  revisionSignatures?:string[];
  retrying?: boolean;
  revisionTasks?: RevisionNoteEntry[];
  contextSignature: string;
  text: string;
  mode: string;
  chapterId: string;
  projectId: string;
  original: Doc;
  selection: Selection;
  id: string;
  model: string;
  complete: boolean;
};
const nav = [
  { id: "write", name: "正文写作", icon: Feather },
  { id: "outline", name: "大纲", icon: Layers },
  { id: "detail", name: "细纲", icon: FileText },
  { id: "story", name: "故事线", icon: GitBranch },
  { id: "character", name: "人物设定", icon: Users },
  { id: "item", name: "物品设定", icon: Package },
  { id: "location", name: "场景设定", icon: MapPin },
  { id: "inspiration", name: "灵感与素材", icon: Sparkles },
  { id: "images", name: "图片", icon: Images },
] as const;
const kindLabels = { character: "人物", item: "物品", location: "场景" };
const kindIcons = { character: Users, item: Package, location: MapPin };
const modeLabels: Record<string, string> = {
  continue: "续写",
  expand: "扩写",
  rewrite: "改写",
  revision: "按标记修改",
  review: "AI 审稿",
  ideas: "原创构思",
  map: "场景平面图（SVG）",
  comfyui: "场景绘制（ComfyUI）",
};
const date = (s: string) =>
  new Date(s).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
function Button({
  children,
  onClick,
  secondary = false,
  disabled = false,
  ...props
}: {
  children: ReactNode;
  onClick?: () => void;
  secondary?: boolean;
  disabled?: boolean;
  title?: string;
}) {
  return (
    <button
      className={"btn " + (secondary ? "secondary" : "")}
      disabled={disabled}
      onClick={onClick}
      {...props}
    >
      {children}
    </button>
  );
}
function Field({
  label,
  children,
  hint,
}: {
  label: string;
  children: ReactNode;
  hint?: string;
}) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
      {hint && <small>{hint}</small>}
    </label>
  );
}
function Empty({
  title,
  text,
  children,
}: {
  title: string;
  text: string;
  children?: ReactNode;
}) {
  return (
    <div className="empty">
      <div className="empty-icon">
        <Feather size={32} />
      </div>
      <h3>{title}</h3>
      <p>{text}</p>
      {children}
    </div>
  );
}
function Modal({
  dialog,
  close,
  notify,
}: {
  dialog: Dialog;
  close: () => void;
  notify: (s: string) => void;
}) {
  const [values, setValues] = useState(
    Object.fromEntries(dialog.fields.map((f) => [f.key, f.value || ""])),
  );
  const [busy, setBusy] = useState(false);
  return (
    <div
      className="modal-overlay"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget && !busy) close();
      }}
    >
      <form
        className="modal"
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          try {
            await dialog.submit(values);
            close();
          } catch (err) {
            notify((err as Error).message);
          } finally {
            setBusy(false);
          }
        }}
      >
        <button type="button" className="modal-close icon" onClick={close}>
          <X size={20} />
        </button>
        <div className="eyebrow">Yantai / 工作台</div>
        <h2>{dialog.title}</h2>
        {dialog.description && <p className="muted">{dialog.description}</p>}
        {dialog.fields.map((f) => (
          <Field key={f.key} label={f.label}>
            {f.area ? (
              <textarea
                autoFocus={f === dialog.fields[0]}
                rows={5}
                value={values[f.key]}
                placeholder={f.placeholder}
                onChange={(e) =>
                  setValues({ ...values, [f.key]: e.target.value })
                }
              />
            ) : (
              <input
                autoFocus={f === dialog.fields[0]}
                value={values[f.key]}
                placeholder={f.placeholder}
                onChange={(e) =>
                  setValues({ ...values, [f.key]: e.target.value })
                }
              />
            )}
          </Field>
        ))}
        <div className="modal-actions">
          <button type="button" className="btn secondary" onClick={close}>
            取消
          </button>
          <button className="btn" disabled={busy}>
            {busy ? (
              <Loader2 className="spin" size={16} />
            ) : (
              <Check size={16} />
            )}{" "}
            {dialog.button || "保存"}
          </button>
        </div>
      </form>
    </div>
  );
}

const characterFields = [
  ["gender", "性别"], ["birthplace", "出生地"], ["birthday", "出生日期"],
  ["ethnicity", "种族 / 民族"], ["appearance", "外貌特征"], ["family", "家庭背景"],
  ["identity", "身份 / 职业"], ["affiliation", "所属势力"], ["relationships", "人物关系"],
] as const;
export default function App() {
  const [books, setBooks] = useState<ProjectInfo[]>([]),
    [project, setProject] = useState<Project | null>(null),
    [shelf, setShelf] = useState(true),
    [page, setPage] = useState<Page>("write"),
    [chapterId, setChapterId] = useState(""),
    [storylineId, setStorylineId] = useState(""),
    [outlineVolumeId, setOutlineVolumeId] = useState("overview"),
    [overviewTab, setOverviewTab] = useState<"outline" | "rules" | "style">("outline"),
    [detailVolumeId, setDetailVolumeId] = useState(""),
    [editorEpoch, setEditorEpoch] = useState(0),
    [saveStatus, setSaveStatus] = useState("已保存"),
    [loading, setLoading] = useState(true),
    [toast, setToast] = useState(""),
    [dialog, setDialog] = useState<Dialog | null>(null),
    [focus, setFocus] = useState(false),
    [rightVisible, setRightVisible] = useState(true),
    [rightTab, setRightTab] = useState("lore"),
    [collapsed, setCollapsed] = useState<string[]>([]),
    [theme, setTheme] = useState(localSettings.getItem("theme") || "light"),
    [viewStage, setViewStage] = useState(""),
    [entityId, setEntityId] = useState(""),
    [config, setConfig] = useState<Config | null>(null),
    [models, setModels] = useState<string[]>([]),
    [testing, setTesting] = useState(false),
    [review, setReview] = useState<Review | null>(null),
    [history, setHistory] = useState<
      { revision: number; createdAt: string; name: string }[] | null
    >(null);
  const [detection, setDetection] = useState<Detection | null>(null),
    [detecting, setDetecting] = useState(false);
  const [instruction, setInstruction] = useState(""),
    [mapSceneId, setMapSceneId] = useState(""),
    [mode, setMode] = useState("continue"),
    [context, setContext] = useState<ContextPackage | null>(null),
    [contextOpen, setContextOpen] = useState(false),
    [candidate, setCandidate] = useState<Candidate | null>(null),
    [generating, setGenerating] = useState(false),
    [query, setQuery] = useState(""),
    [results, setResults] = useState<SearchResult[]>([]),
    [searching, setSearching] = useState(false);
  const [loreImport, setLoreImport] = useState<{rows: Record<string,string>[]; stageId:string} | null>(null);
  const [syncOpen,setSyncOpen]=useState(false);
  const [aiProgress,setAIProgress]=useState<GenerationProgress|null>(null);
  const [revisionExcluded, setRevisionExcluded] = useState<string[]>([]);
  const [, signalEditorReady] = useState(0);
  const [searchTerms, setSearchTerms] = useState<string[]>([]);
  const [styleAnalysis, setStyleAnalysis] = useState("");
  const [inspirationMode, setInspirationMode] = useState("keyword-local");
  const scope = inspirationMode.endsWith("-web") ? "web" : "local";
  const [modelDiscoveryStatus, setModelDiscoveryStatus] = useState("");
  const [modelCapacityStatus, setModelCapacityStatus] = useState("");
  useEffect(() => {
    if (page !== "settings" || !config?.baseUrl) return;
    let active = true;
    const timer = setTimeout(async () => {
      setModelDiscoveryStatus("正在自动读取模型列表……");
      setModels([]);
      try {
        const out = await api<{models: string[]}>("/models/discover", "POST", config);
        if (!active) return;
        setModels(out.models);
        setConfig((current) => current ? {...current, model: current.model || out.models[0] || ""} : current);
        setModelDiscoveryStatus(`已自动读取 ${out.models.length} 个模型`);
      } catch (error) { if (active) setModelDiscoveryStatus(`自动读取失败：${(error as Error).message}，可手动填写模型名。`); }
    }, 600);
    return () => {active = false; clearTimeout(timer);};
  }, [page, config?.provider, config?.baseUrl, config?.apiKey, config?.hasKey]);
  useEffect(() => {
    if (page !== "settings" || !config?.model) {setModelCapacityStatus("");return;}
    let active = true;
    const timer = setTimeout(async () => {
      setModelCapacityStatus("正在读取模型容量……");
      try {
        const out = await api<{contextLimit?: number; outputLimit?: number; source: string}>("/models/info", "POST", config);
        if (!active) return;
        setConfig((current) => current ? {...current,
          contextLimit: out.contextLimit && current.contextLimit === config.contextLimit ? out.contextLimit : current.contextLimit,
          outputLimit: out.outputLimit && current.outputLimit === config.outputLimit ? out.outputLimit : current.outputLimit,
        } : current);
        setModelCapacityStatus(out.source + (out.contextLimit ? ` · 上下文 ${out.contextLimit.toLocaleString()} token` : "") + (out.outputLimit ? ` · 输出 ${out.outputLimit.toLocaleString()} token` : ""));
      } catch (error) {if (active) setModelCapacityStatus(`容量读取失败：${(error as Error).message}，请手动填写。`);}
    }, 600);
    return () => {active = false; clearTimeout(timer);};
  }, [page, config?.provider, config?.baseUrl, config?.model, config?.apiKey, config?.hasKey]);
  const pRef = useRef<Project | null>(null),
    dirty = useRef(0),
    saved = useRef(0),
    saving = useRef<Promise<void> | null>(null),
    editorRef = useRef<Editor | null>(null),
    selectionRef = useRef<Selection>({ from: 0, to: 0, text: "" }),
    abortRef = useRef<AbortController | null>(null),
    timer = useRef<ReturnType<typeof setTimeout> | null>(null),
    importRef = useRef<HTMLInputElement>(null),
    sourceRef = useRef<HTMLInputElement>(null),
    loreRef = useRef<HTMLInputElement>(null),
    searchEpoch = useRef(0);
  const searchAbort = useRef<AbortController | null>(null);
  const chapter = project?.chapters.find((c) => c.id === chapterId),
    stage =
      project?.stages.find((s) => s.id === (viewStage || chapter?.stageId)) ||
      project?.stages[0],
    activeStageId = stage?.id || project?.stages[0]?.id || "";
  const activeChapterRef = useRef(chapterId);
  activeChapterRef.current = chapterId;
  const notify = useCallback((s: string) => {
    setToast(s);
  }, []);
  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(""), 6000);
    return () => clearTimeout(t);
  }, [toast]);
  const refreshBooks = useCallback(async () => {
    setBooks(await api<ProjectInfo[]>("/projects"));
  }, []);
  useEffect(() => {
    Promise.all([refreshBooks(), api<Config>("/settings").then(setConfig)])
      .then(() => {
        const w = window as unknown as {
          go?: { main?: { App?: { MarkReady?: () => Promise<void> } } };
        };
        return w.go?.main?.App?.MarkReady?.();
      })
      .catch((e) => notify(e.message))
      .finally(() => setLoading(false));
  }, [notify, refreshBooks]);
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    localSettings.setItem("theme", theme);
  }, [theme]);
  useEffect(() => {
    const w = window as unknown as {
      go?: { main?: { App?: { SetDirty?: (v: boolean) => Promise<void> } } };
    };
    w.go?.main?.App?.SetDirty?.(dirty.current !== saved.current).catch(
      () => {},
    );
  }, [saveStatus]);
  const setCurrent = useCallback((p: Project) => {
    searchAbort.current?.abort();
    ++searchEpoch.current;
    setSearching(false);
    setLoreImport(null);
    setOutlineVolumeId("overview");
    setDetailVolumeId(p.volumes[0]?.id || "");
    setStorylineId(localSettings.getItem("storyline:" + p.id) || p.storylines[0]?.id || "");
    setEditorEpoch((value) => value + 1);
    pRef.current = p;
    dirty.current = 0;
    saved.current = 0;
    setProject(p);
    setChapterId(
      p.chapters.some((c) => c.id === localSettings.getItem("chapter:" + p.id))
        ? localSettings.getItem("chapter:" + p.id)!
        : p.chapters[0]?.id || "",
    );
    setSaveStatus("已保存");
    setViewStage("");
    setEntityId("");
    setReview(null);
    setCandidate(null);
    setContext(null);
    setResults([]);
    setStyleAnalysis("");
    setShelf(false);
    setPage("write");
  }, []);
  const cacheWarnings = useRef(new Set<string>());
  const cacheDraft = useCallback((p: Project) => {
    void draftCache.write(p).then(ok => {
      if (!ok && !cacheWarnings.current.has(p.id)) {
        cacheWarnings.current.add(p.id);
        notify("草稿恢复缓存暂不可用，仍会保存到本地数据库，请确认顶部保存状态。");
      }
    });
  }, [notify]);
  const edit = useCallback(
    (fn: (p: Project) => void) => {
      if (!pRef.current) return;
      const p = structuredClone(pRef.current);
      fn(p);
      pRef.current = p;
      dirty.current++;
      setProject(p);
      setSaveStatus("待保存");
      cacheDraft(p);
    },
    [cacheDraft],
  );
  const flush: () => Promise<void> = useCallback(async () => {
    if (saving.current) {
      await saving.current;
      if (dirty.current !== saved.current) return flush();
      return;
    }
    if (!pRef.current || dirty.current === saved.current) return;
    const work = async () => {
      while (pRef.current && dirty.current !== saved.current) {
        const snapshot = structuredClone(pRef.current);
        const seq = dirty.current;
        setSaveStatus("保存中");
        const p = await api<Project>(
          `/projects/${snapshot.id}`,
          "PUT",
          snapshot,
        );
        if (pRef.current?.id !== snapshot.id) return;
        pRef.current = {
          ...pRef.current,
          revision: p.revision,
          updatedAt: p.updatedAt,
          images: pRef.current.images?.map(image => {
            const sent = snapshot.images?.find(i => i.id === image.id), stored = p.images?.find(i => i.id === image.id);
            return image.data && sent?.data === image.data && stored ? { ...image, data: undefined, sha: stored.sha, mime: stored.mime, size: stored.size } : image;
          }),
        };
        saved.current = seq;
        setProject(pRef.current);
        if (dirty.current === saved.current) {
          void draftCache.remove(p.id);
          setSaveStatus("已保存");
        } else {
          cacheDraft(pRef.current);
        }
      }
    };
    const promise = work();
    saving.current = promise;
    try {
      await promise;
    } catch (e) {
      setSaveStatus("保存失败");
      notify((e as Error).message);
      throw e;
    } finally {
      saving.current = null;
    }
  }, [notify, cacheDraft]);
  const prepareSync = useCallback(async ():Promise<SyncMarker> => {
    await flush();return {id:pRef.current?.id,seq:dirty.current};
  },[flush]);
  const completeSync = useCallback(async (report:SyncReport,marker:SyncMarker) => {
    await refreshBooks();
    const id=marker.id;
    if(!id||pRef.current?.id!==id||!report.items.some(item=>item.id===id&&item.action==="downloaded"))return;
    if(dirty.current!==saved.current||dirty.current!==marker.seq){notify("服务端版本已下载，本窗口的新编辑仍保留；请先导出草稿，再读取最新项目。");return;}
    const fresh=await api<Project>("/projects/"+id);
    if(pRef.current?.id!==id||dirty.current!==saved.current||dirty.current!==marker.seq)return;
    pRef.current=fresh;setProject(fresh);setEditorEpoch(value=>value+1);setSaveStatus("已保存");
    if(!fresh.chapters.some(c=>c.id===activeChapterRef.current))setChapterId(fresh.chapters[0]?.id||"");
  },[refreshBooks,notify]);
  const syncClient=useSyncClient(prepareSync,completeSync,notify);
  useEffect(() => {
    if (timer.current) clearTimeout(timer.current);
    if (project && dirty.current !== saved.current)
      timer.current = setTimeout(() => {
        flush().catch(() => {});
      }, 800);
    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
  }, [project, flush]);
  useEffect(() => {
    const close = (e: BeforeUnloadEvent) => {
      if (dirty.current !== saved.current) {
        e.preventDefault();
        e.returnValue = "";
      }
    };
    const key = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") {
        e.preventDefault();
        flush().catch(() => {});
      }
    };
    window.addEventListener("beforeunload", close);
    window.addEventListener("keydown", key);
    return () => {
      window.removeEventListener("beforeunload", close);
      window.removeEventListener("keydown", key);
    };
  }, [flush]);
  useEffect(() => {
    if (project && chapterId)
      localSettings.setItem("chapter:" + project.id, chapterId);
    searchAbort.current?.abort();
    ++searchEpoch.current;
    setSearching(false);
    abortRef.current?.abort();
    setGenerating(false);
    setAIProgress(null);
    setCandidate(null);
    setContext(null);
    setReview(null);
    setDetection(null);
    setMapSceneId("");
    setRevisionExcluded([]);
    selectionRef.current = { from: 0, to: 0, text: "" };
  }, [chapterId, project?.id]);
  const deleteLocalProject = (id: string, redownload = false) => {
    if (syncClient.remote || syncClient.busy) return;
    const book = books.find(b => b.id === id);
    setSyncOpen(false);
    setDialog({
      title: redownload ? "重新下载本机项目" : "删除本机项目",
      description: `只移除本机的“${book?.name || id}”，不会删除服务端项目。数据库、历史及未保存草稿会保留在本机备份目录。${redownload ? "确认后重新发起同步，从服务端下载该项目。" : "服务端存在同一项目时，下次同步会重新下载。"}`,
      fields: [], button: redownload ? "保留备份并重新下载" : "保留备份并删除本机副本",
      submit: async () => {
        if (pRef.current?.id === id) {
          if (timer.current) clearTimeout(timer.current);
          await saving.current?.catch(() => {});
        }
        const draft = pRef.current?.id === id ? structuredClone(pRef.current) : await draftCache.read(id);
        const out = await api<{id:string;backupPath:string}>(`/projects/${id}/local`, "DELETE", {draft});
        if (pRef.current?.id === id) {
          abortRef.current?.abort(); searchAbort.current?.abort();
          pRef.current = null; dirty.current = 0; saved.current = 0;
          setProject(null); setChapterId(""); setReview(null); setCandidate(null); setShelf(true); setSaveStatus("已保存");
        }
        syncClient.forgetProject(id);
        await draftCache.remove(id);
        await refreshBooks();
        notify("已移除本机副本，备份保留在：" + out.backupPath);
        if (redownload) { setSyncOpen(true); void syncClient.run(); }
      },
    });
  };
  const openProject = async (id: string) => {
    try {
      await flush();
      abortRef.current?.abort();
      const p = await api<Project>("/projects/" + id);
      const cache = await draftCache.read(id);
      setCurrent(p);
      if (cache) {
        const draft = cache;
        setDialog({
          title: "发现未保存的草稿",
          description:
            "上次退出时留下了恢复缓存。可将草稿恢复为新的保存版本，或取消后继续使用数据库中的内容。",
          fields: [],
          button: "恢复草稿",
          submit: () => {
            draft.revision = p.revision;
            draft.id = p.id;
            edit((current) => Object.assign(current, draft));
          },
        });
      }
    } catch (e) {
      notify((e as Error).message);
    }
  };
  const createBook = (demo = false) =>
    setDialog({
      title: demo ? "创建示例小说" : "新建小说项目",
      description: demo
        ? "示例内容用于体验阶段设定与写作界面，你可以随时修改。"
        : "每本小说拥有独立的正文、设定、故事线和历史记录。",
      fields: demo
        ? []
        : [
            {
              key: "name",
              label: "小说名称",
              placeholder: "为你的故事起一个名字",
            },
          ],
      submit: async (v) => {
        if (!demo && !v.name.trim()) throw Error("请填写小说名称");
        await flush();
        const p = await api<Project>("/projects", "POST", {
          name: v.name,
          demo,
        });
        setCurrent(p);
        await refreshBooks();
      },
    });
  const changeChapter = (id: string) => {
    setChapterId(id);
    setPage("write");
    setViewStage("");
    editorRef.current = null;
  };
  const patchChapter = (values: Partial<Chapter>, id = chapterId) =>
    edit((p) => {
      const c = p.chapters.find((c) => c.id === id);
      if (c) Object.assign(c, values);
    });
  const ready = useCallback((e: Editor) => {
    if(editorRef.current!==e){editorRef.current=e;signalEditorReady(n=>n+1);}
  }, []);
  const addChapter = (volumeId: string) =>
    setDialog({
      title: "新建章节",
      fields: [
        {
          key: "title",
          label: "章节名称",
          value: `第${(project?.chapters.length || 0) + 1}章`,
        },
      ],
      submit: (v) => {
        if (!v.title.trim()) throw Error("章节名称不能为空");
        const id = uid();
        edit((p) =>
          p.chapters.push({
            id,
            title: v.title.trim(),
            volumeId,
            stageId:
              p.stages.find((s) => s.volumeId === volumeId)?.id ||
              p.stages[0].id,
            outline: "",
            summary: "",
            content: doc(),
            text: "",
            entityIds: [],
            excludedIds: [],
          }),
        );
        changeChapter(id);
      },
    });
  const addVolume = () =>
    setDialog({
      title: "新建卷",
      fields: [
        {
          key: "name",
          label: "卷名",
          value: `第${(project?.volumes.length || 0) + 1}卷`,
        },
      ],
      submit: (v) => {
        if (!v.name.trim()) throw Error("卷名不能为空");
        edit((p) => {
          const id = uid();
          p.volumes.push({ id, name: v.name, outline: "", entityIds: [] });
          setOutlineVolumeId(id);
          setDetailVolumeId(id);
          p.stages.push({
            id: uid(),
            name: v.name + " · 初始阶段",
            volumeId: id,
            outline: "",
            entityIds: [],
          });
        });
      },
    });
  const addStage = (volumeId?: string) =>
    setDialog({
      title: "新建创作阶段",
      description:
        "阶段按创建顺序推进。历史版本可继承到后续阶段，未来版本不会出现在早期章节。",
      fields: [
        {
          key: "name",
          label: "阶段名称",
          placeholder: "例如：初入宗门、外出历练",
        },
      ],
      submit: (v) => {
        if (!v.name.trim()) throw Error("阶段名称不能为空");
        const id = uid();
        edit((p) =>
          p.stages.push({
            id,
            name: v.name,
            volumeId: volumeId || stage?.volumeId || chapter?.volumeId || p.volumes[0].id,
            outline: "",
            entityIds: [],
          }),
        );
        setViewStage(id);
      },
    });
  const renameChapter = (id = chapterId) => {
    const chapter = project?.chapters.find((c) => c.id === id);
    if (!chapter) return;
    setDialog({
      title: "编辑章节信息",
      fields: [
        { key: "title", label: "标题", value: chapter.title },
        { key: "outline", label: "章纲", value: chapter.outline, area: true },
        {
          key: "summary",
          label: "前情提要",
          value: chapter.summary,
          area: true,
        },
      ],
      submit: (v) => {
        if (!v.title.trim()) throw Error("标题不能为空");
        patchChapter(v, id);
      },
    });
  };
  const removeChapter = () => {
    if (!chapter) return;
    setDialog({
      title: "删除当前章节",
      description: `将删除“${chapter.title}”。保存历史可以恢复，但会同时恢复当时的其他项目内容。`,
      fields: [],
      button: "删除章节",
      submit: () => {
        const next =
          project?.chapters.find((c) => c.id !== chapterId)?.id || "";
        edit((p) => {
          p.chapters = p.chapters.filter((c) => c.id !== chapterId);
          for (const l of p.storylines)
            for (const b of l.beats)
              if (b.chapterId === chapterId) b.chapterId = "";
        });
        changeChapter(next);
      },
    });
  };
  const reorder = (id: string, delta: number) =>
    edit((p) => {
      const i = p.chapters.findIndex((c) => c.id === id),
        j = i + delta;
      if (
        j >= 0 &&
        j < p.chapters.length &&
        p.chapters[i].volumeId === p.chapters[j].volumeId
      )
        [p.chapters[i], p.chapters[j]] = [p.chapters[j], p.chapters[i]];
    });
  const goShelf = async () => {
    try {
      await flush();
      abortRef.current?.abort();
      setShelf(true);
      await refreshBooks();
    } catch {}
  };
  const exportProject = async (format: string) => {
    if (!project) return;
    try {
      await flush();
      const r = await fetch(
        apiURL(`/projects/${project.id}/export?format=${format}`),
        { headers: apiHeaders() },
      );
      if (!r.ok) throw Error("导出失败");
      const filename = format === "json"
        ? `${project.name}-备份-${new Date().toISOString().replace(/[:.]/g, "-")}.json`
        : project.name + "." + format;
      const completed = await download(await r.blob(), filename);
      if (completed && format === "json") notify("项目备份已导出，可在另一台电脑的Yantai中导入");
    } catch (e) {
      notify((e as Error).message);
    }
  };
  const importProject = async (file: File) => {
    try {
      await flush();
      const data = JSON.parse(await file.text());
      const p = await api<Project>("/import", "POST", data);
      setCurrent(p);
      await refreshBooks();
      notify("项目已导入为独立副本");
    } catch (e) {
      notify("导入失败：" + (e as Error).message);
    }
  };
  const importSource = async (file: File) => {
    try {
      const text = await file.text();
      if (text.length > 1_000_000) throw Error("单份素材请小于 100 万字符");
      setDialog({
        title: "导入参考素材",
        description: "资料仅加入素材库，不会修改正文或设定。",
        fields: [
          { key: "title", label: "素材名称", value: file.name },
          { key: "url", label: "来源网址（可选）" },
        ],
        submit: (v) =>
          edit((p) =>
            p.sources.push({
              id: uid(),
              title: v.title || file.name,
              text,
              url: v.url,
            }),
          ),
      });
    } catch (e) {
      notify((e as Error).message);
    }
  };
  const importLore = async (file: File) => {
    try {
      const text = await file.text();
      if (text.length > 1_000_000) throw Error("设定文件请小于 100 万字符");
      let rows: Record<string,string>[];
      if (file.name.toLowerCase().endsWith(".csv")) rows = parseCSV(text);
      else {
        const data = JSON.parse(text);
        if (!Array.isArray(data) || !data.length || data.length > 200) throw Error("设定 JSON 应为 1 至 200 条记录的数组");
        rows = data.map(e => Object.fromEntries(Object.entries({...e,...e.profile}).filter(([k])=>k!=="profile").map(([k,v])=>[k,String(v ?? "")])));
      }
      setLoreImport({rows,stageId:activeStageId});
    } catch (e) {
      notify("设定导入失败：" + (e as Error).message);
    }
  };
  const applyLoreImport = (rows:Record<string,string>[], skip:boolean) => {
    const target = loreImport?.stageId;
    if (!target || !pRef.current?.stages.some(s=>s.id===target)) throw Error("目标阶段已不存在，请重新选择阶段");
    let added=0;
    edit(p=>{for(const row of rows){
      if(skip && p.entities.some(e=>e.name===row.name && e.kind===row.kind)) continue;
      const id=uid(); const profile=Object.fromEntries(["gender","birthplace","birthday","appearance","identity","relationships"].filter(k=>row[k]).map(k=>[k,row[k]]));
      p.entities.push({id,name:row.name,kind:row.kind as Entity["kind"],base:row.base,profile,global:false,versions:[{stageId:target,traits:row.traits,likes:row.likes,state:row.state,knowledge:row.knowledge,note:row.note}]});
      p.stages.find(s=>s.id===target)!.entityIds.push(id);added++;
    }});
    setLoreImport(null);notify(`已导入 ${added} 条设定，跳过 ${rows.length-added} 条同名记录`);
  };
  const currentRevisionTasks = () => {
    if (!editorRef.current) throw Error("请先打开正文编辑器。");
    return revisionTasks(editorRef.current.state.doc, revisionExcluded);
  };
  const aiInput = (taskMode = mode, tasks?: RevisionNoteEntry[]) => ({
    chapterId,
    revision: pRef.current!.revision,
    mode: taskMode,
    instruction: taskMode === "revision"
      ? revisionInstruction(tasks || currentRevisionTasks()) + (instruction.trim() ? "\n补充要求：" + instruction : "")
      : (instruction || defaultInstruction(taskMode)) + (taskMode === "map" && mapSceneId ? "\n本次只绘制场景：" + (project?.entities.find(e => e.id === mapSceneId)?.name || "当前场景") : ""),
    selection: taskMode === "revision" ? "" : selectionRef.current.text,
  });
  const saveMapToImages = (svg: string, name: string, sourceRunId: string) => {
    const images = pRef.current?.images || [];
    if (images.some(i=>i.sourceRunId===sourceRunId)) { notify("这张地图已在图片栏中"); return; }
    const bytes = new TextEncoder().encode(svg);
    if (images.length >= 200 || images.reduce((n,i)=>n+i.size,0)+bytes.length>32*1024*1024) { notify("图片栏容量已满，地图仍保存在 AI 历史中，可单独导出"); return; }
    const image: ProjectImage = { id:uid(),name,kind:"map",note:"AI 生成的场景平面示意，请结合设定核对。",sourceRunId,size:bytes.length,createdAt:new Date().toISOString(),mime:"image/svg+xml",data:"data:image/svg+xml;base64,"+btoa(Array.from(bytes,b=>String.fromCharCode(b)).join("")) };
    edit(p=>{p.images=[...(p.images||[]),image];});
  };
  const previewContext = async () => {
    try {
      await flush();
      setContext(
        await api<ContextPackage>(
          `/projects/${project!.id}/context`,
          "POST",
          aiInput(),
        ),
      );
      setContextOpen(true);
    } catch (e) {
      notify((e as Error).message);
    }
  };
  const runAI = async (taskMode = mode,reviewAdvice?:string) => {
    if (!chapter || !project || generating) return;
    setMode(taskMode);
    try {
      await flush();
    } catch {
      return;
    }
    const p = pRef.current!;
    if (p.id !== project.id || activeChapterRef.current !== chapterId) return;
    const c = p.chapters.find((c) => c.id === chapterId);
    if (!c) return;
    let tasks: RevisionNoteEntry[] | undefined;
    try { if (taskMode === "revision") tasks = currentRevisionTasks(); }
    catch (e) { notify((e as Error).message); return; }
    const input = aiInput(taskMode, tasks);
    if(taskMode==="review"&&reviewAdvice)input.instruction+="\n请依据以下已有审稿意见生成可替换的完整段落，重新核对当前正文与设定，过期或无依据的意见不强行执行：\n"+reviewAdvice;
    const selection = { ...selectionRef.current };
    const initial: Candidate = {
      revisionSignatures:tasks?[contextSignature(p)]:undefined,
      revisionTasks: tasks,
      text: "",
      mode: taskMode,
      chapterId: c.id,
      projectId: p.id,
      original: structuredClone(c.content),
      contextSignature: contextSignature(p),
      selection,
      id: uid(),
      model: config?.model || "",
      complete: false,
    };
    setCandidate(initial);
    setRightTab("ai");
    setRightVisible(true);
    setGenerating(true);
    const startedAt=Date.now();
    setAIProgress({thinking:"",truncated:false,answerStarted:false,startedAt,attempt:0});
    const controller = new AbortController();
    abortRef.current = controller;
    let completeText = "";
    try {
      for (let attempt = 0; attempt < (tasks || taskMode === "review" ? 2 : 1); attempt++) {
      if(attempt>0)setAIProgress({thinking:"",truncated:false,answerStarted:false,startedAt,attempt});
      let retryNeeded = false;
      completeText = "";
      const r = await fetch(apiURL(`/projects/${p.id}/generate`), {
        method: "POST",
        headers: apiHeaders(true),
        body: JSON.stringify(input),
        signal: controller.signal,
      });
      if (!r.ok) throw Error((await r.json()).error);
      if (!r.body) throw Error("当前环境不支持流式响应");
      const reader = r.body.getReader(),
        decoder = new TextDecoder();
      let buffer = "",
        done = false;
      while (!done) {
        const read = await reader.read();
        if (read.done) break;
        buffer += decoder.decode(read.value, { stream: true });
        let end;
        while ((end = buffer.indexOf("\n\n")) >= 0) {
          const line = buffer.slice(0, end);
          buffer = buffer.slice(end + 2);
          if (!line.startsWith("data: ")) continue;
          const ev = JSON.parse(line.slice(6));
          if (ev.type === "context") setContext(ev.data);
          if (ev.type === "thinking" && typeof ev.data === "string") {
            setAIProgress(current=>{
              if(!current||controller.signal.aborted)return current;
              const thinking=current.thinking+ev.data;
              return {...current,thinking:thinking.slice(-32000),truncated:current.truncated||thinking.length>32000};
            });
          }
          if (ev.type === "delta") {
            setAIProgress(current=>current?{...current,answerStarted:true}:current);
            completeText += ev.data;
            setCandidate({ ...initial, text: completeText });
          }
          if (ev.type === "error") throw Error(ev.data);
          if (ev.type === "done") {
            done = true;
            completeText = ev.data.text;
            const reviewCorrection = taskMode === "review" && attempt === 0 ? reviewRetryInstruction(completeText) : "";
            if (reviewCorrection) {
              retryNeeded = true;
              initial.retrying = true;
              input.instruction += reviewCorrection;
              notify("AI 返回的建议未实际修改，正在按意见重新生成一次。");
              setCandidate({ ...initial, text: "" });
              break;
            }
            if (tasks && attempt === 0) {
              let unchanged: string[] = [];
              try { unchanged = parseRevisionReplacements(completeText, tasks).filter(item => !revisionHasChange(tasks!.find(t => t.id === item.id)!.quote, item.text)).map(item => item.id); } catch { /* Invalid schema is shown by the candidate view; it cannot be accepted. */ }
              if (unchanged.length) {
                retryNeeded = true;
                initial.retrying = true;
                input.instruction += "\n重试纠正：上一轮以下标记原样返回，未落实修改意见：" + unchanged.join("、") + "。这次必须逐条执行意见并返回有实际变化的改文，不能抄回原文。仍须返回全部请求ID的完整JSON结果。";
                setCandidate({ ...initial, text: "" });
                break;
              }
            }
            initial.id = ev.data.id;
            setAIProgress(null);
            initial.model = ev.data.model;
            completeText = ev.data.text;
            setCandidate({ ...initial, text: completeText, complete: true });
            if (pRef.current?.id === p.id)
              edit((current) =>
                current.aiRuns.push({
                  id: initial.id,
                  chapterId: c.id,
                  mode: taskMode,
                  model: initial.model,
                  text: completeText,
                  accepted: false,
                  createdAt: new Date().toISOString(),
                  inputText: c.text,
                }),
              );
            if (taskMode === "map" && pRef.current?.id === p.id) {
              try { const map = parseSceneMap(completeText); saveMapToImages(sceneMapSVG(map), map.title+".svg", initial.id); } catch { /* The invalid response remains in AI history for review. */ }
            }
            await flush();
            break;
          }
        }
      }
      if (!done) throw Error("生成连接中断，候选未完成，可复制已有内容。");
      if (retryNeeded) continue;
      break;
      }
    } catch (e) {
      if ((e as Error).name !== "AbortError") notify((e as Error).message);
      else notify("已停止生成，未修改正文");
    } finally {
      if (abortRef.current === controller) {
        setGenerating(false);
        setAIProgress(null);
        abortRef.current = null;
      }
    }
  };
  const acceptCandidate = () => {
    if (!candidate?.complete || !chapter || !editorRef.current) return;
    if (
      candidate.projectId !== project?.id ||
      candidate.chapterId !== chapterId ||
      (candidate.mode === "revision"
        ? !(candidate.revisionSignatures||[candidate.contextSignature]).includes(contextSignature(pRef.current!))
        : JSON.stringify(chapter.content) !== JSON.stringify(candidate.original) || contextSignature(pRef.current!) !== candidate.contextSignature)
    ) {
      notify("正文或上下文已变化，候选不能直接覆盖。请复制候选或重新生成。");
      return;
    }
    const editor = editorRef.current;
    let acceptedRevisionCount=0;
    if (candidate.mode === "revision") {
      try {
        const originalTasks = candidate.revisionTasks || [];
        const tasks = remainingRevisionTasks(editor.state.doc,originalTasks);
        const replacements = parseRevisionReplacements(candidate.text, originalTasks).filter(r=>tasks.some(t=>t.id===r.id));
        acceptedRevisionCount=replacements.filter(r=>revisionHasChange(tasks.find(t=>t.id===r.id)!.quote,r.text)).length;
        editor.view.dispatch(applyRevisionReplacements(editor.state, tasks, replacements));
        editor.view.dispatch(closeHistory(editor.state.tr));
        editor.view.focus();
      } catch (e) { notify((e as Error).message); return; }
    } else {
    const paras = doc(candidate.text).content!;
    if (candidate.mode === "continue")
      editor
        .chain()
        .focus()
        .insertContentAt(editor.state.doc.content.size, paras)
        .run();
    else if (candidate.selection.text)
      editor
        .chain()
        .focus()
        .insertContentAt(
          { from: candidate.selection.from, to: candidate.selection.to },
          paras,
        )
        .run();
    else editor.commands.setContent(doc(candidate.text));
    }
    edit((p) => {
      const r = p.aiRuns.find((r) => r.id === candidate.id);
      if (r) r.accepted = true;
    });
    setCandidate(null);
    notify(candidate.mode === "revision" ? `已采纳剩余 ${acceptedRevisionCount} 处实际修改；未改动的标记仍待修改，可一次撤销恢复` : "候选已采纳，可通过撤销或版本历史恢复");
  };
  const replaceOneRevision = (id:string) => {
    if(!candidate?.complete || generating || !editorRef.current || !pRef.current)return;
    const editor=editorRef.current;
    if(candidate.projectId!==pRef.current.id || candidate.chapterId!==chapterId || !(candidate.revisionSignatures||[candidate.contextSignature]).includes(contextSignature(pRef.current))){notify("正文或上下文已变化，请重新生成后替换。");return;}
    try{
      const originals=candidate.revisionTasks||[];
      const task=remainingRevisionTasks(editor.state.doc,originals).find(t=>t.id===id);
      if(!task)throw Error("这处标记已处理，无需再次替换。");
      const replacement=parseRevisionReplacements(candidate.text,originals).find(r=>r.id===id)!;
      editor.view.dispatch(applyRevisionReplacements(editor.state,[task],[replacement]));
      editor.view.dispatch(closeHistory(editor.state.tr));editor.view.focus();
      edit(p=>{const run=p.aiRuns.find(r=>r.id===candidate.id);if(run)run.accepted=true;});
      const signature=contextSignature(pRef.current!);
      setCandidate({...candidate,original:structuredClone(editor.getJSON()) as Doc,contextSignature:signature,revisionSignatures:Array.from(new Set([...(candidate.revisionSignatures||[candidate.contextSignature]),signature]))});
      notify("已替换这处原文，其余建议保留，可一次撤销恢复。");
    }catch(e){notify((e as Error).message);}
  };
  const locateSource: LocateSource = (text, hint) => {
    const editor = editorRef.current;
    if (!editor || editor.isDestroyed || activeChapterRef.current !== chapterId) { notify("请先打开对应章节正文。"); return; }
    const matches = sourceMatches(editor.state.doc, text, hint);
    if (!matches.length) { notify("当前正文中找不到这段原文，可能已修改或替换；未跳转到其他相似内容。"); return; }
    const index = nextSourceMatch(matches, editor.state.selection), target = matches[index];
    editor.chain().focus().setTextSelection({ from: target.to, to: target.from }).scrollIntoView().run();
    if (matches.length > 1) notify(`定位原文 ${index + 1}/${matches.length} 处，再次点击切换下一处。`);
  };
  const replaceOneReview = (suggestion:ReviewSuggestion,runId:string) => {
    if(generating || !editorRef.current || !pRef.current)return;
    const run=pRef.current.aiRuns.find(r=>r.id===runId);
    if(!run || run.chapterId!==chapterId){notify("审稿对应的章节已切换，请重新打开原章。");return;}
    try{
      const editor=editorRef.current;
      editor.view.dispatch(replaceReviewParagraph(editor.state,suggestion));
      editor.view.dispatch(closeHistory(editor.state.tr));editor.view.focus();
      edit(p=>{const record=p.aiRuns.find(r=>r.id===runId);if(record)record.accepted=true;});
      notify("已替换建议对应的原段落，可一次撤销恢复。");
    }catch(e){notify((e as Error).message);}
  };
  const replaceAllReview = (runId:string) => {
    if(generating||!editorRef.current||!pRef.current)return;
    const run=pRef.current.aiRuns.find(r=>r.id===runId);
    if(!run||run.chapterId!==chapterId||run.mode!=="review"){notify("审稿对应的章节已切换，请重新打开原章。");return;}
    try{
      const result=parseReviewResult(run.text);
      if(!result)throw Error("这份审稿还没有完整改文，请先按审稿意见生成修改段落。");
      const editor=editorRef.current;
      const batch=replaceReviewParagraphs(editor.state,result.suggestions);
      editor.view.dispatch(batch.transaction);
      editor.view.dispatch(closeHistory(editor.state.tr));editor.view.focus();
      edit(p=>{const record=p.aiRuns.find(r=>r.id===runId);if(record)record.accepted=true;});
      notify(`已一键替换 ${batch.replaced} 段${batch.skipped?`，跳过 ${batch.skipped} 条已处理、无效或冲突建议`:""}，可一次撤销整批修改。`);
    }catch(e){notify((e as Error).message);}
  };
  const appliedRevisionIds = () => editorRef.current&&candidate?.revisionTasks ? revisionNotes(editorRef.current.state.doc).filter(n=>n.resolved&&candidate.revisionTasks!.some(t=>t.id===n.id)).map(n=>n.id) : [];
  const runDetection = async () => {
    if (!config?.detectorUrl?.trim()) { setPage("settings"); notify("请填写独立来源检测服务的完整地址并保存；正文模型地址不能代替检测接口。"); return; }
    setDetecting(true);
    try {
      await flush();
      setDetection(
        await api<Detection>(
          "/projects/" + project!.id + "/detect",
          "POST",
          aiInput("detect"),
        ),
      );
    } catch (e) {
      notify((e as Error).message);
    } finally {
      setDetecting(false);
    }
  };
  const runReview = async () => {
    try {
      await flush();
      setReview(
        await api<Review>(
          `/projects/${project!.id}/review?chapter=${chapterId}`,
        ),
      );
      setRightTab("review");
      setRightVisible(true);
    } catch (e) {
      notify((e as Error).message);
    }
  };
  const search = async (aiAssisted = false) => {
    if (!query.trim() || !project) return;
    const epoch = ++searchEpoch.current;
    const id = project.id;
    searchAbort.current?.abort();
    const controller = new AbortController();
    searchAbort.current = controller;
    setSearching(true);
    setStyleAnalysis("");
    try {
      await flush();
      let r: SearchResult[];
      if (inspirationMode === "semantic") {
        setSearchTerms([]);
        r = await api<SearchResult[]>(`/projects/${id}/semantic-search`, "POST", {chapterId,revision:pRef.current!.revision,query}, controller.signal);
      } else if (aiAssisted) {
        if (!chapterId)
          throw Error("请先打开一个章节，AI 将结合当前细纲规划检索词");
        const planned = await api<{
          queries: string[];
          results: SearchResult[];
        }>(`/projects/${id}/ai-search`, "POST", {
          ...aiInput("search"),
          query,
          scope,
        }, controller.signal);
        r = planned.results;
        if (epoch === searchEpoch.current && id === pRef.current?.id)
          setSearchTerms(planned.queries);
      } else {
        setSearchTerms([]);
        r = await api<SearchResult[]>(
          `/projects/${id}/search?q=${encodeURIComponent(query)}&scope=${scope}`, "GET", undefined, controller.signal,
        );
      }
      if (epoch === searchEpoch.current && id === pRef.current?.id)
        setResults(r);
    } catch (e) {
      if (!controller.signal.aborted && epoch === searchEpoch.current) notify((e as Error).message);
    } finally {
      if (searchAbort.current === controller) searchAbort.current = null;
      if (epoch === searchEpoch.current) setSearching(false);
    }
  };
  const styleReference = async () => {
    if (!project || !chapter) { notify("请先打开一个章节，文风参考将结合当前章纲和设定"); return; }
    const demand = query.trim() || chapter.outline.trim() || chapter.title;
    const epoch = ++searchEpoch.current;
    const id = project.id;
    searchAbort.current?.abort();
    const controller = new AbortController();
    searchAbort.current = controller;
    setSearching(true);
    setStyleAnalysis("");
    setResults([]);
    try {
      await flush();
      const out = await api<{ queries: string[]; results: SearchResult[]; analysis: string }>(`/projects/${id}/style-reference`, "POST", {
        ...aiInput("search"), query: demand, scope: "web",
      }, controller.signal);
      if (epoch === searchEpoch.current && id === pRef.current?.id) {
        setSearchTerms(out.queries); setResults(out.results); setStyleAnalysis(out.analysis);
      }
    } catch (e) { if (!controller.signal.aborted && epoch === searchEpoch.current) notify((e as Error).message); }
    finally { if (searchAbort.current === controller) searchAbort.current = null; if (epoch === searchEpoch.current) setSearching(false); }
  };
  const runInspiration = () => inspirationMode === "style" ? styleReference() : search(inspirationMode.startsWith("ai-"));
  const showHistory = async () => {
    try {
      await flush();
      setHistory(await api(`/projects/${project!.id}/history`));
    } catch (e) {
      notify((e as Error).message);
    }
  };
  const toggleEntity = (id: string, selected: boolean) => {
    if (!chapter) return;
    patchChapter({
      entityIds: selected
        ? [...new Set([...chapter.entityIds, id])]
        : chapter.entityIds.filter((x) => x !== id),
      excludedIds: selected
        ? chapter.excludedIds.filter((x) => x !== id)
        : [...new Set([...chapter.excludedIds, id])],
    });
  };
  const sharedInput = (
    value: string,
    fn: (s: string) => void,
    placeholder?: string,
    rows = 6,
  ) => (
    <textarea
      value={value}
      rows={rows}
      placeholder={placeholder}
      onChange={(e) => fn(e.target.value)}
    />
  );

  const renderShelf = () => (
    <div className="shelf">
      <header className="shelf-header">
        <div className="brand">
          <span className="brand-mark">
            <Feather size={23} />
          </span>
          <div>
            Yantai <small>故事，慢慢写。</small>
          </div>
        </div>
        <div className="inline">
          <button
            className="icon"
            title="切换主题"
            onClick={() => setTheme(theme === "light" ? "dark" : "light")}
          >
            {theme === "light" ? <Moon size={19} /> : <Sun size={19} />}
          </button>
          <Button secondary onClick={()=>setSyncOpen(true)}>服务端同步</Button>
          <Button secondary onClick={() => importRef.current?.click()}>
            <Upload size={16} /> 导入项目
          </Button>
          <Button onClick={() => createBook()}>
            <Plus size={17} /> 新建小说
          </Button>
        </div>
      </header>
      <section className="shelf-intro">
        <div className="eyebrow">YOUR WRITING SPACE</div>
        <h1>每一个故事，都有自己的世界。</h1>
        <p>收好线索，安放人物，从今天的一页开始。</p>
        <div className="shelf-stats">
          <span>
            <b>{books.length}</b> 小说项目
          </span>
          <span>
            <b>{books.reduce((n, b) => n + b.words, 0).toLocaleString()}</b>{" "}
            累计字数
          </span>
          <span className="inline">
            <ShieldCheck size={15} /> 本地保存
          </span>
        </div>
      </section>
      <div className="section-heading">
        <h2>
          我的书架 <span>{books.length}</span>
        </h2>
        <button className="text-button" onClick={() => createBook(true)}>
          体验示例小说 <ChevronRight size={14} />
        </button>
      </div>
      <div className="book-grid">
        {books.map((b, i) => (
          <div className="book-entry" key={b.id}>
          <button
            className="book-card"
            onClick={() => b.error ? notify(b.error + "；请先备份数据，勿直接覆盖此项目。") : openProject(b.id)}
          >
            <div className={"book-cover cover-" + (i % 4)}>
              <div className="cover-frame">
                <span>{b.genre || "长篇小说"}</span>
                <h2>{b.name}</h2>
                <div className="cover-seal">Y</div>
                <small>一个尚在生长的世界</small>
              </div>
            </div>
            <div className="book-info">
              <h3>{b.name}</h3>
              <p>{b.error || b.description || "故事正在这里展开。"}</p>
              <div>
                <span>
                  {b.chapters} 章 · {b.words.toLocaleString()} 字
                </span>
                <span>{date(b.updatedAt)}</span>
              </div>
            </div>
          </button>
          {!syncClient.remote && <button className="text-button book-delete" disabled={syncClient.busy} onClick={() => deleteLocalProject(b.id)}><Trash2 size={13}/> 删除本机项目</button>}
          </div>
        ))}
        <button className="new-book-card" onClick={() => createBook()}>
          <div className="new-book-icon">
            <Plus size={26} />
          </div>
          <h3>写一个新的故事</h3>
          <p>新的世界，从一个想法开始。</p>
        </button>
      </div>
      {!books.length && !loading && (
        <div className="welcome-note">
          <BookOpen size={20} />
          <div>
            <b>欢迎来到Yantai</b>
            <p>新建你的小说，或打开示例，体验阶段设定、故事线与写作工作台。</p>
          </div>
        </div>
      )}
      <footer className="shelf-footer">
        Yantai · 本地小说写作工作台 <span>v0.5.1 / 每次落笔，都有迹可循</span>
      </footer>
    </div>
  );

  const renderOutline = () => {
    if (!project) return null;
    const detailed = page === "detail";
    const volume = project.volumes.find((v) => v.id === (detailed ? detailVolumeId : outlineVolumeId)) || (detailed ? project.volumes[0] : undefined);
    const stages = project.stages.filter((s) => s.volumeId === volume?.id);
    const selectedStage = stages.find((s) => s.id === viewStage) || stages[0];
    return <div className="management">
      <PageHeading eyebrow={detailed ? "STAGES & CHAPTER PLANS" : "STORY ARCHITECTURE"} title={detailed ? "把每一步，写得更清楚。" : "先看见故事，再写下故事。"} text={detailed ? "选择左侧卷纲，安排这一卷的细纲阶段与章节。" : "总览记录全书方向，卷纲安排每一卷的目标与转折。"} action={<Button onClick={detailed ? () => addStage(volume?.id) : addVolume}><Plus size={16} />{detailed ? "新建阶段" : "新建卷"}</Button>} />
      <div className="story-layout">
        <aside className="story-list" aria-label={detailed ? "细纲所属卷" : "大纲列表"}>
          <div className="eyebrow">{detailed ? "选择卷纲" : "大纲"}</div>
          {!detailed && <button aria-pressed={!volume} onClick={() => setOutlineVolumeId("overview")} className={!volume ? "selected" : ""}><BookOpen size={17} /><span className="story-list-label"><b>总览</b><small>全书大纲与世界规则</small></span></button>}
          {project.volumes.map((v) => <button key={v.id} aria-label={"选择" + (detailed ? "细纲卷" : "卷纲") + "：" + v.name} aria-pressed={volume?.id === v.id} className={volume?.id === v.id ? "selected" : ""} onClick={() => {if (detailed) {setDetailVolumeId(v.id);setViewStage("");} else setOutlineVolumeId(v.id);}}><Layers size={17} /><span className="story-list-label"><b>{v.name}</b><small>{detailed ? project.stages.filter((s) => s.volumeId === v.id).length + " 个细纲阶段" : project.chapters.filter((c) => c.volumeId === v.id).length + " 章"}</small></span><ChevronRight size={15} /></button>)}
        </aside>
        <div className="story-detail">
          {!detailed ? <div className={"card" + (!volume ? " overview-card" : "")}>
            <div className="card-heading"><Layers size={18} /><h3>{volume ? volume.name : "全书总览"}</h3><span className="tag">未来规划</span></div>
            {volume ? <>
              <Field label="卷名"><input value={volume.name} onChange={(e) => edit((p) => {p.volumes.find((v) => v.id === volume.id)!.name = e.target.value;})} /></Field>
              <Field label="本卷大纲">{sharedInput(volume.outline, (value) => edit((p) => {p.volumes.find((v) => v.id === volume.id)!.outline = value;}), "本卷的目标、转折与结尾方向", 12)}</Field>
              <Button secondary onClick={() => {setDetailVolumeId(volume.id);setViewStage("");setPage("detail");}}>查看本卷细纲<ChevronRight size={15} /></Button>
            </> : <>
              <div className="overview-tabs" role="tablist" aria-label="全书总览内容">
                {([{key:"outline",label:"全书总览"},{key:"rules",label:"世界规则"},{key:"style",label:"文风约定"}] as const).map(tab => <button key={tab.key} id={"overview-tab-"+tab.key} type="button" role="tab" aria-selected={overviewTab===tab.key} aria-controls="overview-panel" onClick={()=>setOverviewTab(tab.key)}>{tab.label}</button>)}
              </div>
              <div id="overview-panel" role="tabpanel" aria-labelledby={"overview-tab-"+overviewTab}>
                <textarea aria-label={({outline:"全书总览",rules:"世界规则",style:"文风约定"})[overviewTab]} rows={22} value={project[overviewTab]} placeholder={({outline:"主冲突、人物弧线、关键转折与结局方向",rules:"不能违背的世界设定与能力限制",style:"以动作和细节传递情绪"})[overviewTab]} onChange={e=>{const key=overviewTab,value=e.target.value;edit(p=>{p[key]=value;});}} />
              </div>
            </>}
          </div> : <>
            <div className="card"><div className="card-heading"><Layers size={18} /><h3>{volume?.name} · 卷纲</h3></div><p className="outline-preview">{volume?.outline || "还未填写卷纲，可先在大纲页安排本卷方向。"}</p><button className="text-button" onClick={() => {setOutlineVolumeId(volume?.id || "overview");setPage("outline");}}>编辑本卷大纲<ChevronRight size={14} /></button></div>
            {selectedStage ? <div className="card">
              <div className="card-heading"><GitBranch size={18} /><h3>阶段细纲</h3><span className="tag">{stages.length} 个阶段</span></div>
              <div className="stage-tabs">{stages.map((s) => <button key={s.id} className={s.id === selectedStage.id ? "selected" : ""} aria-pressed={s.id === selectedStage.id} onClick={() => setViewStage(s.id)}>{s.name}</button>)}</div>
              <Field label="阶段名称"><input value={selectedStage.name} onChange={(e) => edit((p) => {p.stages.find((s) => s.id === selectedStage.id)!.name = e.target.value;})} /></Field>
              <Field label="阶段目标与限制">{sharedInput(selectedStage.outline, (value) => edit((p) => {p.stages.find((s) => s.id === selectedStage.id)!.outline = value;}), "本阶段发生什么、不能发生什么", 8)}</Field>
              <div className="field-label">本阶段的资料集</div>
              <div className="binding-list">{project.entities.map((entity) => <label key={entity.id}><input type="checkbox" checked={selectedStage.entityIds.includes(entity.id)} onChange={(event) => edit((p) => {const s = p.stages.find((s) => s.id === selectedStage.id)!;s.entityIds = event.target.checked ? [...s.entityIds, entity.id] : s.entityIds.filter((id) => id !== entity.id);})} /><span>{entity.name}</span><small>{kindLabels[entity.kind]}</small></label>)}</div>
              <button className="text-button" onClick={() => {setViewStage(selectedStage.id);loreRef.current?.click();}}><Upload size={14} />导入 JSON / CSV 设定到此阶段</button>
              <h4>本阶段章节细纲</h4>
              {project.chapters.filter((c) => c.stageId === selectedStage.id).map((c) => <div className="chapter-plan" key={c.id}><b>{c.title}</b><Field label={c.title + " · 章纲"}>{sharedInput(c.outline, (value) => patchChapter({outline:value}, c.id), "具体场景、冲突、转折与章末承接", 4)}</Field><button className="text-button" onClick={() => changeChapter(c.id)}>打开正文<ChevronRight size={14} /></button></div>)}
              {!project.chapters.some((c) => c.stageId === selectedStage.id) && <p className="muted">此阶段尚未关联章节，可在正文页选择章节阶段。</p>}
            </div> : <Empty title="本卷还没有细纲阶段" text="点击新建阶段，安排本卷的剧情步骤。" />}
          </>}
        </div>
      </div>
    </div>;
  };

  const addLine = () =>
    setDialog({
      title: "新建故事线",
      fields: [
        {
          key: "name",
          label: "故事线名称",
          placeholder: "例如：身世之谜、师徒关系",
        },
      ],
      submit: (v) => {
        if (!v.name.trim()) throw Error("请输入名称");
        const id = uid();
        edit((p) =>
          p.storylines.push({
            id,
            name: v.name,
            color: "#28584b",
            beats: [],
          }),
        );
        setStorylineId(id);
        localSettings.setItem("storyline:" + project!.id, id);
      },
    });
  const addBeat = (lineId: string) =>
    setDialog({
      title: "添加故事线节点",
      fields: [
        { key: "title", label: "节点名称" },
        { key: "detail", label: "剧情安排与伏笔", area: true },
      ],
      submit: (v) => {
        if (!v.title.trim()) throw Error("请输入节点名称");
        edit((p) =>
          p.storylines
            .find((l) => l.id === lineId)!
            .beats.push({
              id: uid(),
              title: v.title,
              detail: v.detail,
              chapterId: chapterId || "",
              status: "planned",
            }),
        );
      },
    });
  const renderStory = () => {
    if (!project) return null;
    const selected = project.storylines.find((line) => line.id === storylineId) || project.storylines[0];
    return (
      <div className="management">
        <PageHeading
          eyebrow="THREADS & FORESHADOWING"
          title="让每条线索，都有回响。"
          text="把铺垫、推进与回收连在一起，跟随人物与故事的变化。"
          action={
            <Button onClick={addLine}>
              <Plus size={16} /> 新建故事线
            </Button>
          }
        />
        {!project.storylines.length ? (
          <Empty
            title="编织第一条故事线"
            text="主线、人物关系和悬念，可以在不同章节交汇。"
          />
        ) : (
          <div className="story-layout">
            <aside className="story-list" aria-label="故事线列表">
              <div className="eyebrow">故事线 · {project.storylines.length}</div>
              {project.storylines.map((line) => <button key={line.id} className={selected?.id === line.id ? "selected" : ""} aria-pressed={selected?.id === line.id} aria-label={`查看故事线：${line.name}`} onClick={() => {setStorylineId(line.id);localSettings.setItem("storyline:" + project.id, line.id);}}>
                <span className="line-color" style={{background: line.color}} />
                <span className="story-list-label"><b>{line.name || "未命名故事线"}</b><small>{line.beats.length} 个节点</small></span>
                <ChevronRight size={15} />
              </button>)}
              <button className="story-list-add" onClick={addLine}><Plus size={15} />新建故事线</button>
            </aside>
            <div className="story-detail" aria-label="当前故事线">
            {project.storylines.filter((line) => line.id === selected?.id).map((line) => (
              <section className="card story-card" key={line.id}>
                <div className="card-heading">
                  <span
                    className="line-color"
                    style={{ background: line.color }}
                  />
                  <input
                    className="inline-title"
                    aria-label="故事线名称"
                    value={line.name}
                    onChange={(e) =>
                      edit(
                        (p) =>
                          (p.storylines.find((l) => l.id === line.id)!.name =
                            e.target.value),
                      )
                    }
                  />
                  <span className="tag">{line.beats.length} 个节点</span>
                  <button
                    className="icon"
                    title="添加节点"
                    onClick={() => addBeat(line.id)}
                  >
                    <Plus size={17} />
                  </button>
                </div>
                <div className="timeline">
                  {!line.beats.length && <p className="muted">还没有节点，添加一个节点开始安排这条故事线。</p>}
                  {line.beats.map((b, i) => (
                    <div className="beat" key={b.id}>
                      <div className={"beat-dot " + b.status} />
                      <div className="beat-header">
                        <span className="muted">
                          {String(i + 1).padStart(2, "0")}
                        </span>
                        <input
                          aria-label="节点标题"
                          className="inline-title"
                          value={b.title}
                          onChange={(e) =>
                            edit(
                              (p) =>
                                (p.storylines
                                  .find((l) => l.id === line.id)!
                                  .beats.find((x) => x.id === b.id)!.title =
                                  e.target.value),
                            )
                          }
                        />
                        <button
                          className="icon"
                          title="移除节点"
                          onClick={() =>
                            edit((p) => {
                              const l = p.storylines.find(
                                (l) => l.id === line.id,
                              )!;
                              l.beats = l.beats.filter((x) => x.id !== b.id);
                            })
                          }
                        >
                          <X size={14} />
                        </button>
                      </div>
                      <textarea
                        aria-label="节点细节"
                        rows={3}
                        value={b.detail}
                        onChange={(e) =>
                          edit(
                            (p) =>
                              (p.storylines
                                .find((l) => l.id === line.id)!
                                .beats.find((x) => x.id === b.id)!.detail =
                                e.target.value),
                          )
                        }
                      />
                      <div className="beat-controls">
                        <select
                          aria-label="节点状态"
                          value={b.status}
                          onChange={(e) =>
                            edit(
                              (p) =>
                                (p.storylines
                                  .find((l) => l.id === line.id)!
                                  .beats.find((x) => x.id === b.id)!.status =
                                  e.target.value),
                            )
                          }
                        >
                          <option value="planned">待推进</option>
                          <option value="planted">已铺垫</option>
                          <option value="advanced">已推进</option>
                          <option value="resolved">已回收</option>
                          <option value="abandoned">已放弃</option>
                        </select>
                        <select
                          aria-label="关联章节"
                          value={b.chapterId}
                          onChange={(e) =>
                            edit(
                              (p) =>
                                (p.storylines
                                  .find((l) => l.id === line.id)!
                                  .beats.find((x) => x.id === b.id)!.chapterId =
                                  e.target.value),
                            )
                          }
                        >
                          <option value="">暂未关联章节</option>
                          {project.chapters.map((c) => (
                            <option key={c.id} value={c.id}>
                              {c.title}
                            </option>
                          ))}
                        </select>
                      </div>
                    </div>
                  ))}
                </div>
                <button
                  className="text-button"
                  onClick={() => addBeat(line.id)}
                >
                  <Plus size={15} /> 添加下一个节点
                </button>
              </section>
            ))}
            </div>
          </div>
        )}
      </div>
    );
  };

  const addEntity = (kind: Entity["kind"]) =>
    setDialog({
      title: `新建${kindLabels[kind]}设定`,
      fields: [
        { key: "name", label: "名称" },
        { key: "base", label: "基础档案", area: true },
      ],
      submit: (v) => {
        if (!v.name.trim()) throw Error("请输入名称");
        const id = uid();
        edit((p) => {
          p.entities.push({
            id,
            kind,
            name: v.name,
            base: v.base,
            global: false,
            versions: [blankVersion(activeStageId)],
          });
          p.stages.find((s) => s.id === activeStageId)?.entityIds.push(id);
        });
        setEntityId(id);
      },
    });
  const renderLore = () => {
    if (!project || !stage) return null;
    const kind = page as Entity["kind"];
    const list = project.entities.filter((e) => e.kind === kind),
      entity = list.find((e) => e.id === entityId) || list[0],
      v = entity ? resolveVersion(project, entity, activeStageId) : undefined,
      exact = v?.stageId === activeStageId;
    const versionChange = (key: keyof Version, value: string) =>
      edit((p) => {
        const e = p.entities.find((x) => x.id === entity!.id)!;
        let version = e.versions.find((v) => v.stageId === activeStageId);
        if (!version) {
          version = {
            ...(resolveVersion(p, e, activeStageId) ||
              blankVersion(activeStageId)),
            stageId: activeStageId,
          };
          e.versions.push(version);
        }
        version[key] = value;
      });
    const KindIcon = kindIcons[kind];
    return (
      <div className="management">
        <PageHeading
          eyebrow="WORLD & CHARACTERS"
          title={`${kindLabels[kind]}，也有自己的时间。`}
          text="基础档案保持身份，阶段版本记录变化。早期章节不会读取未来状态。"
          action={
            <Button onClick={() => addEntity(kind)}>
              <Plus size={16} /> 新建{kindLabels[kind]}
            </Button>
          }
        />
        <div className="lore-layout">
          <div className="entity-list">
            {list.map((e) => (
              <button
                className={entity?.id === e.id ? "selected" : ""}
                key={e.id}
                onClick={() => setEntityId(e.id)}
              >
                <span className="entity-avatar">
                  <KindIcon size={21} />
                </span>
                <div>
                  <b>{e.name}</b>
                  <small>{e.versions.length} 个阶段版本</small>
                </div>
                <ChevronRight size={15} />
              </button>
            ))}
            <button className="entity-add" onClick={() => addEntity(kind)}>
              <Plus size={16} /> 添加{kindLabels[kind]}
            </button>
          </div>
          {entity ? (
            <div className="entity-detail">
              <div className="card">
                <div className="entity-detail-heading">
                  <span className="entity-avatar large">
                    <KindIcon size={30} />
                  </span>
                  <div>
                    <div className="eyebrow">{kindLabels[kind]}档案</div>
                    <h2>{entity.name}</h2>
                  </div>
                  <button
                    className="icon push danger"
                    title="删除设定"
                    onClick={() =>
                      setDialog({
                        title: "删除设定",
                        description: `删除“${entity.name}”及所有阶段版本，并解除资料绑定。`,
                        fields: [],
                        button: "删除",
                        submit: () => {
                          edit((p) => {
                            p.entities = p.entities.filter(
                              (e) => e.id !== entity.id,
                            );
                            for (const v of p.volumes)
                              v.entityIds = v.entityIds.filter(
                                (id) => id !== entity.id,
                              );
                            for (const s of p.stages)
                              s.entityIds = s.entityIds.filter(
                                (id) => id !== entity.id,
                              );
                            for (const c of p.chapters) {
                              c.entityIds = c.entityIds.filter(
                                (id) => id !== entity.id,
                              );
                              c.excludedIds = c.excludedIds.filter(
                                (id) => id !== entity.id,
                              );
                            }
                          });
                          setEntityId("");
                        },
                      })
                    }
                  >
                    <Trash2 size={17} />
                  </button>
                </div>
                <Field label="名称">
                  <input
                    value={entity.name}
                    onChange={(e) =>
                      edit(
                        (p) =>
                          (p.entities.find((x) => x.id === entity.id)!.name =
                            e.target.value),
                      )
                    }
                  />
                </Field>
                <Field label="基础档案">
                  {sharedInput(
                    entity.base,
                    (v) =>
                      edit(
                        (p) =>
                          (p.entities.find((x) => x.id === entity.id)!.base =
                            v),
                      ),
                    "身份、来历、位置、固有限制等",
                    4,
                  )}
                </Field>
                {kind === "character" && <div className="profile-fields">
                  <h4>常用档案字段</h4>
                  <p className="muted">勾选需要记录的项目，再填写内容；取消勾选会移除该字段。</p>
                  <div className="profile-choices">{characterFields.map(([key, label]) =>
                    <label className="checkbox" key={key}><input type="checkbox" checked={Object.hasOwn(entity.profile || {}, key)} onChange={(event) => {
                      edit((p) => { const e = p.entities.find((x) => x.id === entity.id)!; e.profile ||= {}; if (event.target.checked) e.profile[key] = ""; else delete e.profile[key]; });
                    }} />{label}</label>
                  )}</div>
                  {characterFields.filter(([key]) => Object.hasOwn(entity.profile || {}, key)).map(([key, label]) =>
                    <Field label={label} key={key}><input value={entity.profile?.[key] || ""} placeholder={`填写${label}`} onChange={(event) => edit((p) => { const e = p.entities.find((x) => x.id === entity.id)!; e.profile ||= {}; e.profile[key] = event.target.value; })} /></Field>
                  )}
                </div>}
                <label className="checkbox">
                  <input
                    type="checkbox"
                    checked={entity.global}
                    onChange={(e) =>
                      edit(
                        (p) =>
                          (p.entities.find((x) => x.id === entity.id)!.global =
                            e.target.checked),
                      )
                    }
                  />{" "}
                  在所有章节的资料集常驻
                </label>
              </div>
              <div className="card">
                <div className="card-heading">
                  <History size={18} />
                  <h3>阶段版本</h3>
                  <span className={"tag " + (exact ? "green" : "gold")}>
                    {exact ? "本阶段版本" : v ? "继承历史版本" : "尚无有效版本"}
                  </span>
                </div>
                <Field label="查看或编辑阶段">
                  <select
                    value={activeStageId}
                    onChange={(e) => setViewStage(e.target.value)}
                  >
                    {project.stages.map((s) => (
                      <option key={s.id} value={s.id}>
                        {s.name}
                      </option>
                    ))}
                  </select>
                </Field>
                {!exact && (
                  <div className="info-note">
                    {v
                      ? "显示最近的历史版本。编辑任一字段会建立本阶段的新版本，保留之前的内容。"
                      : "当前阶段没有状态记录，编辑后会建立此阶段版本。"}
                  </div>
                )}
                <div className="two-col">
                  <Field
                    label={
                      kind === "character" ? "性格与行为特点" : "特征与限制"
                    }
                  >
                    {sharedInput(
                      v?.traits || "",
                      (s) => versionChange("traits", s),
                      "",
                      3,
                    )}
                  </Field>
                  <Field
                    label={kind === "character" ? "喜好与厌恶" : "使用条件"}
                  >
                    {sharedInput(
                      v?.likes || "",
                      (s) => versionChange("likes", s),
                      "",
                      3,
                    )}
                  </Field>
                </div>
                <Field label="当前状态">
                  {sharedInput(
                    v?.state || "",
                    (s) => versionChange("state", s),
                    "能力、伤势、所有者、地点或环境变化",
                    4,
                  )}
                </Field>
                <Field
                  label={
                    kind === "character" ? "人物已知的信息" : "已确认的信息"
                  }
                >
                  {sharedInput(
                    v?.knowledge || "",
                    (s) => versionChange("knowledge", s),
                    "避免将未来秘密当作已知事实",
                    3,
                  )}
                </Field>
                <Field label="变更依据与备注">
                  {sharedInput(
                    v?.note || "",
                    (s) => versionChange("note", s),
                    "来源章节、变化原因",
                    3,
                  )}
                </Field>
                <div className="version-trail">
                  {entity.versions.map((ver) => (
                    <button
                      className={ver.stageId === activeStageId ? "active" : ""}
                      key={ver.stageId}
                      onClick={() => setViewStage(ver.stageId)}
                    >
                      {project.stages.find((s) => s.id === ver.stageId)?.name}
                      <ChevronRight size={12} />
                    </button>
                  ))}
                </div>
              </div>
            </div>
          ) : (
            <Empty
              title={`建立你的${kindLabels[kind]}档案`}
              text="每份设定都能随故事阶段变化，并在写作时随手查阅。"
            />
          )}
        </div>
      </div>
    );
  };

  const renderSettings = () => {
    if (!config) return null;
    const set = (patch: Partial<Config>) => setConfig({ ...config, ...patch });
    return (
      <div className="management">
        <PageHeading
          eyebrow="YOUR TOOLS"
          title="让工具，适应你的写作。"
          text="连接本地模型或兼容服务。没有 AI，仍然可以完整地写作和管理设定。"
        />
        <div className="settings-grid">
          <div className="card">
            <div className="card-heading">
              <Sparkles size={18} />
              <h3>AI 模型连接</h3>
              <span className="tag">自带模型</span>
            </div>
            <Field label="服务类型">
              <select
                value={config.provider}
                onChange={(e) =>
                  set({
                    provider: e.target.value,
                    baseUrl:
                      e.target.value === "ollama"
                        ? "http://127.0.0.1:11434"
                        : "http://127.0.0.1:1234/v1",
                  })
                }
              >
                <option value="ollama">Ollama · 本地模型</option>
                <option value="compatible">
                  兼容接口 · LM Studio / 云端服务
                </option>
              </select>
            </Field>
            <Field label="服务地址">
              <input
                value={config.baseUrl}
                onChange={(e) => set({ baseUrl: e.target.value })}
              />
            </Field>
            <Field
              label="模型名称"
              hint="自动读取服务中的模型列表，也可手动填写或从下方列表选择。"
            >
              <input
                list="models"
                value={config.model}
                onChange={(e) => set({ model: e.target.value })}
                placeholder="例如：你的本地模型名称"
              />
              <datalist id="models">
                {models.map((m) => (
                  <option key={m} value={m} />
                ))}
              </datalist>
            </Field>
            <p className="muted" role="status">{modelDiscoveryStatus}</p>
            <p className="muted" role="status">{modelCapacityStatus}</p>
            <Field
              label="API 密钥"
              hint={
                config.hasKey
                  ? "已保存密钥，留空保留。取消下方勾选可清除。"
                  : "本地 Ollama 通常无需密钥。"
              }
            >
              <input
                type="password"
                autoComplete="off"
                value={config.apiKey}
                onChange={(e) => set({ apiKey: e.target.value })}
                placeholder={config.hasKey ? "••••••••（已保存）" : "可选"}
              />
            </Field>
            {config.hasKey && (
              <label className="checkbox">
                <input
                  type="checkbox"
                  checked={config.hasKey}
                  onChange={(e) => set({ hasKey: e.target.checked })}
                />{" "}
                保留已保存密钥
              </label>
            )}
            <div className="two-col">
              <Field label="模型上下文窗口" hint="输入与输出共用的总窗口，无固定最大值，可手动覆盖自动读取的容量。">
                <input
                  type="number"
                  min={1}
                  step={1}
                  value={config.contextLimit}
                  onChange={(e) =>
                    set({ contextLimit: Number(e.target.value) })
                  }
                />
              </Field>
              <Field label="输出 token 上限" hint="单次生成长度，无固定最大值，须小于上下文窗口。">
                <input
                  type="number"
                  min={1}
                  step={1}
                  value={config.outputLimit}
                  onChange={(e) => set({ outputLimit: Number(e.target.value) })}
                />
              </Field>
            </div>
            <Field
              label="AI 来源检测接口完整地址"
              hint="可选：接收 text、language；返回 score（0 至 1，可为空）、label、model、explanation、language。请使用支持中文的独立检测服务。"
            >
              <input
                value={config.detectorUrl || ""}
                onChange={(e) => set({ detectorUrl: e.target.value })}
                placeholder="http://127.0.0.1:端口/detect"
              />
            </Field>
            <Field label="检测服务密钥">
              <input
                type="password"
                value={config.detectorKey || ""}
                onChange={(e) => set({ detectorKey: e.target.value })}
                placeholder={
                  config.hasDetectorKey ? "已保存，留空保留" : "可选"
                }
              />
            </Field>
            <Field label="向量模型（语义检索，可选）"><input aria-label="向量模型" value={config.embeddingModel || ""} onChange={e=>set({embeddingModel:e.target.value})} placeholder="已安装的嵌入模型名称" list="models" /><small className="muted">使用同一个服务地址，模型需支持嵌入接口；不影响正文生成模型。</small></Field>
            <Field label={`温度 · ${config.temperature}`}>
              <input
                type="range"
                min="0"
                max="2"
                step="0.1"
                value={config.temperature}
                onChange={(e) => set({ temperature: Number(e.target.value) })}
              />
            </Field>
            <div className="inline">
              <Button
                onClick={async () => {
                  try {
                    setConfig(await api("/settings", "PUT", config));
                    notify("模型配置已保存");
                  } catch (e) {
                    notify((e as Error).message);
                  }
                }}
              >
                <Check size={16} /> 保存设置
              </Button>
              <Button
                secondary
                disabled={testing}
                onClick={async () => {
                  setTesting(true);
                  try {
                    setConfig(await api("/settings", "PUT", config));
                    const out = await api<{ models: string[] }>(
                      "/models/test",
                      "POST",
                      {},
                    );
                    setModels(out.models);
                    notify(`连接成功，发现 ${out.models.length} 个模型`);
                  } catch (e) {
                    notify((e as Error).message);
                  } finally {
                    setTesting(false);
                  }
                }}
              >
                {testing ? (
                  <Loader2 className="spin" size={16} />
                ) : (
                  <RefreshCw size={16} />
                )}{" "}
                测试连接
              </Button>
            </div>
            {!!models.length && (
              <div className="model-list">
                {models.map((m) => (
                  <button
                    key={m}
                    className="tag"
                    onClick={() => set({ model: m })}
                  >
                    {m}
                  </button>
                ))}
              </div>
            )}
          </div>
          <div>
            <SyncSettings client={syncClient} onDeleteLocal={id=>deleteLocalProject(id,true)} />
            <ComfySettings config={config} onChange={comfy=>set({comfy})} onSaved={setConfig} notify={notify} />
            <div className="card">
              <div className="card-heading">
                <Search size={18} />
                <h3>联网灵感检索</h3>
              </div>
              <p className="muted">
                默认使用免密钥 Bing 搜索，获取真实网页摘要和来源；填写 Brave Search 密钥后优先使用 Brave。文风参考使用当前配置的 AI 模型。
              </p>
              <Field label="Brave Search API 密钥">
                <input
                  type="password"
                  value={config.searchKey}
                  onChange={(e) => set({ searchKey: e.target.value })}
                  placeholder={
                    config.hasSearchKey ? "已保存，留空保留" : "可选"
                  }
                />
              </Field>
              <Button
                secondary
                onClick={async () => {
                  try {
                    setConfig(await api("/settings", "PUT", config));
                    notify("搜索配置已保存");
                  } catch (e) {
                    notify((e as Error).message);
                  }
                }}
              >
                保存搜索配置
              </Button>
            </div>
            <div className="card">
              <div className="card-heading">
                <ShieldCheck size={18} />
                <h3>保存与数据</h3>
              </div>
              {project && (
                <>
                  <Field label="小说名称">
                    <input
                      value={project.name}
                      onChange={(e) => edit((p) => (p.name = e.target.value))}
                    />
                  </Field>
                  <Field label="小说类型">
                    <input
                      value={project.genre}
                      onChange={(e) => edit((p) => (p.genre = e.target.value))}
                    />
                  </Field>
                  <Field label="故事简介">
                    <textarea
                      rows={3}
                      value={project.description}
                      onChange={(e) =>
                        edit((p) => (p.description = e.target.value))
                      }
                    />
                  </Field>
                </>
              )}
              <p>
                正文和设定存入各小说独立的 SQLite 数据库。自动保存保留最近 200
                次项目历史，历史面板显示最近 50 次。
              </p>
              <p className="muted">
                AI
                和搜索密钥仅保存在本机设置文件中，不随项目导出。当前设置文件未加密，备份或共享软件目录前请移除它。
              </p>
              <p className="muted">
                项目 JSON 是完整的可恢复快照；TXT 和 Markdown 用于文稿交换。
              </p>
              <div className="inline">
                <Button secondary onClick={()=>setSyncOpen(true)}>{syncClient.busy?"同步中……":"服务端同步"}</Button>
                <Button secondary onClick={() => exportProject("json")}>
                  <Download size={15} /> 备份项目
                </Button>
                <Button secondary onClick={showHistory}>
                  <History size={15} /> 保存历史
                </Button>
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  };

  const searchPanel = () => (
    <>
      <div className="search-form">
        <div className="search-input">
          <Search size={17} />
          <input
            aria-label="搜索片段"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !searching) runInspiration();
            }}
            placeholder="雨夜、重逢、关于那封信……"
          />
        </div>
        <div className="inline">
          <select aria-label="灵感检索方式" value={inspirationMode} disabled={searching} onChange={(e) => {
            setInspirationMode(e.target.value); setResults([]); setStyleAnalysis(""); setSearchTerms([]);
          }}>
            <option value="keyword-local">本地素材检索</option>
            <option value="semantic">本地语义检索 · 向量模型</option>
            <option value="keyword-web">联网关键词检索</option>
            <option value="ai-local">AI 帮我找片段 · 本地</option>
            <option value="ai-web">AI 帮我找片段 · 联网</option>
            <option value="style">文风描写参考 · 联网</option>
          </select>
          <Button disabled={searching || ((inspirationMode.startsWith("ai-") || inspirationMode === "style") && !chapter)} onClick={runInspiration}>
            {searching ? <Loader2 className="spin" size={15} /> : <Search size={15} />}
            {inspirationMode === "style" ? "生成参考" : "检索"}
          </Button>
        </div>
        {inspirationMode === "semantic" && <p className="muted">按含义检索当前章及之前的正文和导入素材。首次建立向量索引可能较慢；需要配置向量模型。</p>}
        {inspirationMode === "style" && <p className="muted">联网寻找相似桥段，由 AI 分析描写技巧并给出原创示例。未填写关键词时使用当前章纲。</p>}
        {inspirationMode.startsWith("ai-") && <p className="muted">AI 根据当前章规划检索词，再搜索{scope === "web" ? "网络" : "本书正文与导入素材"}中的真实片段。</p>}
        {searching && <Button secondary onClick={()=>{searchAbort.current?.abort();++searchEpoch.current;setSearching(false);}}>停止检索</Button>}
        {searching && <div className="info-note">{inspirationMode === "style" ? "正在联网检索并分析文风……" : "正在检索片段……"}</div>}
        {searchTerms.length > 0 && (
          <div className="info-note">
            AI 检索词：{searchTerms.join("、")}。下方片段来自实际检索。
          </div>
        )}
      </div>
      {styleAnalysis && <div className="style-reference-analysis"><h3>文风描写参考</h3><p className="muted">依据下方网页摘要分析；示例由模型原创，来源编号对应下方参考。</p><AIContent text={styleAnalysis} /></div>}
      {results.length > 0 ? (
        <div className="search-results">
          {results.map((r, i) => (
            <div className="search-result" key={i}>
              <div className="eyebrow">{r.source}{r.source.includes("语义相似度") && <span> · {(r.score / 1000).toFixed(3)}</span>}</div>
              <h4>{styleAnalysis ? `[${i + 1}] ` : ""}{r.title}</h4>
              <p>{r.text.replace(/<[^>]*>/g, "")}</p>
              <div className="inline">
                {r.chapterId && (
                  <button
                    className="text-button"
                    onClick={() => changeChapter(r.chapterId)}
                  >
                    打开章节 <ChevronRight size={13} />
                  </button>
                )}
                {r.url && safeURL(r.url) && (
                  <a
                    href={r.url}
                    target="_blank"
                    rel="noreferrer"
                    onClick={(e) => openExternal(e, r.url)}
                  >
                    查看来源 <Link size={13} />
                  </a>
                )}
                <button
                  className="text-button push"
                  onClick={() =>
                    edit((p) =>
                      p.sources.push({
                        id: uid(),
                        title: r.title,
                        text: r.text.replace(/<[^>]*>/g, ""),
                        url: r.url,
                      }),
                    )
                  }
                >
                  <Plus size={13} /> 收藏
                </button>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="small-empty">
          <Search size={26} />
          <p>
            输入一个场景、线索或关键词。
            <br />
            {inspirationMode === "semantic" ? "请先配置向量模型，按含义查找本书片段。" : inspirationMode === "keyword-local" || inspirationMode === "ai-local" ? "本地检索使用关键词与中文词组匹配。" : "联网检索返回真实网页摘要与来源。"}
          </p>
        </div>
      )}
    </>
  );
  const renderInspiration = () => {
    if (!project) return null;
    return (
      <div className="management">
        <PageHeading
          eyebrow="IDEAS & REFERENCES"
          title="停笔的时候，去找一点火光。"
          text="检索相关片段、收藏资料，参考相似桥段的文风与描写技巧。"
          action={
            <Button secondary onClick={() => sourceRef.current?.click()}>
              <Upload size={16} /> 导入 TXT / Markdown 素材
            </Button>
          }
        />
        <div className="inspiration-grid">
          <div className="card">
            <div className="card-heading">
              <Search size={18} />
              <h3>寻找相关片段</h3>
            </div>
            {searchPanel()}
          </div>
          <div>
            <div className="card">
              <div className="card-heading">
                <Library size={18} />
                <h3>我的素材库</h3>
                <span className="tag">{project.sources.length}</span>
              </div>
              {!project.sources.length ? (
                <p className="muted">导入写作资料，或将搜索结果收藏到这里。</p>
              ) : (
                project.sources.map((s) => (
                  <div className="source-item" key={s.id}>
                    <div>
                      <h4>{s.title}</h4>
                      <p>
                        {s.text.slice(0, 100)}
                        {s.text.length > 100 ? "…" : ""}
                      </p>
                    </div>
                    <button
                      className="icon"
                      title="删除素材"
                      onClick={() =>
                        edit(
                          (p) =>
                            (p.sources = p.sources.filter(
                              (x) => x.id !== s.id,
                            )),
                        )
                      }
                    >
                      <X size={14} />
                    </button>
                  </div>
                ))
              )}
            </div>
          </div>
        </div>
      </div>
    );
  };

  const lorePanel = () => {
    if (!project || !chapter) return null;
    const bound = boundEntities(project, chapter);
    return (
      <>
        <div className="panel-heading">
          <div className="eyebrow">CURRENT CONTEXT</div>
          <h3>本章的世界</h3>
          <p>
            阶段 · {project.stages.find((s) => s.id === chapter.stageId)?.name}
          </p>
        </div>
        <div className="context-summary">
          <span>
            <Layers size={14} /> 当前细纲
          </span>
          <p>
            {project.stages.find((s) => s.id === chapter.stageId)?.outline ||
              "还没有阶段细纲。可以在细纲页补充。"}
          </p>
          <button className="text-button" onClick={() => {setDetailVolumeId(chapter.volumeId);setViewStage(chapter.stageId);setPage("detail");}}>
            编辑细纲 <ChevronRight size={13} />
          </button>
        </div>
        {(["character", "item", "location"] as const).map((kind) => {
          const Icon = kindIcons[kind];
          return (
            <section className="reference-group" key={kind}>
              <div className="reference-heading">
                <Icon size={15} />
                <b>{kindLabels[kind]}</b>
                <span>{bound.filter((e) => e.kind === kind).length}</span>
                <button
                  className="icon push"
                  title={`管理${kindLabels[kind]}`}
                  onClick={() => setPage(kind)}
                >
                  <Plus size={14} />
                </button>
              </div>
              {bound
                .filter((e) => e.kind === kind)
                .map((e) => {
                  const v = resolveVersion(project, e, chapter.stageId);
                  return (
                    <details className="reference-card" key={e.id}>
                      <summary>
                        <span className={"avatar-dot " + kind}>
                          <Icon size={15} />
                        </span>
                        <div>
                          <b>{e.name}</b>
                          <small>
                            {v
                              ? v.stageId === chapter.stageId
                                ? "当前阶段"
                                : "继承历史版本"
                              : "状态待补充"}
                          </small>
                        </div>
                        <ChevronDown size={13} />
                      </summary>
                      <div className="reference-body">
                        <p>{e.base}</p>
                        {characterFields.filter(([key]) => e.profile?.[key]).map(([key,label]) => <p key={key}>{label}：{e.profile![key]}</p>)}
                        {v && (
                          <>
                            <p>
                              <b>{kind === "character" ? "性格" : "特征"}：</b>
                              {v.traits || "未设定"}
                            </p>
                            {v.likes && (
                              <p>
                                <b>喜好 / 条件：</b>
                                {v.likes}
                              </p>
                            )}
                            <p>
                              <b>状态：</b>
                              {v.state || "未知"}
                            </p>
                            {v.knowledge && (
                              <p>
                                <b>已知：</b>
                                {v.knowledge}
                              </p>
                            )}
                          </>
                        )}
                        <button
                          className="text-button"
                          onClick={() => {
                            setPage(e.kind);
                            setEntityId(e.id);
                            setViewStage(chapter.stageId);
                          }}
                        >
                          编辑档案 <ChevronRight size={13} />
                        </button>
                      </div>
                    </details>
                  );
                })}
            </section>
          );
        })}
        <details className="binding-picker">
          <summary>
            <Plus size={14} /> 调整本章资料与 AI 上下文
          </summary>
          <p className="muted">勾选的卡片将加入本章 AI 上下文。</p>
          {project.entities.map((e) => (
            <label className="checkbox" key={e.id}>
              <input
                type="checkbox"
                checked={bound.some((b) => b.id === e.id)}
                onChange={(ev) => toggleEntity(e.id, ev.target.checked)}
              />
              {e.name}
              <small>{kindLabels[e.kind]}</small>
            </label>
          ))}
        </details>
        <div className="panel-note">
          <ShieldCheck size={14} />
          <span>按当前阶段读取设定，未来版本不会提前进入正文上下文。</span>
        </div>
      </>
    );
  };
  const aiPanel = () => (
    <>
      <div className="panel-heading">
        <div className="eyebrow">WRITING COMPANION</div>
        <h3>一起，把这一段写好。</h3>
        <p>
          {mode === "comfyui" ? (config?.comfy?.baseUrl || "尚未配置 ComfyUI") : (config?.model || "尚未配置模型")}{" "}
          <button className="text-button" onClick={() => setPage("settings")}>
            配置
          </button>
        </p>
      </div>
      <Field label="写作任务">
        <select value={mode} onChange={(e) => setMode(e.target.value)}>
          <option value="continue">续写 · 追加到正文</option>
          <option value="expand">扩写 · 选区或整章</option>
          <option value="rewrite">改写 · 选区或整章</option>
          <option value="revision">按标记修改 · 依据修改意见</option>
          <option value="ideas">原创构思 · 不改正文</option>
          <option value="review">AI 审稿 · 不改正文</option>
          <option value="map">场景平面图（SVG）</option>
          <option value="comfyui">场景绘制（ComfyUI）</option>
        </select>
      </Field>
      {mode === "comfyui" && project && chapter && <ComfyImagePanel key={project.id+chapter.id} config={config} project={project} chapter={chapter} onSettings={()=>setPage("settings")} onPrepare={async()=>{
        await flush();
        if(pRef.current?.id!==project.id || activeChapterRef.current!==chapter.id)throw Error("项目或章节已切换，请在当前章节重新生成");
        return {revision:pRef.current.revision,signature:contextSignature(pRef.current)};
      }} onCheck={signature=>{
        if(pRef.current?.id!==project.id || activeChapterRef.current!==chapter.id || contextSignature(pRef.current)!==signature)throw Error("AI 分析期间正文或设定已变化，请重新生成，尚未提交 ComfyUI");
      }} onImages={async images=>{
        if(pRef.current?.id!==project.id)throw Error("项目已切换，请在原项目保存生成图片");
        const existing=pRef.current.images||[];
        const pending=images.filter(im=>!existing.some(old=>old.sourceRunId===im.sourceRunId));
        if(existing.length+pending.length>200 || existing.reduce((n,i)=>n+i.size,0)+pending.reduce((n,i)=>n+i.size,0)>32*1024*1024)throw Error("项目图片栏容量已满，请清理后重试保存。生成预览暂时保留在此处。");
        if(pending.length)edit(p=>{p.images=[...(p.images||[]),...pending];});
        await flush();
      }} />}
      <div hidden={mode === "comfyui"}>
      {mode === "revision" && <section className="ai-revision-picker">
        <h4>本章待修改标记</h4>
        <p className="muted">勾选本次要修改的位置，并在修改标记面板填写意见。生成后逐处对比，采纳时只替换勾选范围并标为已处理。</p>
        {editorRef.current && revisionNotes(editorRef.current.state.doc).filter(n => !n.resolved).map((n, i) => <label key={n.id} className="ai-revision-choice">
          <input type="checkbox" disabled={generating} checked={!revisionExcluded.includes(n.id)} onChange={e => setRevisionExcluded(ids => e.target.checked ? ids.filter(id => id !== n.id) : [...ids, n.id])} />
          <span><b>标记 {i + 1}</b><button type="button" className="ai-revision-quote source-quote" title="定位原文" onClick={e=>{e.preventDefault();locateSource(n.quote,{markId:n.id});}}>{n.quote}</button><small>{n.note || "尚未填写修改意见，请到正文的修改标记面板补充"}</small></span>
        </label>)}
        {(!editorRef.current || !revisionNotes(editorRef.current.state.doc).some(n => !n.resolved)) && <p className="info-note">暂无待修改标记。选中正文，点击工具栏“修改标记”，填写意见后即可使用。</p>}
      </section>}
      {mode === "map" && <Field label="绘制场景"><select aria-label="绘制场景" value={mapSceneId} onChange={e=>setMapSceneId(e.target.value)}><option value="">综合本章有效场景</option>{project && chapter && boundEntities(project,chapter).filter(e=>e.kind==="location").map(e=><option key={e.id} value={e.id}>{e.name}</option>)}</select><small className="muted">用当前文字模型绘制房间、地点及道路连接，可在下方说明中指定布局。地图单独保存，不修改正文。</small></Field>}
      <Field label="这次想怎么写？">
        <textarea
          rows={4}
          value={instruction}
          onChange={(e) => setInstruction(e.target.value)}
          placeholder="例如：让对白更含蓄，保留人物的戒心，不揭露信件的秘密。"
        />
      </Field>
      <div className="inline">
        <Button disabled={generating || !chapter} onClick={() => runAI()}>
          <Sparkles size={15} /> 开始{modeLabels[mode]}
        </Button>
        {generating ? (
          <Button secondary onClick={() => abortRef.current?.abort()}>
            <StopCircle size={15} /> 停止
          </Button>
        ) : (
          <button
            className="text-button"
            disabled={!chapter}
            onClick={previewContext}
          >
            <Eye size={14} /> 查看上下文
          </button>
        )}
      </div>
      {context && (
        <div className={"context-budget " + (!context.fits ? "over" : "")}>
          <Layers size={14} />
          <span>
            估计 {context.estimatedTokens.toLocaleString()} /{" "}
            {context.budget.toLocaleString()} token{" "}
            <small>保守估算，含提示词余量</small>
          </span>
          <button
            className="icon"
            title="查看上下文"
            onClick={() => setContextOpen(true)}
          >
            <ChevronRight size={15} />
          </button>
        </div>
      )}
      {generating&&aiProgress&&<AIProgress progress={aiProgress}/>}
      {candidate && (
        <div className="candidate">
          <div className="candidate-heading">
            <Sparkles size={15} />
            <b>{modeLabels[candidate.mode]}候选</b>
            <span className="tag">
              {candidate.complete ? "已完成" : generating ? "生成中" : "未完成"}
            </span>
          </div>
          {["expand", "rewrite"].includes(candidate.mode) && candidate.text && <TextDiff before={candidate.selection.text || docText(candidate.original)} after={candidate.text} onLocate={locateSource} />}
          {candidate.mode === "revision" && <div className="candidate-text"><RevisionCandidate text={candidate.text} tasks={candidate.revisionTasks || []} complete={candidate.complete} retrying={candidate.retrying} appliedIds={appliedRevisionIds()} onReplace={replaceOneRevision} onLocate={locateSource}/></div>}
          {candidate.mode === "review" && <div className="candidate-text">{candidate.complete?<ReviewResult onReplaceAll={()=>replaceAllReview(candidate.id)} onLocate={locateSource} text={candidate.text} document={editorRef.current?.state.doc} busy={generating} onReplace={suggestion=>replaceOneReview(suggestion,candidate.id)} onGenerate={()=>runAI("review",candidate.text)}/>:<p className="muted">正在审稿并生成可替换的修改段落……</p>}</div>}
          {candidate.mode === "map" && candidate.complete && <SceneMapPreview text={candidate.text} onSave={(svg,name)=>saveMapToImages(svg,name,candidate.id)} onDownload={(svg,name)=>{download(new Blob([svg],{type:"image/svg+xml;charset=utf-8"}),name).catch(e=>notify(e.message));}} />}
          <div className="candidate-text" hidden={candidate.mode === "revision" || candidate.mode === "review" || (candidate.mode === "map" && candidate.complete)}>
            {candidate.text ? <AIContent text={candidate.text} onLocate={locateSource} sourceText={chapter?.text} /> : <span className="muted">等待模型响应……</span>}
          </div>
          <div className="candidate-actions">
            {!["review", "ideas", "map"].includes(candidate.mode) && (
              <Button
                disabled={!candidate.complete || generating || (candidate.mode === "revision" && !canAcceptRevision(candidate.text, candidate.revisionTasks || [],appliedRevisionIds()))}
                onClick={acceptCandidate}
              >
                <Check size={15} /> {candidate.mode === "revision" ? "采纳实际修改" : "采纳候选"}
              </Button>
            )}
            <Button
              secondary
              disabled={!candidate.text}
              onClick={() =>
                navigator.clipboard
                  .writeText(candidate.text)
                  .then(() => notify("候选已复制"))
                  .catch(() => notify("剪贴板不可用，请手动选中复制"))
              }
            >
              <Copy size={14} /> 复制
            </Button>
          </div>
          <small>{["review","map"].includes(candidate.mode) ? "完成的结果自动保存到项目，可在下方历史列表或审稿栏重新查看。" : "候选不会自动改正文。采纳前会检查正文是否已变化。"}</small>
        </div>
      )}
      {project && chapter && project.aiRuns.some(r=>r.chapterId===chapterId && ["review","map"].includes(r.mode)) && <details className="review-section" open={!candidate}><summary>本章审稿与地图历史 · 本地保存</summary><AIRunHistory onReplaceAll={replaceAllReview} onLocate={locateSource} key={project.id+chapter.id} runs={project.aiRuns} chapterId={chapterId} text={chapter.text} document={editorRef.current?.state.doc} busy={generating} onReplace={replaceOneReview} onGenerate={run=>runAI("review",run.text)} onSave={saveMapToImages} onDownload={(svg,name)=>{download(new Blob([svg],{type:"image/svg+xml;charset=utf-8"}),name).catch(e=>notify(e.message));}} /></details>}
      {!candidate && (
        <div className="small-empty">
          <Sparkles size={27} />
          <p>
            大纲、当前细纲、故事线与阶段设定，
            <br />
            会随任务一起交给模型。
          </p>
        </div>
      )}
      </div>
    </>
  );
  const reviewPanel = () => (
    <>
      <div className="panel-heading">
        <div className="eyebrow">EDITORIAL REVIEW</div>
        <h3>把文字，再读一遍。</h3>
        <p>本地检查常见错别字与标点，AI 审稿分析文风与情节。</p>
      </div>
      <div className="inline">
        <Button disabled={!chapter} onClick={runReview}>
          <CheckCircle2 size={15} /> 错别字检查
        </Button>
        <Button
          secondary
          disabled={!chapter || generating}
          onClick={() => runAI("review")}
        >
          <Sparkles size={15} /> AI 审稿
        </Button>
      </div>
      {review && (
        <>
          <div className="review-stats">
            <span>
              <b>{review.words}</b>字
            </span>
            <span>
              <b>{review.paragraphs}</b>段
            </span>
            <span>
              <b>{review.averageLength}</b>平均段长
            </span>
          </div>
          {review.revision !== project?.revision && (
            <div className="info-note">
              项目已更新，请重新检查以获得最新报告。
            </div>
          )}
          <h4>错别字与标点</h4>
          <p className="muted">使用本地常见词语规则，无需模型和密钥；建议需结合语境确认，不会自动替换正文。</p>
          {review.issues.filter((i) => i.kind === "疑似错别字" || i.kind === "标点").length ? (
            review.issues.filter((i) => i.kind === "疑似错别字" || i.kind === "标点").map((i, n) => (
              <div className="review-issue" key={n}>
                <div><span className="tag gold">{i.kind}</span><small>第 {i.paragraph} 段</small></div>
                <blockquote><button className="source-quote" title="定位原文" onClick={()=>locateSource(i.quote,{paragraph:i.paragraph})}>{i.quote}</button></blockquote><p>{i.message}</p>
              </div>
            ))
          ) : <div className="info-note success">未发现规则库中的常见错别字或重复标点。</div>}
          <details className="review-section">
            <summary>其他写作检查 · {review.issues.filter((i) => i.kind !== "疑似错别字" && i.kind !== "标点").length} 项</summary>
            {review.issues.filter((i) => i.kind !== "疑似错别字" && i.kind !== "标点").map((i, n) => (
              <div className="review-issue" key={n}>
                <div><span className="tag gold">{i.kind}</span><small>第 {i.paragraph} 段</small></div>
                <blockquote><button className="source-quote" title="定位原文" onClick={()=>locateSource(i.quote,{paragraph:i.paragraph})}>{i.quote}</button></blockquote><p>{i.message}</p>
              </div>
            ))}
          </details>
        </>
      )}
      <div className="review-section">
        <h4>作者的文风规则</h4>
        <p className="muted">每行填写一个希望检查的表达。</p>
        <textarea
          aria-label="文风规则"
          rows={4}
          value={project?.banned || ""}
          onChange={(e) => edit((p) => (p.banned = e.target.value))}
          placeholder="命运的齿轮\n不禁陷入了沉思"
        />
      </div>
      {project && chapter && <AIRunHistory onReplaceAll={replaceAllReview} onLocate={locateSource} key={project.id+chapter.id} runs={project.aiRuns} chapterId={chapterId} text={chapter.text} document={editorRef.current?.state.doc} busy={generating} onReplace={replaceOneReview} onGenerate={run=>runAI("review",run.text)} reviewsOnly onDownload={()=>{}} />}
      <div className="review-section">
        <h4>AI 使用记录与来源检测</h4>
        <p>
          本章有{" "}
          {project?.aiRuns.filter(
            (r) => r.chapterId === chapterId && r.accepted,
          ).length || 0}{" "}
          次 AI 候选采纳记录。
        </p>
        <p className="muted">
          该记录只反映本应用内的操作。外部导入内容的作者来源未知；规则检查与 AI
          审稿不会计算虚假的“AI 率”。
        </p>
        <p className="muted">
          AI 来源检测属于实验能力，需接入独立检测服务后使用。
        </p>
        <Button
          secondary
          disabled={detecting || !chapter}
          onClick={config?.detectorUrl?.trim() ? runDetection : () => setPage("settings")}
        >
          {detecting ? (
            <Loader2 className="spin" size={14} />
          ) : (
            <ShieldCheck size={14} />
          )}{" "}
          {config?.detectorUrl?.trim() ? "请求来源检测" : "配置来源检测服务"}
        </Button>
        {!config?.detectorUrl?.trim() && <p className="info-note">尚未配置独立来源检测服务。点击上方按钮填写完整检测接口地址与密钥并保存；聊天模型用于审稿，不能返回可靠的作者身份判定。AI 审稿和本地检查仍可正常使用。</p>}
        {detection && (
          <div className="info-note">
            <b>{detection.model}</b>
            <p>
              {detection.label}{" "}
              {detection.score !== null
                ? "· 服务分数 " + detection.score.toFixed(3)
                : ""}
            </p>
            <p>{detection.explanation}</p>
            <small>
              语言：{detection.language} · 输入 {detection.words} 字 ·
              服务分数未经本应用校准
            </small>
            {detection.revision !== project?.revision && (
              <p>项目已修改，请重新检测。</p>
            )}
          </div>
        )}
      </div>
    </>
  );

  return (
    <>
      <input
        ref={importRef}
        type="file"
        accept=".json"
        hidden
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) importProject(f);
          e.target.value = "";
        }}
      />
      <input
        ref={sourceRef}
        type="file"
        accept=".txt,.md"
        hidden
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) importSource(f);
          e.target.value = "";
        }}
      />
      <input
        ref={loreRef}
        type="file"
        accept=".json,.csv"
        hidden
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) importLore(f);
          e.target.value = "";
        }}
      />
      {loreImport && project && <LoreImportDialog rows={loreImport.rows} stageName={project.stages.find(s=>s.id===loreImport.stageId)?.name || ""} entities={project.entities} onClose={()=>setLoreImport(null)} onImport={applyLoreImport} />}
      {loading ? (
        <div className="startup">
          <Feather size={40} />
          <h2>Yantai</h2>
          <Loader2 className="spin" size={22} />
        </div>
      ) : shelf || !project ? (
        renderShelf()
      ) : (
        <div className={"app " + (focus ? "focus-mode" : "")}>
          <aside className="sidebar">
            <div className="brand">
              <span className="brand-mark">
                <Feather size={22} />
              </span>
              <div>
                Yantai <small>小说写作工作台</small>
              </div>
            </div>
            <button className="project-switch" onClick={goShelf}>
              <div className="project-initial">{project.name.slice(0, 1)}</div>
              <div>
                <b>{project.name}</b>
                <small>
                  {project.genre || "长篇小说"} · {project.chapters.length} 章
                </small>
              </div>
              <ChevronDown size={15} />
            </button>
            <nav className="main-nav">
              {nav.map((n) => (
                <button
                  key={n.id}
                  className={page === n.id ? "active" : ""}
                  onClick={() => {
                    setPage(n.id);
                    if (n.id !== "write") {
                      setFocus(false);
                      editorRef.current = null;
                    }
                  }}
                >
                  <n.icon size={18} />
                  {n.name}
                  {page === n.id && <span className="nav-indicator" />}
                </button>
              ))}
            </nav>
            <div className="chapter-tree">
              <div className="tree-heading">
                <span>卷与章节</span>
                <button className="icon" title="新建卷" onClick={addVolume}>
                  <Plus size={16} />
                </button>
              </div>
              {project.volumes.map((v) => (
                <div className="volume" key={v.id}>
                  <div className="volume-heading">
                    <button
                      className="volume-toggle"
                      onClick={() =>
                        setCollapsed(
                          collapsed.includes(v.id)
                            ? collapsed.filter((id) => id !== v.id)
                            : [...collapsed, v.id],
                        )
                      }
                    >
                      {collapsed.includes(v.id) ? (
                        <ChevronRight size={13} />
                      ) : (
                        <ChevronDown size={13} />
                      )}
                      <span>{v.name}</span>
                    </button>
                    <button
                      className="icon"
                      title="新建章节"
                      onClick={() => addChapter(v.id)}
                    >
                      <Plus size={14} />
                    </button>
                  </div>
                  {!collapsed.includes(v.id) &&
                    project.chapters
                      .filter((c) => c.volumeId === v.id)
                      .map((c) => (
                        <div
                          className={
                            "chapter-item " +
                            (chapterId === c.id ? "selected" : "")
                          }
                          key={c.id}
                        >
                          <button onClick={() => changeChapter(c.id)}>
                            <FileText size={14} />
                            <span>{c.title}</span>
                          </button>
                          <div className="chapter-order">
                            <button title="修改章节名" aria-label={`修改章节名：${c.title}`} onClick={() => renameChapter(c.id)}><Pencil size={12} /></button>
                            <button
                              title="上移章节"
                              onClick={() => reorder(c.id, -1)}
                            >
                              <ArrowUp size={11} />
                            </button>
                            <button
                              title="下移章节"
                              onClick={() => reorder(c.id, 1)}
                            >
                              <ArrowDown size={11} />
                            </button>
                          </div>
                        </div>
                      ))}
                </div>
              ))}
            </div>
            <div className="sidebar-bottom">
              <button
                onClick={() => {
                  setPage("settings");
                  setFocus(false);
                }}
              >
                <Settings size={17} /> 模型与设置
              </button>
              <button onClick={goShelf}>
                <Library size={17} /> 返回书架
              </button>
              <div className="local-indicator">
                <span className="status-dot" /> 本地工作空间 <span>v0.5.1</span>
              </div>
            </div>
          </aside>
          <div className="workspace">
            <header className="topbar">
              <div className="breadcrumb">
                <BookOpen size={16} />
                <span>{project.name}</span>
                <ChevronRight size={13} />
                <b>
                  {page === "write"
                    ? chapter?.title || "正文写作"
                    : page === "settings"
                      ? "模型与设置"
                      : nav.find((n) => n.id === page)?.name}
                </b>
              </div>
              <div className="top-actions">
                <Button secondary onClick={() => exportProject("json")}><Download size={15} />备份项目</Button>
                <button
                  className={
                    "save-badge " + (saveStatus === "保存失败" ? "error" : "")
                  }
                  onClick={() => flush().catch(() => {})}
                >
                  {saveStatus === "保存中" ? (
                    <Loader2 className="spin" size={14} />
                  ) : saveStatus === "保存失败" ? (
                    <AlertCircle size={14} />
                  ) : (
                    <CheckCircle2 size={14} />
                  )}{" "}
                  {saveStatus}
                </button>
                <button className="icon" title="保存历史" onClick={showHistory}>
                  <History size={18} />
                </button>
                <button className="icon" title="读取 Agent 最新修改" onClick={async () => {
                  if (dirty.current !== saved.current || saving.current) { notify("请先保存当前修改。若保存冲突，请导出草稿，再重新打开项目。"); return; }
                  try { const latest = await api<Project>(`/projects/${project!.id}`); setCurrent(latest); notify("已读取最新项目内容"); } catch (e) { notify((e as Error).message); }
                }}><RefreshCw size={17} /></button>
                <button
                  className="icon"
                  title="切换主题"
                  onClick={() => setTheme(theme === "light" ? "dark" : "light")}
                >
                  {theme === "light" ? <Moon size={18} /> : <Sun size={18} />}
                </button>
                <details className="export-menu">
                  <summary className="btn secondary">
                    <Download size={15} /> 导出
                  </summary>
                  <div>
                    <button onClick={() => exportProject("txt")}>
                      纯文本 TXT
                    </button>
                    <button onClick={() => exportProject("docx")}>Word 文档（DOCX）</button>
                    <button onClick={() => exportProject("epub")}>电子书（EPUB）</button>
                    <button onClick={() => exportProject("md")}>
                      Markdown 文稿
                    </button>
                    <button onClick={() => exportProject("json")}>
                      完整项目 JSON
                    </button>
                    <button
                      onClick={() => {
                        if (pRef.current)
                          download(
                            new Blob([JSON.stringify(pRef.current, null, 2)], {
                              type: "application/json",
                            }),
                            project.name + "-未保存草稿.json",
                          );
                      }}
                    >
                      导出当前草稿
                    </button>
                  </div>
                </details>
              </div>
            </header>
            {page === "write" ? (
              <>
                <div className="chapter-header">
                  <div>
                    <div className="eyebrow">
                      {project.volumes.find((v) => v.id === chapter?.volumeId)
                        ?.name || "写作工作台"}
                    </div>
                    <button className="chapter-title" onClick={() => renameChapter()}>
                      {chapter?.title || "从新章节开始"} <Feather size={15} />
                    </button>
                  </div>
                  <div className="inline">
                    {chapter && (
                      <select
                        aria-label="当前章阶段"
                        className="stage-select"
                        value={chapter.stageId}
                        onChange={(e) =>
                          patchChapter({ stageId: e.target.value })
                        }
                      >
                        {project.stages.map((s) => (
                          <option key={s.id} value={s.id}>
                            {s.name}
                          </option>
                        ))}
                      </select>
                    )}
                    <button
                      className="icon"
                      title="删除章节"
                      disabled={!chapter}
                      onClick={removeChapter}
                    >
                      <Trash2 size={16} />
                    </button>
                    <button
                      className="icon"
                      title="显示或隐藏右侧"
                      onClick={() => setRightVisible(!rightVisible)}
                    >
                      {rightVisible ? (
                        <PanelRightClose size={19} />
                      ) : (
                        <PanelRightOpen size={19} />
                      )}
                    </button>
                  </div>
                </div>
                <div className="writing-layout">
                  {chapter ? (
                    <WritingEditor
                      key={project.id + chapterId + ":" + editorEpoch}
                      chapter={chapter}
                      projectId={project.id}
                      onChange={(content, text) =>
                        patchChapter({ content, text })
                      }
                      onSelection={(s) => (selectionRef.current = s)}
                      onReady={ready}
                      focus={focus}
                      setFocus={setFocus}
                    />
                  ) : (
                    <Empty
                      title="写下第一行"
                      text="新建章节，让你的故事在这里展开。"
                    >
                      <Button onClick={() => addChapter(project.volumes[0].id)}>
                        <Plus size={16} /> 新建章节
                      </Button>
                    </Empty>
                  )}
                  {rightVisible && !focus && (
                    <aside className="right-panel">
                      <div className="right-tabs">
                        {[
                          { id: "lore", name: "资料", icon: Users },
                          { id: "ai", name: "AI", icon: Sparkles },
                          { id: "review", name: "审稿", icon: CheckCircle2 },
                          { id: "ideas", name: "灵感", icon: Search },
                        ].map((t) => (
                          <button
                            className={rightTab === t.id ? "active" : ""}
                            key={t.id}
                            onClick={() => setRightTab(t.id)}
                          >
                            <t.icon size={15} />
                            {t.name}
                          </button>
                        ))}
                      </div>
                      <div className="panel-scroll">
                        {chapter && rightTab === "lore" && (
                          <div className="chapter-planning">
                            <details className="reference-card">
                              <summary><b>章纲</b><span>点击查看 / 编辑</span></summary>
                              <textarea aria-label="本章章纲" rows={5} value={chapter.outline} onChange={(e) => patchChapter({ outline: e.target.value })} placeholder="本章场景、冲突、转折与承接……" />
                            </details>
                            <details className="reference-card">
                              <summary><b>前情提要</b><span>点击查看 / 编辑</span></summary>
                              <p className="muted">记录本章开头需要承接的前情，也会送入 AI 上下文。</p>
                              <textarea aria-label="本章前情提要" rows={5} value={chapter.summary} onChange={(e) => patchChapter({ summary: e.target.value })} placeholder="此前发生了什么、有哪些未解决的线索……" />
                            </details>
                          </div>
                        )}

                        {rightTab === "lore" ? (
                          lorePanel()
                        ) : rightTab === "ai" ? (
                          aiPanel()
                        ) : rightTab === "review" ? (
                          reviewPanel()
                        ) : (
                          <>
                            <div className="panel-heading">
                              <div className="eyebrow">FIND A SPARK</div>
                              <h3>寻找一点灵感</h3>
                            </div>
                            {searchPanel()}

                          </>
                        )}
                      </div>
                    </aside>
                  )}
                </div>
              </>
            ) : page === "outline" || page === "detail" ? (
              renderOutline()
            ) : page === "story" ? (
              renderStory()
            ) : page === "inspiration" ? (
              renderInspiration()
            ) : page === "settings" ? (
              renderSettings()
            ) : page === "images" ? (
              <ImageLibrary key={project.id} project={project} edit={edit} notify={notify} onDownload={download} />
            ) : (
              renderLore()
            )}
          </div>
        </div>
      )}
      {toast && (
        <div className="toast" role="status">
          <AlertCircle size={17} />
          <span>{toast}</span>
          <button onClick={() => setToast("")}>
            <X size={14} />
          </button>
        </div>
      )}
      {dialog && (
        <Modal
          key={dialog.title}
          dialog={dialog}
          close={() => setDialog(null)}
          notify={notify}
        />
      )}
      {syncOpen&&<div className="modal-overlay"><div className="modal wide"><button className="modal-close icon" aria-label="关闭服务端同步" onClick={()=>setSyncOpen(false)}><X size={20}/></button><SyncSettings client={syncClient} onDeleteLocal={id=>deleteLocalProject(id,true)}/></div></div>}
      {contextOpen && context && (
        <div className="modal-overlay">
          <div className="modal wide">
            <button
              className="modal-close icon"
              onClick={() => setContextOpen(false)}
            >
              <X size={20} />
            </button>
            <div className="eyebrow">MODEL CONTEXT</div>
            <h2>实际交给 AI 的资料</h2>
            <div className={"info-note " + (!context.fits ? "warning" : "")}>
              保守估计 {context.estimatedTokens.toLocaleString()}{" "}
              token，可用预算 {context.budget.toLocaleString()}。
              {context.fits ? "当前预算可发送。" : "超出窗口，发送会被阻止。"}
            </div>
            <div className="context-sections">
              {context.sections.map((s, i) => (
                <details key={i} open={i < 2}>
                  <summary>
                    {s.name}
                    <small>{s.source}</small>
                  </summary>
                  {["当前正文","选中文本"].includes(s.name)&&context.chapterId===chapterId&&<button className="source-locate" onClick={()=>{setContextOpen(false);locateSource(s.text);}}>定位原文</button>}
                  <pre>{s.text}</pre>
                </details>
              ))}
            </div>
          </div>
        </div>
      )}
      {history && (
        <div className="modal-overlay">
          <div className="modal wide">
            <button
              className="modal-close icon"
              onClick={() => setHistory(null)}
            >
              <X size={20} />
            </button>
            <div className="eyebrow">REVISION HISTORY</div>
            <h2>每次落笔，都留有来路。</h2>
            <p className="muted">
              恢复会同时恢复该版本的正文、设定与故事线。当前内容会继续保留在历史中。
            </p>
            <div className="history-list">
              {history.length ? (
                history.map((h) => (
                  <div key={h.revision}>
                    <History size={18} />
                    <div>
                      <b>项目版本 {h.revision}</b>
                      <small>
                        {date(h.createdAt)} · {h.name}
                      </small>
                    </div>
                    <Button
                      secondary
                      onClick={() =>
                        setDialog({
                          title: "恢复历史版本",
                          description: `恢复项目版本 ${h.revision}，包含当时的所有正文与设定。`,
                          fields: [],
                          button: "恢复版本",
                          submit: async () => {
                            await flush();
                            const p = await api<Project>(
                              `/projects/${project!.id}/restore`,
                              "POST",
                              {
                                revision: h.revision,
                                expected: pRef.current!.revision,
                              },
                            );
                            setCurrent(p);
                            setHistory(null);
                            notify("历史版本已恢复");
                          },
                        })
                      }
                    >
                      恢复
                    </Button>
                  </div>
                ))
              ) : (
                <Empty
                  title="尚无历史记录"
                  text="首次修改并保存后，旧版本会出现在这里。"
                />
              )}
            </div>
          </div>
        </div>
      )}
    </>
  );
}
function PageHeading({
  eyebrow,
  title,
  text,
  action,
}: {
  eyebrow: string;
  title: string;
  text: string;
  action?: ReactNode;
}) {
  return (
    <div className="page-heading">
      <div>
        <div className="eyebrow">{eyebrow}</div>
        <h1>{title}</h1>
        <p>{text}</p>
      </div>
      {action}
    </div>
  );
}
async function download(blob: Blob, name: string) {
  const w = window as unknown as {
    go?: {
      main?: {
        App?: { SaveText?: (name: string, text: string) => Promise<string>; SaveBinary?: (name:string, data:string)=>Promise<string> };
      };
    };
  };
  if (w.go?.main?.App?.SaveBinary && /\.(docx|epub|png|jpe?g|gif)$/i.test(name)) {
    const encoded = await new Promise<string>((resolve,reject)=>{const reader=new FileReader();reader.onload=()=>resolve(String(reader.result).split(",")[1]);reader.onerror=()=>reject(Error("读取导出文件失败"));reader.readAsDataURL(blob);});
    return !!(await w.go.main.App.SaveBinary(name,encoded));
  }
  if (w.go?.main?.App?.SaveText) {
    const path = await w.go.main.App.SaveText(name, await blob.text());
    return !!path;
  }
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 10000);
  return true;
}
function safeURL(s: string) {
  try {
    return ["http:", "https:"].includes(new URL(s).protocol);
  } catch {
    return false;
  }
}
function openExternal(e: React.MouseEvent, address: string) {
  const w = window as unknown as {
    go?: { main?: { App?: { OpenExternal?: (s: string) => void } } };
  };
  if (w.go?.main?.App?.OpenExternal) {
    e.preventDefault();
    w.go.main.App.OpenExternal(address);
  }
}
function defaultInstruction(mode: string) {
  return mode === "continue"
    ? "承接当前正文续写，不重复已有段落，按本章细纲推进。"
    : mode === "expand"
      ? "扩写选中段落；若未选中则扩写本章，保留原有事实。"
      : mode === "rewrite"
        ? "改写选中段落；若未选中则改写本章，保持剧情与人设。"
        : mode === "review"
          ? "审查当前章的文风与设定连续性，引用原文并给出具体改法。"
          : mode === "map"
            ? "依据本章当前阶段有效场景绘制平面图，列出房间、地点与道路连接，未知方位在备注中说明为示意；只返回约定的地图 JSON。"
          : "根据当前细纲与设定提出三种原创场景方案。";
}

function contextSignature(p: Project) {
  return JSON.stringify({ ...p, aiRuns: [], revision: 0, updatedAt: "" });
}
