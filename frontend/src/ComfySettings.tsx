import { useRef, useState } from "react";
import { api, type Config, type ComfyConfig } from "./types";
import { comfyTextInputs, inferComfyBindings, parseComfyWorkflow } from "./comfy-workflow";
import flux from "./workflows/flux3.json";
import qwen from "./workflows/qwen21.json";

const defaults: ComfyConfig = { baseUrl:"http://127.0.0.1:8188", apiKey:"", hasKey:false, workflow:"", positiveNode:"", positiveInput:"text", negativeNode:"", negativeInput:"text", outputNode:"" };
export default function ComfySettings({ config, onChange, onSaved, notify }: { config: Config; onChange: (c:ComfyConfig)=>void; onSaved:(c:Config)=>void; notify:(s:string)=>void }) {
  const c = { ...defaults, ...config.comfy, baseUrl:config.comfy?.baseUrl || defaults.baseUrl };
  const [busy,setBusy] = useState(false), [message,setMessage] = useState("");
  const file = useRef<HTMLInputElement>(null);
  let nodes: ReturnType<typeof parseComfyWorkflow> = {};
  try { if(c.workflow) nodes = parseComfyWorkflow(c.workflow); } catch { /* Displayed when saving/importing. */ }
  const fields = comfyTextInputs(nodes);
  const set = (patch:Partial<ComfyConfig>) => { onChange({...c,...patch});setMessage(""); };
  const load = (text:string) => { try { const data=parseComfyWorkflow(text); set({workflow:JSON.stringify(data,null,2),...inferComfyBindings(data)});notify("工作流已载入，请确认提示词和输出节点，然后保存"); } catch(e){notify((e as Error).message);} };
  const save = async(test=false) => {setBusy(true);setMessage("");try { const saved=await api<Config>("/settings","PUT",{...config,comfy:c});onSaved(saved); if(test){const result=await api<{message:string}>("/comfyui/test","POST",{});setMessage(result.message);}else setMessage("ComfyUI 配置已保存"); }catch(e){setMessage((e as Error).message);}finally{setBusy(false);} };
  const selectField = (value:string,negative=false) => {const field=fields.find(f=>JSON.stringify([f.id,f.field])===value);set(negative ? {negativeNode:field?.id||"",negativeInput:field?.field||"text"} : {positiveNode:field?.id||"",positiveInput:field?.field||"text"});};
  return <section className="card comfy-settings">
    <div className="card-heading"><h3>图片生成（ComfyUI）</h3><span className="tag">自有工作流</span></div>
    <p className="muted">先启动 ComfyUI，配置服务地址与工作流。可选 Flux3 / Qwen2.1 文生图方案，也可导入自定义工作流；模型、LoRA、尺寸及采样参数由工作流决定。</p>
    <label>ComfyUI 服务地址<input aria-label="ComfyUI 服务地址" value={c.baseUrl} onChange={e=>set({baseUrl:e.target.value})} placeholder="http://192.168.0.234:19897" /></label>
    <label>访问密钥（可选）<input aria-label="ComfyUI 访问密钥" type="password" value={c.apiKey} onChange={e=>set({apiKey:e.target.value})} placeholder={c.hasKey?"已保存，留空保留":"本地服务通常无需填写；用于 Bearer 认证网关"} /></label>
    <div className="inline"><button className="btn secondary" disabled={busy} onClick={()=>load(JSON.stringify(flux))}>使用 Flux3 文生图</button><button className="btn secondary" disabled={busy} onClick={()=>load(JSON.stringify(qwen))}>使用 Qwen2.1 文生图</button><button className="btn secondary" disabled={busy} onClick={()=>file.current?.click()}>导入 API 工作流</button></div>
    <input type="file" ref={file} accept=".json,application/json" hidden onChange={async e=>{const selected=e.target.files?.[0];e.target.value="";if(!selected)return;if(selected.size>2*1024*1024){notify("工作流最多 2 MiB");return;}load(await selected.text());}} />
    <details className="comfy-workflow-editor"><summary>工作流 JSON · {Object.keys(nodes).length} 个节点</summary><p className="muted">自定义工作流请在 ComfyUI 使用“导出 API”；普通画布 JSON（含 nodes、links、子图）请先在 ComfyUI 打开后导出 API。内置方案已将子图展开。</p><textarea aria-label="ComfyUI 工作流 JSON" rows={12} spellCheck={false} value={c.workflow} onChange={e=>set({workflow:e.target.value})}/></details>
    <label>正向提示词字段<select aria-label="正向提示词字段" value={JSON.stringify([c.positiveNode,c.positiveInput||"text"])} onChange={e=>selectField(e.target.value)}><option value={JSON.stringify(["","text"])}>请选择提示词字段</option>{fields.map(f=><option key={JSON.stringify([f.id,f.field])} value={JSON.stringify([f.id,f.field])}>{f.label}</option>)}</select></label>
    <label>负向提示词字段<select aria-label="负向提示词字段" value={JSON.stringify([c.negativeNode,c.negativeInput||"text"])} onChange={e=>selectField(e.target.value,true)}><option value={JSON.stringify(["","text"])}>不使用负向提示词</option>{fields.map(f=><option key={JSON.stringify([f.id,f.field])} value={JSON.stringify([f.id,f.field])}>{f.label}</option>)}</select></label>
    <label>图片输出节点<select aria-label="图片输出节点" value={c.outputNode} onChange={e=>set({outputNode:e.target.value})}><option value="">读取所有图片输出节点</option>{Object.entries(nodes).map(([id,n])=><option key={id} value={id}>{id} · {n._meta?.title||n.class_type}</option>)}</select></label>
    <div className="inline"><button className="btn" disabled={busy} onClick={()=>void save()}>保存生图配置</button><button className="btn secondary" disabled={busy} onClick={()=>void save(true)}>{busy?"处理中……":"保存并测试 ComfyUI"}</button></div>
    {message&&<p className="info-note" role="status">{message}</p>}
  </section>;
}
