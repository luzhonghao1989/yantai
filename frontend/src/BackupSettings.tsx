import {useEffect,useState} from "react";
import {api} from "./types";
type Backup={id:string;createdAt:string;projects:{id:string;name:string}[];errors?:string[]};
type Settings={enabled:boolean;retentionDays:number};
type State={settings:Settings;backups:Backup[];remote:boolean};
export default function BackupSettings({prepare,onRestored,notify}:{prepare:()=>Promise<unknown>;onRestored:()=>Promise<unknown>;notify:(text:string)=>void}){
 const [data,setData]=useState<State|null>(null),[form,setForm]=useState<Settings>({enabled:true,retentionDays:30}),[busy,setBusy]=useState(false),[error,setError]=useState(""),[restore,setRestore]=useState<{backup:string;project:string;name:string}|null>(null);
 const load=async()=>{const next=await api<State>("/backups");setData(next);setForm(next.settings)};
 useEffect(()=>{void load().catch(e=>setError(e.message))},[]);
 const run=async(action:()=>Promise<unknown>)=>{setBusy(true);setError("");try{await action();await load()}catch(e){setError((e as Error).message)}finally{setBusy(false)}};
 return <section className="card"><div className="card-heading"><h3>每日备份 · {data?.remote?"服务端":"本机"}</h3></div>
 <p className="muted">每天自动备份已保存的全部小说及图片，默认保留 30 天。仅在程序运行期间执行，错过后下次启动补做当天备份。备份不包含模型密钥和同步密钥。</p>
 <label className="inline"><input type="checkbox" checked={form.enabled} disabled={busy} onChange={e=>setForm({...form,enabled:e.target.checked})}/>启用每日备份</label>
 <label>保留天数<input type="number" min={1} max={36500} value={form.retentionDays} disabled={busy} onChange={e=>setForm({...form,retentionDays:Number(e.target.value)})}/></label>
 <p className="muted">保存位置：数据目录下 daily-backups。过期备份只在新备份完整成功后清理。服务端与客户端分别设置；同步范围不影响备份范围。</p>
 <div className="inline"><button className="btn secondary" disabled={busy||!data} onClick={()=>void run(async()=>{await api("/backups/settings","PUT",form);notify("每日备份设置已保存")})}>保存备份设置</button>
 <button className="btn" disabled={busy} onClick={()=>void run(async()=>{await prepare();await api("/backups","POST");notify("备份已完成")})}>{busy?"处理中……":"立即备份"}</button>
 <button className="btn secondary" disabled={busy} onClick={()=>void run(load)}>刷新</button></div>
 {error&&<p className="info-note" role="alert">{error}</p>}
 <details><summary>备份记录与恢复 · {data?.backups.length||0} 份</summary>
 <p className="muted">恢复会新建独立项目，不覆盖当前稿件，也不会恢复已删除项目的旧同步身份。旧副本可在核对后手动删除。</p>
 {data?.backups.map(b=><details key={b.id}><summary>{new Date(b.createdAt).toLocaleString("zh-CN")} · {b.projects.length} 个项目{b.errors?.length?" · 部分失败":""}</summary>
 {b.errors?.map((e,i)=><p className="info-note" key={i}>{e}</p>)}
 {b.projects.map(p=><div className="title-candidate" key={p.id}><span>{p.name}</span><button className="btn secondary" disabled={busy} onClick={()=>setRestore({backup:b.id,project:p.id,name:p.name})}>恢复为新项目</button></div>)}</details>)}
 </details>
 {restore&&<div className="info-note"><p>将从备份恢复“{restore.name}”为独立项目。</p><div className="inline"><button className="btn" disabled={busy} onClick={()=>void run(async()=>{await prepare();await api(`/backups/${restore.backup}/projects/${restore.project}/restore`,"POST");setRestore(null);await onRestored();notify("已恢复为新项目，可返回书架查看")})}>确认恢复</button><button className="btn secondary" disabled={busy} onClick={()=>setRestore(null)}>取消</button></div></div>}
 </section>;
}
