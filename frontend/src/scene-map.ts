export type SceneMap = {
  title: string; note: string;
  nodes: { id: string; label: string; kind: "room" | "place"; x: number; y: number }[];
  edges: { from: string; to: string; label: string; uncertain: boolean }[];
};
export function parseSceneMap(raw: string): SceneMap {
  const text = raw.trim().replace(/^```(?:json)?\s*/i, "").replace(/\s*```$/, "");
  const value = JSON.parse(text);
  const label = (v: unknown, limit: number, required = false): string => {
    if (v == null && !required) return "";
    if (typeof v !== "string" || v.length > limit || (required && !v.trim())) throw Error("地图名称或文字不符合格式");
    return v;
  };
  if (!value || !Array.isArray(value.nodes) || !value.nodes.length || value.nodes.length > 40 || !Array.isArray(value.edges) || value.edges.length > 80) throw Error("地图需包含 1—40 个地点及不超过 80 条连线");
  const ids = new Set<string>();
  const nodes = value.nodes.map((n: Record<string, unknown>) => {
    if (!n || typeof n !== "object") throw Error("地图地点格式错误");
    const id = label(n.id, 80, true);
    if (ids.has(id) || ![n.x, n.y].every(v => typeof v === "number" && Number.isFinite(v) && v >= 0 && v <= 100)) throw Error("地点 ID 须唯一，坐标须为 0—100 的有限数字");
    if (n.kind !== "room" && n.kind !== "place") throw Error("地图地点类型应为 room 或 place");
    ids.add(id); return { id, label: label(n.label, 60, true), kind: n.kind, x: n.x as number, y: n.y as number };
  }) as SceneMap["nodes"];
  const edges = value.edges.map((e: Record<string, unknown>) => {
    if (!e || typeof e !== "object" || !ids.has(String(e.from)) || !ids.has(String(e.to)) || e.from === e.to) throw Error("地图连线引用了无效地点");
    return { from: String(e.from), to: String(e.to), label: label(e.label, 60), uncertain: e.uncertain === true };
  });
  return { title: label(value.title, 100, true), note: label(value.note, 1000), nodes, edges };
}
const escape = (value: string) => value.replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]!);
export function sceneMapSVG(map: SceneMap): string {
  const position = (n: SceneMap["nodes"][number]) => ({ x: 130 + n.x * 9.4, y: 120 + n.y * 5.9 });
  const lines = map.edges.map(e => {
    const a = position(map.nodes.find(n => n.id === e.from)!), b = position(map.nodes.find(n => n.id === e.to)!);
    return `<g><line x1="${a.x}" y1="${a.y}" x2="${b.x}" y2="${b.y}" stroke="#7e9188" stroke-width="3"${e.uncertain ? ' stroke-dasharray="9 6"' : ""}/><text x="${(a.x + b.x) / 2}" y="${(a.y + b.y) / 2 - 12}" text-anchor="middle" font-size="16" paint-order="stroke" stroke="#faf8f2" stroke-width="6" fill="#50645b">${escape(e.label)}</text></g>`;
  }).join("");
  const nodes = map.nodes.map(n => {
    const p = position(n);
    return `<g>${n.kind === "room" ? `<rect x="${p.x - 95}" y="${p.y - 38}" width="190" height="76" rx="8" fill="#e7eee6" stroke="#456a59" stroke-width="2"/>` : `<circle cx="${p.x}" cy="${p.y}" r="10" fill="#b88a4a"/>`}<text x="${p.x}" y="${p.y + (n.kind === "room" ? 6 : 34)}" text-anchor="middle" font-size="18" fill="#253e32">${escape(n.label)}</text></g>`;
  }).join("");
  return `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="800" viewBox="0 0 1200 800" role="img" aria-label="${escape(map.title)}"><rect width="1200" height="800" fill="#faf8f2"/><g font-family="Microsoft YaHei, sans-serif"><text x="40" y="48" font-size="26" fill="#253e32">${escape(map.title)}</text><text x="40" y="78" font-size="15" fill="#687b6f">场景平面示意 · 非比例尺 · 虚线表示待确认连接</text>${lines}${nodes}</g></svg>`;
}
