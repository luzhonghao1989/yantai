import assert from 'node:assert/strict';
import { readFile, writeFile, unlink } from 'node:fs/promises';
import { createRequire } from 'node:module';
const require = createRequire(new URL('../frontend/package.json', import.meta.url));
const ts = require('typescript');
const React = require('react');
const { renderToStaticMarkup } = require('react-dom/server');
// Compile the actual shared component; keep temporary output beside its dependencies.
const temporary = new URL(`../frontend/.ai-content-test-${process.pid}.mjs`, import.meta.url);
try {
  const source = await readFile(new URL('../frontend/src/AIContent.tsx', import.meta.url), 'utf8');
  await writeFile(temporary, ts.transpileModule(source, { compilerOptions: {
    jsx: ts.JsxEmit.ReactJSX, module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022,
  } }).outputText);
  const { default: AIContent } = await import(temporary.href);
  const render = text => renderToStaticMarkup(React.createElement(AIContent, { text }));
  const html = render('# 审稿建议\n\n**人物动机**\n\n- 第一条\n- 第二条\n\n> 引用\n\n| 人物 | 建议 |\n| --- | --- |\n| 小芳 | 补充动机 |\n\n```text\n保留的代码示例\n```');
  for (const expected of ['<h1>审稿建议</h1>', '<strong>人物动机</strong>', '<ul>', '<li>第一条</li>', '<blockquote>', '<table>', '<td>补充动机</td>', '<pre>']) assert(html.includes(expected), expected);
  assert(!html.includes('**人物动机**'));
  const safe = render('<script>alert(1)</script>\n\n[危险](javascript:alert%281%29)\n\n![外链图片](https://example.com/tracker.png)\n\n[参考](https://example.com)');
  assert(!safe.includes('<script'));
  assert(!safe.includes('javascript:'));
  assert(!safe.includes('<img'));
  assert(safe.includes('rel="noopener noreferrer"'));
  assert(safe.includes('外链图片'));
  assert(render('普通正文\n下一行').includes('普通正文\n下一行'));
  assert(render('## 尚在生成\n\n**未完成').includes('尚在生成'));
  const linked=renderToStaticMarkup(React.createElement(AIContent,{text:'原文“她抬眼。”显得突兀。\n\n> 她抬眼。\n\n`她抬眼。`\n\n她抬眼。',sourceText:'她抬眼。',onLocate:()=>{}}));
  assert(linked.includes('source-quote'));assert(linked.includes('定位引用原文'));assert(linked.includes('定位原文'));
  const invented=renderToStaticMarkup(React.createElement(AIContent,{text:'建议“她笑了。”',sourceText:'她抬眼。',onLocate:()=>{}}));
  assert(!invented.includes('source-quote'),'Invented suggested text is not an original quote');
  console.log('AI reading view: formatting, tables, plain prose, partial stream and unsafe content checks passed');
} finally {
  await unlink(temporary).catch(() => {});
}
