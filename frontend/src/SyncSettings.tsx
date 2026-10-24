import {useCallback,useEffect,useRef,useState} from "react";
import {api} from "./types";
export type SyncConfig={serverUrl:string;token:string;tlsFingerprint?:string;hasToken:boolean;automatic:boolean;intervalMinutes:number};
export type SyncItem={id:string;name:string;action:string;message:string;localHash?:string;remoteHash?:string};
export type SyncReport={startedAt:string;finishedAt:string;serverUrl:string;items:SyncItem[]};
type Resolution={id:string;direction:"upload"|"download";localHash:string;remoteHash:string};
export type SyncMarker={id?:string;seq:number};
export function useSyncClient(prepare:()=>Promise<SyncMarker>,complete:(report:SyncReport,marker:SyncMarker)=>Promise<void>,notify:(message:string)=>void){
 const [settings,setSettings]=useState<SyncConfig|null>(null),[remote,setRemote]=useState(false),[last,setLast]=useState<SyncReport|null>(null),[busy,setBusy]=useState(false),[error,setError]=useState("");
 const active=useRef(false),callbacks=useRef({prepare,complete,notify});callbacks.current={prepare,complete,notify};
 useEffect(()=>{let mounted=true;api<{remote:boolean;settings?:SyncConfig;last?:SyncReport}>("/sync/settings").then(v=>{if(mounted){setRemote(v.remote);setSettings(v.settings||null);setLast(v.last||null)}}).catch(e=>{if(mounted)setError(e.message)});return()=>{mounted=false}},[]);
 const run=useCallback(async(resolutions:Resolution[]=[])=>{
  if(active.current)return;active.current=true;setBusy(true);setError("");
  try{
   const marker=await callbacks.current.prepare();
   const report=await api<SyncReport>("/sync/run","POST",{resolutions});setLast(report);
   await callbacks.current.complete(report,marker);
   const count=(action:string)=>report.items.filter(i=>i.action===action).length;
   callbacks.current.notify(`同步完成：上传 ${count("uploaded")}，下载 ${count("downloaded")}，冲突 ${count("conflict")}，失败 ${count("error")}。`);
  }catch(e){setError((e as Error).message);callbacks.current.notify("同步未完成："+(e as Error).message)}finally{active.current=false;setBusy(false)}
 },[]);
 useEffect(()=>{if(remote||!settings?.automatic)return;const timer=setInterval(()=>void run(),settings.intervalMinutes*60000);return()=>clearInterval(timer)},[remote,settings?.automatic,settings?.intervalMinutes,run]);
 const save=async(value:SyncConfig)=>{const saved=await api<SyncConfig>("/sync/settings","PUT",value);setSettings(saved);setError("");notify("服务端同步设置已保存")};
 return {settings,remote,last,busy,error,run,save};
}
export default function SyncSettings({client}:{client:ReturnType<typeof useSyncClient>}){
 const [form,setForm]=useState<SyncConfig>({serverUrl:"",token:"",hasToken:false,automatic:false,intervalMinutes:5}),[saving,setSaving]=useState(false),[error,setError]=useState("");
 useEffect(()=>{if(client.settings)setForm(client.settings)},[client.settings]);
 if(client.remote)return <div className="card"><h3>服务端工作台</h3><p className="muted">这里的修改直接保存在服务器。请在 Yantai.exe 的“服务端同步”中填写服务器的 HTTPS 同步端口地址、访问密钥和必要的证书指纹，即可同步到电脑。</p></div>;
 return <section className="card sync-settings"><div className="card-heading"><h3>服务端同步</h3>{client.busy&&<span className="tag">同步中</span>}</div>
 <p className="muted">同步全部小说项目及图片；本机 AI 密钥、模型地址等设置保持独立。定时同步仅在客户端运行期间执行。</p>
 <label>服务端地址<input aria-label="同步服务端地址" placeholder="https://服务器地址:48120" value={form.serverUrl} disabled={client.busy} onChange={e=>setForm({...form,serverUrl:e.target.value})}/></label>
 <label>服务端证书 SHA-256 指纹<input aria-label="同步证书指纹" placeholder="私有证书请填写服务端启动时输出的指纹" value={form.tlsFingerprint||""} disabled={client.busy} onChange={e=>setForm({...form,tlsFingerprint:e.target.value})}/></label>
 <p className="muted">仅支持 HTTPS 加密连接。自动生成的私有证书需核对指纹；使用有效的域名证书时可留空。同步端口不提供网页。</p>
 <label>服务端访问密钥<input aria-label="同步访问密钥" type="password" autoComplete="off" placeholder={form.hasToken?"已保存，留空沿用":"部署时的 YANTAI_SERVER_TOKEN"} value={form.token} disabled={client.busy} onChange={e=>setForm({...form,token:e.target.value})}/></label>
 <label className="inline"><input type="checkbox" checked={form.automatic} disabled={client.busy} onChange={e=>setForm({...form,automatic:e.target.checked})}/>启用定时同步</label>
 <label>同步间隔（分钟）<input aria-label="同步间隔" type="number" min={1} max={10080} value={form.intervalMinutes} disabled={client.busy} onChange={e=>setForm({...form,intervalMinutes:Number(e.target.value)})}/></label>
 <div className="inline"><button className="btn secondary" disabled={saving||client.busy} onClick={async()=>{setSaving(true);setError("");try{await client.save(form)}catch(e){setError((e as Error).message)}finally{setSaving(false)}}}>{saving?"保存中……":"保存同步设置"}</button>
 <button className="btn" disabled={client.busy||!client.settings?.serverUrl} onClick={()=>void client.run()}>{client.busy?"正在同步……":"立即同步"}</button></div>
 {(error||client.error)&&<p className="info-note" role="alert">{error||client.error}</p>}
 {client.last&&<div className="sync-report"><h4>上次同步 · {new Date(client.last.finishedAt||client.last.startedAt).toLocaleString("zh-CN")}</h4><p className="muted">{client.last.serverUrl}</p>
 {!client.last.items.length&&<p>暂无项目需要同步。</p>}
 {client.last.items.map(item=><div className="sync-item" key={item.id}><strong>{item.name}</strong><p className={item.action==="conflict"||item.action==="error"?"info-note":"muted"}>{item.message}</p>
 {item.action==="conflict"&&client.settings?.serverUrl===client.last?.serverUrl&&<div className="inline"><button className="btn secondary" disabled={client.busy||!item.localHash} onClick={()=>void client.run([{id:item.id,direction:"upload",localHash:item.localHash||"",remoteHash:item.remoteHash||""}])}>保留本机，上传</button><button className="btn secondary" disabled={client.busy||!item.remoteHash} onClick={()=>void client.run([{id:item.id,direction:"download",localHash:item.localHash||"",remoteHash:item.remoteHash||""}])}>使用服务端，下载</button></div>}
 </div>)}</div>}
 </section>;
}
