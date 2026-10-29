import assert from 'node:assert/strict';
import {api, configureAPI, APIError} from '../frontend/src/types.ts';

const originalFetch = globalThis.fetch;
const calls = [];
configureAPI({base:'http://127.0.0.1:12345',token:'test-session'});
globalThis.fetch = async (url, options) => {
  calls.push({url, ...options});
  return {ok:true, json:async()=>({saved:true})};
};
try {
  for (const path of ['/backups','/backups/backup-id/projects/project-id/restore']) {
    assert.deepEqual(await api(path,'POST'),{saved:true});
    const call = calls.at(-1);
    assert.equal(call.headers['Content-Type'],'application/json');
    assert.equal(call.headers['X-Yantai-Token'],'test-session');
    assert.equal(call.body,'{}');
  }
  await api('/projects/id','DELETE');
  assert.equal(calls.at(-1).body,'{}');
  await api('/backups/settings','PUT',{enabled:true,retentionDays:30});
  assert.deepEqual(JSON.parse(calls.at(-1).body),{enabled:true,retentionDays:30});
  await api('/backups');
  assert.equal(calls.at(-1).body,undefined);
  assert.equal(calls.at(-1).headers['Content-Type'],undefined);
  globalThis.fetch=async()=>({ok:false,status:415,json:async()=>({error:'请求必须是 JSON'})});
  await assert.rejects(api('/backups','POST'),error=>error instanceof APIError&&error.status===415&&error.message==='请求必须是 JSON');
  console.log('API JSON requests: passed');
} finally {globalThis.fetch=originalFetch;configureAPI({base:'',token:''});}
