import type { Node } from "@tiptap/pm/model";
import AIContent from "./AIContent";
import TextDiff from "./TextDiff";
import type { LocateSource } from "./source-location";
import { parseReviewResult,reviewTarget,reviewReplacementPresent,reviewHasChange,reviewReplacementPlan,type ReviewSuggestion } from "./review-replacements";

export default function ReviewResult({text,document,onReplace,onReplaceAll,onGenerate,onLocate,busy=false}:{text:string;document?:Node;onReplace?:(suggestion:ReviewSuggestion)=>void;onReplaceAll?:()=>void;onGenerate?:()=>void;onLocate?:LocateSource;busy?:boolean}){
  const result=parseReviewResult(text);
  const plan=document&&result?reviewReplacementPlan(document,result.suggestions):null;
  const malformed=!result&&/^(?:\s*```(?:json)?\s*)?\s*[\[{]|"(?:summary|suggestions)"\s*:/i.test(text);
  return <div className="review-result">
    <AIContent text={result?.summary??text} onLocate={onLocate} sourceText={document?.textBetween(0,document.content.size,"\n","\n")}/>
    {malformed&&<p className="info-note">审稿内容已保留，暂未提取到可替换的完整段落。可点击下方“按审稿意见生成修改段落”，已有内容仍可查看和复制。</p>}
    {!!result?.suggestions.length&&onReplaceAll&&<div className="review-bulk-actions">
      <button className="btn" disabled={busy||!plan?.items.length} onClick={onReplaceAll}>一键替换全部建议{plan?.items.length?`（${plan.items.length} 段）`:""}</button>
      <p className="muted">{!document?"请先打开对应章节正文。":plan?.items.length?`仅替换已核对原文的有效建议${plan.skipped.length?`，跳过 ${plan.skipped.length} 条已处理、无效或冲突建议`:""}；整批可一次撤销。`:"暂无可批量替换的建议；已替换、原文变化、无实际修改或冲突的建议会跳过。"}</p>
    </div>}
    {result?.suggestions.map((suggestion,i)=>{
      let disabled="",applied=false;
      const unchanged=!!suggestion.original.trim()&&!!suggestion.replacement.trim()&&!reviewHasChange(suggestion);
      try{if(!document)throw Error("请打开正文后替换");reviewTarget(document,suggestion);}catch(e){disabled=(e as Error).message;applied=!!document&&reviewReplacementPresent(document,suggestion);}
      return <section className="review-replacement" key={suggestion.id}>
        <h4>建议段落 {i+1}{suggestion.paragraph?` · 第 ${suggestion.paragraph} 段`:""}</h4>
        {suggestion.reason&&<p className="info-note">修改意见：{suggestion.reason}</p>}
        {onLocate&&unchanged&&<button className="source-locate" onClick={()=>onLocate(suggestion.original,{paragraph:suggestion.paragraph})}>定位原文</button>}
        {unchanged?<p className="info-note">AI 未实际修改这段原文，请重新按审稿意见生成改文。</p>:<><TextDiff before={suggestion.original} after={suggestion.replacement} onLocate={onLocate?(text)=>onLocate(text,{paragraph:suggestion.paragraph}):undefined}/><AIContent text={suggestion.replacement}/></>}
        {onReplace&&<button className="btn secondary" disabled={busy||!!disabled} title={applied?"正文已是建议改文":disabled||"仅替换这个原段落，可撤销"} onClick={()=>onReplace(suggestion)}>{unchanged?"未实际修改":applied?"已替换":"替换原文"}</button>}
        {disabled&&!unchanged&&<p className="muted">{applied?"正文已是建议改文，可通过撤销恢复。":disabled}</p>}
      </section>;
    })}
    {!result?.suggestions.length&&onGenerate&&<p className="info-note">这份审稿尚未提供可直接替换的完整段落，可根据意见生成改文。</p>}
    {onGenerate&&<button className="btn secondary" disabled={busy} onClick={onGenerate}>按审稿意见生成修改段落</button>}
  </div>;
}
