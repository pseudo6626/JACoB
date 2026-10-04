(() => {
  const $ = s => document.querySelector(s);
  const state = {
    socket:null,pending:new Map(),core:null,snapshot:null,seq:0,reconnectTimer:null,
    bindings:[],savedTabs:[],tabHTMLCache:new Map(),tabLoadPromises:new Map(),navLayout:{order:[],hiddenDefaults:[]},editingTabId:'',activeTab:'dashboard',themeHTML:'',quitting:false,updating:false,updateInfo:null
  };
  const isLocal = ['127.0.0.1','localhost','::1'].includes(location.hostname);

  const exampleHTML = `<!doctype html>
<html><head><style>
body{margin:0;padding:18px;background:#0a0e12;color:#e8edf0;font:14px system-ui}button{margin:4px;padding:8px 11px;background:#111315;color:#eee5dc;border:1px solid #8a430e;border-radius:0}input{padding:8px;background:#070809;color:#eee5dc;border:1px solid #3b3e41;border-radius:0}img{width:100%;background:#000;border:1px solid #333;margin-top:10px}pre{background:#050607;padding:10px;white-space:pre-wrap}
</style></head><body>
<h2>JACoB Custom Tab</h2><div><input id="system" value="Sol"><button id="type">Type text</button><button id="galaxy">Open Galaxy Map</button></div>
<button id="view">Attach game view</button><img id="game"><pre id="out">SDK ready.</pre>
<script>
const out=document.querySelector('#out');
document.querySelector('#type').onclick=async()=>{try{out.textContent=JSON.stringify(await Elite.input.text(document.querySelector('#system').value),null,2)}catch(e){out.textContent=e.code+': '+e.message}};
document.querySelector('#galaxy').onclick=async()=>{try{out.textContent=JSON.stringify(await Elite.bindings.press('GalaxyMapOpen'),null,2)}catch(e){out.textContent=e.code+': '+e.message}};
document.querySelector('#view').onclick=async()=>{try{await Elite.video.attach(document.querySelector('#game'),{width:640,fps:5,quality:55})}catch(e){out.textContent=e.code+': '+e.message}};
Elite.journal.subscribe('FSDJump',e=>out.textContent='Jumped to '+(e.StarSystem||'?'));
<\/script></body></html>`;

  const SDK = `<script>(()=>{
let seq=0;const pending=new Map(),eventSubs=new Map(),journalSubs=[];
function request(method,params={}){const id='tab-'+(++seq)+'-'+Date.now();parent.postMessage({channel:'jacob-tab',kind:'request',id,method,params},'*');return new Promise((resolve,reject)=>pending.set(id,{resolve,reject}))}
function onEvent(name,cb){if(!eventSubs.has(name))eventSubs.set(name,new Set());eventSubs.get(name).add(cb);return()=>eventSubs.get(name)?.delete(cb)}
window.Elite=Object.freeze({
 api:Object.freeze({version:5}), core:Object.freeze({ping:()=>request('core.ping')}), system:Object.freeze({health:()=>request('system.health')}), state:Object.freeze({get:()=>request('state.get'),subscribe:cb=>onEvent('status',cb)}),
 bindings:Object.freeze({list:()=>request('bindings.list'),diagnostics:()=>request('bindings.diagnostics'),get:name=>request('bindings.get',{name}),reload:()=>request('bindings.reload'),autofill:()=>request('bindings.autofill'),press:action=>request('binding.press',{action}),down:action=>request('binding.down',{action}),up:action=>request('binding.up',{action}),hold:(action,durationMs=1000)=>request('binding.hold',{action,durationMs})}),
 input:Object.freeze({tap:(key,options={})=>request('input.tap',{key,delayMs:options.delayMs||0,modifiers:options.modifiers||[]}),hold:(key,durationMs=250,options={})=>request('input.hold',{key,durationMs,modifiers:options.modifiers||[]}),text:(text,options={})=>request('input.text',{text,intervalMs:options.intervalMs??15})}),
 recorder:Object.freeze({status:()=>request('recorder.status'),start:()=>request('recorder.start'),stop:()=>request('recorder.stop'),subscribe:cb=>onEvent('recorder.input',cb)}),
 overlay:Object.freeze({info:()=>request('overlay.info'),set:scene=>request('overlay.set',{scene}),clear:()=>request('overlay.clear')}),
 video:Object.freeze({info:()=>request('video.info'),url:(options={})=>request('video.url',options),attach:async(el,options={})=>{const u=await request('video.url',options);el.src=u;return u}}),
 net:Object.freeze({fetch:(url,options={})=>request('net.fetch',{url,method:options.method||'GET',headers:options.headers||{},body:typeof options.body==='string'?options.body:(options.body==null?'':JSON.stringify(options.body))})}),
 data:Object.freeze({list:()=>request('elitefiles.list'),get:name=>request('elitefiles.get',{name}),subscribe:(name,cb)=>{const wanted=String(name||'*');return onEvent('eliteFile',payload=>{if(wanted==='*'||String(payload?.name||'').toLowerCase()===wanted.toLowerCase()||String(payload?.file||'').toLowerCase()===wanted.toLowerCase())cb(payload)})}}),
 store:Object.freeze({get:async(key,fallback=null)=>{const r=await request('tabstate.get',{key});return r?.found?r.value:fallback},set:(key,value)=>request('tabstate.set',{key,value}),delete:key=>request('tabstate.delete',{key}),clear:()=>request('tabstate.clear')}),
 files:Object.freeze({download:(name,data,options={})=>{const type=options.type||'text/plain;charset=utf-8';const body=(typeof data==='string'||data instanceof Blob)?data:JSON.stringify(data,null,options.compact?0:2);const blob=data instanceof Blob?data:new Blob([body],{type});const a=document.createElement('a');a.href=URL.createObjectURL(blob);a.download=String(name||'jacob-export.txt').replace(/[\\/:*?"<>|]+/g,'-');a.style.display='none';document.body.appendChild(a);a.click();a.remove();setTimeout(()=>URL.revokeObjectURL(a.href),1500);return{name:a.download,bytes:blob.size,type:blob.type}},json:(name,value,compact=false)=>Elite.files.download(name,value,{type:'application/json',compact})}),
 events:Object.freeze({subscribe:onEvent}), journal:Object.freeze({files:()=>request('journal.files'),read:(options={})=>request('journal.read',{file:options.file||'',event:options.event||'',offset:options.offset||0,limit:options.limit||1000}),subscribe:(eventName,cb)=>{const sub={eventName,cb};journalSubs.push(sub);return()=>{const i=journalSubs.indexOf(sub);if(i>=0)journalSubs.splice(i,1)}}})
});
addEventListener('message',ev=>{const m=ev.data;if(!m||m.channel!=='jacob-host')return;if(m.kind==='response'){const p=pending.get(m.id);if(!p)return;pending.delete(m.id);m.ok?p.resolve(m.result):p.reject(Object.assign(new Error(m.error?.message||'JACoB error'),{code:m.error?.code}));return}if(m.kind==='event'){const set=eventSubs.get(m.event);if(set)for(const cb of set)try{cb(m.data)}catch(e){console.error(e)};const all=eventSubs.get('*');if(all)for(const cb of all)try{cb(m.event,m.data)}catch(e){console.error(e)};if(m.event==='journal')for(const sub of [...journalSubs])if(sub.eventName==='*'||sub.eventName===m.data?.event)try{sub.cb(m.data)}catch(e){console.error(e)}}});
parent.postMessage({channel:'jacob-tab',kind:'ready'},'*');})();<\/script>`;


  const defaultBrand='<strong>JACoB</strong><span>Journal Aligned Control Bridge</span>';
  const defaultFooter='<span>JACoB Alpha 0.2.8</span><a href="/docs/index.html" target="_blank" rel="noopener">Documentation</a>';
  function applyAppearance(html=''){
    state.themeHTML=html||'';
    $('#jacob-user-theme').textContent='';
    $('#brand-slot').innerHTML=defaultBrand;
    $('#header-extra').innerHTML='';
    $('#nav-extra').innerHTML='';
    $('#footer-slot').innerHTML=defaultFooter;
    if($('#home-extra'))$('#home-extra').innerHTML='';
    document.body.className='';
    if(!html.trim())return;
    const doc=new DOMParser().parseFromString(html,'text/html');
    $('#jacob-user-theme').textContent=[...doc.querySelectorAll('style')].map(x=>x.textContent||'').join('\n');
    const slots={brand:'#brand-slot','header-extra':'#header-extra','nav-extra':'#nav-extra','home-extra':'#home-extra',footer:'#footer-slot'};
    for(const [name,target] of Object.entries(slots)){
      const t=doc.querySelector(`template[data-jacob-slot="${name}"]`);
      if(t)$(target).innerHTML=t.innerHTML;
    }
    const bodyClass=doc.querySelector('meta[name="jacob-body-class"]')?.getAttribute('content')||'';
    document.body.className=bodyClass;
    const title=doc.querySelector('title')?.textContent?.trim();
    document.title=title||'JACoB';
  }
  async function loadAppearance(){
    const r=await request('appearance.get');
    const html=r?.html||'';
    applyAppearance(html);
    $('#theme-html').value=html;
    $('#theme-result').textContent=html.trim()?'Custom appearance loaded.':'Default appearance is active.';
    return r;
  }
  async function saveAppearance(){
    const html=$('#theme-html').value;
    try{await request('appearance.save',{html});applyAppearance(html);$('#theme-result').textContent='Appearance saved.'}
    catch(e){$('#theme-result').textContent=pretty(e)}
  }
  async function resetAppearance(){
    try{await request('appearance.reset');$('#theme-html').value='';applyAppearance('');$('#theme-result').textContent='Default appearance restored.'}
    catch(e){$('#theme-result').textContent=pretty(e)}
  }

  function token(){return localStorage.getItem('jacob-pair-token')||''}
  function socketURL(){const proto=location.protocol==='https:'?'wss':'ws';const q=!isLocal&&token()?`?token=${encodeURIComponent(token())}`:'';return `${proto}://${location.host}/ws${q}`}
  function mediaURL(path,options={}){const q=new URLSearchParams();for(const [k,v] of Object.entries(options))if(v!==undefined&&v!==null)q.set(k,String(v));if(!isLocal&&token())q.set('token',token());q.set('_',Date.now());return `${path}?${q.toString()}`}
  function videoURL(options={}){return mediaURL('/api/video.mjpeg',{width:options.width||960,fps:options.fps||5,quality:options.quality||60})}
  function pretty(v){return JSON.stringify(v,null,2)}
  function escapeHTML(s){return String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
  function composeTabHTML(html){const lower=html.toLowerCase(),i=lower.indexOf('<head>');return i>=0?html.slice(0,i+6)+SDK+html.slice(i+6):SDK+html}

  function connect(){
    clearTimeout(state.reconnectTimer);
    if(state.socket)try{state.socket.close()}catch{}
    const ws=new WebSocket(socketURL());state.socket=ws;
    ws.onopen=()=>{setConnection(true);bootstrap()};
    ws.onclose=()=>{setConnection(false);if(state.quitting){showClosed();return}if(state.updating){$('#connection').textContent='UPDATING';state.reconnectTimer=setTimeout(connect,3000);return}state.reconnectTimer=setTimeout(connect,1800)};
    ws.onerror=()=>{};
    ws.onmessage=ev=>{let m;try{m=JSON.parse(ev.data)}catch{return}if(m.type==='response'){const p=state.pending.get(m.id);if(p){state.pending.delete(m.id);m.ok?p.resolve(m.result):p.reject(m.error)}return}if(m.type==='event')handleEvent(m.event,m.data)};
  }
  function request(method,params={}){
    if(!state.socket||state.socket.readyState!==WebSocket.OPEN)return Promise.reject({code:'OFFLINE',message:'core is offline'});
    const id=`ui-${++state.seq}-${Date.now()}`;state.socket.send(JSON.stringify({type:'request',id,method,params}));
    const timeoutMs=(method==='tabs.save'||method==='tabs.get')?90000:30000;
    return new Promise((resolve,reject)=>{state.pending.set(id,{resolve,reject});setTimeout(()=>{if(state.pending.has(id)){state.pending.delete(id);reject({code:'TIMEOUT',message:'request timed out'})}},timeoutMs)});
  }
  async function bootstrap(){try{await Promise.all([refreshHealth(),loadBindings(),loadSavedTabs(),loadAppearance()])}catch{}}

  function handleEvent(kind,data){
    addStream(kind,data);
    if(kind==='core.hello'){
      state.core=data;state.updating=false;if($('#quit-jacob'))$('#quit-jacob').hidden=!data.localClient;if($('#install-update'))$('#install-update').hidden=!data.localClient;$('#core-os').textContent=`${data.os}/${data.arch}`;$('#core-api').textContent=`v${data.apiVersion} (${data.product||'JACoB'} ${data.prototype})`;if($('#update-current'))$('#update-current').textContent=data.version||data.prototype||'—';
      $('#binding-autofill').textContent=data.autoBind?`${data.autoBind.assigned||0} added / ${data.autoBind.scannedActions||0} scanned`:'—';
      $('#input-driver').textContent=`${data.input?.driver||'—'} / ${data.input?.enabled?'enabled':'disabled'}`;$('#capture-driver').textContent=`${data.capture?.driver||'—'} / ${data.capture?.available?'available':'unavailable'}`;if($('#overlay-driver'))$('#overlay-driver').textContent=`${data.overlay?.driver||'—'} / ${data.overlay?.available?'available':'unavailable'}`;
      $('#journal-dir').textContent=data.journalDir||'not detected';$('#bindings-file').textContent=data.bindingsFile||'not detected';if($('#home-journal-state'))$('#home-journal-state').textContent=data.journalDir?'Connected':'Not detected';renderLAN(data.lan);if(data.health)renderHealth(data.health);
    }
    if(kind==='state'){state.snapshot=data;renderSnapshot(data)}
    if(kind==='status'){ $('#status-json').textContent=pretty(data);if(state.snapshot)state.snapshot.status=data }
    if(kind==='journal'){ $('#event-name').textContent=data.event||'journal';if($('#home-event-name'))$('#home-event-name').textContent=data.event||'journal';$('#journal-json').textContent=pretty(data);if(state.snapshot)state.snapshot.lastJournalEvent=data }
    if(kind==='tabs.changed')loadSavedTabs().catch(()=>{});if(kind==='core.update'){state.updating=true;if($('#update-result'))$('#update-result').textContent=`Installing ${data?.version||'update'}…`;}
    if(kind==='appearance.changed')loadAppearance().catch(()=>{});
    if(kind==='core.shutdown'){state.quitting=true;showClosed()}
    relayToAllTabs({channel:'jacob-host',kind:'event',event:kind,data});
  }

  function renderHealth(h={}){
    const el=$('#health-state'),status=(h.status||'unknown').toUpperCase();
    el.textContent=status==='OK'?'READY':status;
    el.className=`status-mark ${h.status==='ok'?'online':h.status==='warning'?'warning':'offline'}`;
    $('#health-detail').textContent=pretty(h);
    if($('#home-health-message')){
      const blockers=h.blockers||[],warnings=h.warnings||[];
      $('#home-health-message').textContent=blockers[0]||warnings[0]||(h.status==='ok'?'JACoB is ready.':'Waiting for host checks…');
    }
  }
  async function refreshHealth(){try{const h=await request('system.health');renderHealth(h);return h}catch(e){renderHealth({status:'blocked',blockers:[e.message||e.code||'health request failed']});throw e}}
  function renderSnapshot(s){
    $('#journal-json').textContent=pretty(s.lastJournalEvent||{});$('#status-json').textContent=pretty(s.status||{});$('#event-name').textContent=s.lastJournalEvent?.event||'Waiting…';
    if($('#home-event-name'))$('#home-event-name').textContent=s.lastJournalEvent?.event||'Waiting…';
    if($('#home-journal-state'))$('#home-journal-state').textContent=s.journalFile||'Waiting for journal';
  }
  function renderLAN(lan={}){$('#lan-enabled').textContent=lan.enabled?'enabled':'disabled';$('#lan-addresses').textContent=(lan.addresses||[]).map(a=>`http://${a}:${lan.port||4510}/`).join('\n')||'—';$('#pair-token').textContent=lan.pairToken||(!isLocal?'hidden on remote clients':'—');$('#remote-token').value=token()}
  function setConnection(on){const el=$('#connection');el.textContent=on?'ONLINE':'OFFLINE';el.className=`status-mark ${on?'online':'offline'}`}
  function addStream(kind,data){const row=document.createElement('div');row.className='stream-line';const summary=kind==='journal'?(data?.event||''):kind==='recorder.input'?`${data?.type||''} ${data?.key||''}`:'';row.innerHTML=`<span class="time">${new Date().toLocaleTimeString()}</span><span class="kind">${escapeHTML(kind)}</span>${escapeHTML(summary)}`;const box=$('#stream');box.prepend(row);while(box.children.length>60)box.lastChild.remove()}

  async function loadBindings(){const r=await request('bindings.list');state.bindings=r.actions||[];$('#binding-dir').textContent=r.directory||'not detected';$('#binding-active').textContent=r.activeFile||'not detected';$('#binding-source').textContent=r.activeSource||'—';$('#binding-count').textContent=String(state.bindings.length);$('#binding-health').textContent=pretty(r.diagnostics||{});renderBindingOptions()}
  function renderBindingOptions(){const filter=$('#binding-filter').value.trim().toLowerCase();const select=$('#binding-action');const before=select.value;select.innerHTML='';for(const a of state.bindings){if(filter&&!a.name.toLowerCase().includes(filter))continue;const o=document.createElement('option');o.value=a.name;o.textContent=a.name;select.appendChild(o)}if([...select.options].some(o=>o.value===before))select.value=before;renderBindingDetail()}
  function renderBindingDetail(){const a=state.bindings.find(x=>x.name===$('#binding-action').value);$('#binding-detail').textContent=a?pretty(a):'No matching action.'}

  function allSDKFrames(){return [...document.querySelectorAll('iframe.jacob-sdk-frame')]}
  function sendToFrame(frame,msg){if(frame?.contentWindow)frame.contentWindow.postMessage(msg,'*')}
  function relayToAllTabs(msg){for(const frame of allSDKFrames())sendToFrame(frame,msg)}
  function seedFrame(frame){if(state.core)sendToFrame(frame,{channel:'jacob-host',kind:'event',event:'core.hello',data:state.core});if(state.snapshot)sendToFrame(frame,{channel:'jacob-host',kind:'event',event:'state',data:state.snapshot})}

  function switchTab(tabName){
    state.activeTab=tabName;
    document.querySelectorAll('.nav').forEach(x=>x.classList.toggle('active',x.dataset.tab===tabName));
    document.querySelectorAll('.panel-page').forEach(x=>x.classList.toggle('active',x.id===tabName));
    if(tabName.startsWith('custom-'))ensureSavedTabLoaded(tabName.slice(7)).catch(e=>showSavedTabLoadError(tabName.slice(7),e));
  }
  function wireNavButton(btn){btn.onclick=()=>switchTab(btn.dataset.tab)}

  const defaultNav={dashboard:'Home',tabmanager:'Tab Manager',tutorial:'Tutorial',settings:'Settings'};
  const hideableDefaults=new Set(['dashboard','tutorial','settings']);
  function navIDForTab(tab){return `custom-${tab.id}`}
  function tabForNavID(id){return state.savedTabs.find(t=>navIDForTab(t)===id)}
  function navLabel(id){return defaultNav[id]||tabForNavID(id)?.name||id}
  function navigationOrder(){
    const expected=['dashboard',...state.savedTabs.map(navIDForTab),'tabmanager','tutorial','settings'];
    const valid=new Set(expected),out=[],seen=new Set();
    for(const id of state.navLayout?.order||[])if(valid.has(id)&&!seen.has(id)){out.push(id);seen.add(id)}
    for(const id of expected)if(!seen.has(id)){out.push(id);seen.add(id)}
    return out;
  }
  function hiddenDefaults(){return new Set(state.navLayout?.hiddenDefaults||[])}
  function renderNavigation(){
    const nav=$('#nav-items');if(!nav)return;nav.innerHTML='';const hidden=hiddenDefaults();
    for(const id of navigationOrder()){
      if(hidden.has(id)&&id!=='tabmanager')continue;
      const btn=document.createElement('button');btn.className='nav';btn.dataset.tab=id;btn.textContent=navLabel(id);btn.title=navLabel(id);wireNavButton(btn);nav.appendChild(btn);
    }
    document.querySelectorAll('.nav').forEach(x=>x.classList.toggle('active',x.dataset.tab===state.activeTab));
  }
  async function persistNavigation(order,hidden){
    try{state.navLayout=await request('tabs.layout.save',{order,hiddenDefaults:hidden});renderNavigation();renderNavigationManager();if(hidden.includes(state.activeTab)&&state.activeTab!=='tabmanager')switchTab('tabmanager');$('#navigation-result').textContent='Navigation manifest saved.'}
    catch(e){$('#navigation-result').textContent=pretty(e)}
  }
  function moveNavigation(id,delta){const order=navigationOrder(),i=order.indexOf(id),j=i+delta;if(i<0||j<0||j>=order.length)return;[order[i],order[j]]=[order[j],order[i]];persistNavigation(order,[...hiddenDefaults()])}
  function toggleDefaultNavigation(id){if(!hideableDefaults.has(id))return;const hidden=hiddenDefaults();hidden.has(id)?hidden.delete(id):hidden.add(id);persistNavigation(navigationOrder(),[...hidden])}
  function renderNavigationManager(){
    const box=$('#navigation-list');if(!box)return;box.innerHTML='';const order=navigationOrder(),hidden=hiddenDefaults();
    order.forEach((id,index)=>{const row=document.createElement('div');row.className='saved-tab-row';const meta=document.createElement('div');meta.className='saved-tab-meta';const custom=id.startsWith('custom-');meta.innerHTML=`<strong>${escapeHTML(navLabel(id))}</strong><span class="muted">${custom?'Custom tab':id==='tabmanager'?'Default · required':'Default'+(hidden.has(id)?' · hidden':'')}</span>`;const actions=document.createElement('div');actions.className='row';const up=document.createElement('button');up.className='secondary compact-button';up.textContent='↑';up.title='Move up';up.disabled=index===0;up.onclick=()=>moveNavigation(id,-1);const down=document.createElement('button');down.className='secondary compact-button';down.textContent='↓';down.title='Move down';down.disabled=index===order.length-1;down.onclick=()=>moveNavigation(id,1);actions.append(up,down);if(!custom){const visibility=document.createElement('button');visibility.className='secondary';if(id==='tabmanager'){visibility.textContent='Required';visibility.disabled=true}else{visibility.textContent=hidden.has(id)?'Show':'Hide';visibility.onclick=()=>toggleDefaultNavigation(id)}actions.appendChild(visibility)}row.append(meta,actions);box.appendChild(row)});
  }

  async function loadSavedTabs(){
    const result=await request('tabs.list');state.savedTabs=result.tabs||[];state.navLayout=result.layout||{order:[],hiddenDefaults:[]};
    const live=new Set(state.savedTabs.map(t=>t.id));
    for(const id of [...state.tabHTMLCache.keys()])if(!live.has(id))state.tabHTMLCache.delete(id);
    renderSavedTabs();return state.savedTabs;
  }
  function savedTabMeta(id){return state.savedTabs.find(t=>t.id===id)}
  function savedTabFrame(id){return document.getElementById(`custom-${id}`)?.querySelector('iframe.jacob-sdk-frame')||null}
  function showSavedTabLoadError(id,error){const frame=savedTabFrame(id);if(!frame)return;const msg=escapeHTML(error?.message||error?.code||'Could not load saved tab');frame.srcdoc=`<!doctype html><body style="background:#080b0d;color:#eee;font:14px system-ui;padding:20px"><h2>Tab load failed</h2><pre style="white-space:pre-wrap">${msg}</pre></body>`;frame.dataset.updatedAt=''}
  function applySavedTabHTML(tab){const frame=savedTabFrame(tab.id);if(!frame)return;if(frame.dataset.updatedAt===String(tab.updatedAt||''))return;frame.dataset.updatedAt=tab.updatedAt||'';frame.srcdoc=composeTabHTML(tab.html||'')}
  async function ensureSavedTabLoaded(id){
    const meta=savedTabMeta(id);if(!meta)throw{code:'TAB_NOT_FOUND',message:'saved tab not found'};
    const cached=state.tabHTMLCache.get(id);
    if(cached&&cached.updatedAt===meta.updatedAt){const tab={...meta,html:cached.html};applySavedTabHTML(tab);return tab}
    if(state.tabLoadPromises.has(id))return state.tabLoadPromises.get(id);
    const task=request('tabs.get',{id}).then(tab=>{state.tabHTMLCache.set(id,{updatedAt:tab.updatedAt||meta.updatedAt||'',html:tab.html||''});applySavedTabHTML(tab);return tab}).finally(()=>state.tabLoadPromises.delete(id));
    state.tabLoadPromises.set(id,task);return task;
  }
  function renderSavedTabs(){
    const pages=$('#custom-pages'),list=$('#saved-tab-list');
    let active=state.activeTab;
    list.innerHTML='';$('#saved-tab-count').textContent=String(state.savedTabs.length);if($('#home-tab-count'))$('#home-tab-count').textContent=String(state.savedTabs.length);
    const wanted=new Set(state.savedTabs.map(t=>`custom-${t.id}`));
    for(const existing of [...pages.querySelectorAll('.custom-user-page')])if(!wanted.has(existing.id))existing.remove();
    for(const tab of state.savedTabs){
      const pageID=`custom-${tab.id}`;
      let section=document.getElementById(pageID);let iframe=section?.querySelector('iframe.jacob-sdk-frame');
      if(!section){section=document.createElement('section');section.id=pageID;section.className='panel-page custom-user-page';iframe=document.createElement('iframe');iframe.className='jacob-sdk-frame saved-tab-frame';iframe.sandbox='allow-scripts allow-downloads';iframe.dataset.savedTabId=tab.id;section.appendChild(iframe);pages.appendChild(section)}
      const cached=state.tabHTMLCache.get(tab.id);
      if(cached&&cached.updatedAt===tab.updatedAt)applySavedTabHTML({...tab,html:cached.html});else if(iframe.dataset.updatedAt&&iframe.dataset.updatedAt!==String(tab.updatedAt||'')){iframe.removeAttribute('srcdoc');iframe.dataset.updatedAt=''}
      const size=Number(tab.sizeBytes||0);const sizeLabel=size?` · ${size>=1024*1024?(size/1024/1024).toFixed(2)+' MB':(size/1024).toFixed(1)+' KB'}`:'';
      const row=document.createElement('div');row.className='saved-tab-row';const meta=document.createElement('div');meta.className='saved-tab-meta';meta.innerHTML=`<strong>${escapeHTML(tab.name)}</strong><span class="muted mono">${escapeHTML(tab.id)}${sizeLabel}</span>`;const actions=document.createElement('div');actions.className='row';const open=document.createElement('button');open.textContent='Open';open.onclick=()=>switchTab(pageID);const edit=document.createElement('button');edit.textContent='Edit';edit.className='secondary';edit.onclick=()=>editSavedTab(tab.id);const del=document.createElement('button');del.textContent='Remove';del.className='danger';del.onclick=()=>deleteSavedTab(tab.id);actions.append(open,edit,del);row.append(meta,actions);list.appendChild(row);
    }
    if(!state.savedTabs.length)list.innerHTML='<div class="muted empty-state">No saved tabs in the manifest.</div>';
    renderNavigation();renderNavigationManager();
    if(hiddenDefaults().has(active)&&active!=='tabmanager')active=navigationOrder().find(id=>id==='tabmanager'||!hiddenDefaults().has(id))||'tabmanager';
    if(active.startsWith('custom-')&&!document.getElementById(active))switchTab('tabmanager');else switchTab(active);
  }

  function previewCurrent(){request('overlay.clear',{layer:'preview'}).catch(()=>{});const frame=$('#custom-frame');frame.dataset.overlayLayer='preview';frame.srcdoc=composeTabHTML($('#custom-html').value)}
  function clearEditor(useExample=false){state.editingTabId='';$('#custom-tab-name').value=useExample?'Example Tab':'';$('#custom-html').value=useExample?exampleHTML:'';$('#editing-tab-label').textContent='New tab';$('#tab-manager-result').textContent='Editing a new tab.';previewCurrent()}
  async function editSavedTab(id){const meta=savedTabMeta(id);if(!meta)return;switchTab('tabmanager');$('#tab-manager-result').textContent=`Loading ${meta.name}…`;try{const tab=await ensureSavedTabLoaded(id);state.editingTabId=id;$('#custom-tab-name').value=tab.name;$('#custom-html').value=tab.html||'';$('#editing-tab-label').textContent=`Editing ${tab.name}`;$('#tab-manager-result').textContent=`Loaded ${tab.name} for editing.`;previewCurrent()}catch(e){$('#tab-manager-result').textContent=pretty(e)}}
  async function saveCurrentTab(){
    const name=$('#custom-tab-name').value.trim(),html=$('#custom-html').value;
    if(!name){$('#tab-manager-result').textContent='Give the tab a name before saving.';$('#custom-tab-name').focus();return}
    try{
      const tab=await request('tabs.save',{id:state.editingTabId||'',name,html});request('overlay.clear',{layer:'preview'}).catch(()=>{});state.editingTabId=tab.id;state.tabHTMLCache.set(tab.id,{updatedAt:tab.updatedAt||'',html});$('#editing-tab-label').textContent=`Editing ${tab.name}`;$('#tab-manager-result').textContent=`Saved ${tab.name}. It is now a persistent JACoB tab.`;
      await loadSavedTabs();switchTab(`custom-${tab.id}`);
    }catch(e){$('#tab-manager-result').textContent=pretty(e)}
  }
  async function deleteSavedTab(id){
    const tab=state.savedTabs.find(t=>t.id===id);if(!tab)return;
    if(!confirm(`Remove saved tab "${tab.name}"?`))return;
    try{await request('tabs.delete',{id});state.tabHTMLCache.delete(id);state.tabLoadPromises.delete(id);if(state.editingTabId===id)clearEditor(false);await loadSavedTabs();$('#tab-manager-result').textContent=`Removed ${tab.name}.`}catch(e){$('#tab-manager-result').textContent=pretty(e)}
  }


  function showClosed(){
    clearTimeout(state.reconnectTimer);
    stopVideo();
    const screen=$('#shutdown-screen');
    if(screen)screen.hidden=false;
    const quit=$('#quit-jacob');
    if(quit){quit.disabled=true;quit.textContent='Closed'}
    setConnection(false);
    $('#connection').textContent='CLOSED';
  }
  async function quitJACoB(){
    if(!state.core?.localClient)return;
    if(!confirm('Close JACoB?\n\nSaved tabs and settings will be kept.'))return;
    const quit=$('#quit-jacob');
    quit.disabled=true;quit.textContent='Closing…';state.quitting=true;
    try{await request('core.shutdown')}catch(e){
      state.quitting=false;quit.disabled=false;quit.textContent='Quit JACoB';
      alert(e?.message||'JACoB could not be closed.');
    }
  }

  $('#quit-jacob').onclick=quitJACoB;
  $('#ping').onclick=async()=>{try{const r=await request('core.ping');$('#ping-result').textContent=`pong ${new Date(r.time).toLocaleTimeString()}`}catch(e){$('#ping-result').textContent=e.message||e.code||'failed'}};
  $('#refresh-health').onclick=refreshHealth;
  $('#autofill-bindings').onclick=async()=>{try{const r=await request('bindings.autofill');$('#binding-autofill').textContent=`${r.assigned||0} added / ${r.scannedActions||0} scanned`;$('#binding-detail').textContent=pretty(r);await refreshHealth();await loadBindings()}catch(e){$('#binding-detail').textContent=pretty(e);await refreshHealth().catch(()=>{})}};
  $('#reload-bindings').onclick=async()=>{try{await request('bindings.reload');await loadBindings()}catch(e){$('#binding-detail').textContent=pretty(e)}};
  $('#binding-filter').oninput=renderBindingOptions;$('#binding-action').onchange=renderBindingDetail;
  $('#press-binding').onclick=async()=>{try{$('#binding-detail').textContent=pretty(await request('binding.press',{action:$('#binding-action').value}))}catch(e){$('#binding-detail').textContent=pretty(e)}};
  $('#save-token').onclick=()=>{const t=$('#remote-token').value.trim();if(t)localStorage.setItem('jacob-pair-token',t);else localStorage.removeItem('jacob-pair-token');$('#network-result').textContent='Pair token saved. Reconnecting…';stopVideo();connect()};

  async function checkForUpdates(){
    const out=$('#update-result'),btn=$('#check-update'),install=$('#install-update');btn.disabled=true;install.disabled=true;out.textContent='Querying the release channel…';
    try{const info=await request('update.check');state.updateInfo=info;$('#update-latest').textContent=info.latestVersion||'—';if(info.available){out.textContent=`${info.releaseName||('JACoB '+info.latestVersion)} is available.${info.installable?' Ready for installation.':' '+(info.installNote||'Manual installation required.')}`;install.disabled=!(info.installable&&state.core?.localClient)}else{out.textContent='Current release confirmed. No newer published build was found.'}}
    catch(e){out.textContent=e?.message||pretty(e)}finally{btn.disabled=false}
  }
  async function installUpdate(){
    const info=state.updateInfo;if(!info?.available)return;if(!confirm(`Install JACoB ${info.latestVersion}?\n\nJACoB will restart after the update is staged.`))return;
    const btn=$('#install-update');btn.disabled=true;state.updating=true;$('#update-result').textContent='Downloading update package…';
    try{const r=await request('update.install');$('#update-result').textContent=`Installing ${r.version||info.latestVersion}…`;$('#connection').textContent='UPDATING'}catch(e){state.updating=false;btn.disabled=false;$('#update-result').textContent=e?.message||pretty(e)}
  }
  $('#check-update').onclick=checkForUpdates;$('#install-update').onclick=installUpdate;

  function setVideoState(on,msg=''){const el=$('#video-state');el.textContent=on?'STREAMING':'STOPPED';el.className=`status-mark ${on?'online':'offline'}`;if(msg)$('#video-result').textContent=msg}
  function startVideo(){if(state.core?.capture&&!state.core.capture.available){setVideoState(false,`Capture unavailable: ${state.core.capture.driver}`);return}const opts={width:Number($('#video-width').value),fps:Number($('#video-fps').value),quality:Number($('#video-quality').value)};$('#game-view-img').src=videoURL(opts);setVideoState(true,pretty({mode:'mjpeg',...opts,url:'authenticated local/LAN stream'}))}
  function stopVideo(){$('#game-view-img').removeAttribute('src');setVideoState(false)}
  $('#video-start').onclick=startVideo;$('#video-stop').onclick=stopVideo;$('#video-snapshot').onclick=()=>{const opts={width:Number($('#video-width').value),quality:Number($('#video-quality').value)};$('#game-view-img').src=mediaURL('/api/video/frame.jpg',opts);setVideoState(false,pretty({mode:'snapshot',...opts}))};

  $('#run-tab').onclick=previewCurrent;$('#save-tab').onclick=saveCurrentTab;$('#new-tab').onclick=()=>clearEditor(false);$('#reset-tab').onclick=()=>clearEditor(true);
  $('#upload-html').onchange=async e=>{const f=e.target.files?.[0];if(!f)return;$('#custom-html').value=await f.text();if(!$('#custom-tab-name').value.trim())$('#custom-tab-name').value=f.name.replace(/\.html?$/i,'');previewCurrent();e.target.value=''};
  $('#save-theme').onclick=saveAppearance;$('#reset-theme').onclick=resetAppearance;
  $('#upload-theme').onchange=async e=>{const f=e.target.files?.[0];if(!f)return;$('#theme-html').value=await f.text();await saveAppearance();e.target.value=''};
  if($('#home-add-tab'))$('#home-add-tab').onclick=()=>switchTab('tabmanager');
  if($('#home-tutorial'))$('#home-tutorial').onclick=()=>switchTab('tutorial');
  if($('#tutorial-tab-manager'))$('#tutorial-tab-manager').onclick=()=>switchTab('tabmanager');

  function overlayLayerForFrame(frame){if(frame?.dataset?.savedTabId)return `tab:${frame.dataset.savedTabId}`;return frame?.dataset?.overlayLayer||'preview'}

  addEventListener('message',async ev=>{
    const frame=allSDKFrames().find(f=>f.contentWindow===ev.source);if(!frame)return;
    const m=ev.data;if(!m||m.channel!=='jacob-tab')return;
    if(m.kind==='ready'){seedFrame(frame);return}
    if(m.kind==='request'){
      if(m.method==='video.url'){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:true,result:videoURL(m.params||{})});return}
      const allowed=new Set(['core.ping','system.health','state.get','bindings.list','bindings.diagnostics','bindings.get','bindings.reload','bindings.autofill','input.tap','input.hold','input.text','binding.press','binding.down','binding.up','binding.hold','recorder.status','recorder.start','recorder.stop','video.info','overlay.info','overlay.set','overlay.clear','net.fetch','elitefiles.list','elitefiles.get','journal.files','journal.read','tabstate.get','tabstate.set','tabstate.delete','tabstate.clear']);
      if(!allowed.has(m.method)){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error:{code:'SDK_DENIED',message:'method not exposed by JACoB SDK'}});return}
      try{let params=m.params||{};if(m.method==='overlay.set'||m.method==='overlay.clear')params={...params,layer:overlayLayerForFrame(frame)};if(m.method.startsWith('tabstate.')){const tabId=frame?.dataset?.savedTabId||'';if(!tabId)throw{code:'TAB_STATE_PREVIEW',message:'persistent state is available after the tab is saved'};params={...params,tabId}}const result=await request(m.method,params);sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:true,result})}catch(error){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error})}
    }
  });

  $('#custom-html').value=localStorage.getItem('jacob-p2-custom-html')||exampleHTML;
  $('#custom-tab-name').value='Custom Tab';
  previewCurrent();

  // Render built-in navigation before the core connection succeeds so a
  // brand-new LAN client can reach Settings and enter its pairing token.
  renderNavigation();

  // New remote browsers have no token in localStorage yet. Open Settings
  // immediately instead of leaving them on an unauthenticated shell.
  if (!isLocal && !token()) {
    switchTab('settings');
    $('#network-result').textContent =
      'Enter the pair token shown on the computer running JACoB.';
    $('#remote-token').focus();
  }

  connect();
})();
