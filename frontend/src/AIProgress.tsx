import { useEffect, useRef, useState } from "react";

export type GenerationProgress = { thinking: string; truncated: boolean; answerStarted: boolean; startedAt: number; attempt: number };
export default function AIProgress({progress}:{progress:GenerationProgress}) {
  const [seconds,setSeconds]=useState(0),[expanded,setExpanded]=useState(true);
  const body=useRef<HTMLPreElement>(null),follow=useRef(true);
  useEffect(()=>{
    const update=()=>setSeconds(Math.max(0,Math.floor((Date.now()-progress.startedAt)/1000)));
    update();const timer=setInterval(update,1000);return()=>clearInterval(timer);
  },[progress.startedAt]);
  useEffect(()=>{if(body.current&&follow.current)body.current.scrollTop=body.current.scrollHeight;},[progress.thinking,expanded]);
  return <section className="ai-progress" aria-label="AI 生成进度">
    <div className="ai-progress-heading" role="status"><span className="ai-progress-dot"/>
      <b>{progress.answerStarted?"正在生成正式结果":progress.thinking?"模型正在思考":"等待模型响应"}</b><small>已用 {seconds} 秒{progress.attempt>0?" · 重试中":""}</small>
    </div>
    {progress.thinking?<details open={expanded} onToggle={e=>setExpanded(e.currentTarget.open)}><summary>模型思考 · 仅生成时显示</summary>
      {progress.truncated&&<small className="muted">仅展示最近的思考内容</small>}
      <pre ref={body} onScroll={e=>{const el=e.currentTarget;follow.current=el.scrollHeight-el.scrollTop-el.clientHeight<36;}}>{progress.thinking}</pre>
    </details>:<p className="muted">{progress.answerStarted?"正在接收模型的正式输出……":"请求已发送；模型返回思考内容后会在这里实时显示。"}</p>}
  </section>;
}
