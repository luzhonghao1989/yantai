import type { Node } from "@tiptap/pm/model";
import { revisionNotes } from "./revision-notes.ts";

export type SourceHint = { markId?: string; paragraph?: number };
export type LocateSource = (text: string, hint?: SourceHint) => void;
export type SourceRange = { from: number; to: number };

// Map UTF-16 characters to document positions, including formatting splits,
// hard breaks and paragraph boundaries. Never reuse offsets from an AI snapshot.
export function sourceMatches(doc: Node, quote: string, hint: SourceHint = {}): SourceRange[] {
  quote = quote.replace(/\r\n?/g, "\n");
  if (!quote.trim()) return [];
  if (hint.markId) {
    const mark = revisionNotes(doc).find(n => n.id === hint.markId);
    return mark && mark.quote === quote ? [{ from: mark.from, to: mark.to }] : [];
  }
  let text = "";
  const starts: number[] = [], ends: number[] = [];
  const blocks: { from: number; to: number; text: string }[] = [];
  doc.descendants((node, pos) => {
    if (!node.isTextblock) return;
    if (blocks.length) { text += "\n"; starts.push(blocks[blocks.length - 1].to); ends.push(pos + 1); }
    const start = text.length;
    node.descendants((inline, offset) => {
      if (inline.isText) {
        for (let i = 0; i < inline.text!.length; i++) {
          text += inline.text![i]; starts.push(pos + 1 + offset + i); ends.push(pos + 2 + offset + i);
        }
      } else if (inline.isLeaf) {
        text += inline.type.name === "hardBreak" ? "\n" : "\uFFFC";
        starts.push(pos + 1 + offset); ends.push(pos + 1 + offset + inline.nodeSize);
      }
    });
    blocks.push({ from: pos + 1, to: pos + node.nodeSize - 1, text: text.slice(start) });
    return false;
  });
  const matches: SourceRange[] = [];
  for (let at = text.indexOf(quote); at >= 0; at = text.indexOf(quote, at + 1)) {
    matches.push({ from: starts[at], to: ends[at + quote.length - 1] });
  }
  // A paragraph hint may prioritize a matching passage, never unrelated text.
  const block = hint.paragraph ? blocks[hint.paragraph - 1] : undefined;
  if (block) matches.sort((a, b) => Number(b.from >= block.from && b.to <= block.to) - Number(a.from >= block.from && a.to <= block.to));
  return matches;
}

export function nextSourceMatch(matches: SourceRange[], selection: SourceRange): number {
  const current = matches.findIndex(m => m.from === selection.from && m.to === selection.to);
  return (current + 1) % Math.max(matches.length, 1);
}
