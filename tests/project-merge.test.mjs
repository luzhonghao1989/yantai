import assert from 'node:assert/strict';
import {mergeProjects,describeSaveConflict} from '../frontend/src/project-merge.ts';
const base={id:'p',name:'小说',revision:1,updatedAt:'old',chapters:[{id:'c',title:'第一章',text:'原文',content:{type:'doc',content:[{type:'paragraph',content:[{type:'text',text:'原文'}]}]}}],images:[{id:'i',name:'地图',sha:'a'}],aiRuns:[{id:'map',mode:'map'}],entities:[{id:'e',name:'人物',versions:[{stageId:'s1',traits:'沉稳'},{stageId:'s2',traits:'谨慎'}]}]};
const clone=()=>structuredClone(base);
const remote=clone();remote.revision=2;remote.updatedAt='new';remote.chapters[0].text='API 新正文';remote.chapters[0].content.content[0].content[0].text='API 新正文';
const local=clone();local.images=[];local.aiRuns=[];
let result=mergeProjects(base,local,remote);
assert.deepEqual(result.conflicts,[]);assert.equal(result.project.chapters[0].text,'API 新正文');assert.equal(result.project.images.length,0);assert.equal(result.project.aiRuns.length,0);assert.equal(result.project.revision,2);
assert.equal(base.images.length,1);assert.equal(remote.images.length,1);
// Same text/document changed differently must not be overwritten.
const edited=clone();edited.chapters[0].text='窗口正文';edited.chapters[0].content.content[0].content[0].text='窗口正文';
result=mergeProjects(base,edited,remote);assert(result.conflicts.some(p=>p.includes('第一章')));assert.equal(result.project.chapters[0].text,'窗口正文');
// Delete versus edit is a conflict; identical deletion is safe.
const changedImage=clone();changedImage.images[0].name='API 重命名';assert(mergeProjects(base,local,changedImage).conflicts.some(p=>p.includes('images')));
assert.equal(mergeProjects(base,local,local).conflicts.length,0);
// Distinct additions and edits to different stages combine by identity.
const a=clone(),b=clone();a.images.push({id:'new-a',name:'A'});b.images.push({id:'new-b',name:'B'});a.entities[0].versions[0].traits='坚定';b.entities[0].versions[1].traits='活泼';
result=mergeProjects(base,a,b);assert.equal(result.conflicts.length,0);assert.deepEqual(new Set(result.project.images.map(i=>i.id)),new Set(['i','new-a','new-b']));assert.deepEqual(result.project.entities[0].versions.map(v=>v.traits),['坚定','活泼']);
// Reordering on one side survives an unrelated edit, differing reorderings conflict.
const orderBase={...clone(),images:[{id:'1'},{id:'2'},{id:'3'}]},o1=structuredClone(orderBase),o2=structuredClone(orderBase);o1.images.reverse();o2.name='改名';
assert.deepEqual(mergeProjects(orderBase,o1,o2).project.images.map(i=>i.id),['3','2','1']);o2.images=[o2.images[1],o2.images[0],o2.images[2]];assert(mergeProjects(orderBase,o1,o2).conflicts.some(p=>p.includes('顺序')));
// Rich text arrays and binding arrays are atomic to avoid merging positional edits.
const docA=clone(),docB=clone();docA.chapters[0].content.content[0].content[0].text='A';docB.chapters[0].content.content[0].content[0].text='B';assert(mergeProjects(base,docA,docB).conflicts.length>0);
assert.throws(()=>mergeProjects(base,local,{...remote,id:'other'}));
console.log('Project three-way merge: API edits + image deletion, same-field conflicts, identity/order, stages and rich-text preservation passed');

const linked={...clone(),volumes:[{id:'v',entityIds:[]}],stages:[{id:'s1',volumeId:'v',entityIds:[]},{id:'s2',volumeId:'v',entityIds:[]}],storylines:[]};linked.chapters[0].volumeId='v';linked.chapters[0].stageId='s1';
const removeStage=structuredClone(linked),addChapter=structuredClone(linked);removeStage.stages.pop();removeStage.entities[0].versions.pop();addChapter.chapters.push({...structuredClone(linked.chapters[0]),id:'new-chapter',stageId:'s2'});
assert(mergeProjects(linked,removeStage,addChapter).conflicts.some(p=>p.includes('关联')),'Cross-record references must pause auto-merge');
assert.equal(describeSaveConflict('chapters[第一章].text'),'章节[第一章].正文');
