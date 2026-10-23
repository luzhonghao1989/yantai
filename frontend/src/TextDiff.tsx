import { useMemo } from "react";
import type { Doc } from "./types";
import type { LocateSource } from "./source-location";

export function docText(doc: Doc): string {
 const blocks:string[]=[];
 const text=(n:Doc):string=>n.type==="hardBreak"?"\n":n.text || (n.content || []).map(text).join("");
 const visit=(n:Doc)=>{if(n.type==="paragraph"||n.type==="heading"){blocks.push(text(n));return}n.content?.forEach(visit)};
 visit(doc);return blocks.join("\n");
}
export default function TextDiff({before,after,onLocate}:{before:string;after:string;onLocate?:LocateSource}) {
 const lines=useMemo(()=>{
  const a=before.split("\n"),b=after.split("\n");
  if(a.length>250||b.length>250)return null;
  const dp=Array.from({length:a.length+1},()=>new Uint16Array(b.length+1));
  for(let i=a.length-1;i>=0;i--)for(let j=b.length-1;j>=0;j--)dp[i][j]=a[i]===b[j]?dp[i+1][j+1]+1:Math.max(dp[i+1][j],dp[i][j+1]);
  const out:{type:string;text:string}[]=[];let i=0,j=0;
  while(i<a.length||j<b.length){if(i<a.length&&j<b.length&&a[i]===b[j]){out.push({type:"same",text:a[i++]});j++}else if(j<b.length&&(i===a.length||dp[i][j+1]>=dp[i+1][j]))out.push({type:"added",text:b[j++]});else out.push({type:"removed",text:a[i++]})}
  return out;
 },[before,after]);
 return <details className="candidate-diff" open><summary>改写对比 · 红色删除 / 绿色新增</summary>{onLocate&&<button className="source-locate" onClick={()=>onLocate(before)}>定位原文</button>}{lines ? lines.map((line,i)=><p key={i} className={"diff-"+line.type}><span>{line.type==="added"?"+":line.type==="removed"?"−":" "}</span>{onLocate&&line.type!=="added"&&line.text.trim()?<button className="source-quote" title="点击定位这段原文；重复内容可再次点击切换" onClick={()=>onLocate(line.text)}>{line.text}</button>:line.text||" "}</p>) : <div><p className="muted">长文本改为并排预览。</p><div className="two-col"><div><h4>修改前</h4><p>{before}</p></div><div><h4>候选</h4><p>{after}</p></div></div></div>}</details>;
}
