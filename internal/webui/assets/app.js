/* JACoB local pairing QR renderer.
   Encodes the JACoB pairing token as QR Version 2-L, byte mode (up to 32 UTF-8 bytes).
   No network requests, third-party services, or token disclosure leave the host. */
(() => {
  'use strict';

  const SIZE = 25;
  const DATA_CODEWORDS = 34;
  const ECC_CODEWORDS = 10;

  function gfMul(x, y) {
    let z = 0;
    for (let i = 7; i >= 0; i--) {
      z = (z << 1) ^ ((z >>> 7) * 0x11D);
      if (((y >>> i) & 1) !== 0) z ^= x;
    }
    return z;
  }

  function rsDivisor(degree) {
    const result = Array(degree).fill(0);
    result[degree - 1] = 1;
    let root = 1;
    for (let i = 0; i < degree; i++) {
      for (let j = 0; j < degree; j++) {
        result[j] = gfMul(result[j], root);
        if (j + 1 < degree) result[j] ^= result[j + 1];
      }
      root = gfMul(root, 0x02);
    }
    return result;
  }

  function rsRemainder(data, degree) {
    const divisor = rsDivisor(degree);
    const result = Array(degree).fill(0);
    for (const b of data) {
      const factor = b ^ result.shift();
      result.push(0);
      for (let i = 0; i < result.length; i++) result[i] ^= gfMul(divisor[i], factor);
    }
    return result;
  }

  function appendBits(out, value, count) {
    for (let i = count - 1; i >= 0; i--) out.push((value >>> i) & 1);
  }

  function dataCodewords(text) {
    const bytes = new TextEncoder().encode(String(text || ''));
    if (bytes.length < 1 || bytes.length > 32) throw new Error('Pair token must be 1-32 UTF-8 bytes for QR v2-L.');
    const bits = [];
    appendBits(bits, 0x4, 4); // Byte mode
    appendBits(bits, bytes.length, 8);
    for (const b of bytes) appendBits(bits, b, 8);
    const capacity = DATA_CODEWORDS * 8;
    appendBits(bits, 0, Math.min(4, capacity - bits.length));
    while (bits.length % 8) bits.push(0);
    const out = [];
    for (let i = 0; i < bits.length; i += 8) {
      let b = 0;
      for (let j = 0; j < 8; j++) b = (b << 1) | bits[i + j];
      out.push(b);
    }
    for (let pad = 0; out.length < DATA_CODEWORDS; pad++) out.push(pad % 2 === 0 ? 0xEC : 0x11);
    return out;
  }

  function formatBits(mask) {
    // Error-correction level L has format value 01.
    const data = (1 << 3) | mask;
    let rem = data << 10;
    const generator = 0x537;
    for (let i = 14; i >= 10; i--) if (((rem >>> i) & 1) !== 0) rem ^= generator << (i - 10);
    return ((data << 10) | rem) ^ 0x5412;
  }

  function matrix(text) {
    const data = dataCodewords(text);
    const codewords = data.concat(rsRemainder(data, ECC_CODEWORDS));
    const bits = [];
    for (const b of codewords) appendBits(bits, b, 8);

    const mod = Array.from({ length: SIZE }, () => Array(SIZE).fill(false));
    const fun = Array.from({ length: SIZE }, () => Array(SIZE).fill(false));
    const setFun = (x, y, dark) => { if (x >= 0 && y >= 0 && x < SIZE && y < SIZE) { mod[y][x] = !!dark; fun[y][x] = true; } };

    function finder(x, y) {
      for (let dy = -1; dy <= 7; dy++) for (let dx = -1; dx <= 7; dx++) {
        const xx = x + dx, yy = y + dy;
        if (xx < 0 || yy < 0 || xx >= SIZE || yy >= SIZE) continue;
        const inPattern = dx >= 0 && dx <= 6 && dy >= 0 && dy <= 6;
        let dark = false;
        if (inPattern) {
          const d = Math.max(Math.abs(dx - 3), Math.abs(dy - 3));
          dark = d === 3 || d <= 1;
        }
        setFun(xx, yy, dark);
      }
    }

    finder(0, 0); finder(SIZE - 7, 0); finder(0, SIZE - 7);

    // QR Version 2 has one non-overlapping alignment pattern at (18,18).
    for (let dy = -2; dy <= 2; dy++) for (let dx = -2; dx <= 2; dx++) {
      const d = Math.max(Math.abs(dx), Math.abs(dy));
      setFun(18 + dx, 18 + dy, d === 2 || d === 0);
    }

    for (let i = 8; i < SIZE - 8; i++) {
      setFun(i, 6, i % 2 === 0);
      setFun(6, i, i % 2 === 0);
    }

    // Reserve both copies of the 15 format modules before data placement.
    for (let i = 0; i <= 5; i++) setFun(8, i, false);
    setFun(8, 7, false); setFun(8, 8, false); setFun(7, 8, false);
    for (let i = 9; i < 15; i++) setFun(14 - i, 8, false);
    for (let i = 0; i < 8; i++) setFun(SIZE - 1 - i, 8, false);
    for (let i = 8; i < 15; i++) setFun(8, SIZE - 15 + i, false);
    setFun(8, SIZE - 8, true); // fixed dark module

    // Fixed mask 0 is standards-compliant and deterministic for local pairing tokens.
    const mask = 0;
    let bitIndex = 0;
    for (let right = SIZE - 1; right >= 1; right -= 2) {
      if (right === 6) right--;
      for (let vert = 0; vert < SIZE; vert++) {
        const y = (((right + 1) & 2) === 0) ? SIZE - 1 - vert : vert;
        for (let j = 0; j < 2; j++) {
          const x = right - j;
          if (fun[y][x]) continue;
          const bit = bitIndex < bits.length ? bits[bitIndex] !== 0 : false;
          const masked = bit !== ((x + y) % 2 === 0);
          mod[y][x] = masked;
          bitIndex++;
        }
      }
    }

    const fmt = formatBits(mask);
    const get = i => ((fmt >>> i) & 1) !== 0;
    for (let i = 0; i <= 5; i++) setFun(8, i, get(i));
    setFun(8, 7, get(6)); setFun(8, 8, get(7)); setFun(7, 8, get(8));
    for (let i = 9; i < 15; i++) setFun(14 - i, 8, get(i));
    for (let i = 0; i < 8; i++) setFun(SIZE - 1 - i, 8, get(i));
    for (let i = 8; i < 15; i++) setFun(8, SIZE - 15 + i, get(i));
    setFun(8, SIZE - 8, true);
    return mod;
  }

  function toSVG(text, border = 4) {
    const m = matrix(text), dim = SIZE + border * 2;
    let d = '';
    for (let y = 0; y < SIZE; y++) for (let x = 0; x < SIZE; x++) if (m[y][x]) d += `M${x + border},${y + border}h1v1h-1z`;
    return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${dim} ${dim}" role="img" aria-label="JACoB pairing token QR code" shape-rendering="crispEdges"><rect width="100%" height="100%" fill="#fff"/><path d="${d}" fill="#000"/></svg>`;
  }

  function render(el, text) {
    if (!el) return false;
    try { el.innerHTML = toSVG(text); return true; }
    catch (err) { el.textContent = 'QR unavailable'; el.title = err.message || String(err); return false; }
  }

  globalThis.PairQR = Object.freeze({ matrix, toSVG, render });
})();

(() => {
  const $ = s => document.querySelector(s);
  const state = {
    socket:null,pending:new Map(),core:null,snapshot:null,seq:0,reconnectTimer:null,
    bindings:[],savedTabs:[],tabHTMLCache:new Map(),tabLoadPromises:new Map(),navLayout:{order:[],hiddenDefaults:[]},editingTabId:'',activeTab:'dashboard',themeHTML:'',locale:{language:'en',supported:[]},quitting:false,updating:false,updateInfo:null,actionRegistry:new Map(),actionInvocations:new Map(),actionSeq:0,actionDiscoveryDone:false,actionDiscoveryPromise:null
  };
  const isLocal = ['127.0.0.1','localhost','::1'].includes(location.hostname);
  const UPDATE_CHECK_INTERVAL_MS=30*60*1000,UPDATE_BADGE_KEY='jacob-update-badge-dismissed';
  let updateCheckTimer=null,updateCheckKickoff=null;
  function dismissedUpdateVersion(){try{return localStorage.getItem(UPDATE_BADGE_KEY)||''}catch{return''}}
  function updateBadgeVisible(){const i=state.updateInfo;return !!(i?.available&&i.latestVersion&&dismissedUpdateVersion()!==String(i.latestVersion))}
  function decorateSettingsUpdateBadge(btn){if(!btn||!updateBadgeVisible())return;const badge=document.createElement('span');badge.className='settings-update-badge';badge.textContent='NEW';badge.setAttribute('aria-hidden','true');btn.appendChild(badge);btn.setAttribute('aria-label',`${navLabel('settings')} — ${tr('update available')}`)}
  function dismissSettingsUpdateBadge(){const i=state.updateInfo;if(i?.available&&i.latestVersion){try{localStorage.setItem(UPDATE_BADGE_KEY,String(i.latestVersion))}catch{}}renderNavigation()}
  function queueUpdateCheckSoon(){clearTimeout(updateCheckKickoff);updateCheckKickoff=setTimeout(()=>checkForUpdates({quiet:true}),5000)}
  function startPeriodicUpdateChecks(){clearInterval(updateCheckTimer);updateCheckTimer=setInterval(()=>checkForUpdates({quiet:true}),UPDATE_CHECK_INTERVAL_MS)}

  const I18N=window.JACOB_I18N||{supported:[{code:'en',name:'English',nativeName:'English'}],strings:{}};
  const textSources=new WeakMap(),attrSources=new WeakMap();
  function interpolate(text,vars={}){return String(text??'').replace(/\{([A-Za-z0-9_]+)\}/g,(m,k)=>Object.prototype.hasOwnProperty.call(vars,k)?String(vars[k]):m)}
  function tr(source,vars={}){const lang=state.locale?.language||'en',dict=I18N.strings?.[lang]||{};return interpolate(dict[source]??source,vars)}
  function localizeDOM(root=document.body){
    if(!root)return;
    const walker=document.createTreeWalker(root,NodeFilter.SHOW_TEXT);
    let node;
    while((node=walker.nextNode())){
      const parent=node.parentElement;if(!parent||parent.closest('script,style,textarea,code,[data-jacob-no-i18n]'))continue;
      let rec=textSources.get(node),current=node.nodeValue||'';
      if(!rec||current!==rec.last)rec={source:current,last:current};
      const match=rec.source.match(/^(\s*)([\s\S]*?)(\s*)$/),key=(match?.[2]||'').trim();
      if(key){const translated=tr(key);rec.last=(match?.[1]||'')+translated+(match?.[3]||'');node.nodeValue=rec.last}else rec.last=current;
      textSources.set(node,rec);
    }
    for(const el of root.querySelectorAll?.('[placeholder],[alt],[title],[aria-label]')||[]){
      if(el.closest('[data-jacob-no-i18n]'))continue;
      let rec=attrSources.get(el)||{};
      for(const attr of ['placeholder','alt','title','aria-label']){
        if(!el.hasAttribute(attr))continue;const current=el.getAttribute(attr)||'',old=rec[attr];
        if(!old||current!==old.last)rec[attr]={source:current,last:current};
        const item=rec[attr];item.last=tr(item.source);el.setAttribute(attr,item.last);
      }
      attrSources.set(el,rec);
    }
  }
  function populateLanguageSelect(){const sel=$('#language-select');if(!sel)return;const langs=state.locale?.supported?.length?state.locale.supported:I18N.supported,before=state.locale?.language||'en';sel.innerHTML=langs.map(x=>`<option value="${escapeHTML(x.code)}">${escapeHTML(x.nativeName||x.name||x.code)}</option>`).join('');sel.value=before}
  function applyLocaleInfo(info={}){const supported=Array.isArray(info.supported)&&info.supported.length?info.supported:I18N.supported;state.locale={language:String(info.language||state.locale?.language||'en'),supported};document.documentElement.lang=state.locale.language;document.documentElement.dir='ltr';populateLanguageSelect();localizeDOM(document.body);if($('#language-result'))$('#language-result').textContent=tr('Language is shared by all connected JACoB browsers.');renderNavigation();if($('#navigation-list'))renderNavigationManager()}
  async function loadLocale(){const r=await request('locale.get');applyLocaleInfo(r);return r}
  async function saveLocale(){const sel=$('#language-select');if(!sel)return;try{const r=await request('locale.save',{language:sel.value});applyLocaleInfo(r);$('#language-result').textContent=tr('Language is shared by all connected JACoB browsers.')}catch(e){$('#language-result').textContent=e?.message||pretty(e)}}

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
let seq=0;const pending=new Map(),eventSubs=new Map(),journalSubs=[],actionHandlers=new Map();
let localeState={language:'en',supported:[{code:'en',name:'English',nativeName:'English'},{code:'ru',name:'Russian',nativeName:'Русский'},{code:'de',name:'German',nativeName:'Deutsch'},{code:'fr',name:'French',nativeName:'Français'},{code:'zh-CN',name:'Simplified Chinese',nativeName:'简体中文'},{code:'es',name:'Spanish',nativeName:'Español'}]};
function localeInterpolate(text,vars={}){return String(text??'').replace(/\{([A-Za-z0-9_]+)\}/g,(m,k)=>Object.prototype.hasOwnProperty.call(vars,k)?String(vars[k]):m)}
function localeTranslate(dictionary,key,vars={}){const lang=localeState.language||'en',table=dictionary?.[lang]||dictionary?.[lang.split('-')[0]]||dictionary?.en||{};return localeInterpolate(table?.[key]??dictionary?.en?.[key]??key,vars)}
// Custom tabs are intentionally networkless except through Elite.net.fetch().
// CSP blocks fetch/XHR/WebSocket/image/form channels. Browser navigation is not
// covered by a shipping CSP directive, so stop external iframe navigations too.
function safeTabNavigation(raw,download=false){try{const u=new URL(String(raw||''),location.href);if(u.protocol==='blob:')return !!download;if(u.protocol==='about:')return true;if(u.href.startsWith(location.href.split('#')[0]+'#'))return true;return false}catch{return false}}
if(globalThis.navigation?.addEventListener)globalThis.navigation.addEventListener('navigate',ev=>{if(!safeTabNavigation(ev.destination?.url,ev.downloadRequest!==null)){try{ev.preventDefault()}catch{}}});
addEventListener('click',ev=>{const a=ev.target?.closest?.('a[href]');if(a&&!safeTabNavigation(a.href,a.hasAttribute('download'))){ev.preventDefault();ev.stopImmediatePropagation()}},true);
addEventListener('submit',ev=>{ev.preventDefault();ev.stopImmediatePropagation()},true);
try{Object.defineProperty(window,'open',{value:()=>null,writable:false,configurable:false})}catch{}
function request(method,params={}){const id='tab-'+(++seq)+'-'+Date.now();parent.postMessage({channel:'jacob-tab',kind:'request',id,method,params},'*');return new Promise((resolve,reject)=>pending.set(id,{resolve,reject}))}
function onEvent(name,cb){if(!eventSubs.has(name))eventSubs.set(name,new Set());eventSubs.get(name).add(cb);return()=>eventSubs.get(name)?.delete(cb)}
window.Elite=Object.freeze({
 api:Object.freeze({version:9}), core:Object.freeze({ping:()=>request('core.ping')}), system:Object.freeze({health:()=>request('system.health')}), state:Object.freeze({get:()=>request('state.get'),subscribe:cb=>onEvent('status',cb)}),
 bindings:Object.freeze({list:()=>request('bindings.list'),diagnostics:()=>request('bindings.diagnostics'),get:name=>request('bindings.get',{name}),reload:()=>request('bindings.reload'),press:action=>request('binding.press',{action}),down:action=>request('binding.down',{action}),up:action=>request('binding.up',{action}),hold:(action,durationMs=1000)=>request('binding.hold',{action,durationMs})}),
 input:Object.freeze({tap:(key,options={})=>request('input.tap',{key,delayMs:options.delayMs||0,modifiers:options.modifiers||[]}),hold:(key,durationMs=250,options={})=>request('input.hold',{key,durationMs,modifiers:options.modifiers||[]}),text:(text,options={})=>request('input.text',{text,intervalMs:options.intervalMs??15})}),
 recorder:Object.freeze({status:()=>request('recorder.status'),start:()=>request('recorder.start'),stop:()=>request('recorder.stop'),subscribe:cb=>onEvent('recorder.input',cb)}),
 overlay:Object.freeze({info:()=>request('overlay.info'),set:scene=>request('overlay.set',{scene}),clear:()=>request('overlay.clear')}),
 video:Object.freeze({info:()=>request('video.info'),url:(options={})=>request('video.url',options),attach:async(el,options={})=>{const u=await request('video.url',options);el.src=u;return u}}),
 vision:Object.freeze({info:()=>request('vision.info'),sample:(options={})=>request('vision.sample',options)}),
 net:Object.freeze({fetch:(url,options={})=>request('net.fetch',{url,method:options.method||'GET',headers:options.headers||{},body:typeof options.body==='string'?options.body:(options.body==null?'':JSON.stringify(options.body))})}),
 data:Object.freeze({list:()=>request('elitefiles.list'),get:name=>request('elitefiles.get',{name}),subscribe:(name,cb)=>{const wanted=String(name||'*');return onEvent('eliteFile',payload=>{if(wanted==='*'||String(payload?.name||'').toLowerCase()===wanted.toLowerCase()||String(payload?.file||'').toLowerCase()===wanted.toLowerCase())cb(payload)})}}),
 store:Object.freeze({get:async(key,fallback=null)=>{const r=await request('tabstate.get',{key});return r?.found?r.value:fallback},set:(key,value)=>request('tabstate.set',{key,value}),delete:key=>request('tabstate.delete',{key}),clear:()=>request('tabstate.clear')}),
 actions:Object.freeze({
  register:async(name,handler,options={})=>{name=String(name||'').trim();if(!name)throw Object.assign(new Error('action name is required'),{code:'BAD_PARAMS'});if(typeof handler!=='function')throw Object.assign(new Error('action handler must be a function'),{code:'BAD_PARAMS'});actionHandlers.set(name,handler);try{await request('actions.register',{name,label:String(options.label||''),description:String(options.description||'')})}catch(error){if(actionHandlers.get(name)===handler)actionHandlers.delete(name);throw error}return async()=>{if(actionHandlers.get(name)===handler)actionHandlers.delete(name);try{return await request('actions.unregister',{name})}catch(error){if(error?.code!=='ACTION_NOT_FOUND')throw error;return{unregistered:true,name}}}},
  unregister:async name=>{name=String(name||'').trim();actionHandlers.delete(name);return request('actions.unregister',{name})},
  list:(options={})=>request('actions.list',{discover:options.discover!==false}),
  invoke:(name,payload=null,options={})=>request('actions.invoke',{name:String(name||'').trim(),payload,timeoutMs:Number(options.timeoutMs)||15000,discover:options.discover!==false})
 }),
 tabs:Object.freeze({list:()=>request('tabs.sdk.list'),activate:target=>request('tabs.activate',{target:String(target||'')})}),
 locale:Object.freeze({get language(){return localeState.language},get supported(){return localeState.supported.map(x=>({...x}))},get:()=>request('locale.get'),t:(dictionary,key,vars={})=>localeTranslate(dictionary,key,vars),subscribe:cb=>{const off=onEvent('locale.changed',cb);queueMicrotask(()=>cb({...localeState,supported:localeState.supported.map(x=>({...x}))}));return off}}),
 files:Object.freeze({download:(name,data,options={})=>{const type=options.type||'text/plain;charset=utf-8';const body=(typeof data==='string'||data instanceof Blob)?data:JSON.stringify(data,null,options.compact?0:2);const blob=data instanceof Blob?data:new Blob([body],{type});const a=document.createElement('a');a.href=URL.createObjectURL(blob);a.download=String(name||'jacob-export.txt').replace(/[\\/:*?"<>|]+/g,'-');a.style.display='none';document.body.appendChild(a);a.click();a.remove();setTimeout(()=>URL.revokeObjectURL(a.href),1500);return{name:a.download,bytes:blob.size,type:blob.type}},json:(name,value,compact=false)=>Elite.files.download(name,value,{type:'application/json',compact})}),
 events:Object.freeze({subscribe:onEvent}), journal:Object.freeze({files:()=>request('journal.files'),read:(options={})=>request('journal.read',{file:options.file||'',event:options.event||'',offset:options.offset||0,limit:options.limit||1000}),subscribe:(eventName,cb)=>{const sub={eventName,cb};journalSubs.push(sub);return()=>{const i=journalSubs.indexOf(sub);if(i>=0)journalSubs.splice(i,1)}}})
});
addEventListener('message',ev=>{const m=ev.data;if(!m||m.channel!=='jacob-host')return;if(m.kind==='response'){const p=pending.get(m.id);if(!p)return;pending.delete(m.id);m.ok?p.resolve(m.result):p.reject(Object.assign(new Error(m.error?.message||'JACoB error'),{code:m.error?.code}));return}if(m.kind==='action-invoke'){const handler=actionHandlers.get(String(m.action||''));if(!handler){parent.postMessage({channel:'jacob-tab',kind:'action-result',id:m.id,ok:false,error:{code:'ACTION_NOT_FOUND',message:'registered action is unavailable'}},'*');return}Promise.resolve().then(()=>handler(m.payload,{action:m.action,caller:m.caller||null})).then(result=>parent.postMessage({channel:'jacob-tab',kind:'action-result',id:m.id,ok:true,result},'*')).catch(error=>parent.postMessage({channel:'jacob-tab',kind:'action-result',id:m.id,ok:false,error:{code:String(error?.code||'ACTION_FAILED'),message:String(error?.message||error||'action failed')}},'*'));return}if(m.kind==='event'){if(m.event==='locale.changed'&&m.data)localeState={language:m.data.language||localeState.language,supported:Array.isArray(m.data.supported)&&m.data.supported.length?m.data.supported:localeState.supported};if(m.event==='core.hello'&&m.data?.locale)localeState={language:m.data.locale.language||localeState.language,supported:Array.isArray(m.data.locale.supported)&&m.data.locale.supported.length?m.data.locale.supported:localeState.supported};const set=eventSubs.get(m.event);if(set)for(const cb of set)try{cb(m.data)}catch(e){console.error(e)};const all=eventSubs.get('*');if(all)for(const cb of all)try{cb(m.event,m.data)}catch(e){console.error(e)};if(m.event==='journal')for(const sub of [...journalSubs])if(sub.eventName==='*'||sub.eventName===m.data?.event)try{sub.cb(m.data)}catch(e){console.error(e)}}});
parent.postMessage({channel:'jacob-tab',kind:'ready'},'*');})();<\/script>`;


  const APPEARANCE_TAGS=new Set(['div','span','strong','b','em','i','small','p','h1','h2','h3','h4','br','hr','a','ul','ol','li']);
  function sanitizeAppearanceCSS(css=''){
    return String(css)
      .replace(/@import[\s\S]*?;/gi,'')
      .replace(/url\s*\([^)]*\)/gi,'none')
      .replace(/expression\s*\([^)]*\)/gi,'')
      .replace(/-moz-binding\s*:[^;}]+/gi,'');
  }
  function safeAppearanceHref(value=''){
    const v=String(value).trim();
    if(v.startsWith('#'))return v;
    if(v.startsWith('/')&&!v.startsWith('//'))return v;
    return '';
  }
  function sanitizeAppearanceFragment(template){
    const frag=template.content.cloneNode(true);
    for(const el of [...frag.querySelectorAll('*')]){
      const tag=el.tagName.toLowerCase();
      if(!APPEARANCE_TAGS.has(tag)){el.remove();continue}
      for(const attr of [...el.attributes]){
        const name=attr.name.toLowerCase();
        const allowed=name==='class'||name==='style'||name==='title'||name==='role'||name==='href'||name==='target'||name==='rel'||name.startsWith('aria-');
        if(!allowed||name.startsWith('on')){el.removeAttribute(attr.name);continue}
        if(name==='style')el.setAttribute('style',sanitizeAppearanceCSS(attr.value));
        if(name==='href'){
          const href=safeAppearanceHref(attr.value);if(href)el.setAttribute('href',href);else el.removeAttribute('href');
        }
        if(name==='target'&&!['_blank','_self'].includes(attr.value))el.removeAttribute('target');
      }
      if(tag==='a'&&el.getAttribute('target')==='_blank')el.setAttribute('rel','noopener noreferrer');
    }
    const box=document.createElement('div');box.appendChild(frag);return box.innerHTML;
  }
  const defaultBrand='<strong>JACoB</strong><span>Journal Aligned Control Bridge</span>';
  const defaultFooter='<span>JACoB Alpha 0.2.10 · SDK 9</span><a href="/docs/index.html" target="_blank" rel="noopener">Documentation</a>';
  function applyAppearance(html=''){
    state.themeHTML=html||'';
    $('#jacob-user-theme').textContent='';
    $('#brand-slot').innerHTML=defaultBrand;
    $('#header-extra').innerHTML='';
    $('#nav-extra').innerHTML='';
    $('#footer-slot').innerHTML=defaultFooter;
    if($('#home-extra'))$('#home-extra').innerHTML='';
    document.body.className='';
    if(!html.trim()){localizeDOM(document.body);return;}
    const doc=new DOMParser().parseFromString(html,'text/html');
    $('#jacob-user-theme').textContent=sanitizeAppearanceCSS([...doc.querySelectorAll('style')].map(x=>x.textContent||'').join('\n'));
    const slots={brand:'#brand-slot','header-extra':'#header-extra','nav-extra':'#nav-extra','home-extra':'#home-extra',footer:'#footer-slot'};
    for(const [name,target] of Object.entries(slots)){
      const t=doc.querySelector(`template[data-jacob-slot="${name}"]`);
      if(t)$(target).innerHTML=sanitizeAppearanceFragment(t);
    }
    const bodyClass=(doc.querySelector('meta[name="jacob-body-class"]')?.getAttribute('content')||'').replace(/[^A-Za-z0-9_\- ]/g,' ').slice(0,200);
    document.body.className=bodyClass;
    const title=doc.querySelector('title')?.textContent?.trim();
    document.title=title||'JACoB';localizeDOM(document.body);
  }
  async function loadAppearance(){
    const r=await request('appearance.get');
    const html=r?.html||'';
    applyAppearance(html);
    $('#theme-html').value=html;
    $('#theme-result').textContent=html.trim()?tr('Custom appearance loaded.'):tr('Default appearance is active.');
    return r;
  }
  async function saveAppearance(){
    const html=$('#theme-html').value;
    try{await request('appearance.save',{html});applyAppearance(html);$('#theme-result').textContent=tr('Appearance saved.')}
    catch(e){$('#theme-result').textContent=pretty(e)}
  }
  async function resetAppearance(){
    try{await request('appearance.reset');$('#theme-html').value='';applyAppearance('');$('#theme-result').textContent=tr('Default appearance restored.')}
    catch(e){$('#theme-result').textContent=pretty(e)}
  }

  function token(){return localStorage.getItem('jacob-pair-token')||''}
  async function copyPairToken(){
    const value=state.core?.lan?.pairToken||'';if(!value)return;
    let ok=false;try{if(navigator.clipboard?.writeText){await navigator.clipboard.writeText(value);ok=true}}catch{}
    if(!ok){const ta=document.createElement('textarea');ta.value=value;ta.readOnly=true;ta.style.cssText='position:fixed;opacity:0;pointer-events:none';document.body.appendChild(ta);ta.select();try{ok=document.execCommand('copy')}catch{}ta.remove()}
    const b=$('#copy-pair-token');if(b){const t=b.textContent;b.textContent=ok?'Copied':'Copy failed';setTimeout(()=>b.textContent=t,1200)}
  }
  function ensurePairUI(){
    const tokenEl=$('#pair-token');if(!tokenEl||$('#copy-pair-token'))return;
    const style=document.createElement('style');style.textContent='.jacob-pair-row{display:flex;gap:7px;align-items:center;flex-wrap:wrap}.jacob-pair-copy{padding:4px 7px;font-size:11px}.jacob-pair-qr{width:148px;margin:10px 0 0 auto;padding:9px;border:1px solid #34383b;background:#090b0c;text-align:center}.jacob-pair-qr[hidden]{display:none}.jacob-pair-qr-code{width:128px;height:128px;margin:auto;background:#fff}.jacob-pair-qr-code svg{display:block;width:100%;height:100%}.jacob-pair-qr-label{margin-top:6px;color:#98918a;font:10px ui-monospace,Consolas,monospace;text-transform:uppercase;letter-spacing:.05em}@media(max-width:760px){.jacob-pair-qr{margin-left:0}}';document.head.appendChild(style);
    const dd=tokenEl.parentElement,wrap=document.createElement('span');wrap.className='jacob-pair-row';dd.insertBefore(wrap,tokenEl);wrap.appendChild(tokenEl);const btn=document.createElement('button');btn.id='copy-pair-token';btn.type='button';btn.className='secondary jacob-pair-copy';btn.textContent='Copy';btn.disabled=true;btn.onclick=copyPairToken;wrap.appendChild(btn);
    const card=tokenEl.closest('article.card')||tokenEl.closest('.card');if(card){const panel=document.createElement('div');panel.id='pair-qr-wrap';panel.className='jacob-pair-qr';panel.hidden=true;panel.innerHTML='<div id="pair-qr" class="jacob-pair-qr-code" aria-live="polite"></div><div class="jacob-pair-qr-label">Scan pairing key</div>';const net=$('#network-result');card.insertBefore(panel,net||null)}
  }
  function renderPairQR(value=''){
    ensurePairUI();value=String(value||'').trim();const w=$('#pair-qr-wrap'),b=$('#pair-qr'),c=$('#copy-pair-token');if(c)c.disabled=!value;if(!w||!b)return;if(!value){w.hidden=true;b.innerHTML='';return}w.hidden=false;if(globalThis.PairQR?.render)globalThis.PairQR.render(b,value);else{b.textContent='QR unavailable'}
  }
  function socketURL(){const proto=location.protocol==='https:'?'wss':'ws';const q=!isLocal&&token()?`?token=${encodeURIComponent(token())}`:'';return `${proto}://${location.host}/ws${q}`}
  function mediaURL(path,options={}){const q=new URLSearchParams();for(const [k,v] of Object.entries(options))if(v!==undefined&&v!==null)q.set(k,String(v));const auth=state.core?.mediaToken||'';if(auth)q.set('token',auth);q.set('_',Date.now());return `${path}?${q.toString()}`}
  function videoURL(options={}){return mediaURL('/api/video.mjpeg',{width:options.width||960,fps:options.fps||5,quality:options.quality||60})}
  function pretty(v){return JSON.stringify(v,null,2)}
  function escapeHTML(s){return String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}

  const TAB_CAPABILITIES={
    control:{label:'Control Elite Dangerous',detail:'send keyboard actions or semantic bindings to the Elite Dangerous window'},
    network:{label:'Use JACoB network access',detail:'send requests to public internet services through JACoB. Because tabs can read Elite journal/state data, grant this only to tabs you trust not to transmit that data'},
    recorder:{label:'Record Elite keyboard input',detail:'observe keyboard input only while Elite Dangerous is the foreground window'},
    overlay:{label:'Draw an Elite overlay',detail:'place graphics in JACoB’s game overlay'},
    video:{label:'View the Elite video feed',detail:'display the focus-gated Elite Dangerous capture stream'},
    vision:{label:'Use derived game vision',detail:'receive feature points and motion measurements computed from Elite-only frames; raw pixels are never exposed'},
    intertab:{label:'Control other JACoB tabs',detail:'invoke actions that other saved tabs explicitly publish through the JACoB action broker'}
  };
  function tabPermissionKey(frame){const id=frame?.dataset?.savedTabId||frame?.dataset?.overlayLayer||'preview';const version=frame?.dataset?.savedTabId?(savedTabMeta(id)?.updatedAt||'unknown'):'session';return `jacob-tab-permissions:${id}:${version}`}
  const previewPermissions=new WeakMap();
  function tabPermissions(frame){
    if(!frame?.dataset?.savedTabId)return previewPermissions.get(frame)||{};
    try{return JSON.parse(localStorage.getItem(tabPermissionKey(frame))||'{}')||{}}catch{return{}}
  }
  function saveTabPermissions(frame,value){
    if(!frame?.dataset?.savedTabId){previewPermissions.set(frame,value||{});return}
    localStorage.setItem(tabPermissionKey(frame),JSON.stringify(value||{}));
  }
  function resetTabPermissions(id){const prefix=`jacob-tab-permissions:${id}:`;for(let i=localStorage.length-1;i>=0;i--){const k=localStorage.key(i);if(k?.startsWith(prefix))localStorage.removeItem(k)}localStorage.removeItem(`jacob-tab-permissions:${id}`)}
  function capabilityForMethod(method){
    if(method==='net.fetch')return'network';
    if(method==='video.url')return'video';
    if(method.startsWith('vision.'))return'vision';
    if(method==='recorder.start'||method==='recorder.stop')return'recorder';
    if(method==='overlay.set'||method==='overlay.clear')return'overlay';
    if(method==='actions.invoke')return'intertab';
    if(method.startsWith('input.')||method.startsWith('binding.'))return'control';
    return'';
  }
  function requireTabCapability(frame,capability){
    if(!capability)return true;
    const grants=tabPermissions(frame);if(grants[capability]===true)return true;if(grants[capability]===false)return false;
    const meta=frame?.dataset?.savedTabId?savedTabMeta(frame.dataset.savedTabId):null;
    const info=TAB_CAPABILITIES[capability]||{label:capability,detail:capability};
    const name=meta?.name||'Tab preview';
    const ok=confirm(`${name} requests permission to ${info.label.toLowerCase()}.

This allows the tab to ${info.detail}.

Allow this capability on this browser?`);
    grants[capability]=ok;saveTabPermissions(frame,grants);return ok;
  }
  const tabRequestRates=new WeakMap();
  function allowTabRequest(frame,method){
    const now=performance.now();let r=tabRequestRates.get(frame);
    if(!r||now-r.windowStart>=1000)r={windowStart:now,count:0,last:new Map()};
    if(r.count>=120){tabRequestRates.set(frame,r);return false}
    let gap=0;
    if(method==='vision.sample'||method==='journal.read')gap=75;
    else if(method==='net.fetch')gap=100;
    else if(method==='tabstate.set'||method==='tabstate.delete'||method==='tabstate.clear')gap=50;
    const last=r.last.get(method)||-Infinity;if(now-last<gap){tabRequestRates.set(frame,r);return false}
    r.count++;r.last.set(method,now);tabRequestRates.set(frame,r);return true;
  }
  function tabCSP(){
    const origin=location.origin.replace(/;/g,'');
    return `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob: ${origin}; media-src ${origin}; connect-src 'none'; font-src data:; object-src 'none'; frame-src 'none'; worker-src 'none'; base-uri 'none'; form-action 'none'">`;
  }
  function composeTabHTML(html){
    const inject=tabCSP()+SDK,lower=html.toLowerCase(),head=lower.indexOf('<head>');
    if(head>=0)return html.slice(0,head+6)+inject+html.slice(head+6);
    const root=lower.indexOf('<html');if(root>=0){const end=html.indexOf('>',root);if(end>=0)return html.slice(0,end+1)+`<head>${inject}</head>`+html.slice(end+1)}
    return `<!doctype html><head>${inject}</head>`+html;
  }

  function connect(){
    clearTimeout(state.reconnectTimer);
    if(state.socket)try{state.socket.close()}catch{}
    const ws=new WebSocket(socketURL());state.socket=ws;
    ws.onopen=()=>{setConnection(true);bootstrap()};
    ws.onclose=()=>{setConnection(false);if(state.quitting){showClosed();return}if(state.updating){$('#connection').textContent=tr('UPDATING');state.reconnectTimer=setTimeout(connect,3000);return}state.reconnectTimer=setTimeout(connect,1800)};
    ws.onerror=()=>{};
    ws.onmessage=ev=>{let m;try{m=JSON.parse(ev.data)}catch{return}if(m.type==='response'){const p=state.pending.get(m.id);if(p){state.pending.delete(m.id);m.ok?p.resolve(m.result):p.reject(m.error)}return}if(m.type==='event')handleEvent(m.event,m.data)};
  }
  function request(method,params={}){
    if(!state.socket||state.socket.readyState!==WebSocket.OPEN)return Promise.reject({code:'OFFLINE',message:'core is offline'});
    const id=`ui-${++state.seq}-${Date.now()}`;state.socket.send(JSON.stringify({type:'request',id,method,params}));
    const timeoutMs=(method==='tabs.save'||method==='tabs.get')?90000:30000;
    return new Promise((resolve,reject)=>{state.pending.set(id,{resolve,reject});setTimeout(()=>{if(state.pending.has(id)){state.pending.delete(id);reject({code:'TIMEOUT',message:'request timed out'})}},timeoutMs)});
  }
  async function bootstrap(){try{await loadLocale()}catch{}try{await Promise.all([refreshHealth(),loadBindings(),loadSavedTabs(),loadAppearance()])}catch{}}

  function handleEvent(kind,data){
    addStream(kind,data);
    if(kind==='core.hello'){
      state.core=data;state.updating=false;if(data.locale)applyLocaleInfo(data.locale);if($('#quit-jacob'))$('#quit-jacob').hidden=!data.localClient;if($('#install-update'))$('#install-update').hidden=!data.localClient;$('#core-os').textContent=`${data.os}/${data.arch}`;$('#core-api').textContent=`v${data.apiVersion} (${data.product||'JACoB'} ${data.prototype})`;if($('#update-current'))$('#update-current').textContent=data.version||data.prototype||'—';queueUpdateCheckSoon();
      $('#binding-autofill').textContent=data.autoBind?`${data.autoBind.assigned||0} added / ${data.autoBind.scannedActions||0} scanned`:'—';
      $('#input-driver').textContent=`${data.input?.driver||'—'} / ${data.input?.enabled?'enabled':'disabled'}`;$('#capture-driver').textContent=`${data.capture?.driver||'—'} / ${data.capture?.available?'available':'unavailable'}`;if($('#overlay-driver'))$('#overlay-driver').textContent=`${data.overlay?.driver||'—'} / ${data.overlay?.available?'available':'unavailable'}`;
      $('#journal-dir').textContent=data.journalDir||tr('not detected');$('#bindings-file').textContent=data.bindingsFile||tr('not detected');if($('#home-journal-state'))$('#home-journal-state').textContent=data.journalDir?'Connected':tr('not detected');renderLAN(data.lan);if(data.health)renderHealth(data.health);
    }
    if(kind==='state'){state.snapshot=data;renderSnapshot(data)}
    if(kind==='status'){ $('#status-json').textContent=pretty(data);if(state.snapshot)state.snapshot.status=data }
    if(kind==='journal'){ $('#event-name').textContent=data.event||'journal';if($('#home-event-name'))$('#home-event-name').textContent=data.event||'journal';$('#journal-json').textContent=pretty(data);if(state.snapshot)state.snapshot.lastJournalEvent=data }
    if(kind==='tabs.changed')loadSavedTabs().catch(()=>{});if(kind==='core.update'){state.updating=true;if($('#update-result'))$('#update-result').textContent=`Installing ${data?.version||'update'}…`;}
    if(kind==='appearance.changed')loadAppearance().catch(()=>{});
    if(kind==='locale.changed')applyLocaleInfo(data||{});
    if(kind==='core.shutdown'){state.quitting=true;showClosed()}
    relayToAllTabs({channel:'jacob-host',kind:'event',event:kind,data});
  }

  function renderHealth(h={}){
    const el=$('#health-state'),status=(h.status||'unknown').toUpperCase();
    el.textContent=status==='OK'?tr('READY'):tr(status);
    el.className=`status-mark ${h.status==='ok'?'online':h.status==='warning'?'warning':'offline'}`;
    $('#health-detail').textContent=pretty(h);
    if($('#home-health-message')){
      const blockers=h.blockers||[],warnings=h.warnings||[];
      $('#home-health-message').textContent=blockers[0]||warnings[0]||(h.status==='ok'?tr('JACoB is ready.'):tr('Waiting for host checks…'));
    }
  }
  async function refreshHealth(){try{const h=await request('system.health');renderHealth(h);return h}catch(e){renderHealth({status:'blocked',blockers:[e.message||e.code||'health request failed']});throw e}}
  function renderSnapshot(s){
    $('#journal-json').textContent=pretty(s.lastJournalEvent||{});$('#status-json').textContent=pretty(s.status||{});$('#event-name').textContent=s.lastJournalEvent?.event||tr('Waiting…');
    if($('#home-event-name'))$('#home-event-name').textContent=s.lastJournalEvent?.event||tr('Waiting…');
    if($('#home-journal-state'))$('#home-journal-state').textContent=s.journalFile||tr('Waiting for journal');
  }
  function renderLAN(lan={}){$('#lan-enabled').textContent=lan.enabled?tr('enabled'):tr('disabled');$('#lan-addresses').textContent=(lan.addresses||[]).map(a=>`http://${a}:${lan.port||4510}/`).join('\n')||'—';const p=lan.pairToken||'';$('#pair-token').textContent=p||(!isLocal?tr('hidden on remote clients'):'—');$('#remote-token').value=token();renderPairQR(p)}
  function setConnection(on){const el=$('#connection');el.textContent=on?tr('ONLINE'):tr('OFFLINE');el.className=`status-mark ${on?'online':'offline'}`}
  function addStream(kind,data){const row=document.createElement('div');row.className='stream-line';const summary=kind==='journal'?(data?.event||''):kind==='recorder.input'?`${data?.type||''} ${data?.key||''}`:'';row.innerHTML=`<span class="time">${new Date().toLocaleTimeString()}</span><span class="kind">${escapeHTML(kind)}</span>${escapeHTML(summary)}`;const box=$('#stream');box.prepend(row);while(box.children.length>60)box.lastChild.remove()}

  async function loadBindings(){const r=await request('bindings.list');state.bindings=r.actions||[];$('#binding-dir').textContent=r.directory||tr('not detected');$('#binding-active').textContent=r.activeFile||tr('not detected');$('#binding-source').textContent=r.activeSource||'—';$('#binding-count').textContent=String(state.bindings.length);$('#binding-health').textContent=pretty(r.diagnostics||{});renderBindingOptions()}
  function renderBindingOptions(){const filter=$('#binding-filter').value.trim().toLowerCase();const select=$('#binding-action');const before=select.value;select.innerHTML='';for(const a of state.bindings){if(filter&&!a.name.toLowerCase().includes(filter))continue;const o=document.createElement('option');o.value=a.name;o.textContent=a.name;select.appendChild(o)}if([...select.options].some(o=>o.value===before))select.value=before;renderBindingDetail()}
  function renderBindingDetail(){const a=state.bindings.find(x=>x.name===$('#binding-action').value);$('#binding-detail').textContent=a?pretty(a):tr('No matching action.')}

  function allSDKFrames(){return [...document.querySelectorAll('iframe.jacob-sdk-frame')]}
  function sendToFrame(frame,msg){if(frame?.contentWindow)frame.contentWindow.postMessage(msg,'*')}
  function sanitizedCoreForTab(core){
    if(!core||typeof core!=='object')return core;
    const copy=typeof structuredClone==='function'?structuredClone(core):JSON.parse(JSON.stringify(core));
    delete copy.mediaToken;delete copy.journalDir;delete copy.bindingsDir;delete copy.bindingsFile;delete copy.bindingsFiles;delete copy.host;copy.localClient=false;
    if(copy.lan&&typeof copy.lan==='object'){copy.lan={...copy.lan};delete copy.lan.pairToken}
    if(copy.health&&typeof copy.health==='object'){copy.health={status:copy.health.status,blockers:Array.isArray(copy.health.blockers)?copy.health.blockers:[],warnings:Array.isArray(copy.health.warnings)?copy.health.warnings:[],gameRunning:!!copy.health.gameRunning}}
    return copy;
  }
  function hasTabCapability(frame,capability){return tabPermissions(frame)?.[capability]===true}
  function relayToAllTabs(msg){
    for(const frame of allSDKFrames()){
      if(msg?.kind==='event'&&msg.event==='recorder.input'&&!hasTabCapability(frame,'recorder'))continue;
      if(msg?.kind==='event'&&msg.event==='core.hello'){sendToFrame(frame,{...msg,data:sanitizedCoreForTab(msg.data)});continue}
      sendToFrame(frame,msg);
    }
  }
  function seedFrame(frame){if(state.core)sendToFrame(frame,{channel:'jacob-host',kind:'event',event:'core.hello',data:sanitizedCoreForTab(state.core)});if(state.locale)sendToFrame(frame,{channel:'jacob-host',kind:'event',event:'locale.changed',data:state.locale});if(state.snapshot)sendToFrame(frame,{channel:'jacob-host',kind:'event',event:'state',data:state.snapshot})}

  const frameReadyWaiters=new WeakMap();
  function publicActionRecord(record){return{name:record.name,label:record.label||record.name,description:record.description||'',tabId:record.tabId,tabName:record.tabName||record.tabId}}
  function actionError(code,message){return{code,message}}
  function clearActionsForFrame(frame,reason='target tab reloaded'){
    for(const [name,record] of [...state.actionRegistry])if(record.frame===frame)state.actionRegistry.delete(name);
    for(const [id,pending] of [...state.actionInvocations])if(pending.targetFrame===frame){clearTimeout(pending.timer);state.actionInvocations.delete(id);pending.reject(actionError('ACTION_TARGET_RELOADED',reason))}
  }
  function markFrameReady(frame){frame.dataset.sdkReady='1';const waiters=frameReadyWaiters.get(frame)||[];frameReadyWaiters.delete(frame);for(const done of waiters)done(true)}
  function waitForFrameReady(frame,timeoutMs=5000){if(frame?.dataset?.sdkReady==='1')return Promise.resolve(true);return new Promise(resolve=>{const list=frameReadyWaiters.get(frame)||[];let settled=false;const done=value=>{if(settled)return;settled=true;clearTimeout(timer);resolve(value)};list.push(done);frameReadyWaiters.set(frame,list);const timer=setTimeout(()=>done(false),timeoutMs)})}
  function registerTabAction(frame,params={}){
    const tabId=frame?.dataset?.savedTabId||'';if(!tabId)throw actionError('ACTION_PREVIEW','preview tabs cannot publish cross-tab actions');
    const name=String(params.name||'').trim();if(!/^[A-Za-z0-9][A-Za-z0-9._:-]{0,95}$/.test(name))throw actionError('BAD_PARAMS','action name must be 1-96 letters, numbers, dots, colons, underscores, or hyphens');
    const existing=state.actionRegistry.get(name);if(existing&&existing.frame!==frame)throw actionError('ACTION_CONFLICT',`action ${name} is already published by ${existing.tabName||existing.tabId}`);
    const meta=savedTabMeta(tabId);const record={name,label:String(params.label||name).slice(0,120),description:String(params.description||'').slice(0,300),tabId,tabName:meta?.name||tabId,frame};state.actionRegistry.set(name,record);return publicActionRecord(record)
  }
  function unregisterTabAction(frame,name){name=String(name||'').trim();const existing=state.actionRegistry.get(name);if(!existing)throw actionError('ACTION_NOT_FOUND','registered action was not found');if(existing.frame!==frame)throw actionError('ACTION_NOT_OWNER','a tab may unregister only its own action');state.actionRegistry.delete(name);return{unregistered:true,name}}
  async function discoverTabActions(){
    if(state.actionDiscoveryDone)return;
    if(state.actionDiscoveryPromise)return state.actionDiscoveryPromise;
    state.actionDiscoveryPromise=(async()=>{const loads=state.savedTabs.map(async tab=>{try{await ensureSavedTabLoaded(tab.id);const frame=savedTabFrame(tab.id);if(frame)await waitForFrameReady(frame,5000)}catch{}});await Promise.all(loads);await new Promise(resolve=>setTimeout(resolve,100));state.actionDiscoveryDone=true})().finally(()=>{state.actionDiscoveryPromise=null});
    return state.actionDiscoveryPromise
  }
  function listRegisteredActions(){return[...state.actionRegistry.values()].map(publicActionRecord).sort((a,b)=>a.name.localeCompare(b.name))}
  function callerInfo(frame){const id=frame?.dataset?.savedTabId||'';const meta=id?savedTabMeta(id):null;return{id,tabId:id,tabName:meta?.name||(id?'Saved tab':'Tab preview')}}
  async function invokeRegisteredAction(callerFrame,params={}){
    const name=String(params.name||'').trim();if(!name)throw actionError('BAD_PARAMS','action name is required');
    let record=state.actionRegistry.get(name);if(!record&&params.discover!==false){await discoverTabActions();record=state.actionRegistry.get(name)}
    if(!record)throw actionError('ACTION_NOT_FOUND',`no loaded saved tab publishes ${name}`);
    const timeoutMs=Math.max(500,Math.min(60000,Number(params.timeoutMs)||15000));const id=`action-${++state.actionSeq}-${Date.now()}`;
    return new Promise((resolve,reject)=>{const timer=setTimeout(()=>{state.actionInvocations.delete(id);reject(actionError('ACTION_TIMEOUT',`${name} did not finish within ${timeoutMs} ms`))},timeoutMs);state.actionInvocations.set(id,{resolve,reject,timer,targetFrame:record.frame});sendToFrame(record.frame,{channel:'jacob-host',kind:'action-invoke',id,action:name,payload:params.payload,caller:callerInfo(callerFrame)})})
  }
  function resolveSDKTabTarget(target){const raw=String(target||'').trim();if(!raw)return'';if(navigationOrder().includes(raw))return raw;const byId=state.savedTabs.find(tab=>tab.id===raw);if(byId)return navIDForTab(byId);const lower=raw.toLowerCase(),byName=state.savedTabs.find(tab=>String(tab.name||'').toLowerCase()===lower);if(byName)return navIDForTab(byName);for(const id of Object.keys(defaultNav))if(id.toLowerCase()===lower||navLabel(id).toLowerCase()===lower)return id;return''}
  function sdkTabList(){const hidden=hiddenNavigation();return navigationOrder().map(id=>{const custom=id.startsWith('custom-'),tab=custom?tabForNavID(id):null;return{id:custom?tab?.id||id.slice(7):id,navId:id,name:navLabel(id),type:custom?'custom':'default',hidden:hidden.has(id),active:state.activeTab===id,loaded:custom?savedTabFrame(tab?.id||'')?.dataset?.sdkReady==='1':true}})}
  async function activateSDKTab(target){const id=resolveSDKTabTarget(target);if(!id)throw actionError('TAB_NOT_FOUND','requested JACoB tab was not found');if(id.startsWith('custom-'))await ensureSavedTabLoaded(id.slice(7));switchTab(id);return{id,name:navLabel(id),hidden:hiddenNavigation().has(id)}}

  function switchTab(tabName){
    state.activeTab=tabName;
    if(tabName==='settings')dismissSettingsUpdateBadge();
    document.querySelectorAll('.nav').forEach(x=>x.classList.toggle('active',x.dataset.tab===tabName));
    document.querySelectorAll('.panel-page').forEach(x=>x.classList.toggle('active',x.id===tabName));
    if(tabName.startsWith('custom-'))ensureSavedTabLoaded(tabName.slice(7)).catch(e=>showSavedTabLoadError(tabName.slice(7),e));
  }
  function wireNavButton(btn){btn.onclick=()=>switchTab(btn.dataset.tab)}

  const defaultNav={dashboard:'Home',tabmanager:'Tab Manager',tutorial:'Tutorial',settings:'Settings'};
  function navIDForTab(tab){return `custom-${tab.id}`}
  function tabForNavID(id){return state.savedTabs.find(t=>navIDForTab(t)===id)}
  function navLabel(id){return defaultNav[id]?tr(defaultNav[id]):tabForNavID(id)?.name||id}
  function navigationOrder(){
    const expected=['dashboard',...state.savedTabs.map(navIDForTab),'tabmanager','tutorial','settings'];
    const valid=new Set(expected),out=[],seen=new Set();
    for(const id of state.navLayout?.order||[])if(valid.has(id)&&!seen.has(id)){out.push(id);seen.add(id)}
    for(const id of expected)if(!seen.has(id)){out.push(id);seen.add(id)}
    return out;
  }
  function hiddenNavigation(){return new Set(state.navLayout?.hiddenDefaults||[])}
  function renderNavigation(){
    const nav=$('#nav-items');if(!nav)return;nav.innerHTML='';const hidden=hiddenNavigation();
    for(const id of navigationOrder()){
      if(hidden.has(id)&&id!=='tabmanager')continue;
      const btn=document.createElement('button');btn.className='nav';btn.dataset.tab=id;btn.textContent=navLabel(id);btn.title=navLabel(id);if(id==='settings')decorateSettingsUpdateBadge(btn);wireNavButton(btn);nav.appendChild(btn);
    }
    document.querySelectorAll('.nav').forEach(x=>x.classList.toggle('active',x.dataset.tab===state.activeTab));
  }
  async function persistNavigation(order,hidden){
    try{state.navLayout=await request('tabs.layout.save',{order,hiddenDefaults:hidden});renderNavigation();renderNavigationManager();if(hidden.includes(state.activeTab)&&state.activeTab!=='tabmanager')switchTab('tabmanager');$('#navigation-result').textContent=tr('Navigation manifest saved.')}
    catch(e){$('#navigation-result').textContent=pretty(e)}
  }
  function moveNavigation(id,delta){const order=navigationOrder(),i=order.indexOf(id),j=i+delta;if(i<0||j<0||j>=order.length)return;[order[i],order[j]]=[order[j],order[i]];persistNavigation(order,[...hiddenNavigation()])}
  function toggleNavigationVisibility(id){if(id==='tabmanager'||!navigationOrder().includes(id))return;const hidden=hiddenNavigation();hidden.has(id)?hidden.delete(id):hidden.add(id);persistNavigation(navigationOrder(),[...hidden])}
  function renderNavigationManager(){
    const box=$('#navigation-list');if(!box)return;box.innerHTML='';const order=navigationOrder(),hidden=hiddenNavigation();
    order.forEach((id,index)=>{const row=document.createElement('div');row.className='saved-tab-row';const meta=document.createElement('div');meta.className='saved-tab-meta';const custom=id.startsWith('custom-'),isHidden=hidden.has(id);meta.innerHTML=`<strong${custom?' data-jacob-no-i18n="true"':''}>${escapeHTML(navLabel(id))}</strong><span class="muted">${id==='tabmanager'?tr('Default · required'):(custom?(isHidden?tr('Custom tab · hidden'):tr('Custom tab')):(isHidden?tr('Default · hidden'):tr('Default')))}</span>`;const actions=document.createElement('div');actions.className='row';const up=document.createElement('button');up.className='secondary compact-button';up.textContent='↑';up.title=tr('Move up');up.disabled=index===0;up.onclick=()=>moveNavigation(id,-1);const down=document.createElement('button');down.className='secondary compact-button';down.textContent='↓';down.title=tr('Move down');down.disabled=index===order.length-1;down.onclick=()=>moveNavigation(id,1);actions.append(up,down);const visibility=document.createElement('button');visibility.className='secondary';if(id==='tabmanager'){visibility.textContent=tr('Required');visibility.disabled=true}else{visibility.textContent=isHidden?tr('Show'):tr('Hide');visibility.onclick=()=>toggleNavigationVisibility(id)}actions.appendChild(visibility);row.append(meta,actions);box.appendChild(row)});localizeDOM(box);
  }

  async function loadSavedTabs(){
    const before=state.savedTabs.map(t=>`${t.id}:${t.updatedAt||''}`).join('|');const result=await request('tabs.list');state.savedTabs=result.tabs||[];state.navLayout=result.layout||{order:[],hiddenDefaults:[]};const after=state.savedTabs.map(t=>`${t.id}:${t.updatedAt||''}`).join('|');if(before!==after)state.actionDiscoveryDone=false;
    const live=new Set(state.savedTabs.map(t=>t.id));
    for(const id of [...state.tabHTMLCache.keys()])if(!live.has(id))state.tabHTMLCache.delete(id);
    renderSavedTabs();return state.savedTabs;
  }
  function savedTabMeta(id){return state.savedTabs.find(t=>t.id===id)}
  function savedTabFrame(id){return document.getElementById(`custom-${id}`)?.querySelector('iframe.jacob-sdk-frame')||null}
  function showSavedTabLoadError(id,error){const frame=savedTabFrame(id);if(!frame)return;const msg=escapeHTML(error?.message||error?.code||'Could not load saved tab');frame.srcdoc=`<!doctype html><body style="background:#080b0d;color:#eee;font:14px system-ui;padding:20px"><h2>Tab load failed</h2><pre style="white-space:pre-wrap">${msg}</pre></body>`;frame.dataset.updatedAt=''}
  function applySavedTabHTML(tab){const frame=savedTabFrame(tab.id);if(!frame)return;if(frame.dataset.updatedAt===String(tab.updatedAt||''))return;clearActionsForFrame(frame);frame.dataset.sdkReady='';frame.dataset.updatedAt=tab.updatedAt||'';frame.srcdoc=composeTabHTML(tab.html||'')}
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
    for(const existing of [...pages.querySelectorAll('.custom-user-page')])if(!wanted.has(existing.id)){const frame=existing.querySelector('iframe.jacob-sdk-frame');if(frame)clearActionsForFrame(frame,'target tab was removed');existing.remove()}
    for(const tab of state.savedTabs){
      const pageID=`custom-${tab.id}`;
      let section=document.getElementById(pageID);let iframe=section?.querySelector('iframe.jacob-sdk-frame');
      if(!section){section=document.createElement('section');section.id=pageID;section.className='panel-page custom-user-page';iframe=document.createElement('iframe');iframe.className='jacob-sdk-frame saved-tab-frame';iframe.sandbox='allow-scripts allow-downloads';iframe.dataset.savedTabId=tab.id;section.appendChild(iframe);pages.appendChild(section)}
      const cached=state.tabHTMLCache.get(tab.id);
      if(cached&&cached.updatedAt===tab.updatedAt)applySavedTabHTML({...tab,html:cached.html});else if(iframe.dataset.updatedAt&&iframe.dataset.updatedAt!==String(tab.updatedAt||'')){clearActionsForFrame(iframe);iframe.removeAttribute('srcdoc');iframe.dataset.sdkReady='';iframe.dataset.updatedAt=''}
      const size=Number(tab.sizeBytes||0);const sizeLabel=size?` · ${size>=1024*1024?(size/1024/1024).toFixed(2)+' MB':(size/1024).toFixed(1)+' KB'}`:'';
      const row=document.createElement('div');row.className='saved-tab-row';const meta=document.createElement('div');meta.className='saved-tab-meta';meta.innerHTML=`<strong>${escapeHTML(tab.name)}</strong><span class="muted mono">${escapeHTML(tab.id)}${sizeLabel}</span>`;const actions=document.createElement('div');actions.className='row';const open=document.createElement('button');open.textContent=tr('Open');open.onclick=()=>switchTab(pageID);const edit=document.createElement('button');edit.textContent=tr('Edit');edit.className='secondary';edit.onclick=()=>editSavedTab(tab.id);const perms=document.createElement('button');perms.textContent='Reset permissions';perms.className='secondary';perms.title='Forget permissions granted to this tab on this browser';perms.onclick=()=>{if(confirm(`Reset permissions for \"${tab.name}\" on this browser?`)){resetTabPermissions(tab.id);$('#tab-manager-result').textContent=`Permissions reset for ${tab.name}. The tab will ask again when it needs sensitive access.`}};const del=document.createElement('button');del.textContent=tr('Remove');del.className='danger';del.onclick=()=>deleteSavedTab(tab.id);actions.append(open,edit,perms,del);row.append(meta,actions);list.appendChild(row);
    }
    if(!state.savedTabs.length)list.innerHTML=`<div class="muted empty-state">${escapeHTML(tr('No saved tabs in the manifest.'))}</div>`;
    localizeDOM(list);renderNavigation();renderNavigationManager();
    if(hiddenNavigation().has(active)&&active!=='tabmanager')active=navigationOrder().find(id=>id==='tabmanager'||!hiddenNavigation().has(id))||'tabmanager';
    if(active.startsWith('custom-')&&!document.getElementById(active))switchTab('tabmanager');else switchTab(active);
  }

  function previewCurrent(){request('overlay.clear',{layer:'preview'}).catch(()=>{});const frame=$('#custom-frame');previewPermissions.delete(frame);frame.dataset.overlayLayer='preview';frame.srcdoc=composeTabHTML($('#custom-html').value)}
  function clearEditor(useExample=false){state.editingTabId='';$('#custom-tab-name').value=useExample?'Example Tab':'';$('#custom-html').value=useExample?exampleHTML:'';$('#editing-tab-label').textContent=tr('New tab');$('#tab-manager-result').textContent=tr('Editing a new tab.');previewCurrent()}
  async function editSavedTab(id){const meta=savedTabMeta(id);if(!meta)return;switchTab('tabmanager');$('#tab-manager-result').textContent=`Loading ${meta.name}…`;try{const tab=await ensureSavedTabLoaded(id);state.editingTabId=id;$('#custom-tab-name').value=tab.name;$('#custom-html').value=tab.html||'';$('#editing-tab-label').textContent=`Editing ${tab.name}`;$('#tab-manager-result').textContent=`Loaded ${tab.name} for editing.`;previewCurrent()}catch(e){$('#tab-manager-result').textContent=pretty(e)}}
  async function saveCurrentTab(){
    const name=$('#custom-tab-name').value.trim(),html=$('#custom-html').value;
    if(!name){$('#tab-manager-result').textContent=tr('Give the tab a name before saving.');$('#custom-tab-name').focus();return}
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
    if(quit){quit.disabled=true;quit.textContent=tr('Closed')}
    setConnection(false);
    $('#connection').textContent=tr('CLOSED');
  }
  async function quitJACoB(){
    if(!state.core?.localClient)return;
    if(!confirm(tr('Close JACoB?\n\nSaved tabs and settings will be kept.')))return;
    const quit=$('#quit-jacob');
    quit.disabled=true;quit.textContent=tr('Closing…');state.quitting=true;
    try{await request('core.shutdown')}catch(e){
      state.quitting=false;quit.disabled=false;quit.textContent=tr('Quit JACoB');
      alert(e?.message||tr('JACoB could not be closed.'));
    }
  }

  $('#quit-jacob').onclick=quitJACoB;
  $('#ping').onclick=async()=>{try{const r=await request('core.ping');$('#ping-result').textContent=`pong ${new Date(r.time).toLocaleTimeString()}`}catch(e){$('#ping-result').textContent=e.message||e.code||'failed'}};
  $('#refresh-health').onclick=refreshHealth;
  $('#autofill-bindings').onclick=async()=>{try{const r=await request('bindings.autofill');$('#binding-autofill').textContent=`${r.assigned||0} added / ${r.scannedActions||0} scanned`;$('#binding-detail').textContent=pretty(r);await refreshHealth();await loadBindings()}catch(e){$('#binding-detail').textContent=pretty(e);await refreshHealth().catch(()=>{})}};
  $('#reload-bindings').onclick=async()=>{try{await request('bindings.reload');await loadBindings()}catch(e){$('#binding-detail').textContent=pretty(e)}};
  $('#binding-filter').oninput=renderBindingOptions;$('#binding-action').onchange=renderBindingDetail;
  $('#press-binding').onclick=async()=>{try{$('#binding-detail').textContent=pretty(await request('binding.press',{action:$('#binding-action').value}))}catch(e){$('#binding-detail').textContent=pretty(e)}};
  $('#save-token').onclick=()=>{const t=$('#remote-token').value.trim();if(t)localStorage.setItem('jacob-pair-token',t);else localStorage.removeItem('jacob-pair-token');$('#network-result').textContent=tr('Pair token saved. Reconnecting…');stopVideo();connect()};

  async function checkForUpdates({quiet=false}={}){
    const out=$('#update-result'),btn=$('#check-update'),install=$('#install-update');if(!state.socket||state.socket.readyState!==WebSocket.OPEN)return;if(quiet&&!state.core?.localClient)return;if(!quiet){btn.disabled=true;install.disabled=true;out.textContent=tr('Querying the release channel…')}
    try{const info=await request('update.check');state.updateInfo=info;$('#update-latest').textContent=info.latestVersion||'—';install.disabled=!(info.available&&info.installable&&state.core?.localClient);if(info.available&&state.activeTab==='settings'&&info.latestVersion){try{localStorage.setItem(UPDATE_BADGE_KEY,String(info.latestVersion))}catch{}}renderNavigation();if(info.available){out.textContent=`${info.releaseName||('JACoB '+info.latestVersion)} is available.${info.installable?' Ready for installation.':' '+(info.installNote||'Manual installation required.')}`}else{out.textContent=tr('Current release confirmed. No newer published build was found.')}}
    catch(e){if(!quiet)out.textContent=e?.message||pretty(e)}finally{if(!quiet)btn.disabled=false}
  }
  async function installUpdate(){
    const info=state.updateInfo;if(!info?.available)return;if(!confirm(`Install JACoB ${info.latestVersion}?\n\nJACoB will restart after the update is staged.`))return;
    const btn=$('#install-update');btn.disabled=true;state.updating=true;$('#update-result').textContent=tr('Downloading update package…');
    try{const r=await request('update.install');$('#update-result').textContent=`Installing ${r.version||info.latestVersion}…`;$('#connection').textContent=tr('UPDATING')}catch(e){state.updating=false;btn.disabled=false;$('#update-result').textContent=e?.message||pretty(e)}
  }
  $('#check-update').onclick=()=>checkForUpdates({quiet:false});$('#install-update').onclick=installUpdate;

  function setVideoState(on,msg=''){const el=$('#video-state');el.textContent=on?tr('STREAMING'):tr('STOPPED');el.className=`status-mark ${on?'online':'offline'}`;if(msg)$('#video-result').textContent=msg}
  function startVideo(){if(state.core?.capture&&!state.core.capture.available){setVideoState(false,`Capture unavailable: ${state.core.capture.driver}`);return}const opts={width:Number($('#video-width').value),fps:Number($('#video-fps').value),quality:Number($('#video-quality').value)};$('#game-view-img').src=videoURL(opts);setVideoState(true,pretty({mode:'mjpeg',...opts,url:'authenticated local/LAN stream'}))}
  function stopVideo(){$('#game-view-img').removeAttribute('src');setVideoState(false)}
  $('#video-start').onclick=startVideo;$('#video-stop').onclick=stopVideo;$('#video-snapshot').onclick=()=>{const opts={width:Number($('#video-width').value),quality:Number($('#video-quality').value)};$('#game-view-img').src=mediaURL('/api/video/frame.jpg',opts);setVideoState(false,pretty({mode:'snapshot',...opts}))};

  $('#run-tab').onclick=previewCurrent;$('#save-tab').onclick=saveCurrentTab;$('#new-tab').onclick=()=>clearEditor(false);$('#reset-tab').onclick=()=>clearEditor(true);
  $('#upload-html').onchange=async e=>{const f=e.target.files?.[0];if(!f)return;$('#custom-html').value=await f.text();if(!$('#custom-tab-name').value.trim())$('#custom-tab-name').value=f.name.replace(/\.html?$/i,'');previewCurrent();e.target.value=''};
  $('#save-theme').onclick=saveAppearance;$('#reset-theme').onclick=resetAppearance;
  if($('#language-select'))$('#language-select').onchange=saveLocale;
  $('#upload-theme').onchange=async e=>{const f=e.target.files?.[0];if(!f)return;$('#theme-html').value=await f.text();await saveAppearance();e.target.value=''};
  if($('#home-add-tab'))$('#home-add-tab').onclick=()=>switchTab('tabmanager');
  if($('#home-tutorial'))$('#home-tutorial').onclick=()=>switchTab('tutorial');
  if($('#tutorial-tab-manager'))$('#tutorial-tab-manager').onclick=()=>switchTab('tabmanager');

  function overlayLayerForFrame(frame){if(frame?.dataset?.savedTabId)return `tab:${frame.dataset.savedTabId}`;return frame?.dataset?.overlayLayer||'preview'}

  addEventListener('message',async ev=>{
    const frame=allSDKFrames().find(f=>f.contentWindow===ev.source);if(!frame)return;
    const m=ev.data;if(!m||m.channel!=='jacob-tab')return;
    if(m.kind==='ready'){markFrameReady(frame);seedFrame(frame);return}
    if(m.kind==='action-result'){const pending=state.actionInvocations.get(m.id);if(!pending||pending.targetFrame!==frame)return;clearTimeout(pending.timer);state.actionInvocations.delete(m.id);m.ok?pending.resolve(m.result):pending.reject(m.error||actionError('ACTION_FAILED','action failed'));return}
    if(m.kind==='request'){
      if(!allowTabRequest(frame,m.method)){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error:{code:'RATE_LIMITED',message:'custom-tab request rate exceeded'}});return}
      if(m.method==='actions.register'){try{const result=registerTabAction(frame,m.params);sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:true,result})}catch(error){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error})}return}
      if(m.method==='actions.unregister'){try{const result=unregisterTabAction(frame,m.params?.name);sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:true,result})}catch(error){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error})}return}
      if(m.method==='actions.list'){try{if(m.params?.discover!==false)await discoverTabActions();sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:true,result:{actions:listRegisteredActions()}})}catch(error){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error})}return}
      if(m.method==='actions.invoke'){if(!requireTabCapability(frame,'intertab')){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error:{code:'PERMISSION_DENIED',message:'inter-tab control permission was not granted'}});return}try{const result=await invokeRegisteredAction(frame,m.params);sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:true,result})}catch(error){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error})}return}
      if(m.method==='tabs.sdk.list'){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:true,result:{tabs:sdkTabList()}});return}
      if(m.method==='tabs.activate'){try{const result=await activateSDKTab(m.params?.target);sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:true,result})}catch(error){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error})}return}
      if(m.method==='video.url'){
        if(!requireTabCapability(frame,'video')){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error:{code:'PERMISSION_DENIED',message:'video permission was not granted'}});return}
        sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:true,result:videoURL(m.params||{})});return
      }
      const allowed=new Set(['core.ping','system.health','state.get','bindings.list','bindings.diagnostics','bindings.get','bindings.reload','input.tap','input.hold','input.text','binding.press','binding.down','binding.up','binding.hold','recorder.status','recorder.start','recorder.stop','video.info','vision.info','vision.sample','overlay.info','overlay.set','overlay.clear','net.fetch','elitefiles.list','elitefiles.get','journal.files','journal.read','tabstate.get','tabstate.set','tabstate.delete','tabstate.clear','locale.get']);
      if(!allowed.has(m.method)){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error:{code:'SDK_DENIED',message:'method not exposed by JACoB SDK'}});return}
      const capability=capabilityForMethod(m.method);if(capability&&!requireTabCapability(frame,capability)){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error:{code:'PERMISSION_DENIED',message:`${capability} permission was not granted`}});return}
      try{let params=m.params||{};if(m.method==='overlay.set'||m.method==='overlay.clear')params={...params,layer:overlayLayerForFrame(frame)};if(m.method.startsWith('tabstate.')){const tabId=frame?.dataset?.savedTabId||'';if(!tabId)throw{code:'TAB_STATE_PREVIEW',message:'persistent state is available after the tab is saved'};params={...params,tabId}}const result=await request(m.method,params);sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:true,result})}catch(error){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error})}
    }
  });

  $('#custom-html').value=localStorage.getItem('jacob-p2-custom-html')||exampleHTML;
  $('#custom-tab-name').value='Custom Tab';
  previewCurrent();
  applyLocaleInfo({language:'en',supported:I18N.supported});

  // Render built-in navigation before the core connection succeeds so a
  // brand-new LAN client can reach Settings and enter its pairing token.
  renderNavigation();

  // New remote browsers have no token in localStorage yet. Open Settings
  // immediately instead of leaving them on an unauthenticated shell.
  if (!isLocal && !token()) {
    switchTab('settings');
    $('#network-result').textContent =
      tr('Enter the pair token shown on the computer running JACoB.');
    $('#remote-token').focus();
  }

  ensurePairUI();
  startPeriodicUpdateChecks();
  connect();
})();
