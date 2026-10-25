import { useState } from "react";
import { parseSceneMap, sceneMapSVG } from "./scene-map";
export default function SceneMapPreview({ text, onDownload, onSave, onDelete }: { onDelete?: () => void; text: string; onDownload: (svg: string, name: string) => void; onSave?: (svg:string,name:string)=>void }) {
  const [expanded, setExpanded] = useState(false), [zoom, setZoom] = useState(100);
  try {
    const map = parseSceneMap(text), svg = sceneMapSVG(map), src = "data:image/svg+xml;charset=utf-8," + encodeURIComponent(svg);
    return <div className={expanded ? "scene-map-preview expanded" : "scene-map-preview"} role={expanded ? "dialog" : undefined} aria-label={expanded ? "场景地图大图" : "场景地图"} aria-modal={expanded ? true : undefined}>
      <div className="inline"><b>{map.title}</b><button className="text-button" onClick={() => setExpanded(!expanded)}>{expanded ? "关闭大图" : "放大查看"}</button><button className="text-button" onClick={() => onDownload(svg, map.title.replace(/[<>:"/\\|?*\r\n]/g, "_") + ".svg")}>导出 SVG</button></div>
      {onSave && <button className="text-button" onClick={()=>onSave(svg,map.title+".svg")}>保存到图片</button>}
      {onDelete && <button className="text-button image-delete" onClick={() => { setExpanded(false); onDelete(); }}>删除生成图片</button>}
      {expanded && <label>缩放 <input aria-label="地图缩放" type="range" min="50" max="200" value={zoom} onChange={e => setZoom(Number(e.target.value))} />{zoom}%</label>}
      <div className="scene-map-canvas"><img src={src} alt={map.title + "：场景地点与连接关系"} style={expanded ? { width: `${zoom}%`, maxWidth: "none" } : { width: "100%" }} /></div>
      <p className="muted">{map.note || "位置与距离为 AI 布局示意，请结合设定核对。"}</p>
    </div>;
  } catch (e) { return <p className="info-note">地图暂时无法展示：{(e as Error).message}。可复制原始结果，或要求模型按地图格式重新生成。</p>; }
}
