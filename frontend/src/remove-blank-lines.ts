import type { Node } from "@tiptap/pm/model";

// Ranges refer to the original document; callers delete them from right to left.
export function blankLineRanges(doc: Node): { from: number; to: number }[] {
  const ranges: { from: number; to: number }[] = [];
  doc.descendants((node, pos) => {
    if (!node.isTextblock) return;
    const lines: { from: number; to: number; text: string }[] = [];
    let start = pos + 1, text = "";
    node.forEach((child, offset) => {
      if (child.type.name === "hardBreak") {
        lines.push({ from: start, to: pos + 1 + offset, text });
        start = pos + 1 + offset + child.nodeSize; text = "";
      } else text += child.isText ? child.text : "\uFFFC";
    });
    lines.push({ from: start, to: pos + 1 + node.content.size, text });
    if (lines.every(line => !line.text.trim())) {
      ranges.push({ from: pos, to: pos + node.nodeSize });
    } else {
      const kept = lines.filter(line => line.text.trim());
      if (kept[0].from > pos + 1) ranges.push({ from: pos + 1, to: kept[0].from });
      for (let i = 1; i < kept.length; i++) {
        const from = kept[i - 1].to + 1, to = kept[i].from;
        if (to > from) ranges.push({ from, to });
      }
      const last = kept[kept.length - 1];
      if (last.to < pos + 1 + node.content.size) ranges.push({ from: last.to, to: pos + 1 + node.content.size });
    }
    return false;
  });
  const merged: typeof ranges = [];
  for (const range of ranges.sort((a, b) => a.from - b.from)) {
    const last = merged[merged.length - 1];
    if (last && range.from <= last.to) last.to = Math.max(last.to, range.to);
    else merged.push({ ...range });
  }
  return merged;
}
