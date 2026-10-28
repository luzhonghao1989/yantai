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
const reviewFormatInstruction = "\n重试纠正：上一轮审稿的结构无法读取。请重新审查同一份正文，只返回一个完整、标准的 JSON 对象：{\"summary\":\"审稿总结\",\"suggestions\":[{\"paragraph\":1,\"original\":\"逐字引用完整原段落\",\"replacement\":\"按意见实际修改后的完整段落\",\"reason\":\"修改理由\"}]}。所有字符串使用英文双引号，字符串中的双引号、反斜杠和换行必须正确转义；不输出前后说明或代码围栏。没有可确定的修改时 suggestions 返回空数组。不得补造原文或把修改意见冒充改文。";
export function reviewRetryInstruction(text: string): string {
  const parsed = parseReviewResult(text);
  if (!parsed && (/^\s*(?:```(?:json)?\s*)?[{[]/i.test(text) || /"(?:summary|suggestions)"\s*:/.test(text))) return reviewFormatInstruction;
  const unchanged = parsed?.suggestions.filter(s => s.original.trim() && s.replacement.trim() && !reviewHasChange(s)) || [];
  if (!unchanged.length) return "";
  return "\n重试纠正：上一轮这些建议的 replacement 与 original 没有实际变化（仅改空白也不算修改）。请根据 reason 实际改写，保留剧情与设定；不能把原文抄回作为改文。如果无法提出有依据的修改，删除该条建议并在 summary 中说明。重新返回完整的 summary 和 suggestions JSON，保留其他有效建议。需要纠正的建议：\n" + JSON.stringify(unchanged.map(({paragraph,original,reason})=>({paragraph,original,reason})));
}

// Repair only unambiguous JSON syntax outside prose: trailing commas and raw
// control characters in strings. Never invent missing quotes/brackets or text.
function normalizeReviewJSON(text: string) {
  let quoted=false, escaped=false, out="";
  for(let i=0;i<text.length;i++) {
    const c=text[i];
    if(quoted) {
      if(escaped) {out+=c;escaped=false;}
      else if(c==="\\") {out+=c;escaped=true;}
      else if(c==='"') {out+=c;quoted=false;}
      else out+=c.charCodeAt(0)<32?JSON.stringify(c).slice(1,-1):c;
    } else {
      if(c==='"') quoted=true;
      if(c===",") {let j=i+1;while(/\s/.test(text[j]||"")&&j<text.length)j++;if(text[j]==="}"||text[j]==="]")continue;}
      out+=c;
    }
  }
  return out;
}
function reviewJSONCandidates(text: string) {
  const candidates=[text.trim().replace(/^\uFEFF/,"")];
  for(const match of text.matchAll(/```(?:json|javascript)?\s*([\s\S]*?)```/gi)) {if(candidates.length>=20)break;candidates.push(match[1]);}
  let start=-1, quoted=false, escaped=false;const stack:string[]=[];
  for(let i=0;i<text.length&&candidates.length<40;i++) {
    const c=text[i];
    if(start<0) {if(c==="{"||c==="["){start=i;stack.push(c);}continue;}
    if(quoted) {if(escaped)escaped=false;else if(c==="\\")escaped=true;else if(c==='"')quoted=false;continue;}
    if(c==='"') {quoted=true;continue;}
    if(c==="{"||c==="[")stack.push(c);
    if(c==="}"||c==="]") {
      if(stack.pop()!==(c==="}"?"{":"[")){start=-1;stack.length=0;continue;}
      if(!stack.length){candidates.push(text.slice(start,i+1));start=-1;}
    }
  }
  return [...new Set(candidates)];
}
const reviewObject = (value:unknown):value is Record<string,unknown> => !!value&&typeof value==="object"&&!Array.isArray(value);
const reviewField = (record:Record<string,unknown>,names:string[]) => names.map(name=>record[name]).find(value=>typeof value==="string") as string|undefined;
function reviewShape(value:unknown,depth=0):ReviewResult|null {
  if(depth>4)return null;
  if(typeof value==="string") {try{return reviewShape(JSON.parse(normalizeReviewJSON(value)),depth+1);}catch{return null;}}
  if(!reviewObject(value)&&!Array.isArray(value))return null;
  if(reviewObject(value)) {
    for(const key of ["result","data","review","审稿结果"]) {const nested=reviewShape(value[key],depth+1);if(nested)return nested;}
  }
  const record=reviewObject(value)?value:{};
  const summary=reviewField(record,["summary","overview","总结","审稿总结","总评","总体评价"]);
  let list:unknown=Array.isArray(value)?value:["suggestions","replacements","edits","修改建议","建议列表"].map(key=>record[key]).find(Array.isArray);
  if(!Array.isArray(list)) {if(summary!==undefined)return {summary,suggestions:[]};return null;}
  // A bare array must actually contain replacement records, not arbitrary JSON.
  if(Array.isArray(value)&&(!list.length||!list.some(item=>reviewObject(item)&&reviewField(item,["original","originalText","原文","原段落"])!==undefined)))return null;
  return {summary:summary??"模型未提供审稿总结，以下为提取到的修改建议。",suggestions:list.filter(reviewObject).map((item,i)=>{
    const paragraph=item.paragraph??item.paragraphIndex??item["段落序号"]??item["段落"];
    const number=typeof paragraph==="number"?paragraph:typeof paragraph==="string"&&/^\d+$/.test(paragraph)?Number(paragraph):undefined;
    return {id:`suggestion-${i}`,original:(reviewField(item,["original","originalText","原文","原段落"])??"").replace(/\r\n?/g,"\n"),replacement:(reviewField(item,["replacement","revisedText","modifiedText","修改后","改写后","修改后文本","改文"])??"").replace(/\r\n?/g,"\n"),reason:reviewField(item,["reason","explanation","comment","修改意见","修改理由","理由"])??"",paragraph:Number.isSafeInteger(number)&&Number(number)>0?number:undefined};
  })};
}
export function parseReviewResult(text: string): ReviewResult | null {
  for(const candidate of reviewJSONCandidates(text)) {
    try {const result=reviewShape(JSON.parse(normalizeReviewJSON(candidate)));if(result)return result;}catch{/* Try the next complete object without altering prose. */}
  }
  return null;
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
