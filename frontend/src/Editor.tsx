import { localSettings } from "./local-settings";
import FindReplace from "./FindReplace";
import { blankLineRanges } from "./remove-blank-lines";
import RevisionNotes from "./RevisionNotes";
import { RevisionNote } from "./revision-notes";
import { closeHistory } from "@tiptap/pm/history";
import { useEditor, EditorContent, type Editor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { TextStyle, FontFamily } from "@tiptap/extension-text-style";
import Placeholder from "@tiptap/extension-placeholder";
import {
  Bold,
  Italic,
  Undo2,
  Redo2,
  Focus,
  AlignLeft,
  Type,
  Search,
  ListFilter,
  MessageSquareText,
} from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { Chapter, Doc } from "./types";
export type Selection = { from: number; to: number; text: string };
type Props = {
  chapter: Chapter;
  projectId: string;
  onChange: (content: Doc, text: string) => void;
  onSelection: (s: Selection) => void;
  onReady: (editor: Editor) => void;
  focus: boolean;
  setFocus: (v: boolean) => void;
};
const pureCharacterCount = (text: string) => (text.match(/[\p{L}\p{N}]/gu) || []).length;

export default function WritingEditor({
  chapter,
  projectId,
  onChange,
  onSelection,
  onReady,
  focus,
  setFocus,
}: Props) {
  const callbacks = useRef({ onChange, onSelection });
  callbacks.current = { onChange, onSelection };
  const [font, setFont] = useState(
    localSettings.getItem("display-font") || "Songti SC, SimSun, serif",
  );
  const [size, setSize] = useState(
    Number(localSettings.getItem("display-size") || 20),
  );
  const [line, setLine] = useState(
    Number(localSettings.getItem("display-line") || 2),
  );
  const [indent, setIndent] = useState(true);
  const [count, setCount] = useState(chapter.text.replace(/\s/g, "").length);
  const [pureCount, setPureCount] = useState(pureCharacterCount(chapter.text));
  const composing = useRef(false);
  const paperScroll = useRef<HTMLDivElement>(null);
  const [findOpen, setFindOpen] = useState(false);
  const [notesOpen, setNotesOpen] = useState(false);
  const [selectedNote, setSelectedNote] = useState("");
  const positionKey = "writing-position:" + projectId + ":" + chapter.id;
  const editor = useEditor({
    extensions: [
      StarterKit,
      TextStyle,
      FontFamily,
      RevisionNote,
      Placeholder.configure({ placeholder: "故事，从这里开始……" }),
    ],
    content: chapter.content,
    editorProps: {
      handleClick: (_view, _pos, event) => {
        const target = event.target instanceof Element ? event.target.closest("[data-revision-id]") : null;
        if (target) { setSelectedNote(target.getAttribute("data-revision-id") || ""); setNotesOpen(true); }
        return false;
      },
      attributes: { class: "manuscript" },
      handleDOMEvents: {
        compositionstart: () => {
          composing.current = true;
          return false;
        },
        compositionend: () => {
          composing.current = false;
          queueMicrotask(() => {
            if (editor) {
              callbacks.current.onChange(
                editor.getJSON() as Doc,
                editor.getText({ blockSeparator: "\n" }),
              );
              setCount(editor.getText().replace(/\s/g, "").length);
              setPureCount(pureCharacterCount(editor.getText()));
            }
          });
          return false;
        },
      },
    },
    onUpdate: ({ editor }) => {
      setCount(editor.getText().replace(/\s/g, "").length);
      setPureCount(pureCharacterCount(editor.getText()));
      if (!composing.current)
        callbacks.current.onChange(
          editor.getJSON() as Doc,
          editor.getText({ blockSeparator: "\n" }),
        );
    },
    onSelectionUpdate: ({ editor }) => {
      const { from, to } = editor.state.selection;
      localSettings.setItem(positionKey + ":cursor", String(from));
      callbacks.current.onSelection({
        from,
        to,
        text: editor.state.doc.textBetween(from, to, "\n"),
      });
    },
  });
  useEffect(() => {
    if (editor) onReady(editor);
  }, [editor, onReady]);
  useEffect(() => {
    localSettings.setItem("display-font", font);
    localSettings.setItem("display-size", String(size));
    localSettings.setItem("display-line", String(line));
  }, [font, size, line]);
  useEffect(() => {
    if (!editor) return;
    const cursor = Number(localSettings.getItem(positionKey + ":cursor") || 1);
    editor.commands.setTextSelection(Math.max(1, Math.min(Number.isFinite(cursor) ? cursor : 1, Math.max(1,editor.state.doc.content.size-1))));
    const frame = requestAnimationFrame(() => { if (paperScroll.current) paperScroll.current.scrollTop = Number(localSettings.getItem(positionKey + ":scroll") || 0); });
    return () => cancelAnimationFrame(frame);
  }, [editor, positionKey]);
  useEffect(() => {
    const key = (e: KeyboardEvent) => { if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "h") {e.preventDefault();setFindOpen(true);} };
    window.addEventListener("keydown", key); return () => window.removeEventListener("keydown", key);
  }, []);
  if (!editor) return null;
  const removeBlankLines = () => {
    if (composing.current) return;
    const ranges = blankLineRanges(editor.state.doc);
    if (!ranges.length) return;
    let transaction = closeHistory(editor.state.tr);
    for (const range of [...ranges].reverse()) transaction = transaction.delete(range.from, range.to);
    if (!transaction.docChanged || transaction.doc.eq(editor.state.doc)) return;
    editor.view.dispatch(transaction.scrollIntoView());
    editor.view.dispatch(closeHistory(editor.state.tr));
    editor.view.focus();
  };
  return (
    <div className="writing-surface">
      <div className="editor-toolbar">
        <div className="toolbar-group">
          <Type size={15} />
          <select
            aria-label="显示字体"
            value={font}
            onChange={(e) => setFont(e.target.value)}
          >
            <option value="Songti SC, SimSun, serif">宋体</option>
            <option value="Microsoft YaHei, sans-serif">微软雅黑</option>
            <option value="KaiTi, STKaiti, serif">楷体</option>
            <option value="FangSong, serif">仿宋</option>
          </select>
          <select
            aria-label="显示字号"
            value={size}
            onChange={(e) => setSize(Number(e.target.value))}
          >
            {[16, 18, 20, 22, 24, 28].map((n) => (
              <option key={n} value={n}>
                {n} px
              </option>
            ))}
          </select>
        </div>
        <div className="toolbar-group">
          <button
            title="加粗"
            className={editor.isActive("bold") ? "active" : ""}
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => editor.chain().focus().toggleBold().run()}
          >
            <Bold size={16} />
          </button>
          <button
            title="斜体"
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => editor.chain().focus().toggleItalic().run()}
          >
            <Italic size={16} />
          </button>
          <select
            aria-label="选区字体"
            defaultValue=""
            onChange={(e) => {
              if (e.target.value)
                editor.chain().focus().setFontFamily(e.target.value).run();
              e.target.value = "";
            }}
          >
            <option value="">选区字体</option>
            <option value="SimSun">宋体</option>
            <option value="Microsoft YaHei">微软雅黑</option>
            <option value="KaiTi">楷体</option>
          </select>
        </div>
        <div className="toolbar-group">
          <select
            aria-label="行距"
            value={line}
            onChange={(e) => setLine(Number(e.target.value))}
          >
            <option value={1.6}>1.6 倍行距</option>
            <option value={2}>2 倍行距</option>
            <option value={2.4}>2.4 倍行距</option>
          </select>
          <button
            title="首行缩进"
            className={indent ? "active" : ""}
            onClick={() => setIndent(!indent)}
          >
            <AlignLeft size={16} />
          </button>
        </div>
        <div className="toolbar-group push">
          <button
            title="撤销"
            onClick={() => editor.chain().focus().undo().run()}
          >
            <Undo2 size={16} />
          </button>
          <button
            title="重做"
            onClick={() => editor.chain().focus().redo().run()}
          >
            <Redo2 size={16} />
          </button>
          <button title="查找替换 · Ctrl+H" aria-label="查找替换" className={findOpen ? "active" : ""} onClick={() => setFindOpen(v => !v)}><Search size={16} /></button>
          <button title="去除当前章空白行 · 可撤销" aria-label="去除空白行" onMouseDown={e => e.preventDefault()} onClick={removeBlankLines}><ListFilter size={16} /></button>
          <button title="修改标记 · 选中文字后添加待修改说明" aria-label="修改标记" className={notesOpen ? "active" : ""} onMouseDown={e => e.preventDefault()} onClick={() => setNotesOpen(v => !v)}><MessageSquareText size={16} /></button>
          <button
            title="专注模式"
            className={focus ? "active" : ""}
            onClick={() => setFocus(!focus)}
          >
            <Focus size={16} />
          </button>
        </div>
      </div>
      {findOpen && <FindReplace editor={editor} onClose={() => setFindOpen(false)} />}
      {notesOpen && <RevisionNotes editor={editor} selected={selectedNote} onSelect={setSelectedNote} onClose={() => setNotesOpen(false)} />}
      <div className="paper-scroll" ref={paperScroll} onScroll={(e) => localSettings.setItem(positionKey + ":scroll", String(e.currentTarget.scrollTop))}>
        <article
          className={"paper " + (indent ? "indented" : "")}
          style={
            {
              "--writing-font": font,
              "--writing-size": `${size}px`,
              "--writing-line": line,
            } as React.CSSProperties
          }
        >
          <div className="chapter-eyebrow">MANUSCRIPT / 正文</div>
          <h1>{chapter.title}</h1>
          <div className="paper-rule" />
          <EditorContent editor={editor} />
          <div className="paper-end">✦</div>
        </article>
      </div>
      <div className="editor-status">
        <span>
          <span className="status-dot" /> 写作中
        </span>
        <span>
          {count.toLocaleString()} 字{" "}
          <span className="muted">· 不含空白，含标点</span>
          <span title="仅统计汉字、字母和数字，不含空白、标点和符号"> · 纯字数 {pureCount.toLocaleString()} 字</span>
        </span>
        <span>
          Ctrl + S 保存 <span className="muted">/</span> Ctrl + Z 撤销
        </span>
      </div>
    </div>
  );
}
