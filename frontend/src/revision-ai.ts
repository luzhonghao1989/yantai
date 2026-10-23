import type { Node } from "@tiptap/pm/model";
import { Fragment, Slice } from "@tiptap/pm/model";
import type { EditorState } from "@tiptap/pm/state";
import { closeHistory } from "@tiptap/pm/history";
import { revisionNotes, type RevisionNoteEntry } from "./revision-notes.ts";

export type RevisionReplacement = { id: string; text: string };
export function revisionTasks(doc: Node, excluded: string[] = []): RevisionNoteEntry[] {
  const tasks = revisionNotes(doc).filter(n => !n.resolved && !excluded.includes(n.id));
  if (!tasks.length) throw Error("请先在正文添加待修改标记，并勾选本次要修改的内容。");
  if (tasks.some(n => !n.note.trim())) throw Error("请先为勾选的标记填写修改意见。");
  for (const task of tasks) {
    let fragmented = false;
    doc.nodesBetween(task.from, task.to, node => {
      if (node.isInline && !node.marks.some(m => m.type.name === "revisionNote" && m.attrs.id === task.id)) fragmented = true;
    });
    if (fragmented) throw Error("有标记已分散为不连续片段，请重新标记后再生成，避免影响未标记的正文。");
  }
  return tasks;
}

export function revisionInstruction(tasks: RevisionNoteEntry[]) {
  return '这是实际改稿任务，不是摘录或审稿。逐条执行作者note中的修改意见，quote只用于定位原文，严禁把quote原样复制为text。保留不涉及修改意见的事实；作者明确要求修改的内容必须按意见改写，若与其他设定冲突则不要臆造解决。每处只返回该标记范围的新正文，不返回整章，不包含范围外的文字。返回 JSON：{"replacements":[{"id":"原标记ID","text":"修改后正文"}]}。每个ID必须恰好一次，不得遗漏或新增，text不能空白，不添加Markdown或解释。返回前逐项对照原文，确认改文有实际变化且回应修改意见。\n修改标记（quote是原文，note是作者修改意见）：\n'
    + JSON.stringify(tasks.map(({ id, quote, note }) => ({ id, quote, note })));
}

export function parseRevisionReplacements(text: string, tasks: RevisionNoteEntry[]): RevisionReplacement[] {
  const json = text.trim().replace(/^```(?:json)?\s*\n([\s\S]*?)\n```\s*$/i, "$1");
  let data;
  try { data = JSON.parse(json); } catch { throw Error("模型未返回有效的逐处修改结果，请重新生成。正文未改动。"); }
  if (!Array.isArray(data?.replacements) || data.replacements.length !== tasks.length) throw Error("模型返回的标记数量不一致，请重新生成。正文未改动。");
  const found = new Map<string, string>();
  for (const item of data.replacements) {
    if (!item || typeof item.id !== "string" || !tasks.some(t => t.id === item.id) || found.has(item.id) || typeof item.text !== "string" || !item.text.trim())
      throw Error("模型返回了未知、重复或空白的标记结果，请重新生成。正文未改动。");
    found.set(item.id, item.text.replace(/\r\n?/g, "\n"));
  }
  return tasks.map(t => ({ id: t.id, text: found.get(t.id)! }));
}

// Whitespace-only changes must not silently resolve an author's revision request.
export function revisionHasChange(before: string, after: string) {
  return before.replace(/\s/gu, "") !== after.replace(/\s/gu, "");
}
export function changedRevisionReplacements(text: string, tasks: RevisionNoteEntry[]) {
  return parseRevisionReplacements(text, tasks).filter(r => revisionHasChange(tasks.find(t => t.id === r.id)!.quote, r.text));
}
export function canAcceptRevision(text: string, tasks: RevisionNoteEntry[],appliedIds:string[]=[]) {
  try { return changedRevisionReplacements(text, tasks).some(r=>!appliedIds.includes(r.id)); } catch { return false; }
}

// Anchors after our own earlier replacements can move; ID + quote + opinion must still match.
export function remainingRevisionTasks(doc: Node,tasks:RevisionNoteEntry[]){
  const notes=revisionNotes(doc);
  return tasks.flatMap(task=>{
    const current=notes.find(n=>n.id===task.id);
    if(current?.resolved)return [];
    if(!current || current.quote!==task.quote || current.note!==task.note)throw Error("标记原文或修改意见已变化，请重新生成。");
    return [current];
  });
}

// Replace disjoint ranges from the end in one undoable transaction.
export function applyRevisionReplacements(state: EditorState, tasks: RevisionNoteEntry[], replacements: RevisionReplacement[]) {
  const current = revisionTasks(state.doc, revisionNotes(state.doc).filter(n => !tasks.some(t => t.id === n.id)).map(n => n.id));
  if (JSON.stringify(current) !== JSON.stringify(tasks)) throw Error("标记或修改意见已变化，请重新生成。");
  const checked = parseRevisionReplacements(JSON.stringify({ replacements }), tasks);
  const changed = checked.filter(r => revisionHasChange(tasks.find(t => t.id === r.id)!.quote, r.text));
  if (!changed.length) throw Error("模型未实际修改原文，不能采纳。标记仍待修改，请补充意见后重新生成。");
  let tr = closeHistory(state.tr);
  for (const task of tasks.filter(t => changed.some(r => r.id === t.id)).sort((a, b) => b.from - a.from)) {
    const text = checked.find(r => r.id === task.id)!.text;
    const marks = (state.doc.nodeAt(task.from)?.marks || state.doc.resolve(task.from).marks()).filter(m => m.type.name !== "revisionNote");
    marks.push(state.schema.marks.revisionNote.create({ id: task.id, note: task.note, resolved: true }));
    const paragraphs = text.split("\n").map(line => state.schema.nodes.paragraph.create(null, line ? state.schema.text(line, marks) : undefined));
    tr = tr.replaceRange(task.from, task.to, new Slice(Fragment.fromArray(paragraphs), 1, 1));
  }
  return tr.scrollIntoView();
}
