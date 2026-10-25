import type { LocateSource } from "./source-location";
import { useState } from "react";
import type { AIRun } from "./types";
import SceneMapPreview from "./SceneMapPreview";
import ReviewResult from "./ReviewResult";
import type { Node } from "@tiptap/pm/model";
import type { ReviewSuggestion } from "./review-replacements";
export default function AIRunHistory({ runs, chapterId, text, reviewsOnly = false, onDownload, onSave, onDeleteMap, document,onReplace,onReplaceAll,onGenerate,onLocate,busy }: { onDeleteMap?: (id:string) => void; runs: AIRun[]; chapterId: string; text: string; reviewsOnly?: boolean; onDownload: (svg: string, name: string) => void; onSave?: (svg:string,name:string,id:string)=>void;document?:Node;onReplace?:(suggestion:ReviewSuggestion,runId:string)=>void;onGenerate?:(run:AIRun)=>void;onReplaceAll?:(runId:string)=>void;busy?:boolean;onLocate?:LocateSource }) {
  const [selected, setSelected] = useState("");
  const records = runs.filter(r => r.chapterId === chapterId && (reviewsOnly ? r.mode === "review" : ["review", "map"].includes(r.mode))).slice().reverse();
  const current = records.find(r => r.id === selected) || records[0];
  if (!current) return null;
  return <section className="review-section ai-saved-results"><h4>{reviewsOnly ? "已保存的 AI 审稿" : "本章已保存的审稿与地图"}</h4><select aria-label={reviewsOnly ? "AI 审稿历史" : "AI 结果历史"} value={current.id} onChange={e => setSelected(e.target.value)}>{records.map(r => <option key={r.id} value={r.id}>{r.mode === "map" ? "场景平面图（SVG）" : "AI 审稿"} · {new Date(r.createdAt).toLocaleString("zh-CN")}</option>)}</select>
    <small className="muted">{current.model} · 已保存到项目；可随项目备份导入</small>
    {current.inputText === undefined ? <p className="muted">这是历史生成结果，请结合当前正文核对。</p> : current.inputText !== text && <p className="info-note">正文已变化，这份结果基于之前的稿件；可重新生成。</p>}
    {current.mode === "map" ? <SceneMapPreview onDelete={onDeleteMap ? () => onDeleteMap(current.id) : undefined} text={current.text} onDownload={onDownload} onSave={onSave ? (svg,name)=>onSave(svg,name,current.id) : undefined} /> : <div className="saved-review-text"><ReviewResult onReplaceAll={onReplaceAll?()=>onReplaceAll(current.id):undefined} onLocate={onLocate} text={current.text} document={document} busy={busy} onReplace={onReplace?(suggestion)=>onReplace(suggestion,current.id):undefined} onGenerate={onGenerate?()=>onGenerate(current):undefined}/></div>}
  </section>;
}
