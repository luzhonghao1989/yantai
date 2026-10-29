import { useMemo } from "react";

export default function TitleCandidates({ text, complete, disabled, onAccept }: { text: string; complete: boolean; disabled: boolean; onAccept: (title: string) => void }) {
  const titles = useMemo(() => {
    try {
      const data = JSON.parse(text.trim().replace(/^```(?:json)?\s*/i, "").replace(/\s*```$/, ""));
      if (!Array.isArray(data.titles)) return [];
      return [...new Set<string>(data.titles.filter((t: unknown): t is string => typeof t === "string").map((t: string) => t.trim()).filter(Boolean))];
    } catch { return []; }
  }, [text]);
  if (!complete) return <p className="muted">正在为本章构思多个标题……</p>;
  if (!titles.length) return <p className="info-note">未取得有效的标题候选，请重新生成。原始结果可通过复制按钮保留。</p>;
  return <div className="title-candidates">{titles.map(title => <div className="title-candidate" key={title}><span>{title}</span><button className="btn secondary" disabled={disabled} onClick={() => onAccept(title)}>用此标题</button></div>)}</div>;
}
