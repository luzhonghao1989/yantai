import type { Project } from "./types";

const object = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v);
function equal(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (Array.isArray(a) && Array.isArray(b)) return a.length === b.length && a.every((v, i) => equal(v, b[i]));
  if (object(a) && object(b)) {
    const keys = [...new Set([...Object.keys(a), ...Object.keys(b)])];
    return keys.every(key => equal(a[key], b[key]));
  }
  return false;
}

// Compare against the last confirmed database snapshot. A deletion is a change,
// so delete-vs-edit must be reported rather than resurrecting or losing a record.
export function mergeProjects(base: Project, local: Project, remote: Project): { project: Project; conflicts: string[] } {
  if (base.id !== local.id || local.id !== remote.id) throw Error("不能合并不同项目");
  const conflicts: string[] = [];
  const merge = (b: unknown, l: unknown, r: unknown, path: string): unknown => {
    if (equal(l, r) || equal(b, r)) return structuredClone(l);
    if (equal(b, l)) return structuredClone(r);
    if (object(b) && object(l) && object(r)) {
      const result: Record<string, unknown> = Object.create(null);
      for (const key of new Set([...Object.keys(b), ...Object.keys(l), ...Object.keys(r)])) {
        const value = merge(b[key], l[key], r[key], path ? `${path}.${key}` : key);
        if (value !== undefined) result[key] = value;
      }
      return result;
    }
    if (Array.isArray(b) && Array.isArray(l) && Array.isArray(r)) {
      const key = ["id", "stageId"].find(k => [...b, ...l, ...r].every(v => object(v) && typeof v[k] === "string") && [b, l, r].every(list => new Set(list.map(v => v[k])).size === list.length));
      if (key) {
        const ids = (list: Record<string, unknown>[]) => list.map(v => v[key] as string);
        const bi = ids(b), li = ids(l), ri = ids(r);
        const changedOrder = (side: string[]) => !equal(side.filter(id => bi.includes(id)), bi.filter(id => side.includes(id)));
        const localOrder = changedOrder(li), remoteOrder = changedOrder(ri);
        if (localOrder && remoteOrder && !equal(li.filter(id => ri.includes(id)), ri.filter(id => li.includes(id)))) conflicts.push(`${path}（顺序）`);
        const order = [...new Set(localOrder ? [...li, ...ri] : [...ri, ...li])];
        const maps = [b, l, r].map(list => new Map(list.map(v => [v[key], v])));
        return order.map(id => {
          const record = maps[1].get(id) || maps[2].get(id) || maps[0].get(id);
          return merge(maps[0].get(id), maps[1].get(id), maps[2].get(id), `${path}[${String(record?.title || record?.name || id)}]`);
        }).filter(v => v !== undefined);
      }
    }
    conflicts.push(path);
    return structuredClone(l);
  };
  const metadata = { revision: remote.revision, updatedAt: remote.updatedAt };
  const project = merge({ ...base, ...metadata }, { ...local, ...metadata }, remote, "") as Project;
  // Individually valid snapshots can combine into dangling references (for
  // example, deleting a stage while the other side adds a chapter in it).
  if (Array.isArray(base.volumes) && Array.isArray(base.stages)) {
    const volumes = new Set(project.volumes.map(v => v.id)), stages = new Set(project.stages.map(v => v.id)), entities = new Set(project.entities.map(v => v.id)), chapters = new Set(project.chapters.map(v => v.id));
    const bindings = [...project.volumes, ...project.stages, ...project.chapters];
    if (!volumes.size || !stages.size || project.stages.some(v => !volumes.has(v.volumeId)) || project.chapters.some(v => !volumes.has(v.volumeId) || !stages.has(v.stageId))) conflicts.push("卷、阶段或章节关联需要核对");
    if (bindings.some(v => (v.entityIds || []).some(id => !entities.has(id))) || project.chapters.some(v => (v.excludedIds || []).some(id => !entities.has(id))) || project.entities.some(v => v.versions.some(version => !stages.has(version.stageId)))) conflicts.push("人物、物品或场景绑定需要核对");
    if (project.storylines.some(v => v.beats.some(beat => beat.chapterId && !chapters.has(beat.chapterId)))) conflicts.push("故事线引用的章节需要核对");
  }
  if ((project.images || []).length > 200 || (project.images || []).reduce((total, image) => total + (image.size || 0), 0) > 32 * 1024 * 1024) conflicts.push("合并后的图片超过项目容量");
  return { project, conflicts };
}

export function describeSaveConflict(path: string): string {
  const labels: Record<string, string> = { chapters: "章节", text: "正文", content: "正文格式", images: "图片", aiRuns: "AI 记录", entities: "设定", versions: "阶段版本", traits: "性格", likes: "喜好", name: "名称", title: "标题", outline: "大纲", summary: "前情提要", volumes: "卷", stages: "阶段", storylines: "故事线", beats: "事件", sources: "素材", description: "简介", note: "备注", profile: "基础档案", gender: "性别", birthplace: "出生地", entityIds: "资料绑定", excludedIds: "排除资料" };
  return path.replace(/(^|\.)([a-zA-Z]+)(?=\.|\[|$)/g, (_match, prefix: string, key: string) => prefix + (labels[key] || key));
}
