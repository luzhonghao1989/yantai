import assert from 'node:assert/strict';
import {getSchema} from '../frontend/node_modules/@tiptap/core/dist/index.js';
import StarterKit from '../frontend/node_modules/@tiptap/starter-kit/dist/index.js';
import {EditorState,TextSelection} from '../frontend/node_modules/prosemirror-state/dist/index.js';
import {history,closeHistory,undo} from '../frontend/node_modules/prosemirror-history/dist/index.js';
import {RevisionNote,revisionNotes,changeRevisionNote,canMarkRevision} from '../frontend/src/revision-notes.ts';
const schema=getSchema([StarterKit,RevisionNote]);
const original=schema.node('doc',null,[schema.node('paragraph',null,[schema.text('需要修改的对白。',[schema.mark('bold')])]),schema.node('paragraph',null,[schema.text('补充动作。')])]);
let state=EditorState.create({schema,doc:original,plugins:[history()]});
state=state.apply(state.tr.setSelection(TextSelection.create(state.doc,1,7)));assert(canMarkRevision(state));
state=state.apply(closeHistory(state.tr).addMark(1,7,schema.mark('revisionNote',{id:'test-note',note:'对白自然些',resolved:false})));state=state.apply(closeHistory(state.tr));
assert(!canMarkRevision(state));assert.equal(revisionNotes(state.doc)[0].quote,'需要修改的对');assert.equal(state.doc.textContent,original.textContent);
// Prefix insertion must move the anchor, retaining text formatting and metadata.
state=state.apply(state.tr.insertText('开场：',1));let note=revisionNotes(state.doc)[0];assert.equal(note.from,4);assert.equal(note.note,'对白自然些');assert(state.doc.nodeAt(4).marks.some(m=>m.type.name==='bold'));
state=state.apply(changeRevisionNote(state,'test-note',{resolved:true,note:'已补动作'}));state=state.apply(closeHistory(state.tr));
assert.equal(revisionNotes(state.doc)[0].resolved,true);assert.deepEqual(revisionNotes(schema.nodeFromJSON(state.doc.toJSON())),revisionNotes(state.doc));
const beforeRemove=state.doc;state=state.apply(changeRevisionNote(state,'test-note',null));state=state.apply(closeHistory(state.tr));assert.equal(revisionNotes(state.doc).length,0);assert.equal(state.doc.textContent,beforeRemove.textContent);assert(undo(state,tr=>state=state.apply(tr)));assert(state.doc.eq(beforeRemove));
note=revisionNotes(state.doc)[0];state=state.apply(state.tr.delete(note.from,note.to));assert.equal(revisionNotes(state.doc).length,0);
// One note spanning multiple paragraphs must update every marked fragment.
state=EditorState.create({schema,doc:original});state=state.apply(state.tr.addMark(1,state.doc.content.size-1,schema.mark('revisionNote',{id:'multi'})));assert.equal(revisionNotes(state.doc).length,1);state=state.apply(changeRevisionNote(state,'multi',{note:'跨段修改',resolved:true}));state.doc.descendants(n=>{for(const m of n.marks)if(m.type.name==='revisionNote'){assert.equal(m.attrs.note,'跨段修改');assert.equal(m.attrs.resolved,true)}});
console.log('Revision notes: anchoring, overlap guard, formatting, status, JSON, delete and undo passed');
