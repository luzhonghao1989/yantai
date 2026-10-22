import assert from 'node:assert/strict';
import {parseSceneMap,sceneMapSVG} from '../frontend/src/scene-map.ts';
const plan={title:'旧宅<测试>',note:'未知方位仅示意',nodes:[{id:'a',label:'正屋',kind:'room',x:20,y:20},{id:'b',label:'庭院',kind:'place',x:70,y:70}],edges:[{from:'a',to:'b',label:'走廊',uncertain:true}]};
const parsed=parseSceneMap('```json\n'+JSON.stringify(plan)+'\n```');assert.deepEqual(parsed,plan);
const svg=sceneMapSVG(parsed);assert(svg.includes('旧宅&lt;测试&gt;'));assert(svg.includes('stroke-dasharray'));assert(svg.includes('走廊'));assert(!svg.includes('<script'));
const bad=structuredClone(plan);bad.nodes[0].x=Infinity;assert.throws(()=>parseSceneMap(JSON.stringify(bad)));bad.nodes[0].x=20;bad.nodes[1].id='a';assert.throws(()=>parseSceneMap(JSON.stringify(bad)));bad.nodes[1].id='b';bad.edges[0].to='missing';assert.throws(()=>parseSceneMap(JSON.stringify(bad)));
const injection=structuredClone(plan);injection.nodes[0].label='<script>alert(1)</script>';injection.title='" onload="alert(1)';const safe=sceneMapSVG(parseSceneMap(JSON.stringify(injection)));assert(!safe.includes('<script>'));assert(safe.includes('&lt;script&gt;'));assert(!safe.includes('aria-label="" onload='));
assert.throws(()=>parseSceneMap('{"nodes":[],"edges":[]}'));assert.throws(()=>parseSceneMap(JSON.stringify({...plan,nodes:Array(41).fill(plan.nodes[0])})));
console.log('Scene map: plan parsing, schema bounds, references, SVG escaping and uncertain paths passed');
