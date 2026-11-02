import assert from 'node:assert/strict';
import {readFile,writeFile,unlink} from 'node:fs/promises';
import {createRequire} from 'node:module';
const require=createRequire(new URL('../frontend/package.json',import.meta.url));
const ts=require('typescript'),React=require('react');
const {renderToStaticMarkup}=require('react-dom/server');
const temporary=new URL(`../frontend/.remembered-input-test-${process.pid}.mjs`,import.meta.url);
const originalWindow=globalThis.window;
const values=new Map();
globalThis.window={localStorage:{getItem:key=>values.get(key)??null,setItem:(key,value)=>values.set(key,value),removeItem:key=>values.delete(key)}};
try{
 const source=(await readFile(new URL('../frontend/src/useRememberedInput.ts',import.meta.url),'utf8')).replace('"./local-settings"','"./src/local-settings.ts"');
 await writeFile(temporary,ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText);
 const {default:useRememberedInput}=await import(temporary.href);
 function reopen(key,fallback='',allowed){let result;function Panel(){result=useRememberedInput(key,fallback,allowed);return React.createElement('span',null,result[0])}renderToStaticMarkup(React.createElement(Panel));return result}
 const review='ai-input:book-a:chapter-a:instruction:review';
 reopen(review)[1]('保留对白口语，检查动作衔接');
 assert.equal(reopen(review)[0],'保留对白口语，检查动作衔接');
 for(const other of ['ai-input:book-b:chapter-a:instruction:review','ai-input:book-a:chapter-b:instruction:review','ai-input:book-a:chapter-a:instruction:rewrite']) assert.equal(reopen(other)[0],'');
 reopen(review)[1]('');assert.equal(reopen(review)[0],'');
 reopen('mode','continue')[1]('review');assert.equal(reopen('mode','continue',['continue','review'])[0],'review');
 values.set('mode','invalid');assert.equal(reopen('mode','continue',['continue','review'])[0],'continue');
 const size=values.size;reopen(null)[1]('no project');assert.equal(values.size,size);
 console.log('Remembered AI input: reopen, project/chapter/task isolation, clearing and mode validation passed');
}finally{globalThis.window=originalWindow;await unlink(temporary).catch(()=>{})}
