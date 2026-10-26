export type Doc = {
  type: string;
  content?: Doc[];
  text?: string;
  attrs?: Record<string, unknown>;
  marks?: { type: string; attrs?: Record<string, unknown> }[];
};
export type Volume = {
  id: string;
  name: string;
  outline: string;
  entityIds: string[];
};
export type Stage = {
  id: string;
  name: string;
  volumeId: string;
  outline: string;
  entityIds: string[];
};
export type Chapter = {
  id: string;
  title: string;
  volumeId: string;
  stageId: string;
  outline: string;
  summary: string;
  content: Doc;
  text: string;
  entityIds: string[];
  excludedIds: string[];
};
export type Version = {
  stageId: string;
  traits: string;
  likes: string;
  state: string;
  knowledge: string;
  note: string;
};
export type Entity = {
  id: string;
  kind: "character" | "item" | "location";
  name: string;
  base: string;
  profile?: Record<string, string>;
  global: boolean;
  versions: Version[];
};
export type Beat = {
  id: string;
  title: string;
  chapterId: string;
  status: string;
  detail: string;
};
export type Storyline = {
  id: string;
  name: string;
  color: string;
  beats: Beat[];
};
export type Source = { id: string; title: string; text: string; url: string };
export type AIRun = {
  inputText?: string;
  id: string;
  chapterId: string;
  mode: string;
  model: string;
  text: string;
  accepted: boolean;
  createdAt: string;
};
export type Project = {
  images?: ProjectImage[];
  schema: number;
  id: string;
  name: string;
  genre: string;
  description: string;
  outline: string;
  rules: string;
  style: string;
  banned: string;
  revision: number;
  updatedAt: string;
  volumes: Volume[];
  stages: Stage[];
  chapters: Chapter[];
  entities: Entity[];
  storylines: Storyline[];
  sources: Source[];
  aiRuns: AIRun[];
};
export type ProjectImage = { id: string; name: string; kind: "map" | "character" | "scene" | "other"; note: string; sha?: string; mime?: string; data?: string; size: number; createdAt: string; sourceRunId?: string };
export type ProjectInfo = {
  error?: string;
  id: string;
  name: string;
  genre: string;
  description: string;
  updatedAt: string;
  words: number;
  chapters: number;
};
export type Config = {
  requestTimeoutMinutes?: number;
  idleTimeoutMinutes?: number;
  comfy?: ComfyConfig;
  embeddingModel?: string;
  provider: string;
  baseUrl: string;
  model: string;
  apiKey: string;
  hasKey: boolean;
  contextLimit: number;
  outputLimit: number;
  temperature: number;
  searchKey: string;
  hasSearchKey: boolean;
  detectorUrl: string;
  detectorKey: string;
  hasDetectorKey: boolean;
};
export type ComfyConfig = {
  baseUrl: string; apiKey: string; hasKey: boolean; workflow: string;
  positiveNode: string; positiveInput: string; negativeNode: string; negativeInput: string; outputNode: string;
};
export type Detection = {
  score: number | null;
  label: string;
  model: string;
  explanation: string;
  language: string;
  revision: number;
  chapterId: string;
  words: number;
};
export type ContextPackage = {
  revision: number;
  chapterId: string;
  stageId: string;
  sections: { name: string; text: string; source: string }[];
  estimatedTokens: number;
  budget: number;
  fits: boolean;
};
export type Review = {
  revision: number;
  chapterId: string;
  words: number;
  paragraphs: number;
  averageLength: number;
  issues: { kind: string; paragraph: number; quote: string; message: string }[];
  aiRecords: number;
};
export type SearchResult = {
  title: string;
  text: string;
  source: string;
  url: string;
  chapterId: string;
  score: number;
};
export const uid = () => crypto.randomUUID().replaceAll("-", "");
export const doc = (text = ""): Doc => ({
  type: "doc",
  content: text
    .split("\n")
    .map((p) => ({
      type: "paragraph",
      content: p ? [{ type: "text", text: p }] : [],
    })),
});
export const words = (text: string) =>
  Array.from(text.replace(/\s/g, "")).length;
export const blankVersion = (stageId: string): Version => ({
  stageId,
  traits: "",
  likes: "",
  state: "",
  knowledge: "",
  note: "",
});
export function resolveVersion(p: Project, e: Entity, stageId: string) {
  const current = p.stages.findIndex((s) => s.id === stageId);
  return e.versions
    .map((v) => ({ v, index: p.stages.findIndex((s) => s.id === v.stageId) }))
    .filter((x) => x.index >= 0 && x.index <= current)
    .sort((a, b) => b.index - a.index)[0]?.v;
}
export function boundEntities(p: Project, c: Chapter) {
  const ids = new Set([
    ...p.entities.filter((e) => e.global).map((e) => e.id),
    ...(p.volumes.find((v) => v.id === c.volumeId)?.entityIds || []),
    ...(p.stages.find((s) => s.id === c.stageId)?.entityIds || []),
    ...c.entityIds,
  ]);
  c.excludedIds.forEach((id) => ids.delete(id));
  return p.entities.filter((e) => ids.has(e.id));
}
let base = "",
  token = "";
export function configureAPI(info: { base: string; token: string }) {
  base = info.base;
  token = info.token;
}
export function apiURL(path: string) {
  return base + "/api" + path;
}
export function apiHeaders(json = false): Record<string, string> {
  return {
    ...(token ? { "X-Yantai-Token": token } : {}),
    ...(json ? { "Content-Type": "application/json" } : {}),
  };
}
export class APIError extends Error {
  status: number;
  constructor(message: string, status: number) { super(message); this.name = "APIError"; this.status = status; }
}
export async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const r = await fetch(apiURL(path), {
    method,
    signal,
    headers: apiHeaders(body !== undefined),
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!r.ok) {
    let msg = "请求失败";
    try {
      msg = (await r.json()).error || msg;
    } catch {
      msg += ` (${r.status})`;
    }
    throw new APIError(msg, r.status);
  }
  return r.json();
}
