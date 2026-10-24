import type { Node } from "@tiptap/pm/model";
import { Fragment, Slice } from "@tiptap/pm/model";
import type { EditorState } from "@tiptap/pm/state";
import { closeHistory } from "@tiptap/pm/history";
import { revisionHasChange } from "./revision-ai.ts";

export type ReviewSuggestion = { id: string; original: string; replacement: string; reason: string; paragraph?: number };
export type ReviewResult = { summary: string; suggestions: ReviewSuggestion[] };
export function reviewHasChange(suggestion: ReviewSuggestion) {
  return !!suggestion.original.trim() && !!suggestion.replacement.trim() && revisionHasChange(suggestion.original, suggestion.replacement);
}
export function reviewRetryInstruction(text: string): string {
  const unchanged = parseReviewResult(text)?.suggestions.filter(s => s.original.trim() && s.replacement.trim() && !reviewHasChange(s)) || [];
  if (!unchanged.length) return "";
  return "\n重试纠正：上一轮这些建议的 replacement 与 original 没有实际变化（仅改空白也不算修改）。请根据 reason 实际改写，保留剧情与设定；不能把原文抄回作为改文。如果无法提出有依据的修改，删除该条建议并在 summary 中说明。重新返回完整的 summary 和 suggestions JSON，保留其他有效建议。需要纠正的建议：\n" + JSON.stringify(unchanged.map(({paragraph,original,reason})=>({paragraph,original,reason})));
}
export function parseReviewResult(text: string): ReviewResult | null {
  try {
    const data=JSON.parse(text.trim().replace(/^```(?:json)?\s*\n([\s\S]*?)\n```\s*$/i,"$1"));
    if(typeof data?.summary!=="string" || !Array.isArray(data.suggestions))return null;
    return {summary:data.summary,suggestions:data.suggestions.filter((s:unknown):s is Record<string,unknown>=>!!s&&typeof s==="object").map((s:Record<string,unknown>,i:number)=>({
      id:`suggestion-${i}`,original:typeof s.original==="string"?s.original.replace(/\r\n?/g,"\n"):"",replacement:typeof s.replacement==="string"?s.replacement.replace(/\r\n?/g,"\n"):"",reason:typeof s.reason==="string"?s.reason:"",paragraph:Number.isInteger(s.paragraph)&&Number(s.paragraph)>0?Number(s.paragraph):undefined,
    }))};
  }catch{return null;}
}
export function reviewParagraphs(doc: Node) {
  const paragraphs:{from:number;to:number;text:string;paragraph:number}[]=[];
  doc.descendants((node,pos)=>{if(node.isTextblock){paragraphs.push({from:pos+1,to:pos+node.nodeSize-1,text:node.textBetween(0,node.content.size,"\n"),paragraph:paragraphs.length+1});return false;}});
  return paragraphs;
}
export function reviewTarget(doc: Node,suggestion:ReviewSuggestion){
  if(!suggestion.original.trim()||!suggestion.replacement.trim())throw Error("AI 未提供完整原段落和替换正文，请重新生成修改段落。");
  if(!revisionHasChange(suggestion.original,suggestion.replacement))throw Error("建议与原文没有实际变化，无需替换。");
  const matches=reviewParagraphs(doc).filter(p=>p.text===suggestion.original);
  if(matches.length===1)return matches[0];
  // A prior accepted edit can change paragraph numbers. Never use a stale ordinal
  // to choose between identical passages.
  if(matches.length>1)throw Error("原文有多个相同段落，无法确定替换位置，请重新生成并指定段落。");
  throw Error("原段落已变化、已替换或引用不完整，请重新生成后再替换。");
}
export function reviewReplacementPresent(doc:Node,suggestion:ReviewSuggestion){
  if (!reviewHasChange(suggestion)) return false;
  const paragraphs=reviewParagraphs(doc),lines=suggestion.replacement.split("\n");
  return !!suggestion.replacement.trim()&&paragraphs.some((_,i)=>lines.every((line,j)=>paragraphs[i+j]?.text===line));
}
export function reviewReplacementPlan(doc:Node,suggestions:ReviewSuggestion[]){
  const candidates:{suggestion:ReviewSuggestion;target:ReturnType<typeof reviewTarget>}[]=[];
  const skipped:{suggestion:ReviewSuggestion;reason:string}[]=[];
  for(const suggestion of suggestions){
    try{candidates.push({suggestion,target:reviewTarget(doc,suggestion)});}
    catch(e){skipped.push({suggestion,reason:(e as Error).message});}
  }
  const items=candidates.filter((item,i)=>{
    const conflict=candidates.some((other,j)=>i!==j&&item.target.from<other.target.to&&other.target.from<item.target.to);
    if(conflict)skipped.push({suggestion:item.suggestion,reason:"同一原段落有多条冲突建议，请逐条选择替换。"});
    return !conflict;
  });
  return {items,skipped};
}
function reviewSlice(state:EditorState,suggestion:ReviewSuggestion,from:number){
  const marks=(state.doc.nodeAt(from)?.marks||[]).filter(m=>m.type.name!=="revisionNote");
  const paragraphs=suggestion.replacement.split("\n").map(line=>state.schema.nodes.paragraph.create(null,line?state.schema.text(line,marks):undefined));
  return new Slice(Fragment.fromArray(paragraphs),1,1);
}
export function replaceReviewParagraphs(state:EditorState,suggestions:ReviewSuggestion[]){
  // Resolve every anchor against the same document before making any edits.
  // Reverse order prevents new paragraphs or text from retargeting another item.
  const plan=reviewReplacementPlan(state.doc,suggestions);
  if(!plan.items.length)throw Error("没有可一键替换的建议：原文已变化、已替换、无实际改动或存在冲突。");
  let transaction=closeHistory(state.tr);
  for(const {suggestion,target} of [...plan.items].sort((a,b)=>b.target.from-a.target.from)){
    transaction=transaction.replaceRange(target.from,target.to,reviewSlice(state,suggestion,target.from));
  }
  return {transaction:transaction.scrollIntoView(),replaced:plan.items.length,skipped:plan.skipped.length};
}
export function replaceReviewParagraph(state:EditorState,suggestion:ReviewSuggestion){
  const target=reviewTarget(state.doc,suggestion);
  return closeHistory(state.tr).replaceRange(target.from,target.to,reviewSlice(state,suggestion,target.from)).scrollIntoView();
}
