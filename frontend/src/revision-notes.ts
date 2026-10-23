import { Mark } from "@tiptap/core";
import type { Node } from "@tiptap/pm/model";
import type { EditorState, Transaction } from "@tiptap/pm/state";
import { closeHistory } from "@tiptap/pm/history";

export const RevisionNote = Mark.create({
  name: "revisionNote",
  inclusive: false,
  addAttributes() {
    return {
      id: { default: "", parseHTML: el => el.getAttribute("data-revision-id") },
      note: { default: "", parseHTML: el => el.getAttribute("data-revision-note") || "" },
      resolved: { default: false, parseHTML: el => el.getAttribute("data-revision-resolved") === "true" },
    };
  },
  parseHTML() { return [{ tag: "span[data-revision-id]" }]; },
  renderHTML({ mark }) {
    return ["span", {
      "data-revision-id": mark.attrs.id,
      "data-revision-note": mark.attrs.note,
      "data-revision-resolved": String(mark.attrs.resolved),
      class: "revision-note",
      title: (mark.attrs.resolved ? "已处理：" : "待修改：") + (mark.attrs.note || "点击查看修改标记"),
    }, 0];
  },
});

export type RevisionNoteEntry = { id: string; note: string; resolved: boolean; from: number; to: number; quote: string };
export function revisionNotes(doc: Node): RevisionNoteEntry[] {
  const found = new Map<string, RevisionNoteEntry>();
  doc.descendants((node, pos) => {
    if (!node.isInline) return;
    const mark = node.marks.find(m => m.type.name === "revisionNote" && m.attrs.id);
    if (!mark) return;
    const existing = found.get(mark.attrs.id);
    if (existing) { existing.to = pos + node.nodeSize; existing.quote += node.isText ? node.text : "\n"; }
    else found.set(mark.attrs.id, { id: mark.attrs.id, note: mark.attrs.note || "", resolved: !!mark.attrs.resolved, from: pos, to: pos + node.nodeSize, quote: node.text || "\n" });
  });
  return [...found.values()].map(note => ({ ...note, quote: doc.textBetween(note.from, note.to, "\n") }));
}

// Update every fragment of a mark, including fragments split by typing/formatting.
export function changeRevisionNote(state: EditorState, id: string, patch: { note?: string; resolved?: boolean } | null): Transaction {
  const tr = closeHistory(state.tr);
  state.doc.descendants((node, pos) => {
    if (!node.isInline) return;
    for (const mark of node.marks) {
      if (mark.type.name !== "revisionNote" || mark.attrs.id !== id) continue;
      tr.removeMark(pos, pos + node.nodeSize, mark);
      if (patch) tr.addMark(pos, pos + node.nodeSize, mark.type.create({ ...mark.attrs, ...patch }));
    }
  });
  return tr;
}

export function canMarkRevision(state: EditorState): boolean {
  const { from, to, empty } = state.selection;
  if (empty || !state.doc.textBetween(from, to, "\n").trim()) return false;
  let overlaps = false;
  state.doc.nodesBetween(from, to, node => {
    if (node.marks.some(m => m.type.name === "revisionNote")) overlaps = true;
  });
  return !overlaps;
}
