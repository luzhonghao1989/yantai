import assert from 'node:assert/strict';
import {parseCSV,loreKind} from '../frontend/src/lore-import.ts';
const rows=parseCSV('\uFEFF名称,类型,基础设定\r\n林晚,人物,"有逗号,和\n换行"\r\n铜铃,物品,"他说""你好"""\r\n');
assert.equal(rows[0]['基础设定'],'有逗号,和\n换行');
assert.equal(rows[1]['基础设定'],'他说"你好"');
assert.equal(loreKind('人物'),'character');assert.equal(loreKind('场景'),'location');
for(const text of ['a,a\nx,y','a,b\nx','a,b\nx,"broken']) assert.throws(()=>parseCSV(text));
console.log('CSV parser checks passed');
