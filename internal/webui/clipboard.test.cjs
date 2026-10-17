// Exercise the actual injected SDK and host permission/message broker.
// No browser, game, or desktop clipboard is touched by these tests.
const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const src=fs.readFileSync(path.join(__dirname,'assets/app.js'),'utf8');

function harness(allow=true){
  const frames=[{dataset:{savedTabId:'survey'},contentWindow:{}}];
  let version='1',prompts=0;const storage=new Map(),calls=[],replies=[];
  const c=vm.createContext({
    savedTabMeta:()=>({name:'Survey',updatedAt:version}),
    localStorage:{getItem:k=>storage.get(k),setItem:(k,v)=>storage.set(k,v)},
    confirm:()=>{prompts++;return allow},
    allSDKFrames:()=>frames,allowTabRequest:()=>true,
    sendToFrame:(_,m)=>replies.push(m),
    request:async(method,params)=>{calls.push({method,params});return {written:true,backend:'test'}},
    addEventListener:(name,fn)=>{if(name==='message')c.listener=fn}
  });
  const start=src.indexOf('  const TAB_CAPABILITIES=');
  const end=src.indexOf('  const tabRequestRates=',start);
  vm.runInContext(src.slice(start,end),c);
  const broker=src.indexOf("  addEventListener('message',async ev=>{");
  const brokerEnd=src.indexOf("  $('#custom-html').value=",broker);
  vm.runInContext(src.slice(broker,brokerEnd),c);
  return {calls,replies,frames,get prompts(){return prompts},changeVersion:()=>version='2',
    send:(method,params={},source=frames[0].contentWindow)=>c.listener({source,data:{channel:'jacob-tab',kind:'request',id:'request',method,params}})};
}

test('clipboard info does not prompt; writing gets a separate remembered permission',async()=>{
  const h=harness();await h.send('clipboard.info');assert.equal(h.prompts,0);
  await h.send('clipboard.writeText',{text:'Зоря 🌟'});
  await h.send('clipboard.writeText',{text:'next'});
  assert.equal(h.prompts,1);assert.equal(h.calls.length,3);
  assert.equal(h.calls[1].params.text,'Зоря 🌟');
  h.changeVersion();await h.send('clipboard.writeText',{text:'updated code'});
  assert.equal(h.prompts,2);
});

test('denial is remembered and blocks every clipboard write',async()=>{
  const h=harness(false);
  await h.send('clipboard.writeText',{text:'one'});await h.send('clipboard.writeText',{text:'two'});
  assert.equal(h.prompts,1);assert.equal(h.calls.length,0);
  assert.ok(h.replies.every(r=>r.error.code==='PERMISSION_DENIED'));
});

test('permission is scoped to the saved tab; previews do not share grants',async()=>{
  const h=harness();await h.send('clipboard.writeText',{text:'first'});
  h.frames[0].dataset.savedTabId='other';await h.send('clipboard.writeText',{text:'second'});
  h.frames[0].dataset={};await h.send('clipboard.writeText',{text:'preview'});
  assert.equal(h.prompts,3);
});

test('clipboard reading and messages from unknown windows are not exposed',async()=>{
  const h=harness();await h.send('clipboard.readText');
  assert.equal(h.replies[0].error.code,'SDK_DENIED');
  await h.send('clipboard.writeText',{text:'untrusted'},{});
  assert.equal(h.calls.length,0);assert.equal(h.prompts,0);
});

test('actual injected SDK forwards text unchanged and waits for host acknowledgement',async()=>{
  const start=src.indexOf('  const SDK = `')+'  const SDK = '.length;
  const end=src.indexOf('\n',src.indexOf("kind:'ready'",start));
  const markup=vm.runInNewContext(src.slice(start,end).trim().replace(/;$/,''));
  const code=markup.replace(/^<script>/,'').replace(/<\/script>$/,'');
  const messages=[],listeners=[];
  const c=vm.createContext({location:{href:'about:srcdoc'},parent:{postMessage:m=>messages.push(m)},
    addEventListener:(name,cb)=>{if(name==='message')listeners.push(cb)},console,queueMicrotask});
  c.window=c;
  vm.runInContext(code,c);
  assert.equal(typeof c.Elite.clipboard.info,'function');
  assert.equal(c.Elite.clipboard.readText,undefined);
  let settled=false;
  const p=c.Elite.clipboard.writeText('Зоря 🌟').then(r=>{settled=true;return r});
  await Promise.resolve();assert.equal(settled,false);
  const req=messages.at(-1);assert.equal(req.method,'clipboard.writeText');assert.equal(req.params.text,'Зоря 🌟');
  listeners[0]({data:{channel:'jacob-host',kind:'response',id:req.id,ok:true,result:{written:true}}});
  assert.equal((await p).written,true);
  const failed=c.Elite.clipboard.writeText('next');const req2=messages.at(-1);
  listeners[0]({data:{channel:'jacob-host',kind:'response',id:req2.id,ok:false,error:{code:'CLIPBOARD_BUSY'}}});
  await assert.rejects(failed,e=>e.code==='CLIPBOARD_BUSY');
});
