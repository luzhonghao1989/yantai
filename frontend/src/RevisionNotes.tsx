import { useEffect, useState } from "react";
import type { Editor } from "@tiptap/react";
import { closeHistory } from "@tiptap/pm/history";
import { canMarkRevision, changeRevisionNote, revisionNotes } from "./revision-notes";

export default function RevisionNotes({ editor, selected, onSelect, onClose }: { editor: Editor; selected: string; onSelect: (id: string) => void; onClose: () => void }) {
  const [, refresh] = useState(0);
  const [showResolved, setShowResolved] = useState(false);
  useEffect(() => { const update = () => refresh(n => n + 1); editor.on("transaction", update); return () => { editor.off("transaction", update); }; }, [editor]);
  const notes = revisionNotes(editor.state.doc), pending = notes.filter(n => !n.resolved).length;
  useEffect(() => {
    if (revisionNotes(editor.state.doc).some(n => n.id === selected && n.resolved)) setShowResolved(true);
  }, [editor, selected]);
  const commit = (tr: ReturnType<typeof changeRevisionNote>) => {
    editor.view.dispatch(tr);
    editor.view.dispatch(closeHistory(editor.state.tr));
  };
  const add = () => {
    if (!canMarkRevision(editor.state)) return;
    const id = crypto.randomUUID();
    commit(closeHistory(editor.state.tr).addMark(editor.state.selection.from, editor.state.selection.to, editor.schema.marks.revisionNote.create({ id, note: "", resolved: false })));
    onSelect(id);
  };
  return <section className="revision-notes-panel" aria-label="修改标记面板">
    <div className="revision-notes-heading">
      <b>修改标记 <span className="muted">{pending} 处待修改</span></b>
      <button className="btn secondary" disabled={!canMarkRevision(editor.state)} onMouseDown={e => e.preventDefault()} onClick={add}>标记选中文字</button>
      <label><input type="checkbox" checked={showResolved} onChange={e => setShowResolved(e.target.checked)} />显示已处理</label>
      <button className="icon" aria-label="关闭修改标记" onClick={onClose}>×</button>
    </div>
    <p className="muted">选中正文后点击标记，再填写修改说明。点击高亮或下方原文可以定位；标记随正文编辑移动，支持撤销。</p>
    <div className="revision-notes-list">
      {notes.filter(n => showResolved || !n.resolved).map(n => <div key={n.id} className={"revision-note-card" + (selected === n.id ? " selected" : "")}>
        <button className="revision-note-quote" title="定位正文" onClick={() => { onSelect(n.id); editor.chain().focus().setTextSelection({ from: n.from, to: n.to }).scrollIntoView().run(); }}>{n.resolved ? "已处理 · " : "待修改 · "}{n.quote}</button>
        <textarea aria-label={"修改说明：" + n.quote} placeholder="例如：这里的对白需要更自然，补充人物动作……" rows={2} value={n.note} onChange={e => commit(changeRevisionNote(editor.state, n.id, { note: e.target.value }))} />
        <div className="revision-note-actions"><button className="text-button" onClick={() => commit(changeRevisionNote(editor.state, n.id, { resolved: !n.resolved }))}>{n.resolved ? "重新待修改" : "标为已处理"}</button><button className="text-button" onClick={() => commit(changeRevisionNote(editor.state, n.id, null))}>移除标记</button></div>
      </div>)}
      {!notes.some(n => showResolved || !n.resolved) && <p className="muted">{notes.length ? "待修改内容已处理完，可勾选显示已处理。" : "本章还没有修改标记。选中需要修改的文字开始标记。"}</p>}
    </div>
  </section>;
}
