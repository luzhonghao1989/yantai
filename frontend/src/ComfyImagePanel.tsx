import { useEffect, useRef, useState } from "react";
import { api, boundEntities, type Chapter, type Config, type Project, type ProjectImage } from "./types";

type ScenePrompt = { title: string; description: string; prompt: string; negativePrompt: string };

export default function ComfyImagePanel({config,project,chapter,onSettings,onPrepare,onCheck,onImages,onDelete}:{onDelete:(image:ProjectImage,onDeleted:()=>void)=>void;config:Config|null;project:Project;chapter:Chapter;onSettings:()=>void;onPrepare:()=>Promise<{revision:number;signature:string}>;onCheck:(signature:string)=>void;onImages:(images:ProjectImage[])=>Promise<void>}){
  const [requirements,setRequirements]=useState(""),[generated,setGenerated]=useState<ScenePrompt|null>(null),[seed,setSeed]=useState(""),[entityId,setEntityId]=useState(""),[kind,setKind]=useState<ProjectImage["kind"]>("scene");
  const [busy,setBusy]=useState(false),[status,setStatus]=useState(""),[images,setImages]=useState<ProjectImage[]>([]),[saved,setSaved]=useState(false);
  const abort=useRef<AbortController|null>(null);
  useEffect(()=>()=>{abort.current?.abort();},[]);
  const entities=boundEntities(project,chapter).filter(e=>e.kind==="location");
  const save=async(result:ProjectImage[])=>{await onImages(result);setSaved(true);setStatus(`已保存 ${result.length} 张图片到项目图片栏`);};
  const run=async()=>{
    if(busy)return;
    if(seed.trim() && (!/^\d+$/.test(seed.trim()) || !Number.isSafeInteger(Number(seed)))) {setStatus("种子须为非负安全整数，留空随机");return;}
    const controller=new AbortController();abort.current=controller;setBusy(true);setSaved(false);setImages([]);setGenerated(null);setStatus("1/3 正在保存本章最新正文……");
    const targetKind=kind;let requestedComfy=false;
    try{
      const snapshot=await onPrepare();
      if(controller.signal.aborted)throw new DOMException("已停止","AbortError");
      setStatus("1/3 AI 正在阅读本章正文与设定，提炼场景并编写画面提示词……");
      const scene=await api<ScenePrompt>(`/projects/${project.id}/scene-prompt`,"POST",{chapterId:chapter.id,revision:snapshot.revision,instruction:requirements,entityId},controller.signal);
      setGenerated(scene);
      if(controller.signal.aborted)throw new DOMException("已停止","AbortError");
      onCheck(snapshot.signature);
      setStatus("2/3 AI 提示词已生成，正在自动提交 ComfyUI……");
      requestedComfy=true;
      const job=await api<{id:string;seed:number}>("/comfyui/jobs","POST",{prompt:scene.prompt,negative:scene.negativePrompt,seed:seed.trim()?Number(seed):undefined},controller.signal);
      setStatus(`2/3 ComfyUI 正在绘制「${scene.title}」 · 种子 ${job.seed}`);
      const deadline=Date.now()+30*60*1000;
      while(!controller.signal.aborted){
        if(Date.now()>deadline)throw Error("等待超过 30 分钟，可到 ComfyUI 查看任务与结果");
        const result=await api<{status:string;images?:ProjectImage[]}>(`/comfyui/jobs/${job.id}`,"GET",undefined,controller.signal);
        if(result.status==="completed"){
          const promptNote=Array.from(`章节：${chapter.title}\n场景：${scene.title}\n选景依据：${scene.description}\n正向提示词：${scene.prompt}\n负向提示词：${scene.negativePrompt}`).slice(0,6000).join("");
          const title=Array.from(scene.title.replace(/[<>:"/\\|?*\r\n]/g,"_")).slice(0,60).join("");
          const output=(result.images||[]).map((image,i)=>({...image,kind:targetKind,name:`${title}-${new Date().toLocaleString("zh-CN").replace(/[/:]/g,"-")}-${i+1}${image.mime==="image/jpeg"?".jpg":image.mime==="image/gif"?".gif":".png"}`,note:`${image.note}\n${promptNote}`}));
          if(!output.length)throw Error("ComfyUI 未返回图片");setImages(output);setStatus("3/3 正在保存场景图到项目图片栏……");await save(output);break;
        }
        await new Promise<void>((resolve,reject)=>{const stop=()=>{clearTimeout(timer);reject(new DOMException("已停止等待","AbortError"));};const timer=setTimeout(()=>{controller.signal.removeEventListener("abort",stop);resolve();},1500);controller.signal.addEventListener("abort",stop,{once:true});if(controller.signal.aborted)stop();});
      }
    }catch(e){if(!controller.signal.aborted)setStatus((e as Error).message);else setStatus(requestedComfy?"已停止等待；如任务已提交，仍可在 ComfyUI 查看。":"已停止 AI 场景分析，尚未提交 ComfyUI。");}finally{if(abort.current===controller){setBusy(false);abort.current=null;}}
  };
  if(!config?.comfy?.workflow || !config.comfy.baseUrl)return <div className="info-note"><p>请先在模型与设置中配置 ComfyUI 地址和工作流。</p><button className="btn" onClick={onSettings}>配置 ComfyUI</button></div>;
  if(!config.model?.trim())return <div className="info-note"><p>请先配置 AI 文字模型。它会读取本章、自动编写场景提示词，再交给 ComfyUI 生图。</p><button className="btn" onClick={onSettings}>配置章节分析模型</button></div>;
  return <section className="comfy-image-panel">
    <p className="info-note">本章正文与设定 → AI 自动生成画面提示词 → ComfyUI 绘制 → 保存到图片栏</p>
    <label>绘制哪个场景<select aria-label="生图参考资料" value={entityId} disabled={busy} onChange={e=>setEntityId(e.target.value)}><option value="">AI 根据本章自动选景</option>{entities.map(e=><option key={e.id} value={e.id}>{e.name}</option>)}</select></label>
    <label>补充画面要求（可选）<textarea aria-label="场景图补充要求" rows={3} value={requirements} disabled={busy} onChange={e=>setRequirements(e.target.value)} placeholder="可留空；例如：画本章雨后的旧宅，以环境为主，采用俯视构图。" /></label>
    <label>图片分类<select aria-label="生图图片分类" value={kind} disabled={busy} onChange={e=>setKind(e.target.value as ProjectImage["kind"])}><option value="scene">场景</option><option value="map">地图</option><option value="character">人物</option><option value="other">其他</option></select></label>
    <label>种子<input aria-label="ComfyUI 种子" value={seed} disabled={busy} onChange={e=>setSeed(e.target.value)} placeholder="留空随机" inputMode="numeric" /></label>
    <div className="inline"><button className="btn" disabled={busy||!chapter.text.trim()} onClick={()=>void run()}>{busy?"生成中……":"根据本章生成场景图"}</button>{busy?<button className="btn secondary" onClick={()=>abort.current?.abort()}>停止</button>:<button className="text-button" onClick={onSettings}>生图设置</button>}</div>
    {status&&<p className="info-note" role="status">{status}</p>}
    {generated&&<details className="comfy-workflow-editor"><summary>查看 AI 生成的提示词 · {generated.title}</summary><p className="muted">{generated.description}</p><label>正向提示词<textarea aria-label="AI 生成的正向提示词" rows={6} readOnly value={generated.prompt}/></label><label>负向提示词<textarea aria-label="AI 生成的负向提示词" rows={2} readOnly value={generated.negativePrompt}/></label></details>}
    {images.filter(image=>!saved || (project.images||[]).some(old=>old.id===image.id)).map(image=><figure key={image.id}><img src={image.data} alt={image.name}/><figcaption>{image.name}</figcaption><button className="text-button image-delete" disabled={busy} onClick={()=>onDelete(image,()=>{setImages(current=>current.filter(i=>i.id!==image.id));setStatus("图片已删除");})}>删除生成图片</button></figure>)}
    {!!images.length&&!saved&&!busy&&<button className="btn" onClick={()=>void save(images).catch(e=>setStatus(e.message))}>重试保存到图片栏</button>}
  </section>;
}
