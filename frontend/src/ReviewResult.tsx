import type { Node } from "@tiptap/pm/model";
import AIContent from "./AIContent";
import TextDiff from "./TextDiff";
import type { LocateSource } from "./source-location";
import { parseReviewResult,reviewTarget,reviewReplacementPresent,reviewHasChange,type ReviewSuggestion } from "./review-replacements";

export default function ReviewResult({text,document,onReplace,onGenerate,onLocate,busy=false}:{text:string;document?:Node;onReplace?:(suggestion:ReviewSuggestion)=>void;onGenerate?:()=>void;onLocate?:LocateSource;busy?:boolean}){
  const result=parseReviewResult(text);
  const malformed=!result&&/^\s*(?:```json\s*)?\{/.test(text);
  return <div className="review-result">
    {malformed?<p className="info-note">AI 返回的审稿格式不完整，暂时无法生成可替换段落，请重新生成。</p>:<AIContent text={result?.summary??text} onLocate={onLocate} sourceText={document?.textBetween(0,document.content.size,"\n","\n")}/>}
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
