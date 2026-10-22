import { useMemo, useState } from "react";
import { loreFields, loreKind } from "./lore-import";
import type { Entity } from "./types";

export default function LoreImportDialog({rows, entities, stageName, onClose, onImport}: {
  rows: Record<string,string>[]; entities: Entity[]; stageName:string;
  onClose:()=>void; onImport:(rows:Record<string,string>[], skipDuplicates:boolean)=>void;
}) {
  const headers = Array.from(new Set(rows.flatMap(r => Object.keys(r))));
  const [mapping,setMapping] = useState<Record<string,string>>(() => Object.fromEntries(loreFields.map(([key,label]) => [key,headers.find(h=>h.toLowerCase()===key || h===label || (key==="name" && ["姓名","名字"].includes(h)) || (key==="base" && h==="基础档案")) || ""])));
  const [kind,setKind] = useState("character"), [skip,setSkip]=useState(true), [error,setError]=useState("");
  const mapped = useMemo(() => rows.map(r => Object.fromEntries(loreFields.map(([key]) => [key,key==="kind" ? (loreKind(r[mapping[key]] || kind) || "") : (r[mapping[key]] || "").trim()]))), [rows,mapping,kind]);
  const seen = new Set(entities.map(e => e.kind + "\0" + e.name));
  let duplicates = 0; for (const r of mapped) {const key = r.kind + "\0" + r.name; if (seen.has(key)) duplicates++; else seen.add(key);}
  const invalid = mapped.filter(r=>!r.name || !r.kind).length;
  return <div className="modal-overlay"><div className="modal lore-import-modal">
    <button className="modal-close icon" aria-label="关闭导入预览" onClick={onClose}>×</button>
    <div className="eyebrow">IMPORT PREVIEW</div><h2>导入设定 · 字段映射与预览</h2>
    <p className="muted">导入到「{stageName}」，共 {rows.length} 条；已有同名 {duplicates} 条，无效 {invalid} 条。已有内容不会被覆盖。</p>
    <div className="two-col"><label>未映射类型时使用<select aria-label="默认设定类型" value={kind} onChange={e=>setKind(e.target.value)}><option value="character">人物</option><option value="item">物品</option><option value="location">场景</option></select></label><label>同名处理<select aria-label="同名设定处理" value={skip?"skip":"new"} onChange={e=>setSkip(e.target.value==="skip")}><option value="skip">跳过已有同名档案</option><option value="new">作为新的独立档案</option></select></label></div>
    <div className="import-preview"><table><thead><tr><th>名称</th><th>类型</th><th>基础设定</th><th>状态</th></tr></thead><tbody>{mapped.slice(0,8).map((r,i)=><tr key={i}><td>{r.name||"缺少名称"}</td><td>{({character:"人物",item:"物品",location:"场景"} as Record<string,string>)[r.kind] || "无效类型"}</td><td>{r.base.slice(0,80)}</td><td>{r.state.slice(0,80)}</td></tr>)}</tbody></table></div>
    <details className="import-field-details" open={invalid>0}><summary>字段映射 · 点击调整</summary><div className="import-mapping">{loreFields.map(([key,label])=><label key={key}>{label}<select aria-label={label+"映射"} value={mapping[key]} onChange={e=>setMapping({...mapping,[key]:e.target.value})}><option value="">{key==="kind"?"使用默认类型":"不导入"}</option>{headers.map(h=><option key={h} value={h}>{h}</option>)}</select></label>)}</div></details>
    {error && <p role="alert">{error}</p>}
    <div className="modal-actions"><button className="btn secondary" onClick={onClose}>取消</button><button className="btn" disabled={invalid>0} onClick={()=>{try{onImport(mapped,skip)}catch(e){setError((e as Error).message)}}}>确认导入</button></div>
  </div></div>;
}
