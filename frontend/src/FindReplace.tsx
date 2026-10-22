import { useEffect, useState } from "react";
import type { Editor } from "@tiptap/react";
import { TextSelection } from "@tiptap/pm/state";

export default function FindReplace({editor,onClose}:{editor:Editor;onClose:()=>void}) {
 const [query,setQuery]=useState(""),[replacement,setReplacement]=useState(""),[,setRevision]=useState(0);
 useEffect(()=>{const refresh=()=>setRevision(v=>v+1);editor.on("transaction",refresh);return()=>{editor.off("transaction",refresh)}},[editor]);
 const matches:{from:number;to:number}[]=[];
 if(query) editor.state.doc.descendants((node,pos)=>{
  if(!node.isTextblock) return;
  const text=node.textBetween(0,node.content.size,"\uFFFC","\uFFFC");let start=0,index=0;
  while((index=text.indexOf(query,start))>=0){matches.push({from:pos+1+index,to:pos+1+index+query.length});start=index+query.length}
  return false;
 });
 const current=matches.findIndex(m=>m.from===editor.state.selection.from&&m.to===editor.state.selection.to);
 const next=()=>{const m=matches.find(m=>m.from>=editor.state.selection.to)||matches[0];if(m)editor.chain().focus().setTextSelection(m).scrollIntoView().run()};
 const replace=(all:boolean)=>{
  if(!matches.length) return;
  const targets=all?matches:[matches[current>=0?current:0]];
  let tr=editor.state.tr;
  for(const m of [...targets].reverse())tr=tr.insertText(replacement,m.from,m.to);
  const first=targets[0];tr.setSelection(TextSelection.create(tr.doc,first.from,first.from+replacement.length));
  editor.view.dispatch(tr.scrollIntoView());editor.view.focus();
 };
 return <div className="find-replace">
  <div className="inline"><input aria-label="查找正文" placeholder="查找内容（当前章）" value={query} onChange={e=>setQuery(e.target.value)} onKeyDown={e=>{if(e.key==="Enter"){e.preventDefault();next()}}}/><span>{matches.length} 处</span><button className="btn secondary" disabled={!matches.length} onClick={next}>下一处</button><button className="icon" aria-label="关闭查找替换" onClick={onClose}>×</button></div>
  <div className="inline"><input aria-label="替换为" placeholder="替换为（留空即删除）" value={replacement} onChange={e=>setReplacement(e.target.value)}/><button className="btn secondary" disabled={!matches.length} onClick={()=>replace(false)}>替换此处</button><button className="btn secondary" disabled={!matches.length} onClick={()=>replace(true)}>全部替换</button></div>
  <small className="muted">按原文精确匹配，整章替换也可通过撤销恢复。</small>
 </div>;
}
