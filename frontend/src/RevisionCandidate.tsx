import type { RevisionNoteEntry } from "./revision-notes";
import { parseRevisionReplacements, revisionHasChange } from "./revision-ai";
import AIContent from "./AIContent";
import TextDiff from "./TextDiff";
import type { LocateSource } from "./source-location";

export default function RevisionCandidate({ text, tasks, complete, retrying, appliedIds=[],onReplace,onLocate }: { text: string; tasks: RevisionNoteEntry[]; complete: boolean; retrying?: boolean; appliedIds?:string[];onReplace?:(id:string)=>void;onLocate?:LocateSource }) {
  if (!complete) return <p className="muted">{retrying ? "模型返回了未修改的内容，正在自动重试一次……" : `正在按 ${tasks.length} 处标记生成修改，完成后展示逐处对比……`}</p>;
  try {
    const replacements = parseRevisionReplacements(text, tasks);
    return <div className="revision-candidate">{tasks.map((task, i) => <section key={task.id}>
      <h4>标记 {i + 1}</h4><p className="info-note">修改意见：{task.note}</p>
      {onLocate&&!revisionHasChange(task.quote,replacements[i].text)&&<button className="source-locate" onClick={()=>onLocate(task.quote,{markId:task.id})}>定位原文</button>}
      {revisionHasChange(task.quote, replacements[i].text) ? <TextDiff before={task.quote} after={replacements[i].text} onLocate={onLocate?(text)=>onLocate(text,text===task.quote?{markId:task.id}:undefined):undefined}/> : <p className="info-note" role="alert">模型未实际修改这处原文，将保留待修改状态。请补充更具体的意见后重新生成。</p>}
      <AIContent text={replacements[i].text} />
      {onReplace&&<button className="btn secondary" disabled={appliedIds.includes(task.id)||!revisionHasChange(task.quote,replacements[i].text)} onClick={()=>onReplace(task.id)}>{appliedIds.includes(task.id)?"已替换":"替换原文"}</button>}
    </section>)}</div>;
  } catch (e) { return <p className="info-note" role="alert">{(e as Error).message}</p>; }
}
