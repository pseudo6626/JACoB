/* JACoB local QR renderer.
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
    if (bytes.length < 1 || bytes.length > 32) throw new Error('QR content must be 1-32 UTF-8 bytes for QR v2-L.');
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
    return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${dim} ${dim}" role="img" aria-label="JACoB local QR code" shape-rendering="crispEdges"><rect width="100%" height="100%" fill="#fff"/><path d="${d}" fill="#000"/></svg>`;
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
    socket:null,pending:new Map(),core:null,snapshot:null,seq:0,reconnectTimer:null,deviceRevoked:false,macros:[],macroDraft:null,macroSelected:'',macroRecording:false,macroPolicy:null,
    bindings:[],savedTabs:[],tabHTMLCache:new Map(),tabLoadPromises:new Map(),navLayout:{order:[],hiddenDefaults:[]},editingTabId:'',activeTab:'dashboard',themeHTML:'',locale:{language:'en',supported:[]},quitting:false,updating:false,updateInfo:null,actionRegistry:new Map(),actionInvocations:new Map(),actionSeq:0,actionDiscoveryDone:false,actionDiscoveryPromise:null,paths:null
  };
  const isLocal = ['127.0.0.1','localhost','::1'].includes(location.hostname);
  try{localStorage.removeItem('jacob-pair-token')}catch{}
  function privilegedClient(){return !!(isLocal||state.core?.privilegedClient||state.core?.adminClient)}
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
function sdkID64(value){if(typeof value==='number'){if(!Number.isSafeInteger(value)||value<0)throw Object.assign(new Error('id64 Number is unsafe; pass the decimal ID64 as a string'),{code:'BAD_PARAMS'});return String(value)}const s=String(value??'').trim();if(!/^\d+$/.test(s))throw Object.assign(new Error('id64 must be an unsigned decimal integer string'),{code:'BAD_PARAMS'});return s}
function sdkNetShape(url,options={}){const out={url:String(url||''),method:options.method||'GET',headers:options.headers||{},body:typeof options.body==='string'?options.body:(options.body==null?'':JSON.stringify(options.body))};for(const k of ['group','minIntervalMs','retry','cacheTtlMs'])if(options[k]!=null)out[k]=options[k];return out}
function cloneSDKValue(value){if(value===undefined)return undefined;if(typeof structuredClone==='function')return structuredClone(value);return JSON.parse(JSON.stringify(value))}
function closeVideoElement(el){if(!el||typeof el.removeAttribute!=='function')return false;try{if(typeof el.pause==='function')el.pause()}catch{};try{el.removeAttribute('src');el.removeAttribute('srcset');if(el.dataset)delete el.dataset.jacobVideoActive;if(typeof el.load==='function')el.load();return true}catch{return false}}

const hud=Object.freeze({
 scene:(items=[],options={})=>Object.assign({theme:'cockpit',space:'normalized'},cloneSDKValue(options),{items:Array.isArray(items)?cloneSDKValue(items):[]}),
 text:(text,x,y,options={})=>Object.assign({},cloneSDKValue(options),{type:'text',x:Number(x),y:Number(y),text:String(text??'')}),
 metric:(x,y,value,options={})=>Object.assign({},cloneSDKValue(options),{type:'metric',x:Number(x),y:Number(y),value:String(value??'')}),
 panel:(x,y,w,h,options={})=>Object.assign({},cloneSDKValue(options),{type:'panel',x:Number(x),y:Number(y),w:Number(w),h:Number(h)}),
 progress:(x,y,w,h,progress,options={})=>Object.assign({},cloneSDKValue(options),{type:'progress',x:Number(x),y:Number(y),w:Number(w),h:Number(h),progress:Number(progress)}),
 reticle:(x,y,options={})=>Object.assign({},cloneSDKValue(options),{type:'reticle',x:Number(x),y:Number(y)}),
 bracket:(x,y,w,h,options={})=>Object.assign({},cloneSDKValue(options),{type:'bracket',x:Number(x),y:Number(y),w:Number(w),h:Number(h)}),
 chevron:(x,y,options={})=>Object.assign({},cloneSDKValue(options),{type:'chevron',x:Number(x),y:Number(y)}),
 arc:(x,y,r,startAngle,sweepAngle,options={})=>Object.assign({},cloneSDKValue(options),{type:'arc',x:Number(x),y:Number(y),r:Number(r),startAngle:Number(startAngle),sweepAngle:Number(sweepAngle)}),
 ring:(x,y,r,options={})=>Object.assign({},cloneSDKValue(options),{type:'ring',x:Number(x),y:Number(y),r:Number(r)})
});
window.Elite=Object.freeze({
 clipboard:Object.freeze({info:()=>request('clipboard.info'),writeText:text=>request('clipboard.writeText',{text})}),
 api:Object.freeze({version:15}), core:Object.freeze({ping:()=>request('core.ping')}), system:Object.freeze({health:()=>request('system.health'),performance:()=>request('system.performance'),capabilities:()=>request('system.capabilities')}), state:Object.freeze({get:()=>request('state.get'),subscribe:cb=>onEvent('status',cb)}),
 bindings:Object.freeze({list:()=>request('bindings.list'),diagnostics:()=>request('bindings.diagnostics'),get:name=>request('bindings.get',{name}),reload:()=>request('bindings.reload'),press:action=>request('binding.press',{action}),down:action=>request('binding.down',{action}),up:action=>request('binding.up',{action}),hold:(action,durationMs=1000)=>request('binding.hold',{action,durationMs}),releaseAll:()=>request('bindings.releaseAll')}),
 input:Object.freeze({tap:(key,options={})=>request('input.tap',{key,delayMs:options.delayMs||0,modifiers:options.modifiers||[]}),hold:(key,durationMs=250,options={})=>request('input.hold',{key,durationMs,modifiers:options.modifiers||[]}),text:(text,options={})=>request('input.text',{text,intervalMs:options.intervalMs??15}),releaseAll:()=>request('input.releaseAll')}),
 recorder:Object.freeze({status:()=>request('recorder.status'),start:()=>request('recorder.start'),stop:()=>request('recorder.stop'),subscribe:cb=>onEvent('recorder.input',cb)}),
 macros:Object.freeze({list:()=>request('macros.list'),get:name=>request('macros.get',{name}),run:(name,args={})=>request('macros.run',{name,args}),exitMenus:()=>request('macros.run',{name:'exit_menus'}),openGalaxyMap:()=>request('macros.run',{name:'open_galaxy_map'}),openSystemMap:()=>request('macros.run',{name:'open_system_map'}),openNavigationPanel:()=>request('macros.run',{name:'open_navigation_panel'}),openCommsPanel:()=>request('macros.run',{name:'open_comms_panel'}),openRolePanel:()=>request('macros.run',{name:'open_role_panel'}),openInternalPanel:()=>request('macros.run',{name:'open_internal_panel'}),openQuickComms:()=>request('macros.run',{name:'open_quick_comms'}),openCodexDiscovery:()=>request('macros.run',{name:'open_codex_discovery'}),plotRoute:system=>request('macros.run',{name:'plot_route',args:{system}})}),
 overlay:Object.freeze({info:()=>request('overlay.info'),set:scene=>request('overlay.set',{scene}),clear:()=>request('overlay.clear'),hud}),
 video:Object.freeze({info:()=>request('video.info'),url:(options={})=>request('video.url',options),attach:async(el,options={})=>{if(!el)throw Object.assign(new Error('video element is required'),{code:'BAD_PARAMS'});closeVideoElement(el);const u=await request('video.url',options);el.src=u;if(el.dataset)el.dataset.jacobVideoActive='1';return u},close:el=>closeVideoElement(el)}),
 vision:Object.freeze({
 info:()=>request('vision.info'),
 sample:(options={})=>request('vision.sample',options),
 inspect:(spec={})=>request('vision.inspect',spec),
 calibrate:(options={})=>request('vision.calibrate',options),
 configure:async(id,options={})=>{
  id=String(id||'').trim();if(!id)throw Object.assign(new Error('vision configuration id is required'),{code:'BAD_PARAMS'});
  const key='__vision.config.'+id,saved=await request('tabstate.get',{key});
  if(saved?.found&&!options.force)return saved.value;
  if(options.defaults&&!options.force&&options.calibrate!==true){await request('tabstate.set',{key,value:options.defaults});return options.defaults}
  const value=await request('vision.calibrate',{...options,id,defaults:options.defaults||null});
  await request('tabstate.set',{key,value});return value
 },
 reset:async id=>request('tabstate.delete',{key:'__vision.config.'+String(id||'').trim()}),
 debug:Object.freeze({show:(options={})=>request('vision.debug.show',options),clear:()=>request('vision.debug.clear')})
}),
 spatial:Object.freeze({
 info:()=>request('spatial.info'),
 begin:(options={})=>request('spatial.begin',options),
 update:(scene,options={})=>request('spatial.update',{scene,...options}),
 observe:(scene,observation={})=>request('spatial.observe',{scene,...observation}),
 pose:scene=>request('spatial.pose',{scene}),
 landmarks:scene=>request('spatial.landmarks',{scene}),
 project:(scene,spec={})=>request('spatial.project',{scene,...spec}),
 export:scene=>request('spatial.export',{scene}),
 import:(data,options={})=>request('spatial.import',{data,...options}),
 end:scene=>request('spatial.end',{scene})
 }),
 galaxy:Object.freeze({
  parseSystemName:name=>request('galaxy.parseSystemName',{name:String(name||'')}),
  decodeAddress:id64=>request('galaxy.decodeAddress',{id64:sdkID64(id64)}),
  encodeAddress:async spec=>{const r=await request('galaxy.encodeAddress',spec||{});return r?.id64||''},
  addressForSequence:async(id64,sequence)=>{const r=await request('galaxy.addressForSequence',{id64:sdkID64(id64),sequence});return r?.id64||''},
  boxel:id64=>request('galaxy.boxel',{id64:sdkID64(id64)}),
  boxelHierarchy:(id64,options={})=>request('galaxy.boxelHierarchy',{id64:sdkID64(id64),position:options.position||null})
 }),
 catalog:Object.freeze({system:Object.freeze({get:(spec={})=>request('catalog.system.get',{...spec,id64:sdkID64(spec.id64),provider:spec.provider||'spansh',detail:spec.detail||'summary'})})}),
 net:Object.freeze({
  fetch:(url,options={})=>request('net.fetch',sdkNetShape(url,options)),
  fetchQueued:(url,options={})=>request('net.fetchQueued',sdkNetShape(url,options)),
  fetchBatch:(requests,options={})=>request('net.fetchBatch',{requests:(requests||[]).map(r=>sdkNetShape(r?.url,r||{})),group:options.group||'',concurrency:options.concurrency||0,minIntervalMs:options.minIntervalMs??null,pauseEvery:options.pauseEvery||0,pauseMs:options.pauseMs||0,retry:options.retry??null,cacheTtlMs:options.cacheTtlMs??null})
 }),
 data:Object.freeze({list:()=>request('elitefiles.list'),get:name=>request('elitefiles.get',{name}),subscribe:(name,cb)=>{const wanted=String(name||'*');return onEvent('eliteFile',payload=>{if(wanted==='*'||String(payload?.name||'').toLowerCase()===wanted.toLowerCase()||String(payload?.file||'').toLowerCase()===wanted.toLowerCase())cb(payload)})}}),
 store:Object.freeze({
 get:async(key,fallback=null)=>{const r=await request('tabstate.get',{key});return r?.found?r.value:fallback},
 set:(key,value)=>request('tabstate.set',{key,value}),
 delete:key=>request('tabstate.delete',{key}),
 clear:()=>request('tabstate.clear'),
 update:async(key,updater,options={})=>{if(typeof updater!=='function')throw Object.assign(new Error('store.update requires an updater function'),{code:'BAD_PARAMS'});const retries=Math.max(1,Math.min(20,Number(options.retries)||8));for(let attempt=1;attempt<=retries;attempt++){const before=await request('tabstate.get',{key}),found=!!before?.found,current=found?cloneSDKValue(before.value):cloneSDKValue(options.defaultValue??null),next=await updater(cloneSDKValue(current),{found,attempt});if(next===undefined)throw Object.assign(new Error('store.update updater must return a JSON value; use store.delete() to remove a key'),{code:'BAD_PARAMS'});const result=await request('tabstate.cas',{key,expectedFound:found,expected:before?.value??null,value:next});if(result?.swapped)return result.value}throw Object.assign(new Error('store.update could not commit because the value kept changing'),{code:'TAB_STATE_CONFLICT'})},
 batch:operations=>request('tabstate.batch',{operations:Array.isArray(operations)?operations:[]})
}),
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
  const defaultFooter='<span>JACoB Alpha 0.2.19 · SDK 15</span><a href="/docs/index.html" target="_blank" rel="noopener">Documentation</a>';
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

  function token(){return ''}
  async function copyText(value,button){
    value=String(value||'');if(!value)return false;let ok=false;
    try{if(navigator.clipboard?.writeText){await navigator.clipboard.writeText(value);ok=true}}catch{}
    if(!ok){const ta=document.createElement('textarea');ta.value=value;ta.readOnly=true;ta.style.cssText='position:fixed;opacity:0;pointer-events:none';document.body.appendChild(ta);ta.select();try{ok=document.execCommand('copy')}catch{}ta.remove()}
    if(button){const t=button.textContent;button.textContent=ok?'Copied':'Copy failed';setTimeout(()=>button.textContent=t,1200)}
    return ok
  }
  async function copyPairToken(){return copyText(state.core?.lan?.pairToken||'',$('#copy-pair-token'))}
  async function copyLANURL(){return copyText(state.lanURL||'',$('#copy-lan-url'))}
  function ensurePairUI(){
    const pair=$('#copy-pair-token'),url=$('#copy-lan-url');
    if(pair&&!pair.dataset.bound){pair.dataset.bound='1';pair.onclick=copyPairToken}
    if(url&&!url.dataset.bound){url.dataset.bound='1';url.onclick=copyLANURL}
  }
  function renderPairQR(tokenValue='',urlValue=''){
    ensurePairUI();tokenValue=String(tokenValue||'').trim();urlValue=String(urlValue||'').trim();
    const wrap=$('#pair-qr-wrap'),tokenQR=$('#pair-qr'),urlQR=$('#url-qr'),pairCopy=$('#copy-pair-token'),urlCopy=$('#copy-lan-url');
    if(pairCopy)pairCopy.disabled=!tokenValue;if(urlCopy)urlCopy.disabled=!urlValue;if(!wrap)return;
    wrap.hidden=!(tokenValue||urlValue);
    if(tokenQR){tokenQR.innerHTML='';if(tokenValue){if(globalThis.PairQR?.render)globalThis.PairQR.render(tokenQR,tokenValue);else tokenQR.textContent='QR unavailable'}}
    if(urlQR){urlQR.innerHTML='';if(urlValue){if(globalThis.PairQR?.render)globalThis.PairQR.render(urlQR,urlValue);else urlQR.textContent='QR unavailable'}}
  }
  function deviceTime(value){if(!value)return'—';const d=new Date(value);return Number.isNaN(d.getTime())?'—':d.toLocaleString()}
  function renderConnectedDevices(result={}){
    const list=$('#device-list'),count=$('#device-count');if(!list)return;const devices=result.devices||[];if(count)count.textContent=String(devices.length);const canManage=isLocal;
    if(!devices.length){list.innerHTML='<div class="muted empty-state">No paired remote devices. A device appears here after it uses the current pair key.</div>';return}
    list.innerHTML='';
    for(const d of devices){
      const row=document.createElement('div');row.className='saved-tab-row';const meta=document.createElement('div');meta.className='saved-tab-meta';const short=String(d.id||'').slice(0,8);
      meta.innerHTML=`<strong>${escapeHTML(d.name||'Browser device')}</strong><span class="muted">${d.connected?'Connected':'Offline'} · ${escapeHTML(short)} · Last seen ${escapeHTML(deviceTime(d.lastSeenAt))}</span>`;
      const actions=document.createElement('div');actions.className='row';
      const rememberLabel=document.createElement('label');rememberLabel.className='compact';const remember=document.createElement('input');remember.type='checkbox';remember.checked=!!d.remember;remember.disabled=!canManage;remember.onchange=()=>setDeviceFlag(d,'remember',remember.checked,remember);rememberLabel.append(remember,document.createTextNode(' Remember me'));
      const adminLabel=document.createElement('label');adminLabel.className='compact';const admin=document.createElement('input');admin.type='checkbox';admin.checked=!!d.admin;admin.disabled=!canManage;admin.onchange=()=>setDeviceFlag(d,'admin',admin.checked,admin);adminLabel.append(admin,document.createTextNode(' Admin'));
      const revoke=document.createElement('button');revoke.className='danger secondary';revoke.textContent='Revoke';revoke.hidden=!canManage;revoke.onclick=()=>revokeDevice(d);
      actions.append(rememberLabel,adminLabel,revoke);row.append(meta,actions);list.appendChild(row)
    }
    if(!canManage){const out=$('#device-manager-result');if(out)out.textContent='Device trust is read-only here. Remember me, Admin, and Revoke can only be changed from the physical JACoB host browser.'}
  }
  async function loadConnectedDevices(){
    if(!privilegedClient()){renderConnectedDevices({devices:[]});return null}
    try{const r=await request('devices.list');state.devices=r.devices||[];renderConnectedDevices(r);return r}catch(e){const out=$('#device-manager-result');if(out)out.textContent=e?.message||pretty(e);return null}
  }
  async function setDeviceFlag(device,field,value,input){
    if(!isLocal){input.checked=!value;return}
    if(field==='admin'&&value){const ok=confirm(`Give ${device.name||'this device'} Admin access?\n\nAdmin devices can perform host-reserved JACoB actions, including installing/editing tabs, changing paths, running host diagnostics, installing updates, and closing JACoB. They cannot manage device trust or grant Admin to another device.`);if(!ok){input.checked=false;return}}
    input.disabled=true;const out=$('#device-manager-result');
    try{await request(field==='admin'?'devices.setAdmin':'devices.setRemember',{id:device.id,value});if(out)out.textContent=`${device.name||'Device'} updated.`;await loadConnectedDevices()}catch(e){input.checked=!value;if(out)out.textContent=e?.message||pretty(e)}finally{input.disabled=false}
  }
  async function revokeDevice(device){
    if(!isLocal)return;if(!confirm(`Revoke ${device.name||'this device'}?\n\nIts current JACoB connection will be closed and its saved credential will stop working.`))return;
    const out=$('#device-manager-result');try{await request('devices.revoke',{id:device.id});if(out)out.textContent=`${device.name||'Device'} revoked.`;await loadConnectedDevices()}catch(e){if(out)out.textContent=e?.message||pretty(e)}
  }
  async function refreshDeviceCookie(){
    if(isLocal)return;try{await fetch('/api/device/refresh',{method:'POST',credentials:'same-origin'})}catch{}
  }
  function macroStatus(text){const out=$('#macro-result');if(out)out.textContent=String(text??'')}
  function cloneMacro(value){return JSON.parse(JSON.stringify(value||{}))}
  function macroPolicyAccepted(){return !!state.macroPolicy?.accepted}
  function macroCanEdit(){return privilegedClient()&&macroPolicyAccepted()}
  function renderMacroPolicy(){
    const card=$('#macro-policy-card'),check=$('#macro-policy-check'),button=$('#macro-policy-accept'),label=$('#macro-policy-state');if(!card)return;
    const accepted=macroPolicyAccepted(),physicalHost=!!state.core?.localClient;card.classList.toggle('good',accepted);card.classList.toggle('warn',!accepted);
    if(check){check.disabled=accepted||!physicalHost;if(accepted)check.checked=true}
    if(button){button.disabled=accepted||!physicalHost||!check?.checked;button.textContent=accepted?'Agreement accepted':'I agree — unlock macro editor'}
    if(label)label.textContent=accepted?`Accepted ${state.macroPolicy?.acceptedAt?new Date(state.macroPolicy.acceptedAt).toLocaleString():''}`:physicalHost?'Required before creating, recording, deleting, resetting, or editing macros.':'Agreement must be accepted from the JACoB host computer.';
  }
  async function acceptMacroPolicy(){
    if(!state.core?.localClient||!$('#macro-policy-check')?.checked)return;try{state.macroPolicy=await request('macros.policy.accept');renderMacroPolicy();renderMacroEditor();macroStatus('Macro use agreement accepted. The editor is unlocked on privileged devices.')}catch(e){macroStatus(`${e?.code||'MACRO_POLICY'}: ${e?.message||e}`)}
  }
  function macroParamsFromField(){return ($('#macro-parameters')?.value||'').split(/[\s,]+/).map(x=>x.trim()).filter(Boolean)}
  function macroApplyMeta(){
    if(!state.macroDraft)return null;
    state.macroDraft.name=$('#macro-name')?.value||'';state.macroDraft.description=$('#macro-description')?.value||'';state.macroDraft.parameters=macroParamsFromField();
    if(!state.macroDraft.builtIn)state.macroDraft.id=$('#macro-id')?.value.trim()||'';
    return state.macroDraft
  }
  function macroValue(step){if(step.type==='binding')return step.action||'';if(step.type==='raw')return step.key||'';return step.text||''}
  function macroSetValue(step,value){if(step.type==='binding')step.action=value;else if(step.type==='raw')step.key=value;else step.text=value}
  function macroStepRow(step,index){
    const canEdit=macroCanEdit(),row=document.createElement('div');row.className='saved-tab-row';row.style.cssText='display:grid;grid-template-columns:30px 105px minmax(150px,1fr) 130px 86px 86px 86px auto;gap:6px;align-items:center;margin-bottom:6px';
    const num=document.createElement('span');num.className='muted mono';num.textContent=String(index+1);
    const type=document.createElement('select');for(const [value,label] of [['binding','Binding'],['raw','Raw key'],['text','Text']]){const o=document.createElement('option');o.value=value;o.textContent=label;o.selected=step.type===value;type.appendChild(o)}type.disabled=!canEdit;
    const value=document.createElement('input');value.className='mono';value.value=macroValue(step);value.placeholder=step.type==='binding'?'UI_Select':step.type==='raw'?'SC:1C':'text or {{parameter}}';value.disabled=!canEdit;
    const mods=document.createElement('input');mods.className='mono';mods.value=(step.modifiers||[]).join('+');mods.placeholder='CTRL+SHIFT';mods.title='Raw-key modifiers';mods.disabled=!canEdit||step.type!=='raw';
    const delay=document.createElement('input');delay.type='number';delay.min='0';delay.max='15000';delay.step='10';delay.value=step.delayMs||0;delay.title='Wait before step (ms)';delay.disabled=!canEdit;
    const hold=document.createElement('input');hold.type='number';hold.min='0';hold.max='10000';hold.step='10';hold.value=step.holdMs||0;hold.title='Hold duration (0 = tap)';hold.disabled=!canEdit||step.type==='text';
    const interval=document.createElement('input');interval.type='number';interval.min='0';interval.max='250';interval.step='1';interval.value=step.intervalMs||0;interval.title='Text interval (ms)';interval.disabled=!canEdit||step.type!=='text';
    const controls=document.createElement('div');controls.className='row';controls.style.gap='3px';
    const up=document.createElement('button');up.type='button';up.className='secondary';up.textContent='↑';up.disabled=!canEdit||index===0;
    const down=document.createElement('button');down.type='button';down.className='secondary';down.textContent='↓';down.disabled=!canEdit||index===state.macroDraft.steps.length-1;
    const remove=document.createElement('button');remove.type='button';remove.className='danger secondary';remove.textContent='×';remove.disabled=!canEdit||state.macroDraft.steps.length<=1;
    controls.append(up,down,remove);row.append(num,type,value,mods,delay,hold,interval,controls);
    type.onchange=()=>{step.type=type.value;if(step.type==='binding'&&!step.action)step.action='UI_Select';if(step.type==='raw'&&!step.key)step.key='SC:1C';if(step.type==='text'&&!step.text)step.text='text';renderMacroSteps()};
    value.oninput=()=>macroSetValue(step,value.value);mods.oninput=()=>{step.modifiers=mods.value.split(/[+,\s]+/).map(x=>x.trim()).filter(Boolean)};delay.oninput=()=>step.delayMs=Math.max(0,Number(delay.value)||0);hold.oninput=()=>step.holdMs=Math.max(0,Number(hold.value)||0);interval.oninput=()=>step.intervalMs=Math.max(0,Number(interval.value)||0);
    up.onclick=()=>{macroApplyMeta();const a=state.macroDraft.steps;a.splice(index-1,0,a.splice(index,1)[0]);renderMacroSteps()};down.onclick=()=>{macroApplyMeta();const a=state.macroDraft.steps;a.splice(index+1,0,a.splice(index,1)[0]);renderMacroSteps()};remove.onclick=()=>{macroApplyMeta();state.macroDraft.steps.splice(index,1);renderMacroSteps()};
    return row
  }
  function renderMacroSteps(){const box=$('#macro-step-list');if(!box)return;box.innerHTML='';const steps=state.macroDraft?.steps||[];if(!steps.length){box.innerHTML='<div class="muted empty-state">No steps.</div>';return}for(let i=0;i<steps.length;i++)box.appendChild(macroStepRow(steps[i],i))}
  function renderMacroEditor(){
    const d=state.macroDraft,canEdit=macroCanEdit();if(!d)return;
    $('#macro-name').value=d.name||'';$('#macro-id').value=d.id||'';$('#macro-description').value=d.description||'';$('#macro-parameters').value=(d.parameters||[]).join(', ');$('#macro-id').disabled=!canEdit||!!d.builtIn;
    for(const id of ['macro-name','macro-description','macro-parameters','macro-add-binding','macro-add-raw','macro-add-text','macro-save','macro-duplicate','macro-new','macro-record']){const el=$('#'+id);if(el)el.disabled=!canEdit}
    const rd=$('#macro-reset-delete');if(rd){rd.disabled=!canEdit;rd.textContent=d.builtIn?'Reset default':'Delete macro'}
    renderMacroSteps();renderMacroPolicy();macroStatus(`${d.builtIn?'Built-in':'Custom'} macro · ${d.steps?.length||0} step${d.steps?.length===1?'':'s'}${canEdit?'':macroPolicyAccepted()?' · Read-only on this device':' · Editor locked until macro use agreement is accepted'}`)
  }
  function chooseMacro(id){const d=(state.macros||[]).find(x=>x.id===id);if(!d)return;state.macroSelected=d.id;state.macroDraft=cloneMacro(d);const sel=$('#macro-select');if(sel)sel.value=d.id;renderMacroEditor()}
  async function loadMacros(preferred=''){
    try{const [r,policy]=await Promise.all([request('macros.list'),request('macros.policy')]);state.macroPolicy=policy||null;state.macros=r?.macros||[];renderMacroPolicy();const sel=$('#macro-select');if(sel){sel.innerHTML='';for(const d of state.macros){const o=document.createElement('option');o.value=d.id;o.textContent=`${d.builtIn?'DEFAULT · ':''}${d.name}`;sel.appendChild(o)}const wanted=preferred||state.macroSelected||state.macros[0]?.id||'';if(wanted)chooseMacro(state.macros.some(x=>x.id===wanted)?wanted:state.macros[0]?.id)}const count=$('#macro-count');if(count)count.textContent=String(state.macros.length);return r}catch(e){macroStatus(`${e?.code||'MACRO'}: ${e?.message||e}`);return null}
  }
  function newMacro(steps=[{type:'binding',action:'UI_Select',delayMs:0,holdMs:0}]){if(!macroCanEdit())return;state.macroSelected='';state.macroDraft={id:'',name:'New macro',description:'',builtIn:false,parameters:[],steps:cloneMacro(steps)};const sel=$('#macro-select');if(sel)sel.value='';renderMacroEditor();$('#macro-name')?.select()}
  function duplicateMacro(){if(!macroCanEdit()||!state.macroDraft)return;macroApplyMeta();const d=cloneMacro(state.macroDraft);const originalCount=d.steps?.length||0;d.id='';d.builtIn=false;d.name=`${d.name||'Macro'} copy`;if(d.steps?.length>12)d.steps=d.steps.slice(0,12);state.macroDraft=d;state.macroSelected='';renderMacroEditor();if(originalCount>12)macroStatus(`Custom macros are limited to 12 steps. The duplicate was trimmed from ${originalCount} to 12 steps; review it before saving.`)}
  function addMacroStep(type){if(!macroCanEdit()||!state.macroDraft)return;macroApplyMeta();if(!state.macroDraft.builtIn&&state.macroDraft.steps.length>=12){macroStatus('Custom macros are limited to 12 steps.');return}const step=type==='binding'?{type:'binding',action:'UI_Select',delayMs:0,holdMs:0}:type==='raw'?{type:'raw',key:'SC:1C',modifiers:[],delayMs:0,holdMs:70}:{type:'text',text:'text',delayMs:0,intervalMs:18};state.macroDraft.steps.push(step);renderMacroSteps()}
  async function saveMacro(showMessage=true){
    if(!macroCanEdit()||!state.macroDraft)return null;const def=macroApplyMeta();if(showMessage)macroStatus('Saving macro…');
    try{const saved=await request('macros.save',{macro:def});state.macroSelected=saved.id;state.macroDraft=cloneMacro(saved);await loadMacros(saved.id);if(showMessage)macroStatus(`Saved ${saved.name}.`);return saved}catch(e){macroStatus(`${e?.code||'MACRO_SAVE'}: ${e?.message||e}`);return null}
  }
  async function resetOrDeleteMacro(){
    if(!macroCanEdit()||!state.macroDraft)return;const d=state.macroDraft;if(d.builtIn){if(!confirm(`Reset ${d.name} to the JACoB default sequence?`))return;try{await request('macros.reset',{id:d.id});await loadMacros(d.id);macroStatus(`${d.name} reset to default.`)}catch(e){macroStatus(e?.message||String(e))};return}
    if(!d.id){newMacro();return}if(!confirm(`Delete ${d.name}?`))return;try{await request('macros.delete',{id:d.id});state.macroSelected='';await loadMacros();macroStatus(`${d.name} deleted.`)}catch(e){macroStatus(e?.message||String(e))}
  }
  function recordedMacroSteps(events=[]){
    const downs=new Map(),actions=[];for(const ev of events){if(!ev||ev.isModifier)continue;if(ev.type==='down'){downs.set(ev.pressId,ev);continue}if(ev.type!=='up')continue;const down=downs.get(ev.pressId);if(!down)continue;downs.delete(ev.pressId);const names=[],seen=new Set();for(const m of (down.matches||ev.matches||[])){const name=typeof m==='string'?m:m?.action;if(name&&!seen.has(name)){seen.add(name);names.push(name)}}const start=Number(down.atMs)||0,end=Number(ev.atMs)||start,hold=Math.max(20,Number(ev.durationMs)||end-start||70);actions.push({pressId:ev.pressId,startMs:start,endMs:end,holdMs:hold,key:down.key,modifiers:down.modifiers||[],binding:names.length===1?names[0]:''})}
    actions.sort((a,b)=>a.startMs-b.startMs||Number(a.pressId)-Number(b.pressId));let previousEnd=0;return actions.map(a=>{const delayMs=Math.max(0,Math.round(a.startMs-previousEnd));previousEnd=Math.max(previousEnd,a.endMs);return a.binding?{type:'binding',action:a.binding,delayMs,holdMs:a.holdMs}:{type:'raw',key:a.key,modifiers:a.modifiers||[],delayMs,holdMs:a.holdMs}})
  }
  async function toggleMacroRecording(){
    if(!macroCanEdit())return;const button=$('#macro-record');
    if(!state.macroRecording){try{const status=await request('recorder.status');if(!status?.available)throw new Error('Action Recorder is unavailable on this platform build.');const r=await request('recorder.start');state.macroRecording=true;if(button)button.textContent='Stop & use recording';macroStatus(`RECORDING // ${r?.driver||status.driver||'recorder'}\nSwitch to Elite and perform the macro. Return here and press Stop & use recording.`)}catch(e){macroStatus(`${e?.code||'RECORDER'}: ${e?.message||e}`)};return}
    try{const result=await request('recorder.stop');state.macroRecording=false;if(button)button.textContent='Record macro';let steps=recordedMacroSteps(result?.events||[]);if(!steps.length){macroStatus('Recording stopped, but no complete Elite key presses were captured.');return}const originalCount=steps.length;if(steps.length>12)steps=steps.slice(0,12);state.macroSelected='';state.macroDraft={id:'',name:'Recorded macro',description:'Recorded with JACoB Action Recorder.',builtIn:false,parameters:[],steps};renderMacroEditor();macroStatus(originalCount>12?`Captured ${originalCount} steps. Custom macros are limited to 12, so only the first 12 were imported. Review before saving.`:`Captured ${steps.length} macro step${steps.length===1?'':'s'}. Review the mappings/timing, name it, then Save macro.`)}catch(e){state.macroRecording=false;if(button)button.textContent='Record macro';macroStatus(`${e?.code||'RECORDER'}: ${e?.message||e}`)}
  }
  async function runSelectedMacro(){
    let def=state.macroDraft;if(!def)return;if(macroCanEdit()){const saved=await saveMacro(false);if(!saved)return;def=saved}else{def=(state.macros||[]).find(x=>x.id===state.macroSelected)||def}
    const args={};for(const name of def.parameters||[]){const value=prompt(`Value for ${name}:`,'');if(value===null)return;args[name]=value}
    macroStatus(`Running ${def.name}…`);try{const r=await request('macros.run',{name:def.id,args});macroStatus(pretty(r))}catch(e){macroStatus(`${e?.code||'MACRO'}: ${e?.message||e}`)}
  }
  function formatNetworkDoctor(r={}){
    const lines=[];
    lines.push(`Listener: ${r.bindOK?'READY':'CHECK'} · ${r.bind||'unknown'}`);
    if(r.preferredURL)lines.push(`Recommended mobile URL: ${r.preferredURL}`);
    if(Array.isArray(r.addresses)&&r.addresses.length>1)lines.push(`Other interfaces: ${r.addresses.filter(x=>x!==r.preferredAddress).join(', ')||'none'}`);
    const host=r.host||{};
    if(host.supported){
      const profiles=(host.profiles||[]).map(p=>`${p.interfaceAlias||p.name||'network'}: ${p.networkCategory||'Unknown'}`).join(', ');
      lines.push(`Windows network: ${profiles||'no active profile reported'}`);
      lines.push(`Firewall: ${host.firewallOK?'READY':host.firewallRulePresent?'CHECK RULE':'RULE MISSING'}`);
      if(host.firewallRulePresent)lines.push(`Firewall scope: ${host.firewallProfiles||'unknown'} · TCP ${host.firewallPort||'?'} · ${(host.firewallRemote||[]).join(', ')||'unknown remote scope'}`);
      if(host.error)lines.push(`Windows check: ${host.error}`);
    }
    for(const warning of r.warnings||[])lines.push(`Warning: ${warning}`);
    if(!r.warnings?.length&&r.bindOK&&(!host.supported||host.firewallOK))lines.push('Result: READY FOR MOBILE CONNECTION');
    return lines.join('\n')
  }
  async function runNetworkDoctor(repair=false){
    const out=$('#network-result');if(!privilegedClient()){if(out)out.textContent=tr('Network Doctor requires the host browser or a host-approved admin device.');return null}
    if(repair){
      const ok=confirm('JACoB is about to ask Windows for administrator approval for one specific task:\n\nCreate or repair the “JACoB Local Network” inbound firewall rule for TCP 6626, limited to LocalSubnet on Private/Domain networks.\n\nNormal JACoB updates do not require administrator access. Continue?');
      if(!ok){if(out)out.textContent='Firewall repair cancelled. No administrator request was made.';return null}
    }
    if(out)out.textContent=repair?'Waiting for Windows administrator approval to repair the JACoB firewall rule…':'Checking listener, network profile, addresses and firewall…';
    try{const r=await request(repair?'network.repair':'network.diagnostics');if(out)out.textContent=formatNetworkDoctor(r);const b=$('#repair-network');if(b)b.hidden=!(r?.host?.supported&&!r?.host?.firewallOK);return r}
    catch(e){if(out)out.textContent=`${e?.code||'NETWORK'}: ${e?.message||e}`;throw e}
  }
  function socketURL(){const proto=location.protocol==='https:'?'wss':'ws';return `${proto}://${location.host}/ws`}
  function mediaURL(path,options={}){const q=new URLSearchParams();for(const [k,v] of Object.entries(options))if(v!==undefined&&v!==null)q.set(k,String(v));const auth=state.core?.mediaToken||'';if(auth)q.set('token',auth);q.set('_',Date.now());return `${path}?${q.toString()}`}
  function videoURL(options={}){const p={width:options.width||960,fps:options.fps||5,quality:options.quality||60};if(options.matte===true||options.matte===1||options.matte==='1')p.matte=1;return mediaURL('/api/video.mjpeg',p)}
  function pretty(v){return JSON.stringify(v,null,2)}
  function escapeHTML(s){return String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}

  const TAB_CAPABILITIES={
    clipboard:{label:'Write the host clipboard',detail:'replace text in the clipboard on the computer running JACoB, including while Elite has focus; this does not allow reading the clipboard'},
    control:{label:'Control Elite Dangerous',detail:'send keyboard actions or semantic bindings to the Elite Dangerous window'},
    network:{label:'Use JACoB network access',detail:'send requests to public internet services through JACoB. Because tabs can read Elite journal/state data, grant this only to tabs you trust not to transmit that data'},
    recorder:{label:'Record Elite keyboard input',detail:'observe keyboard input only while Elite Dangerous is the foreground window'},
    overlay:{label:'Draw an Elite overlay',detail:'place graphics in JACoB’s game overlay'},
    video:{label:'View the Elite video feed',detail:'display the focus-gated Elite Dangerous capture stream'},
    vision:{label:'Use derived game vision',detail:'receive derived measurements, OCR text and calibration results computed from Elite-only frames; raw pixels are never exposed to custom tabs'},
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
    if(method==='clipboard.writeText')return'clipboard';
    if(method.startsWith('net.')||method.startsWith('catalog.'))return'network';
    if(method==='video.url')return'video';
    if(method.startsWith('vision.')||method.startsWith('spatial.'))return'vision';
    if(method==='recorder.start'||method==='recorder.stop')return'recorder';
    if(method==='overlay.set'||method==='overlay.clear')return'overlay';
    if(method==='actions.invoke')return'intertab';
    if(method==='macros.run'||method.startsWith('input.')||method.startsWith('binding.'))return'control';
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
    if(method==='vision.sample'||method==='vision.inspect'||method==='spatial.update'||method==='journal.read')gap=75;
    else if(method==='net.fetch'||method==='net.fetchQueued'||method==='net.fetchBatch'||method==='catalog.system.get')gap=100;
    else if(method==='tabstate.set'||method==='tabstate.cas'||method==='tabstate.batch'||method==='tabstate.delete'||method==='tabstate.clear')gap=50;
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
    ws.onopen=()=>{setConnection(true);if(!isLocal&&$('#network-result'))$('#network-result').textContent=tr('Paired. Mobile connection is online.');bootstrap()};
    ws.onclose=()=>{setConnection(false);if(state.quitting){showClosed();return}if(state.updating){$('#connection').textContent=tr('UPDATING');state.reconnectTimer=setTimeout(connect,3000);return}if(state.deviceRevoked){if($('#network-result'))$('#network-result').textContent='This device was revoked. Enter the current pair key to pair again.';return}if(!isLocal&&$('#network-result'))$('#network-result').textContent=tr('Enter the current pair key shown on the computer running JACoB. Remembered devices reconnect automatically.');state.reconnectTimer=setTimeout(connect,1800)};
    ws.onerror=()=>{if(!isLocal&&$('#network-result'))$('#network-result').textContent=tr('JACoB is reachable. Enter the current pair key if this device is not remembered.')};
    ws.onmessage=ev=>{let m;try{m=JSON.parse(ev.data)}catch{return}if(m.type==='response'){const p=state.pending.get(m.id);if(p){state.pending.delete(m.id);m.ok?p.resolve(m.result):p.reject(m.error)}return}if(m.type==='event')handleEvent(m.event,m.data)};
  }
  function request(method,params={}){
    if(!state.socket||state.socket.readyState!==WebSocket.OPEN)return Promise.reject({code:'OFFLINE',message:'core is offline'});
    const id=`ui-${++state.seq}-${Date.now()}`;state.socket.send(JSON.stringify({type:'request',id,method,params}));
    const timeoutMs=(method==='tabs.save'||method==='tabs.get')?90000:30000;
    return new Promise((resolve,reject)=>{state.pending.set(id,{resolve,reject});setTimeout(()=>{if(state.pending.has(id)){state.pending.delete(id);reject({code:'TIMEOUT',message:'request timed out'})}},timeoutMs)});
  }
  async function bootstrap(){try{await loadLocale()}catch{}const jobs=[refreshHealth(),loadBindings(),loadSavedTabs(),loadAppearance(),loadMacros()];if(privilegedClient()){jobs.push(loadPathSettings());jobs.push(loadConnectedDevices())}try{await Promise.all(jobs)}catch{}refreshTutorial().catch(()=>{})}

  function handleEvent(kind,data){
    addStream(kind,data);
    if(kind==='core.hello'){
      state.core=data;state.updating=false;if(!communityRegistryLoadedAt)setTimeout(()=>communityLoadRegistry().catch(()=>{}),50);if(data.locale)applyLocaleInfo(data.locale);state.deviceRevoked=false;if(!isLocal)refreshDeviceCookie();if(privilegedClient()){loadConnectedDevices().catch(()=>{});loadPathSettings().catch(()=>{})}if($('#quit-jacob'))$('#quit-jacob').hidden=!(data.localClient||data.privilegedClient);if($('#install-update'))$('#install-update').hidden=!(data.localClient||data.privilegedClient);$('#core-os').textContent=`${data.os}/${data.arch}`;$('#core-api').textContent=`v${data.apiVersion} (${data.product||'JACoB'} ${data.prototype})`;if($('#update-current'))$('#update-current').textContent=data.version||data.prototype||'—';queueUpdateCheckSoon();
      $('#binding-autofill').textContent=data.autoBind?`${data.autoBind.assigned||0} added / ${data.autoBind.scannedActions||0} scanned`:'—';
      $('#input-driver').textContent=`${data.input?.driver||'—'} / ${data.input?.enabled?'enabled':'disabled'}`;$('#capture-driver').textContent=`${data.capture?.driver||'—'} / ${data.capture?.available?'available':'unavailable'}`;const gameViewBlock=$('#game-view-img')?.closest('details.settings-block');if(gameViewBlock)gameViewBlock.hidden=!data.capture?.available;if($('#overlay-driver'))$('#overlay-driver').textContent=`${data.overlay?.driver||'—'} / ${data.overlay?.available?'available':'unavailable'}`;
      $('#journal-dir').textContent=data.journalDir||tr('not detected');$('#bindings-file').textContent=data.bindingsFile||tr('not detected');if($('#home-journal-state'))$('#home-journal-state').textContent=data.journalDir?'Connected':tr('not detected');renderLAN(data.lan);if(data.health)renderHealth(data.health);refreshTutorial().catch(()=>{});
    }
    if(kind==='state'){state.snapshot=data;renderSnapshot(data);refreshTutorial().catch(()=>{})}
    if(kind==='status'){ $('#status-json').textContent=pretty(data);if(state.snapshot)state.snapshot.status=data }
    if(kind==='journal'){ $('#event-name').textContent=data.event||'journal';if($('#home-event-name'))$('#home-event-name').textContent=data.event||'journal';$('#journal-json').textContent=pretty(data);if(state.snapshot)state.snapshot.lastJournalEvent=data }
    if(kind==='tabs.changed')loadSavedTabs().catch(()=>{});if(kind==='core.update'){state.updating=true;if($('#update-result'))$('#update-result').textContent=`Installing ${data?.version||'update'}…`;}
    if(kind==='devices.changed'&&privilegedClient())loadConnectedDevices().catch(()=>{});
    if(kind==='device.permissions'&&!isLocal){
      const d=data?.device||{};if(state.core){state.core.device=d;state.core.adminClient=!!data?.admin;state.core.privilegedClient=!!data?.privileged}
      refreshDeviceCookie();if($('#quit-jacob'))$('#quit-jacob').hidden=!privilegedClient();if($('#install-update'))$('#install-update').hidden=!privilegedClient();if(privilegedClient()){loadPathSettings().catch(()=>{});loadConnectedDevices().catch(()=>{})}refreshTutorial().catch(()=>{})
    }
    if(kind==='device.revoked'&&!isLocal){
      state.deviceRevoked=true;fetch('/api/device/logout',{method:'POST',credentials:'same-origin'}).catch(()=>{});state.core=null;$('#network-result').textContent='This device was revoked by the JACoB host. Enter the current pair key to pair again.';try{state.socket?.close()}catch{}
    }
    if(kind==='macros.changed')loadMacros(data?.id||'').catch(()=>{});
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
  function pathSummary(r={}){
    const saved=r.saved||{},eff=r.effective||{},src=r.source||{},env=r.environment||{},lines=[];
    lines.push(`Journal: ${eff.journalDir||'not detected'} [${src.journalDir||'auto'}]`);
    lines.push(`Bindings: ${eff.bindingsDir||'not detected'} [${src.bindingsDir||'auto'}]`);
    const controls=eff.controlSchemeDirs||[];lines.push(`ControlSchemes: ${controls.length?controls.join('\n  '):'none detected'} [${src.controlSchemeDirs||'auto'}]`);
    if(env.journalDir||env.bindingsDir||env.controlSchemeDirs)lines.push('Environment override active; it takes priority over saved manual paths.');
    if(r.restartRequired)lines.push('Saved paths differ from the running core. Quit and reopen JACoB to apply them.');
    lines.push(`Saved config: ${r.configFile||'—'}`);return lines.join('\n')
  }
  async function loadPathSettings(){
    if(!privilegedClient())return null;
    const r=await request('paths.get');state.paths=r;
    if($('#path-journal'))$('#path-journal').value=r.saved?.journalDir||'';
    if($('#path-bindings'))$('#path-bindings').value=r.saved?.bindingsDir||'';
    if($('#path-control-schemes'))$('#path-control-schemes').value=(r.saved?.controlSchemeDirs||[]).join('\n');
    if($('#path-result'))$('#path-result').textContent=pathSummary(r);return r
  }
  async function savePathSettings(reset=false){
    if(!privilegedClient())return;
    const out=$('#path-result');if(out)out.textContent=reset?'Restoring automatic detection…':'Validating and saving paths…';
    try{
      const method=reset?'paths.reset':'paths.save';
      const params=reset?{}:{journalDir:$('#path-journal').value,bindingsDir:$('#path-bindings').value,controlSchemeDirs:$('#path-control-schemes').value.split(/\r?\n|;/).map(x=>x.trim()).filter(Boolean)};
      const r=await request(method,params);state.paths=r;
      if(reset){$('#path-journal').value='';$('#path-bindings').value='';$('#path-control-schemes').value=''}
      out.textContent=pathSummary(r);refreshTutorial().catch(()=>{})
    }catch(e){out.textContent=`${e?.code||'PATH'}: ${e?.message||e}`}
  }
  function tutorialMark(prefix,status,detail){
    const mark=$(`#tutorial-${prefix}-state`),out=$(`#tutorial-${prefix}-detail`);if(!mark||!out)return;
    const kind=status==='ready'?'online':status==='check'?'warning':'offline';mark.className=`status-mark ${kind}`;mark.textContent=status==='ready'?tr('READY'):status==='check'?tr('CHECK'):tr('BLOCKED');out.textContent=detail
  }
  async function refreshTutorial(){
    const core=state.core,snap=state.snapshot;
    if(!core){tutorialMark('core','blocked','Core connection is offline.');return}
    tutorialMark('core',Number(core.apiVersion)===12?'ready':'check',`${core.product||'JACoB'} ${core.version||core.prototype||''}\nSDK ${core.apiVersion||'?'} · ${core.os||'?'} / ${core.arch||'?'}`);
    let paths=state.paths;if(privilegedClient()&&!paths){try{paths=await loadPathSettings()}catch{}}
    const journalDir=core.journalDir||paths?.effective?.journalDir||'',bindingsDir=core.bindingsDir||paths?.effective?.bindingsDir||'';
    tutorialMark('path',journalDir&&bindingsDir?'ready':'check',privilegedClient()&&paths?pathSummary(paths):`Journal: ${journalDir||'not detected'}\nBindings: ${bindingsDir||'not detected'}`);
    const jf=snap?.journalFile||'',files=Object.keys(snap?.eliteFiles||{});
    tutorialMark('journal',jf?'ready':journalDir?'check':'blocked',`Journal file: ${jf||'none selected'}\nLatest event: ${snap?.lastJournalEvent?.event||'none'}\nJSON snapshots: ${files.length?files.join(', '):'none seen yet'}`);
    let b=null;try{b=await request('bindings.list')}catch{}
    const inputOK=!!core.input?.available,active=b?.activeFile||core.bindingsFile||'',count=b?.actions?.length??core.bindingsCount??0;
    tutorialMark('input',active&&inputOK?'ready':active||inputOK?'check':'blocked',`Active preset: ${active||'not resolved'}\nParsed commands: ${count}\nInput: ${core.input?.driver||'—'} · ${core.input?.available?'available':'unavailable'}`);
    let spatial=null;try{spatial=await request('spatial.info')}catch{}
    const visualOK=!!core.capture?.available&&!!core.overlay?.available&&!!core.vision?.available;
    tutorialMark('visual',visualOK?'ready':'check',`Capture: ${core.capture?.driver||'—'} · ${core.capture?.available?'available':'unavailable'}\nOverlay: ${core.overlay?.driver||'—'} · ${core.overlay?.available?'available':'unavailable'}\nVision: ${core.vision?.available?'available':'unavailable'}\nSpatial: ${spatial?.available===false?'unavailable':'available'}`);
    const lan=core.lan||{};let netdiag=null;
    if(privilegedClient()&&lan.enabled){try{netdiag=await request('network.diagnostics')}catch(e){netdiag={error:e?.message||String(e)}}}
    if(!lan.enabled){tutorialMark('lan','check','LAN disabled. Local use is fully supported; enable LAN only if you want phone/tablet access.')}else{
      const host=netdiag?.host||{},lanReady=!!netdiag?.bindOK&&(!host.supported||!!host.firewallOK),lines=[`Enabled on port ${lan.port||6626}`,`Preferred address: ${lan.preferredAddress||netdiag?.preferredAddress||'not resolved'}`];
      if(netdiag){lines.push(`Listener: ${netdiag.bindOK?'ready':'check'}`);if(host.supported)lines.push(`Windows firewall: ${host.firewallOK?'ready':host.firewallRulePresent?'rule needs repair':'rule missing'}`);for(const warning of netdiag.warnings||[])lines.push(`Warning: ${warning}`);if(netdiag.error)lines.push(`Network Doctor: ${netdiag.error}`)}
      tutorialMark('lan',lanReady?'ready':'check',lines.join('\n'))
    }
    const diag=(state.savedTabs||[]).find(t=>t.name==='JACoB Diagnostics');tutorialMark('diagnostics',diag?'ready':'check',diag?`Installed as ${diag.id}. Use Install / refresh to replace it with this build's diagnostic tab.`:'Not installed yet.')
  }
  async function installDiagnosticsTab(){
    const out=$('#tutorial-diagnostics-detail');if(!privilegedClient()){if(out)out.textContent='Install Diagnostics from the host browser or a host-approved admin device.';return}
    try{
      out.textContent='Loading bundled diagnostics tab…';const resp=await fetch('/diagnostics-tab.html',{cache:'no-store'});if(!resp.ok)throw new Error(`diagnostics tab HTTP ${resp.status}`);const html=await resp.text();
      await loadSavedTabs();const existing=state.savedTabs.find(t=>t.name==='JACoB Diagnostics');const tab=await request('tabs.save',{id:existing?.id||'',name:'JACoB Diagnostics',html});
      await loadSavedTabs();out.textContent='Diagnostics tab installed. Opening it now.';switchTab(`custom-${tab.id}`);refreshTutorial().catch(()=>{})
    }catch(e){out.textContent=e?.message||pretty(e)}
  }
  function renderSnapshot(s){
    $('#journal-json').textContent=pretty(s.lastJournalEvent||{});$('#status-json').textContent=pretty(s.status||{});$('#event-name').textContent=s.lastJournalEvent?.event||tr('Waiting…');
    if($('#home-event-name'))$('#home-event-name').textContent=s.lastJournalEvent?.event||tr('Waiting…');
    if($('#home-journal-state'))$('#home-journal-state').textContent=s.journalFile||tr('Waiting for journal');
  }
  function renderLAN(lan={}){
    $('#lan-enabled').textContent=lan.enabled?tr('enabled'):tr('disabled');
    const addresses=lan.addresses||[],preferred=lan.preferredAddress||addresses[0]||'',port=lan.port||6626,url=preferred?`http://${preferred}:${port}/`:'';state.lanURL=url;
    const other=addresses.filter(a=>a!==preferred).map(a=>`http://${a}:${port}/`);$('#lan-url').textContent=url||'—';$('#lan-addresses').textContent=other.join('\n')||'—';
    const p=lan.pairToken||'';$('#pair-token').textContent=p||(!isLocal?tr('hidden on remote clients'):'—');if(state.core?.device)$('#remote-token').value='';renderPairQR(p,url)
  }
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
    delete copy.mediaToken;delete copy.adminClient;delete copy.privilegedClient;delete copy.device;delete copy.journalDir;delete copy.bindingsDir;delete copy.bindingsFile;delete copy.bindingsFiles;delete copy.host;copy.localClient=false;
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

  function keepActiveNavVisible(){
    if(!window.matchMedia?.('(max-width:900px)').matches)return;
    const active=[...document.querySelectorAll('.nav')].find(x=>x.classList.contains('active'));
    if(active)requestAnimationFrame(()=>active.scrollIntoView({block:'nearest',inline:'center'}));
  }
  function markActiveNav(tabName){
    document.querySelectorAll('.nav').forEach(x=>{const active=x.dataset.tab===tabName;x.classList.toggle('active',active);if(active)x.setAttribute('aria-current','page');else x.removeAttribute('aria-current')});
    keepActiveNavVisible();
  }
  function switchTab(tabName){
    state.activeTab=tabName;
    if(tabName==='settings')dismissSettingsUpdateBadge();
    markActiveNav(tabName);
    document.querySelectorAll('.panel-page').forEach(x=>x.classList.toggle('active',x.id===tabName));
    if(tabName.startsWith('custom-'))ensureSavedTabLoaded(tabName.slice(7)).catch(e=>showSavedTabLoadError(tabName.slice(7),e));
  }
  function wireNavButton(btn){btn.onclick=()=>switchTab(btn.dataset.tab)}

  const defaultNav={dashboard:'Home',community:'Community',tabmanager:'Tab Manager',tutorial:'Tutorial',settings:'Settings'};
  function navIDForTab(tab){return `custom-${tab.id}`}
  function tabForNavID(id){return state.savedTabs.find(t=>navIDForTab(t)===id)}
  function navLabel(id){return defaultNav[id]?tr(defaultNav[id]):tabForNavID(id)?.name||id}
  function navigationOrder(){
    const expected=['dashboard','community',...state.savedTabs.map(navIDForTab),'tabmanager','tutorial','settings'];
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
    markActiveNav(state.activeTab);
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
  function applySavedTabHTML(tab){const frame=savedTabFrame(tab.id);if(!frame)return;if(frame.dataset.updatedAt===String(tab.updatedAt||''))return;request('overlay.clearPrefix',{prefix:`tab:${tab.id}`}).catch(()=>{});clearActionsForFrame(frame);frame.dataset.sdkReady='';frame.dataset.updatedAt=tab.updatedAt||'';frame.srcdoc=composeTabHTML(tab.html||'')}
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
    for(const existing of [...pages.querySelectorAll('.custom-user-page')])if(!wanted.has(existing.id)){const id=existing.id.slice(7);request('overlay.clearPrefix',{prefix:`tab:${id}`}).catch(()=>{});const frame=existing.querySelector('iframe.jacob-sdk-frame');if(frame)clearActionsForFrame(frame,'target tab was removed');existing.remove()}
    for(const tab of state.savedTabs){
      const pageID=`custom-${tab.id}`;
      let section=document.getElementById(pageID);let iframe=section?.querySelector('iframe.jacob-sdk-frame');
      if(!section){section=document.createElement('section');section.id=pageID;section.className='panel-page custom-user-page';iframe=document.createElement('iframe');iframe.className='jacob-sdk-frame saved-tab-frame';iframe.sandbox='allow-scripts allow-downloads';iframe.dataset.savedTabId=tab.id;section.appendChild(iframe);pages.appendChild(section)}
      const cached=state.tabHTMLCache.get(tab.id);
      if(cached&&cached.updatedAt===tab.updatedAt)applySavedTabHTML({...tab,html:cached.html});else if(iframe.dataset.updatedAt&&iframe.dataset.updatedAt!==String(tab.updatedAt||'')){clearActionsForFrame(iframe);iframe.removeAttribute('srcdoc');iframe.dataset.sdkReady='';iframe.dataset.updatedAt=''}
      const size=Number(tab.sizeBytes||0);const sizeLabel=size?` · ${size>=1024*1024?(size/1024/1024).toFixed(2)+' MB':(size/1024).toFixed(1)+' KB'}`:'';
      const row=document.createElement('div');row.className='saved-tab-row';const meta=document.createElement('div');meta.className='saved-tab-meta';meta.innerHTML=`<strong>${escapeHTML(tab.name)}</strong><span class="muted mono">${escapeHTML(tab.id)}${sizeLabel}</span>`;const actions=document.createElement('div');actions.className='row';const open=document.createElement('button');open.textContent=tr('Open');open.onclick=()=>switchTab(pageID);const edit=document.createElement('button');edit.textContent=tr('Edit');edit.className='secondary';edit.onclick=()=>editSavedTab(tab.id);const share=document.createElement('button');share.textContent=tab.community?.packageId?(tab.community.localAuthor?'Publish update':'Community…'):'Share';share.className='secondary';share.title=tab.community?.packageId?'Publish an update or create a remix':'Prepare this tab for Community Tabs';share.onclick=()=>openCommunityShare(tab.id);const perms=document.createElement('button');perms.textContent='Reset permissions';perms.className='secondary';perms.title='Forget permissions granted to this tab on this browser';perms.onclick=()=>{if(confirm(`Reset permissions for \"${tab.name}\" on this browser?`)){resetTabPermissions(tab.id);$('#tab-manager-result').textContent=`Permissions reset for ${tab.name}. The tab will ask again when it needs sensitive access.`}};const del=document.createElement('button');del.textContent=tr('Remove');del.className='danger';del.onclick=()=>deleteSavedTab(tab.id);actions.append(open,edit,share,perms,del);row.append(meta,actions);list.appendChild(row);
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
  const COMMUNITY_SUBMISSION_URL='https://github.com/pseudo6626/JACoB/issues/new?template=community-tab.yml';
  function communitySlug(value){return String(value||'tab').toLowerCase().normalize('NFKD').replace(/[^a-z0-9]+/g,'-').replace(/^-+|-+$/g,'').slice(0,48)||'tab'}
  function communityRandomHex(bytes=3){const buf=new Uint8Array(bytes);crypto.getRandomValues(buf);return [...buf].map(x=>x.toString(16).padStart(2,'0')).join('')}
  function communityPackageID(name){return `community.${communitySlug(name)}.${communityRandomHex(3)}`}
  function communityVersionTuple(v){const m=String(v||'').match(/^(\d+)\.(\d+)\.(\d+)$/);return m?m.slice(1).map(Number):[1,0,0]}
  function communityNextVersion(base,change){let [major,minor,patch]=communityVersionTuple(base);if(change==='breaking')return `${major+1}.0.0`;if(change==='feature')return `${major}.${minor+1}.0`;return `${major}.${minor}.${patch+1}`}
  function communityDetectCapabilities(html){const rules={clipboard:/\bElite\.clipboard\b/,control:/\bElite\.(?:input|bindings|macros)\b/,network:/\bElite\.(?:net|catalog)\b/,recorder:/\bElite\.recorder\b/,overlay:/\bElite\.(?:overlay|hud)\b/,video:/\bElite\.video\b/,vision:/\bElite\.(?:vision|spatial)\b/,intertab:/\bElite\.actions\.invoke\b/};return Object.entries(rules).filter(([,re])=>re.test(html)).map(([name])=>name).sort()}
  async function communitySHA256(bytes){const digest=await crypto.subtle.digest('SHA-256',bytes);return [...new Uint8Array(digest)].map(x=>x.toString(16).padStart(2,'0')).join('')}
  function communityBytesToBase64(bytes){let binary='';const chunk=0x8000;for(let i=0;i<bytes.length;i+=chunk)binary+=String.fromCharCode(...bytes.subarray(i,Math.min(bytes.length,i+chunk)));return btoa(binary)}
  function communityDownload(name,text){const blob=new Blob([text],{type:'application/json;charset=utf-8'}),url=URL.createObjectURL(blob),a=document.createElement('a');a.href=url;a.download=name;a.style.display='none';document.body.appendChild(a);a.click();a.remove();setTimeout(()=>URL.revokeObjectURL(url),2000)}
  function communityTags(value){return [...new Set(String(value||'').split(',').map(x=>communitySlug(x)).filter(Boolean))].slice(0,12)}
  function communityEscape(value){return escapeHTML(String(value??''))}
  const COMMUNITY_CATEGORIES=['accessibility','bgs','combat','engineering','exploration','fleet-carrier','hud','mining','navigation','racing','trading','utility','other'];
  function communityCategoryLabel(value){return String(value||'other').split('-').map(x=>x?x[0].toUpperCase()+x.slice(1):'').join(' ')}
  function communityCurrentBase(meta){return meta?.publishedVersion||meta?.lastSubmittedVersion||'1.0.0'}
  async function communityRegistryLookup(packageId){if(!packageId)return null;try{const url=`https://raw.githubusercontent.com/pseudo6626/JACoB/main/registry/tabs/${encodeURIComponent(packageId)}.json`,res=await request('net.fetch',{url,method:'GET'});if(Number(res?.status)!==200)return null;return JSON.parse(res.body||'null')}catch{return null}}
  function communityDialog(){let dlg=document.getElementById('community-share-dialog');if(dlg)return dlg;dlg=document.createElement('dialog');dlg.id='community-share-dialog';dlg.style.cssText='width:min(720px,calc(100vw - 32px));max-height:calc(100vh - 32px);overflow:auto;background:#090d11;color:#e8edf0;border:1px solid #8a430e;padding:0;box-shadow:0 18px 70px #000a;';dlg.addEventListener('click',ev=>{if(ev.target===dlg)dlg.close()});document.body.appendChild(dlg);return dlg}
  async function openCommunityShare(id,forcedMode=''){
    const meta=savedTabMeta(id);if(!meta)return;let tab;
    try{tab=await ensureSavedTabLoaded(id)}catch(e){$('#tab-manager-result').textContent=pretty(e);return}
    const c=meta.community||null,hasCommunity=!!c?.packageId,registryEntry=hasCommunity?await communityRegistryLookup(c.packageId):null;
    let mode=forcedMode||(hasCommunity?(c.localAuthor?'update':'remix'):'publish');
    const dlg=communityDialog(),base=registryEntry?.version||communityCurrentBase(c);
    const modeOptions=hasCommunity?`<option value="update" ${mode==='update'?'selected':''}>Publish update (GitHub verifies maintainer access)</option><option value="remix" ${mode==='remix'?'selected':''}>Share as a remix</option>`:`<option value="publish">Publish this tab</option>`;
    dlg.innerHTML=`<form method="dialog" style="padding:22px;display:grid;gap:14px"><div><div class="eyebrow">Community Tabs</div><h2 style="margin:.2em 0">${hasCommunity?'Share or update':'Share this tab'}</h2><p class="muted">JACoB prepares the version, hash, package ID, capability list and registry metadata. You only describe the tab.</p></div><label>What are you doing?<select id="community-mode" style="width:100%;margin-top:6px">${modeOptions}</select></label><label>Tab<input id="community-name" value="${communityEscape(tab.name)}" readonly style="width:100%;margin-top:6px"></label><label>Your display name<input id="community-author" value="${communityEscape(c?.author||'')}" maxlength="80" placeholder="CMDR name or handle" style="width:100%;margin-top:6px"></label><label>What does it do?<textarea id="community-description" maxlength="500" rows="3" placeholder="A short description for people browsing Community Tabs." style="width:100%;margin-top:6px">${communityEscape(c?.description||'')}</textarea></label><label>Category<select id="community-category" style="width:100%;margin-top:6px">${COMMUNITY_CATEGORIES.map(x=>`<option value="${x}" ${(c?.category||'utility')===x?'selected':''}>${communityCategoryLabel(x)}</option>`).join('')}</select></label><label>Tags <span class="muted">(optional, comma separated)</span><input id="community-tags" value="${communityEscape((c?.tags||[]).join(', '))}" placeholder="mining, racing, exploration" style="width:100%;margin-top:6px"></label><label id="community-change-wrap" style="${mode==='update'?'':'display:none'}">Type of update<select id="community-change" style="width:100%;margin-top:6px"><option value="fix">Small fix</option><option value="feature">New features</option><option value="breaking">Major / breaking changes</option></select></label><label id="community-notes-wrap" style="${mode==='publish'?'display:none':''}">What changed?<textarea id="community-notes" maxlength="2000" rows="3" placeholder="A short note for users." style="width:100%;margin-top:6px"></textarea></label><div style="border:1px solid #644016;padding:10px 12px;background:#171007"><strong>Before you publish</strong><div class="muted" style="margin-top:4px">The submission contains the complete HTML for this tab and will become public. Remove passwords, private API keys, personal data, or anything else that should not be published.</div></div><div id="community-preview" class="muted"></div><div class="row" style="justify-content:flex-end"><button value="cancel" class="secondary">Cancel</button><button type="button" id="community-prepare">Prepare submission</button></div></form>`;
    const modeEl=dlg.querySelector('#community-mode'),changeWrap=dlg.querySelector('#community-change-wrap'),notesWrap=dlg.querySelector('#community-notes-wrap'),preview=dlg.querySelector('#community-preview');
    const updatePreview=()=>{mode=modeEl.value;changeWrap.style.display=mode==='update'?'':'none';notesWrap.style.display=mode==='publish'?'none':'';const capabilities=communityDetectCapabilities(tab.html||'');let version='1.0.0';if(mode==='update')version=communityNextVersion(base,dlg.querySelector('#community-change').value);preview.textContent=`JACoB will prepare ${version}. Detected capabilities: ${capabilities.length?capabilities.join(', '):'none of the sensitive SDK capability groups'}.`;};
    modeEl.onchange=updatePreview;dlg.querySelector('#community-change').onchange=updatePreview;updatePreview();
    dlg.querySelector('#community-prepare').onclick=async()=>{
      const author=dlg.querySelector('#community-author').value.trim(),description=dlg.querySelector('#community-description').value.trim(),category=dlg.querySelector('#community-category').value,tags=communityTags(dlg.querySelector('#community-tags').value),notes=dlg.querySelector('#community-notes').value.trim();
      if(!author){dlg.querySelector('#community-author').focus();return}if(!description){dlg.querySelector('#community-description').focus();return}
      const bytes=new TextEncoder().encode(tab.html||'');if(!bytes.length||bytes.length>4*1024*1024){preview.textContent='This tab cannot be submitted because its HTML is empty or larger than 4 MiB.';return}
      preview.textContent='Preparing submission…';
      try{
        let packageId=c?.packageId||communityPackageID(tab.name),basedOn=c?.basedOn||null,version='1.0.0';
        if(mode==='update'){version=communityNextVersion(base,dlg.querySelector('#community-change').value)}
        if(mode==='remix'){basedOn={packageId:c.packageId,version:communityCurrentBase(c)};packageId=communityPackageID(tab.name);version='1.0.0'}
        const sha256=await communitySHA256(bytes),capabilities=communityDetectCapabilities(tab.html||''),sdk=Number(state.core?.apiVersion||state.core?.sdkVersion||15)||15;
        const submission={schema:1,kind:'jacob-community-submission',action:mode,createdAt:new Date().toISOString(),tab:{packageId,name:tab.name,description,author,version,sdk,category,tags,notes,baseVersion:mode==='update'?base:'',basedOn:mode==='remix'?basedOn:null},source:{encoding:'base64',sizeBytes:bytes.length,sha256,html:communityBytesToBase64(bytes)},detectedCapabilities:capabilities};
        const community={packageId,publishedVersion:c?.publishedVersion||'',lastSubmittedVersion:version,publishedSha256:c?.publishedSha256||'',registry:'official',author,description,category,tags,basedOn:mode==='remix'?basedOn:(c?.basedOn||null),localAuthor:true,status:'prepared'};
        const saved=await request('tabs.community.set',{id,community});const local=state.savedTabs.find(x=>x.id===id);if(local)local.community=saved.community||community;
        const filename=`${communitySlug(tab.name)}-${version}.jacobsubmit.json`,payload=JSON.stringify(submission,null,2)+'\n';
        dlg.innerHTML=`<div style="padding:22px;display:grid;gap:14px"><div class="eyebrow">Ready</div><h2 style="margin:0">${communityEscape(tab.name)} ${version}</h2><p>JACoB prepared everything needed for the community registry.</p><div class="card"><div><strong>Package</strong> <span class="mono">${communityEscape(packageId)}</span></div><div><strong>Capabilities</strong> ${communityEscape(capabilities.length?capabilities.join(', '):'none')}</div><div><strong>File</strong> ${communityEscape(filename)}</div></div><ol style="margin:0;padding-left:22px"><li>Download the submission file.</li><li>Open the GitHub form.</li><li>Upload that file and click Submit.</li></ol><div class="row" style="justify-content:flex-end"><button id="community-download">Download submission</button><button id="community-open-github">Open GitHub form</button><button id="community-done" class="secondary">Done</button></div></div>`;
        dlg.querySelector('#community-download').onclick=()=>communityDownload(filename,payload);dlg.querySelector('#community-open-github').onclick=()=>window.open(`${COMMUNITY_SUBMISSION_URL}&title=${encodeURIComponent(`[Community Tab] ${tab.name} ${version}`)}`,'_blank','noopener');dlg.querySelector('#community-done').onclick=()=>dlg.close();
        await loadSavedTabs();
      }catch(e){preview.textContent=e?.message||String(e)}
    };
    dlg.showModal();
  }
  const COMMUNITY_REGISTRY_INDEX='https://raw.githubusercontent.com/pseudo6626/JACoB/main/registry/index.json';
  const COMMUNITY_REGISTRY_STATS='https://raw.githubusercontent.com/pseudo6626/JACoB/main/registry/stats.json';
  let communityRegistryTabs=[],communityRegistryStats={downloads:{}},communityRegistryLoadedAt=0;
  function communityVersionCompare(a,b){const aa=communityVersionTuple(a),bb=communityVersionTuple(b);for(let i=0;i<3;i++){if(aa[i]>bb[i])return 1;if(aa[i]<bb[i])return-1}return 0}
  function communityInstalledByPackage(){const out=new Map();for(const tab of state.savedTabs){const pid=tab.community?.packageId;if(pid)out.set(pid,tab)}return out}
  function communitySDKCompatible(entry){const need=Number(String(entry?.jacobSdk||'').replace(/^>=/,''))||0,current=Number(state.core?.apiVersion||state.core?.sdkVersion||15)||15;return !need||current>=need}
  function communityReleaseTag(entry){return `community--${entry.id}--v${entry.version}`}
  function communityReleaseAsset(entry){return `${entry.version}.html`}
  function communityReleaseURL(entry){return `https://github.com/pseudo6626/JACoB/releases/download/${encodeURIComponent(communityReleaseTag(entry))}/${encodeURIComponent(communityReleaseAsset(entry))}`}
  function communityRawArtifactURL(entry){return `https://raw.githubusercontent.com/pseudo6626/JACoB/main/${String(entry?.artifact?.path||'').split('/').map(encodeURIComponent).join('/')}`}
  async function communityFetchText(url){const res=await request('net.fetch',{url,method:'GET',cacheTtlMs:120000});if(Number(res?.status)!==200)throw new Error(`Registry request returned HTTP ${res?.status||0}`);return String(res.body||'')}
  async function communityFetchJSON(url){return JSON.parse(await communityFetchText(url))}
  async function communityLoadRegistry(force=false){
    const result=$('#community-registry-result');if(result)result.textContent=force?'Refreshing Community Tabs…':'Loading Community Tabs…';
    try{
      const [index,stats]=await Promise.all([communityFetchJSON(COMMUNITY_REGISTRY_INDEX),communityFetchJSON(COMMUNITY_REGISTRY_STATS).catch(()=>({schema:1,downloads:{}}))]);
      communityRegistryTabs=Array.isArray(index?.tabs)?index.tabs:[];communityRegistryStats=stats&&typeof stats==='object'?stats:{downloads:{}};communityRegistryLoadedAt=Date.now();
      if(result)result.textContent=`${communityRegistryTabs.length} community tab${communityRegistryTabs.length===1?'':'s'} available. Download counts are aggregate GitHub release downloads; JACoB does not identify who installed them.`;
      communityPopulateFilters();communityRenderRegistry();
    }catch(e){communityRegistryTabs=[];if(result)result.textContent=`Community registry unavailable: ${e?.message||e}`;communityRenderRegistry()}
  }
  function communityPopulateFilters(){
    const category=$('#community-category-filter');if(category){const before=category.value||'';const cats=[...new Set(communityRegistryTabs.map(x=>x.category||'utility'))].sort();category.innerHTML='<option value="">All categories</option>'+cats.map(x=>`<option value="${communityEscape(x)}">${communityEscape(communityCategoryLabel(x))}</option>`).join('');category.value=cats.includes(before)?before:''}
  }
  function communityFilteredTabs(){
    const q=String($('#community-search')?.value||'').trim().toLowerCase(),category=$('#community-category-filter')?.value||'',cap=$('#community-capability-filter')?.value||'',status=$('#community-status-filter')?.value||'',sort=$('#community-sort')?.value||'downloads',installed=communityInstalledByPackage();
    let rows=communityRegistryTabs.filter(entry=>{
      const local=installed.get(entry.id),hasUpdate=!!(local&&communityVersionCompare(entry.version,local.community?.publishedVersion||'0.0.0')>0);
      if(category&&(entry.category||'utility')!==category)return false;if(cap&&(!Array.isArray(entry.capabilities)||!entry.capabilities.includes(cap)))return false;
      if(status==='installed'&&!local)return false;if(status==='updates'&&!hasUpdate)return false;if(status==='not-installed'&&local)return false;
      if(q){const hay=[entry.name,entry.description,entry.author,entry.id,entry.category,...(entry.tags||[]),...(entry.capabilities||[]),...(entry.maintainers||[])].join(' ').toLowerCase();if(!hay.includes(q))return false}
      return true;
    });
    const downloads=id=>Number(communityRegistryStats?.downloads?.[id]||0);
    if(sort==='newest')rows.sort((a,b)=>String(b.updatedAt||'').localeCompare(String(a.updatedAt||''))||String(a.name).localeCompare(String(b.name)));
    else if(sort==='name')rows.sort((a,b)=>String(a.name).localeCompare(String(b.name)));
    else rows.sort((a,b)=>downloads(b.id)-downloads(a.id)||String(b.updatedAt||'').localeCompare(String(a.updatedAt||'')));
    return rows;
  }
  function communityRenderRegistry(){
    const host=$('#community-results');if(!host)return;const rows=communityFilteredTabs(),installed=communityInstalledByPackage(),downloads=id=>Number(communityRegistryStats?.downloads?.[id]||0);
    if(!rows.length){host.innerHTML='<div class="muted empty-state">No Community Tabs match those filters.</div>';return}
    host.innerHTML='';
    for(const entry of rows){
      const local=installed.get(entry.id),current=local?.community?.publishedVersion||'',hasUpdate=!!(local&&communityVersionCompare(entry.version,current||'0.0.0')>0),compatible=communitySDKCompatible(entry),card=document.createElement('article');card.className='community-tab-card';
      const caps=(entry.capabilities||[]).map(x=>`<span class="community-chip">${communityEscape(x)}</span>`).join(''),tags=(entry.tags||[]).map(x=>`<span class="community-chip subtle">${communityEscape(x)}</span>`).join('');
      card.innerHTML=`<div class="community-card-head"><div><div class="section-kicker">${communityEscape(communityCategoryLabel(entry.category||'utility'))}</div><h2>${communityEscape(entry.name)}</h2><div class="muted">by ${communityEscape(entry.author||'Unknown')} · v${communityEscape(entry.version)}</div></div><div class="community-downloads"><strong>${downloads(entry.id).toLocaleString()}</strong><span>downloads</span></div></div><p>${communityEscape(entry.description||'')}</p><div class="community-chip-row">${caps}${tags}</div><div class="community-card-foot"><span class="muted">${compatible?`SDK ${communityEscape(entry.jacobSdk||'')}`:`Requires ${communityEscape(entry.jacobSdk||'newer JACoB')}`}${entry.derivedFrom?` · remix of ${communityEscape(entry.derivedFrom.id)}`:''}</span><div class="row community-actions"></div></div>`;
      const actions=card.querySelector('.community-actions');
      if(local){const open=document.createElement('button');open.className='secondary';open.textContent='Open';open.onclick=()=>switchTab(navIDForTab(local));actions.appendChild(open);if(hasUpdate){const update=document.createElement('button');update.textContent=`Update to ${entry.version}`;update.disabled=!compatible;update.onclick=()=>communityInstallEntry(entry,local.id);actions.appendChild(update)}else{const badge=document.createElement('span');badge.className='status-mark online';badge.textContent='INSTALLED';actions.appendChild(badge)}}
      else{const install=document.createElement('button');install.textContent=compatible?'Install':'Needs newer JACoB';install.disabled=!compatible;install.onclick=()=>communityInstallEntry(entry,'');actions.appendChild(install)}
      host.appendChild(card);
    }
  }
  async function communityDownloadArtifact(entry){
    let text='';try{text=await communityFetchText(communityReleaseURL(entry))}catch{text=await communityFetchText(communityRawArtifactURL(entry))}
    const bytes=new TextEncoder().encode(text),expectedSize=Number(entry?.artifact?.sizeBytes||0),expectedHash=String(entry?.artifact?.sha256||'').toLowerCase(),actualHash=await communitySHA256(bytes);
    if(expectedSize&&bytes.length!==expectedSize)throw new Error(`Package size mismatch (${bytes.length} vs ${expectedSize})`);if(!expectedHash||actualHash!==expectedHash)throw new Error('Package SHA-256 did not match the registry. Installation was refused.');return text;
  }
  async function communityInstallEntry(entry,existingID=''){
    if(!privilegedClient()){alert('Community tab installation must be done from the browser running on the JACoB host.');return}
    const result=$('#community-registry-result');try{
      if(existingID){const existing=await ensureSavedTabLoaded(existingID),meta=savedTabMeta(existingID),published=meta?.community?.publishedSha256||'';if(published){const currentHash=await communitySHA256(new TextEncoder().encode(existing.html||''));if(currentHash!==published&&!confirm('This installed tab has local edits. Updating will replace those edits.\n\nCancel and use Tab Manager → Community… → Share as a remix if you want to keep them.'))return}}
      if(result)result.textContent=`Downloading and verifying ${entry.name} ${entry.version}…`;const html=await communityDownloadArtifact(entry),saved=await request('tabs.save',{id:existingID||'',name:entry.name,html});
      const old=existingID?savedTabMeta(existingID)?.community:null,community={packageId:entry.id,publishedVersion:entry.version,lastSubmittedVersion:'',publishedSha256:entry.artifact.sha256,registry:'official',author:entry.author||'',description:entry.description||'',category:entry.category||'utility',tags:entry.tags||[],basedOn:entry.derivedFrom?{packageId:entry.derivedFrom.id,version:entry.derivedFrom.version}:null,localAuthor:!!old?.localAuthor,status:'published'};
      await request('tabs.community.set',{id:saved.id,community});state.tabHTMLCache.set(saved.id,{updatedAt:saved.updatedAt||'',html});await loadSavedTabs();if(result)result.textContent=`${entry.name} ${entry.version} installed and verified.`;communityRenderRegistry();
    }catch(e){if(result)result.textContent=`Install failed: ${e?.message||e}`}
  }
  function communityWireBrowser(){
    const refresh=$('#community-refresh'),share=$('#community-share-own');if(refresh)refresh.onclick=()=>communityLoadRegistry(true);if(share)share.onclick=()=>{switchTab('tabmanager');$('#tab-manager-result').textContent='Choose a saved tab and click Share to publish it to Community Tabs.'};
    for(const id of ['community-search','community-category-filter','community-capability-filter','community-status-filter','community-sort']){const el=$(`#${id}`);if(el)el.addEventListener(id==='community-search'?'input':'change',communityRenderRegistry)}
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
    if(!privilegedClient())return;
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
  function browserDeviceName(){
    const ua=navigator.userAgent||'',platform=navigator.userAgentData?.platform||navigator.platform||'Device';let browser='Browser';
    if(/Edg\//.test(ua))browser='Edge';else if(/Chrome\//.test(ua))browser='Chrome';else if(/Firefox\//.test(ua))browser='Firefox';else if(/Safari\//.test(ua))browser='Safari';
    return `${browser} · ${platform}`.slice(0,80)
  }
  async function pairRemoteDevice(){
    const out=$('#network-result');if(isLocal){if(out)out.textContent='The host browser is already trusted.';return}
    const pairKey=$('#remote-token').value.trim();if(!pairKey){if(out)out.textContent='Enter the current pair key shown on the JACoB host.';return}
    if(out)out.textContent='Pairing this browser…';
    try{
      const resp=await fetch('/api/pair',{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json'},body:JSON.stringify({pairKey,name:browserDeviceName()})});
      if(!resp.ok)throw new Error((await resp.text()).trim()||`Pairing failed (${resp.status})`);
      state.deviceRevoked=false;$('#remote-token').value='';if(out)out.textContent='Paired. Connecting…';stopVideo();connect()
    }catch(e){if(out)out.textContent=e?.message||String(e)}
  }
  $('#save-token').onclick=pairRemoteDevice;
  if($('#run-network-doctor'))$('#run-network-doctor').onclick=()=>runNetworkDoctor(false);
  if($('#repair-network'))$('#repair-network').onclick=()=>runNetworkDoctor(true);

  async function checkForUpdates({quiet=false}={}){
    const out=$('#update-result'),btn=$('#check-update'),install=$('#install-update');if(!state.socket||state.socket.readyState!==WebSocket.OPEN)return;if(quiet&&!privilegedClient())return;if(!quiet){btn.disabled=true;install.disabled=true;out.textContent=tr('Querying the release channel…')}
    try{const info=await request('update.check');state.updateInfo=info;$('#update-latest').textContent=info.latestVersion||'—';install.disabled=!(info.available&&info.installable&&privilegedClient());if(info.available&&state.activeTab==='settings'&&info.latestVersion){try{localStorage.setItem(UPDATE_BADGE_KEY,String(info.latestVersion))}catch{}}renderNavigation();if(info.available){out.textContent=`${info.releaseName||('JACoB '+info.latestVersion)} is available.${info.installable?' Ready for installation.':' '+(info.installNote||'Manual installation required.')}`}else{out.textContent=tr('Current release confirmed. No newer published build was found.')}}
    catch(e){if(!quiet)out.textContent=e?.message||pretty(e)}finally{if(!quiet)btn.disabled=false}
  }
  async function installUpdate(){
    const info=state.updateInfo;if(!info?.available)return;if(!confirm(`Install JACoB ${info.latestVersion}?\n\nJACoB will restart after the update is staged.`))return;
    const btn=$('#install-update');btn.disabled=true;state.updating=true;$('#update-result').textContent=tr('Downloading update package…');
    try{const r=await request('update.install');$('#update-result').textContent=`Installing ${r.version||info.latestVersion}…`;$('#connection').textContent=tr('UPDATING')}catch(e){state.updating=false;btn.disabled=false;$('#update-result').textContent=e?.message||pretty(e)}
  }
  $('#check-update').onclick=()=>checkForUpdates({quiet:false});$('#install-update').onclick=installUpdate;

  function setVideoState(on,msg=''){const el=$('#video-state');el.textContent=on?tr('STREAMING'):tr('STOPPED');el.className=`status-mark ${on?'online':'offline'}`;if(msg)$('#video-result').textContent=msg}
  function startVideo(){if(state.core?.capture&&!state.core.capture.available){setVideoState(false,`Capture unavailable: ${state.core.capture.driver}`);return}const opts={width:Number($('#video-width').value),fps:Number($('#video-fps').value),quality:Number($('#video-quality').value),matte:!!$('#video-privacy-matte')?.checked};$('#game-view-img').src=videoURL(opts);setVideoState(true,pretty({mode:'mjpeg',...opts,url:'authenticated local/LAN stream'}))}
  function stopVideo(){$('#game-view-img').removeAttribute('src');setVideoState(false)}
  $('#video-start').onclick=startVideo;$('#video-stop').onclick=stopVideo;$('#video-snapshot').onclick=()=>{const opts={width:Number($('#video-width').value),quality:Number($('#video-quality').value),matte:!!$('#video-privacy-matte')?.checked};$('#game-view-img').src=mediaURL('/api/video/frame.jpg',{...opts,matte:opts.matte?1:undefined});setVideoState(false,pretty({mode:'snapshot',...opts}))};

  $('#run-tab').onclick=previewCurrent;$('#save-tab').onclick=saveCurrentTab;$('#new-tab').onclick=()=>clearEditor(false);$('#reset-tab').onclick=()=>clearEditor(true);
  $('#upload-html').onchange=async e=>{const f=e.target.files?.[0];if(!f)return;$('#custom-html').value=await f.text();if(!$('#custom-tab-name').value.trim())$('#custom-tab-name').value=f.name.replace(/\.html?$/i,'');previewCurrent();e.target.value=''};
  $('#save-theme').onclick=saveAppearance;$('#reset-theme').onclick=resetAppearance;
  if($('#language-select'))$('#language-select').onchange=saveLocale;
  $('#upload-theme').onchange=async e=>{const f=e.target.files?.[0];if(!f)return;$('#theme-html').value=await f.text();await saveAppearance();e.target.value=''};
  if($('#home-add-tab'))$('#home-add-tab').onclick=()=>switchTab('tabmanager');
  if($('#community-refresh'))communityWireBrowser();
  if($('#home-tutorial'))$('#home-tutorial').onclick=()=>switchTab('tutorial');
  if($('#tutorial-tab-manager'))$('#tutorial-tab-manager').onclick=()=>switchTab('tabmanager');
  if($('#tutorial-refresh'))$('#tutorial-refresh').onclick=()=>refreshTutorial();
  if($('#tutorial-open-paths'))$('#tutorial-open-paths').onclick=()=>{switchTab('settings');const d=$('#elite-paths-block');if(d)d.open=true};
  if($('#tutorial-open-bindings'))$('#tutorial-open-bindings').onclick=()=>{switchTab('settings');const d=$('#elite-bindings-block');if(d)d.open=true};
  if($('#tutorial-open-lan'))$('#tutorial-open-lan').onclick=()=>{switchTab('settings');$('#lan-access-block')?.scrollIntoView({behavior:'smooth',block:'center'})};
  if($('#tutorial-install-diagnostics'))$('#tutorial-install-diagnostics').onclick=installDiagnosticsTab;
  if($('#save-paths'))$('#save-paths').onclick=()=>savePathSettings(false);
  if($('#reset-paths'))$('#reset-paths').onclick=()=>savePathSettings(true);
  if($('#macro-select'))$('#macro-select').onchange=e=>chooseMacro(e.target.value);
  if($('#macro-new'))$('#macro-new').onclick=()=>newMacro();
  if($('#macro-duplicate'))$('#macro-duplicate').onclick=duplicateMacro;
  if($('#macro-record'))$('#macro-record').onclick=toggleMacroRecording;
  if($('#macro-add-binding'))$('#macro-add-binding').onclick=()=>addMacroStep('binding');
  if($('#macro-add-raw'))$('#macro-add-raw').onclick=()=>addMacroStep('raw');
  if($('#macro-add-text'))$('#macro-add-text').onclick=()=>addMacroStep('text');
  if($('#macro-save'))$('#macro-save').onclick=()=>saveMacro(true);
  if($('#macro-test'))$('#macro-test').onclick=runSelectedMacro;
  if($('#macro-reset-delete'))$('#macro-reset-delete').onclick=resetOrDeleteMacro;
  if($('#macro-policy-check'))$('#macro-policy-check').onchange=renderMacroPolicy;
  if($('#macro-policy-accept'))$('#macro-policy-accept').onclick=acceptMacroPolicy;


  const visionCalibrationState={active:null};
  function visionRegionOK(r){
    return !!r&&Number.isFinite(Number(r.x))&&Number.isFinite(Number(r.y))&&Number.isFinite(Number(r.width))&&Number.isFinite(Number(r.height))&&
      Number(r.x)>=0&&Number(r.y)>=0&&Number(r.width)>0&&Number(r.height)>0&&Number(r.x)+Number(r.width)<=1.000001&&Number(r.y)+Number(r.height)<=1.000001;
  }
  function visionDebugLayerForFrame(frame){return `${overlayLayerForFrame(frame)}:vision-debug`}
  function visionDebugScene(spec={}){
    const region=spec.region||spec.config?.region;
    if(!visionRegionOK(region))throw actionError('BAD_PARAMS','vision debug requires a normalized region');
    const palette=['#00d7ff','#ff8b2c','#85ff8b','#ff61d2','#d7ff54','#9c8cff','#ff5757','#ffffff'];
    const items=[
      {type:'rect',x:Number(region.x),y:Number(region.y),w:Number(region.width),h:Number(region.height),stroke:palette[0],fill:'#00000000',lineWidth:2},
      {type:'text',x:Number(region.x),y:Math.max(0,Number(region.y)-.018),text:String(spec.label||'VISION'),color:palette[0],fontSize:15}
    ];
    const ops=spec.operations||spec.config?.operations||{};
    let i=1;
    for(const [name,op] of Object.entries(ops)){
      const sub=op?.subregion;if(!visionRegionOK(sub))continue;
      const r={x:Number(region.x)+Number(sub.x)*Number(region.width),y:Number(region.y)+Number(sub.y)*Number(region.height),width:Number(sub.width)*Number(region.width),height:Number(sub.height)*Number(region.height)};
      const color=palette[i%palette.length];i++;
      items.push({type:'rect',x:r.x,y:r.y,w:r.width,h:r.height,stroke:color,fill:'#00000000',lineWidth:2});
      items.push({type:'text',x:r.x,y:Math.max(0,r.y-.015),text:String(name).slice(0,32),color,fontSize:13});
    }
    return {space:'normalized',items};
  }
  async function showVisionDebug(frame,spec={}){
    if(!requireTabCapability(frame,'vision'))throw actionError('PERMISSION_DENIED','vision permission was not granted');
    if(!state.core?.capture?.available)throw actionError('VISION_UNAVAILABLE','screen capture is not installed or unavailable in this build');
    const layer=visionDebugLayerForFrame(frame),scene=visionDebugScene(spec);
    const result=await request('overlay.set',{layer,scene});
    return {shown:true,layer,items:scene.items.length,overlay:result?.overlay||null};
  }
  async function clearVisionDebug(frame){
    const layer=visionDebugLayerForFrame(frame);
    try{return await request('overlay.clear',{layer})}catch(error){if(error?.code==='OVERLAY_UNAVAILABLE')return{cleared:false,layer};throw error}
  }
  function rgbHex(r,g,b){return '#'+[r,g,b].map(v=>Math.max(0,Math.min(255,Math.round(v))).toString(16).padStart(2,'0')).join('').toUpperCase()}
  function rgbToHSV(r,g,b){
    r/=255;g/=255;b/=255;const max=Math.max(r,g,b),min=Math.min(r,g,b),d=max-min;let h=0;
    if(d){if(max===r)h=((g-b)/d)%6;else if(max===g)h=(b-r)/d+2;else h=(r-g)/d+4;h*=60;if(h<0)h+=360}
    return{h:Math.round(h),s:Math.round((max?d/max:0)*100),v:Math.round(max*100)}
  }
  function ensureVisionCalibrationUI(){
    let root=$('#jacob-vision-calibration');if(root)return root;
    const style=document.createElement('style');
    style.textContent=`#jacob-vision-calibration{position:fixed;inset:0;z-index:2147483000;background:#050708f2;color:#e9eef1;font:14px system-ui;display:flex;flex-direction:column}#jacob-vision-calibration[hidden]{display:none}.vision-cal-head{display:flex;gap:12px;align-items:center;padding:10px 14px;border-bottom:1px solid #543014;background:#0a0d0f}.vision-cal-head strong{font-size:16px}.vision-cal-head .spacer{flex:1}.vision-cal-stage{position:relative;flex:1;min-height:0;display:grid;place-items:center;overflow:hidden;background:#000}.vision-cal-stage canvas{max-width:100%;max-height:100%;cursor:crosshair;image-rendering:auto;touch-action:none;user-select:none;-webkit-user-select:none}.vision-cal-foot{padding:10px 14px;border-top:1px solid #543014;background:#0a0d0f;display:flex;gap:10px;align-items:center;flex-wrap:wrap}.vision-cal-foot button,.vision-cal-head button,.vision-cal-foot select{background:#111416;color:#eee;border:1px solid #8a430e;border-radius:0;padding:7px 10px}.vision-cal-foot button.primary{background:#9a4a11}.vision-cal-status{font:12px ui-monospace,Consolas,monospace;color:#aeb6bb;min-width:320px}.vision-cal-mode.active{outline:1px solid #00d7ff}.vision-cal-help{color:#b8b1a8}`;
    document.head.appendChild(style);
    root=document.createElement('div');root.id='jacob-vision-calibration';root.hidden=true;
    root.innerHTML=`<div class="vision-cal-head"><strong>JACoB Vision calibration</strong><span id="vision-cal-title" class="vision-cal-help"></span><span class="spacer"></span><button id="vision-cal-cancel">Cancel</button></div><div class="vision-cal-stage"><canvas id="vision-cal-canvas"></canvas></div><div class="vision-cal-foot"><button id="vision-cal-region" class="vision-cal-mode active">Drag region</button><button id="vision-cal-color" class="vision-cal-mode">Pick color</button><label>Average <select id="vision-cal-average"><option>1</option><option>5</option><option selected>11</option><option>21</option></select></label><button id="vision-cal-preview">Preview saved bounds in Elite</button><span id="vision-cal-status" class="vision-cal-status">Waiting for Elite frame…</span><span class="spacer"></span><button id="vision-cal-use" class="primary">Use calibration</button></div>`;
    document.body.appendChild(root);
    return root;
  }
  async function runVisionCalibration(frame,options={}){
    if(!requireTabCapability(frame,'vision'))throw actionError('PERMISSION_DENIED','vision permission was not granted');
    if(!state.core?.capture?.available)throw actionError('VISION_UNAVAILABLE','screen capture is not installed or unavailable in this build');
    if(visionCalibrationState.active)throw actionError('VISION_CALIBRATION_BUSY','another Vision calibration is already active');
    const root=ensureVisionCalibrationUI(),canvas=$('#vision-cal-canvas'),ctx=canvas.getContext('2d',{willReadFrequently:true}),status=$('#vision-cal-status'),title=$('#vision-cal-title');
    const defaults=options.defaults||{};
    let region=visionRegionOK(defaults.region)?{...defaults.region}:visionRegionOK(options.region)?{...options.region}:{x:.2,y:.2,width:.6,height:.5};
    let color=String(defaults.color||options.color||'#FF8B2C').toUpperCase();
    let sample=null,mode='region',drag=null,img=null,sourceCanvas=document.createElement('canvas'),sourceCtx=sourceCanvas.getContext('2d',{willReadFrequently:true});
    const pickColor=options.pickColor===true||options.mode==='regionColor'||options.mode==='color';
    const avgSelect=$('#vision-cal-average');avgSelect.value=String([1,5,11,21].includes(Number(options.averageSize))?Number(options.averageSize):11);
    title.textContent=String(options.label||options.id||'Select the area JACoB should inspect');
    $('#vision-cal-color').hidden=!pickColor;
    function cssPoint(ev){const r=canvas.getBoundingClientRect();return{x:(ev.clientX-r.left)*canvas.width/r.width,y:(ev.clientY-r.top)*canvas.height/r.height}}
    function normRect(a,b){const x0=Math.max(0,Math.min(a.x,b.x)),y0=Math.max(0,Math.min(a.y,b.y)),x1=Math.min(canvas.width,Math.max(a.x,b.x)),y1=Math.min(canvas.height,Math.max(a.y,b.y));return{x:x0/canvas.width,y:y0/canvas.height,width:Math.max(2,x1-x0)/canvas.width,height:Math.max(2,y1-y0)/canvas.height}}
    function sampleColorAt(p){
      const n=Number(avgSelect.value)||11,h=Math.floor(n/2),x0=Math.max(0,Math.floor(p.x)-h),y0=Math.max(0,Math.floor(p.y)-h),x1=Math.min(canvas.width,x0+n),y1=Math.min(canvas.height,y0+n),data=sourceCtx.getImageData(x0,y0,Math.max(1,x1-x0),Math.max(1,y1-y0)).data;
      let r=0,g=0,b=0,count=0;for(let i=0;i<data.length;i+=4){if(data[i+3]<16)continue;r+=data[i];g+=data[i+1];b+=data[i+2];count++}
      if(!count)return; r/=count;g/=count;b/=count;color=rgbHex(r,g,b);sample={x:p.x/canvas.width,y:p.y/canvas.height,averageSize:n,rgb:{r:Math.round(r),g:Math.round(g),b:Math.round(b)},hsv:rgbToHSV(r,g,b)};draw()
    }
    function draw(){
      if(!img)return;ctx.clearRect(0,0,canvas.width,canvas.height);ctx.drawImage(img,0,0,canvas.width,canvas.height);
      const x=region.x*canvas.width,y=region.y*canvas.height,w=region.width*canvas.width,h=region.height*canvas.height;
      ctx.save();ctx.lineWidth=Math.max(2,canvas.width/700);ctx.strokeStyle='#00D7FF';ctx.fillStyle='#00D7FF18';ctx.fillRect(x,y,w,h);ctx.strokeRect(x,y,w,h);ctx.fillStyle='#00D7FF';ctx.font=`${Math.max(13,canvas.width/90)}px sans-serif`;ctx.fillText('VISION',x+4,Math.max(16,y-5));if(sample){const sx=sample.x*canvas.width,sy=sample.y*canvas.height;ctx.strokeStyle=color;ctx.beginPath();ctx.arc(sx,sy,Math.max(7,canvas.width/160),0,Math.PI*2);ctx.stroke()}ctx.restore();
      const hsv=sample?.hsv;status.textContent=`region x ${region.x.toFixed(4)} y ${region.y.toFixed(4)} w ${region.width.toFixed(4)} h ${region.height.toFixed(4)}${pickColor?`  ·  ${color}${hsv?`  HSV ${hsv.h}° ${hsv.s}% ${hsv.v}%`:''}`:''}`;
    }
    function setMode(next){mode=next;$('#vision-cal-region').classList.toggle('active',mode==='region');$('#vision-cal-color').classList.toggle('active',mode==='color')}
    root.hidden=false;visionCalibrationState.active={frame,root};
    try{
      status.textContent='Focusing Elite and taking a calibration snapshot…';
      await request('vision.calibration.arm',{timeoutMs:8000});
      const ready=await new Promise((resolve,reject)=>{
        const started=Date.now(),timer=setInterval(async()=>{try{const s=await request('vision.calibration.status');if(s.ready){clearInterval(timer);resolve(s)}else if(!s.armed&&Date.now()-started>1000){clearInterval(timer);reject(actionError('VISION_CALIBRATION_FAILED',s.reason||'no Elite frame captured'))}else if(Date.now()-started>12000){clearInterval(timer);reject(actionError('VISION_CALIBRATION_TIMEOUT','JACoB could not capture a verified Elite frame'))}}catch(e){clearInterval(timer);reject(e)}},120);
      });
      img=new Image();img.decoding='async';
      const loaded=new Promise((resolve,reject)=>{img.onload=resolve;img.onerror=()=>reject(actionError('VISION_CALIBRATION_FRAME','captured frame could not be loaded'))});
      img.src=mediaURL('/api/vision/calibration.jpg',{_v:Date.now()});await loaded;
      canvas.width=img.naturalWidth;canvas.height=img.naturalHeight;sourceCanvas.width=img.naturalWidth;sourceCanvas.height=img.naturalHeight;sourceCtx.drawImage(img,0,0,img.naturalWidth,img.naturalHeight);draw();
      status.textContent='Frozen Elite snapshot. Drag the area to inspect'+(pickColor?' or choose Pick color and click the target color.':'.');
      if(pickColor)setMode(options.startMode==='color'?'color':'region');
      canvas.onpointerdown=ev=>{if(ev.cancelable)ev.preventDefault();if(mode==='region'){drag=cssPoint(ev);try{canvas.setPointerCapture?.(ev.pointerId)}catch{}}};
      canvas.onpointermove=ev=>{if(mode==='region'&&drag){if(ev.cancelable)ev.preventDefault();region=normRect(drag,cssPoint(ev));draw()}};
      canvas.onpointerup=ev=>{if(ev.cancelable)ev.preventDefault();const p=cssPoint(ev);if(mode==='region'&&drag){region=normRect(drag,p);drag=null;draw()}else if(mode==='color')sampleColorAt(p)};
      canvas.onpointercancel=ev=>{if(ev.cancelable)ev.preventDefault();drag=null;draw()};
      $('#vision-cal-region').onclick=()=>setMode('region');$('#vision-cal-color').onclick=()=>setMode('color');
      avgSelect.onchange=()=>{if(sample)sampleColorAt({x:sample.x*canvas.width,y:sample.y*canvas.height})};
      $('#vision-cal-preview').onclick=async()=>{try{await showVisionDebug(frame,{region,label:options.label||options.id||'VISION'});status.textContent+='  ·  bounds sent to Elite overlay'}catch(e){status.textContent=e.message||String(e)}};
      return await new Promise((resolve,reject)=>{
        $('#vision-cal-use').onclick=()=>{if(!visionRegionOK(region)){reject(actionError('BAD_PARAMS','select a valid Vision region'));return}resolve({region:{x:+region.x.toFixed(6),y:+region.y.toFixed(6),width:+region.width.toFixed(6),height:+region.height.toFixed(6)},...(pickColor?{color,sample}:{}),capturedAt:ready.capturedAt||null})};
        $('#vision-cal-cancel').onclick=()=>reject(actionError('VISION_CALIBRATION_CANCELLED','Vision calibration was cancelled'));
      });
    }finally{
      root.hidden=true;canvas.onpointerdown=canvas.onpointermove=canvas.onpointerup=canvas.onpointercancel=null;visionCalibrationState.active=null;request('vision.calibration.clear').catch(()=>{});
    }
  }
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
      if(m.method==='vision.calibrate'){
        if(!requireTabCapability(frame,'vision')){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error:{code:'PERMISSION_DENIED',message:'vision permission was not granted'}});return}
        try{const result=await runVisionCalibration(frame,m.params||{});sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:true,result})}catch(error){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error})}return
      }
      if(m.method==='vision.debug.show'){
        try{const result=await showVisionDebug(frame,m.params||{});sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:true,result})}catch(error){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error})}return
      }
      if(m.method==='vision.debug.clear'){
        try{const result=await clearVisionDebug(frame);sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:true,result})}catch(error){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error})}return
      }
      if(m.method==='video.url'){
        if(!requireTabCapability(frame,'video')){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error:{code:'PERMISSION_DENIED',message:'video permission was not granted'}});return}
        sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:true,result:videoURL(m.params||{})});return
      }
      const allowed=new Set(['clipboard.info','clipboard.writeText','core.ping','system.health','state.get','bindings.list','bindings.diagnostics','bindings.get','bindings.reload','input.tap','input.hold','input.text','binding.press','binding.down','binding.up','binding.hold','recorder.status','recorder.start','recorder.stop','macros.list','macros.get','macros.run','video.info','vision.info','vision.sample','vision.inspect','spatial.info','spatial.begin','spatial.update','spatial.observe','spatial.pose','spatial.landmarks','spatial.project','spatial.export','spatial.import','spatial.end','overlay.info','overlay.set','overlay.clear','galaxy.parseSystemName','galaxy.decodeAddress','galaxy.encodeAddress','galaxy.addressForSequence','galaxy.boxel','galaxy.boxelHierarchy','catalog.system.get','net.fetch','net.fetchQueued','net.fetchBatch','elitefiles.list','elitefiles.get','journal.files','journal.read','tabstate.get','tabstate.set','tabstate.cas','tabstate.batch','tabstate.delete','tabstate.clear','locale.get']);
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
