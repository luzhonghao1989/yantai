// Integration fixture only: deliberately labelled canned output, never a production model.
import http from 'node:http';
import fs from 'node:fs';
const server=http.createServer(async(req,res)=>{
 if(req.url==='/api/show'){res.setHeader('Content-Type','application/json');res.end(JSON.stringify({model_info:{'general.architecture':'fixture','fixture.context_length':262144}}));return;}
 if(req.url==='/api/tags'){res.setHeader('Content-Type','application/json');res.end(JSON.stringify({models:[{name:'integration-test-fixture'}]}));return;}
 if(req.url==='/api/embed'){
  let body='';for await(const chunk of req)body+=chunk;
  const {input}=JSON.parse(body);
  res.setHeader('Content-Type','application/json');
  res.end(JSON.stringify({embeddings:input.map(text=>(text.includes('眼泪')||text.includes('悲伤'))?[1,0]:[0,1])}));return;
 }
 if(req.url==='/api/chat'){
  let body='';for await(const chunk of req)body+=chunk;
  fs.writeFileSync(new URL('./last-context.json',import.meta.url),body);
  res.setHeader('Content-Type','application/x-ndjson');
  const parts=body.includes('你是小说场景平面图规划助手')?[JSON.stringify({title:'青石镇旧宅 · 集成测试示意',note:'集成测试固定布局，不代表模型实际推理效果。',nodes:[{id:'room',label:'正屋',kind:'room',x:20,y:25},{id:'yard',label:'庭院',kind:'place',x:65,y:30},{id:'gate',label:'院门',kind:'room',x:65,y:75}],edges:[{from:'room',to:'yard',label:'石径',uncertain:false},{from:'yard',to:'gate',label:'待确认通道',uncertain:true}]})]:body.includes('你是中文小说审稿编辑')?['【集成测试审稿】第一段引用：雨在傍晚停了。建议补充环境动作，保留人物戒心。']:body.includes('你是文风与场景描写参考助手')?['【集成测试文风参考】\n参考写法：依据网页摘要 [1]，用环境推动动作。\n原创示例：她听见门外的雨声，指尖停在信封的封口上。']:body.includes('queries')?['{"queries":["信封","旧渡口"]}']:['【集成测试候选】','她将信收进衣袋，','推开门，向旧渡口走去。'];
  let i=0;const timer=setInterval(()=>{if(res.destroyed){clearInterval(timer);return;}if(i<parts.length)res.write(JSON.stringify({message:{content:parts[i++]},done:false})+'\n');else{clearInterval(timer);res.end(JSON.stringify({message:{content:''},done:true})+'\n');}},300);return;
 }
 res.writeHead(404);res.end();
});server.listen(49121,'127.0.0.1',()=>console.log('Integration fixture: 127.0.0.1:49121'));
