import assert from 'node:assert/strict';
import {getSchema} from '../frontend/node_modules/@tiptap/core/dist/index.js';
import StarterKit from '../frontend/node_modules/@tiptap/starter-kit/dist/index.js';
import {EditorState} from '../frontend/node_modules/prosemirror-state/dist/index.js';
import {history,closeHistory,undo} from '../frontend/node_modules/prosemirror-history/dist/index.js';
import {parseReviewResult,reviewTarget,replaceReviewParagraph,replaceReviewParagraphs,reviewReplacementPlan,reviewReplacementPresent,reviewHasChange,reviewRetryInstruction} from '../frontend/src/review-replacements.ts';
const schema=getSchema([StarterKit]);
const para=(value,marks=[])=>schema.node('paragraph',null,value?schema.text(value,marks):undefined);
const original=schema.node('doc',null,[para('前文保持。',[schema.mark('italic')]),para('她说你好。',[schema.mark('bold')]),para('他走了。'),para('结尾保持。')]);
let state=EditorState.create({schema,doc:original,plugins:[history()]});
const result=parseReviewResult('```json\n'+JSON.stringify({summary:'## 审稿总结\n对白略直白。',suggestions:[{original:'她说你好。',replacement:'她抬起眼：“你来了？”',reason:'增加动作，让对白自然。',paragraph:2},{original:'他走了。',replacement:'他收起信纸。\n门帘轻轻落下。',reason:'增加动作',paragraph:3}]})+'\n```');
assert(result);assert.equal(result.suggestions.length,2);assert.equal(parseReviewResult('历史的普通审稿意见'),null);
const [first,second]=result.suggestions;
state=state.apply(replaceReviewParagraph(state,first));state=state.apply(closeHistory(state.tr));
assert.equal(state.doc.textBetween(0,state.doc.content.size,'\n'),'前文保持。\n她抬起眼：“你来了？”\n他走了。\n结尾保持。');
assert(state.doc.child(0).firstChild.marks.some(m=>m.type.name==='italic'));
assert(state.doc.child(1).firstChild.marks.some(m=>m.type.name==='bold'));
assert.throws(()=>reviewTarget(state.doc,first),/已变化/);
assert(reviewReplacementPresent(state.doc,first));
state=state.apply(replaceReviewParagraph(state,second));state=state.apply(closeHistory(state.tr));
assert.equal(state.doc.textBetween(0,state.doc.content.size,'\n'),'前文保持。\n她抬起眼：“你来了？”\n他收起信纸。\n门帘轻轻落下。\n结尾保持。');
assert(undo(state,tr=>state=state.apply(tr)));assert.equal(state.doc.child(2).textContent,'他走了。');
assert(undo(state,tr=>state=state.apply(tr)));assert(state.doc.eq(original));
assert(!reviewReplacementPresent(state.doc,first));
assert.throws(()=>reviewTarget(original,{...first,original:'她说'}),/不完整/);
assert.throws(()=>reviewTarget(original,{...first,replacement:first.original}),/没有实际变化/);
for(const replacement of [first.original,' 她说 你好。\n']) {
  const unchanged={...first,replacement};
  assert.equal(reviewHasChange(unchanged),false);
  assert.equal(reviewReplacementPresent(original,unchanged),false,'Unchanged output must never be reported as applied');
  const correction=reviewRetryInstruction(JSON.stringify({summary:'审稿',suggestions:[unchanged,second]}));
  assert(correction.includes(first.original));assert(correction.includes(first.reason));
  assert(!correction.includes(second.original),'Only unchanged suggestions need corrective instructions');
}
assert.equal(reviewRetryInstruction(JSON.stringify({summary:'无需修改',suggestions:[]})),'');
assert.equal(reviewRetryInstruction(JSON.stringify(result)),'');
assert.equal(reviewRetryInstruction('旧版审稿意见'),'');
assert.equal(reviewRetryInstruction(JSON.stringify({summary:'缺少改文',suggestions:[{...first,replacement:''}]})),'');
const repeated=schema.node('doc',null,[para('她说你好。'),para('她说你好。')]);
assert.throws(()=>reviewTarget(repeated,first),/多个相同段落/);
assert.throws(()=>reviewTarget(original,{...first,replacement:''}),/完整原段落/);
// Batch anchors are resolved before editing and committed as one undo event.
state=EditorState.create({schema,doc:original,plugins:[history()]});
const unchanged={...first,id:'unchanged',original:'结尾保持。',replacement:'结尾保持。'};
const stale={...first,id:'stale',original:'不存在的旧段落'};
const batch=replaceReviewParagraphs(state,[second,unchanged,first,stale]);
assert.equal(batch.replaced,2);assert.equal(batch.skipped,2);
state=state.apply(batch.transaction);state=state.apply(closeHistory(state.tr));
assert.equal(state.doc.textBetween(0,state.doc.content.size,'\n'),'前文保持。\n她抬起眼：“你来了？”\n他收起信纸。\n门帘轻轻落下。\n结尾保持。');
assert(state.doc.child(0).firstChild.marks.some(m=>m.type.name==='italic'));
assert(state.doc.child(1).firstChild.marks.some(m=>m.type.name==='bold'));
assert.throws(()=>replaceReviewParagraphs(state,[first,second]),/没有可一键替换/);
assert(undo(state,tr=>state=state.apply(tr)));assert(state.doc.eq(original),'One undo restores entire batch');
state=state.apply(replaceReviewParagraph(state,first));state=state.apply(closeHistory(state.tr));
const remaining=replaceReviewParagraphs(state,[first,second]);assert.equal(remaining.replaced,1);assert.equal(remaining.skipped,1);
const conflict={...first,id:'conflict',replacement:'另一种改法。'};
const plan=reviewReplacementPlan(original,[first,conflict,second]);assert.equal(plan.items.length,1);assert.equal(plan.skipped.length,2);assert(plan.skipped.every(s=>s.reason.includes('冲突')));
assert.throws(()=>replaceReviewParagraphs(EditorState.create({schema,doc:repeated}),[first]),/没有可一键替换/);
const chainDoc=schema.node('doc',null,[para('原文甲。'),para('原文乙。')]);
const chainState=EditorState.create({schema,doc:chainDoc});
const chain=replaceReviewParagraphs(chainState,[{...first,original:'原文甲。',replacement:'原文乙。'},{...second,original:'原文乙。',replacement:'原文丙。'}]);
assert.equal(chainState.apply(chain.transaction).doc.textBetween(0,chainState.apply(chain.transaction).doc.content.size,'\n'),'原文乙。\n原文丙。','New replacement text must not retarget another suggestion');
console.log('Review replacement: structured/legacy parsing, exact whole-paragraph anchors, sequential replacements, multiline output, formatting, duplicate/stale rejection, atomic batch replacement, conflict skips and independent undo passed');

// Common model formatting variations must preserve exact original/replacement text.
const canonical={summary:'需要调整对白。',suggestions:[{original:'她说你好。',replacement:'她抬起眼：“你来了？”',reason:'让对白自然。',paragraph:2}]};
for(const raw of [
 '以下为审稿结果：\n'+JSON.stringify(canonical)+'\n以上为建议。',
 '```JSON '+JSON.stringify(canonical)+'```',
 JSON.stringify({result:canonical}),JSON.stringify({data:JSON.stringify(canonical)}),
 JSON.stringify(canonical).replace(/}$/,',}').replace(/](?=,)/,',]'),
 JSON.stringify({suggestions:canonical.suggestions}),
 JSON.stringify(canonical.suggestions),
 JSON.stringify({总评:canonical.summary,修改建议:[{原文:canonical.suggestions[0].original,修改后:canonical.suggestions[0].replacement,修改理由:'让对白自然。',段落序号:'2'}]}),
 JSON.stringify({summary:canonical.summary,edits:[{originalText:canonical.suggestions[0].original,revisedText:canonical.suggestions[0].replacement,explanation:'让对白自然。',paragraphIndex:2}]}),
 ]) {
 const parsed=parseReviewResult(raw);assert(parsed,raw);assert.equal(parsed.suggestions.length,1);assert.equal(parsed.suggestions[0].original,canonical.suggestions[0].original);assert.equal(parsed.suggestions[0].replacement,canonical.suggestions[0].replacement);assert.equal(reviewTarget(original,parsed.suggestions[0]).paragraph,2);assert.equal(reviewRetryInstruction(raw),'');
}
const special={summary:'原文含有 },] 和反斜杠，不得改动引用。',suggestions:[{original:'他说：“}，] \"”。',replacement:'她看着他：“说完了？”',reason:'标点测试'}]};
assert.equal(parseReviewResult(JSON.stringify(special)).suggestions[0].original,special.suggestions[0].original);
const rawLines='{"summary":"第一行\n第二行","suggestions":[]}';
assert.equal(parseReviewResult(rawLines).summary,'第一行\n第二行');
for(const broken of ['{"summary":"建议","suggestions":[', '```json\n{"summary":"建议","suggestions":[{', '{"summary":"错误的 " 引号","suggestions":[]}']) {assert.equal(parseReviewResult(broken),null);assert(reviewRetryInstruction(broken).includes('标准的 JSON'));}
assert.equal(parseReviewResult('{"arbitrary":"unrelated"}'),null);
assert.equal(parseReviewResult('[{"title":"不是替换建议"}]'),null);
const adviceOnly=parseReviewResult(JSON.stringify({summary:'建议',suggestions:[{original:'她说你好。',suggestion:'增加动作'}]}));
assert.equal(adviceOnly.suggestions[0].replacement,'');assert.throws(()=>reviewTarget(original,adviceOnly.suggestions[0]),/完整原段落/,'Advice must never be treated as replacement prose');
console.log('Review formatting: surrounding prose/fences, wrappers, aliases, absent summary, bare arrays, safe syntax repair, exact anchors and malformed/truncated rejection passed');
