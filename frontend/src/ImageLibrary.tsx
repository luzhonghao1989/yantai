import { useEffect, useRef, useState } from "react";
import { apiHeaders, apiURL, uid, type Project, type ProjectImage } from "./types";

function ImageAsset({ projectId, image }: { projectId: string; image: ProjectImage }) {
  const [src, setSrc] = useState(""), [error, setError] = useState("");
  useEffect(() => {
    if (image.data) { setSrc(image.data); return; }
    const abort = new AbortController(); let url = "";
    setSrc(""); setError("");
    fetch(apiURL(`/projects/${projectId}/images/${image.id}?v=${image.sha || ""}`), { headers: apiHeaders(), signal: abort.signal }).then(r => { if (!r.ok) throw Error("图片读取失败"); return r.blob(); }).then(blob => { if (!abort.signal.aborted) { url = URL.createObjectURL(blob); setSrc(url); } }).catch(e => { if (!abort.signal.aborted) setError(e.message); });
    return () => { abort.abort(); if (url) URL.revokeObjectURL(url); };
  }, [projectId, image.id, image.sha, image.data]);
  return src ? <img src={src} alt={image.name} /> : <span className="muted">{error || "加载图片……"}</span>;
}
const kinds = { map: "地图", character: "人物", scene: "场景", other: "其他" };
export default function ImageLibrary({ project, edit, onDownload, notify, onDelete }: { onDelete: (image: ProjectImage) => void; project: Project; edit: (fn: (p: Project) => void) => void; onDownload: (blob: Blob, name: string) => Promise<unknown>; notify: (s: string) => void }) {
  const [kind, setKind] = useState<ProjectImage["kind"]>("map"), [filter, setFilter] = useState("all"), [selected, setSelected] = useState("");
  const input = useRef<HTMLInputElement>(null), images = project.images || [], current = images.find(i => i.id === selected);
  const importImages = async (files: File[]) => {
    try {
      if (images.length + files.length > 200 || images.reduce((n, i) => n + i.size, 0) + files.reduce((n, f) => n + f.size, 0) > 32 * 1024 * 1024) throw Error("每个项目最多 200 张图片、图片总量最多 32 MiB");
      const imported: ProjectImage[] = [];
      for (const file of files) {
        if (file.size > 8 * 1024 * 1024) throw Error(`${file.name} 超过单张 8 MiB 上限`);
        if (!/\.(png|jpe?g|gif|svg)$/i.test(file.name)) throw Error("支持 PNG、JPEG、GIF、静态 SVG");
        const data = await new Promise<string>((resolve, reject) => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result)); reader.onerror = () => reject(Error("图片读取失败")); reader.readAsDataURL(file); });
        imported.push({ id: uid(), name: file.name, kind, note: "", data, size: file.size, createdAt: new Date().toISOString() });
      }
      edit(p => { p.images = [...(p.images || []), ...imported]; }); setFilter("all"); notify(`已添加 ${imported.length} 张图片，正在保存到本地`);
    } catch (e) { notify((e as Error).message); }
  };
  const exportImage = async (image: ProjectImage) => {
    try {
      const response = await fetch(image.data || apiURL(`/projects/${project.id}/images/${image.id}`), image.data ? undefined : { headers: apiHeaders() });
      if (!response.ok) throw Error("图片读取失败");
      const blob = await response.blob();
      const ext = blob.type === "image/svg+xml" ? "svg" : blob.type === "image/gif" ? "gif" : blob.type === "image/jpeg" ? "jpg" : "png";
      const name = image.name.replace(/[<>:"/\\|?*\r\n]/g, "_").replace(/\.(png|jpe?g|gif|svg)$/i, "") + "." + ext;
      await onDownload(blob, name);
    } catch (e) { notify((e as Error).message); }
  };
  const patch = (value: Partial<ProjectImage>) => edit(p => { Object.assign(p.images!.find(i => i.id === selected)!, value); });
  return <div className="management image-library">
    <div className="image-library-heading"><div><div className="eyebrow">PROJECT IMAGES</div><h1>图片</h1><p className="muted">地图、人物与场景参考 · AI 生成的地图会自动存入这里 · 随完整项目备份保存</p></div><div className="inline"><select aria-label="导入图片分类" value={kind} onChange={e => setKind(e.target.value as ProjectImage["kind"])}>{Object.entries(kinds).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</select><button className="btn" onClick={() => input.current?.click()}>导入图片</button><input ref={input} type="file" multiple accept=".png,.jpg,.jpeg,.gif,.svg" hidden onChange={e => { const files = Array.from(e.target.files || []); e.target.value = ""; void importImages(files); }} /></div></div>
    <p className="muted">PNG / JPEG / GIF / 静态 SVG；单张最多 8 MiB，项目图片总量最多 32 MiB。图片原文件独立保存，历史版本只记录引用。</p>
    <div className="stage-tabs" aria-label="图片分类">{Object.entries({ all: "全部", ...kinds }).map(([k, v]) => <button key={k} className={filter === k ? "selected" : ""} aria-pressed={filter === k} onClick={() => setFilter(k)}>{v} · {images.filter(i => k === "all" || i.kind === k).length}</button>)}</div>
    <div className="image-library-grid">{images.filter(i => filter === "all" || i.kind === filter).map(image => <div key={image.id} className="image-library-card"><button className="image-library-open" aria-label={`查看图片：${image.name}`} onClick={() => setSelected(image.id)}><div className="image-library-thumb"><ImageAsset projectId={project.id} image={image} /></div><b>{image.name}</b><small>{kinds[image.kind]}{image.sourceRunId ? " · AI 生成" : ""}</small></button><button className="text-button image-delete" aria-label={`删除图片：${image.name}`} onClick={() => onDelete(image)}>删除图片</button></div>)}</div>
    {!images.length && <div className="small-empty"><h3>还没有图片</h3><p>导入地图、人物或场景图片，或到 AI 中生成场景平面图。</p></div>}
    {current && <div className="image-library-dialog" role="dialog" aria-modal="true" aria-label="图片详情"><div className="inline"><h2>{current.name}</h2><button className="btn secondary" onClick={() => void exportImage(current)}>导出图片</button><button className="btn secondary" onClick={() => setSelected("")}>关闭图片</button></div><div className="image-library-detail"><div className="image-library-preview"><ImageAsset projectId={project.id} image={current} /></div><div><label>名称<input aria-label="图片名称" value={current.name} onChange={e => patch({ name: e.target.value })} /></label><label>分类<select aria-label="图片分类" value={current.kind} onChange={e => patch({ kind: e.target.value as ProjectImage["kind"] })}>{Object.entries(kinds).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</select></label><label>备注<textarea aria-label="图片备注" rows={8} value={current.note} onChange={e => patch({ note: e.target.value })} /></label><p className="muted">{(current.size / 1024).toFixed(1)} KiB · {new Date(current.createdAt).toLocaleString("zh-CN")}</p><button className="text-button" onClick={() => { setSelected(""); onDelete(current); }}>删除图片</button><p className="muted">移除后可通过项目历史恢复。</p></div></div></div>}
  </div>;
}
