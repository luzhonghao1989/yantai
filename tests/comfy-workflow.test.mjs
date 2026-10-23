import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {parseComfyWorkflow,inferComfyBindings} from '../frontend/src/comfy-workflow.ts';
for(const [name,positive,negative,field] of [['flux3','39:30','39:37','text'],['qwen21','459:452','459:452','prompt']]){
 const text=await readFile(new URL(`../frontend/src/workflows/${name}.json`,import.meta.url),'utf8');
 const graph=parseComfyWorkflow(text),binding=inferComfyBindings(graph);
 assert.equal(binding.positiveNode,positive);assert.equal(binding.negativeNode,negative);assert.equal(binding.positiveInput,field);
 assert.equal(graph[binding.outputNode].class_type,'SaveImage');
 assert.deepEqual(parseComfyWorkflow(JSON.stringify({prompt:graph})),graph);
 for(const node of Object.values(graph))for(const value of Object.values(node.inputs))if(Array.isArray(value)){assert(graph[value[0]],`Missing link target ${value[0]}`);assert(Number.isInteger(value[1]));}
}
assert.throws(()=>parseComfyWorkflow('{"nodes":[],"links":[]}'),/导出 API/);
assert.throws(()=>parseComfyWorkflow('{}'));
assert.throws(()=>parseComfyWorkflow('bad'));
console.log('ComfyUI workflows: Flux/Qwen bindings, custom node fields, subgraph flattening links and API format validation passed');
