import { findModuleChild, PanelSection, PanelSectionRow, staticClasses } from "@decky/ui";
import { addEventListener, removeEventListener, definePlugin, routerHook } from "@decky/api";
import { useEffect, useMemo, useState } from "react";
import { FaCrosshairs } from "react-icons/fa";

type Point = { x:number; y:number };
type Item = {
  type:string; x?:number; y?:number; x2?:number; y2?:number; w?:number; h?:number; r?:number;
  points?:Point[]; text?:string; color?:string; stroke?:string; fill?:string;
  lineWidth?:number; fontSize?:number; fontFamily?:string; fontWeight?:number; italic?:boolean;
  align?:string; opacity?:number; glow?:number; glowColor?:string; outlineWidth?:number;
  outlineColor?:string; shadowX?:number; shadowY?:number; shadowColor?:string;
  cornerRadius?:number; dash?:string;
};
type Scene = { z?:number; space?:string; width?:number; height?:number; opacity?:number; items:Item[] };
type Snapshot = {
  hudVersion:number; version:number; visible:boolean; layers:Record<string,Scene>;
  connected?:boolean; rendererConnected?:boolean;
};

enum UIComposition {
  Hidden = 0,
  Notification = 1,
  Overlay = 2,
  Opaque = 3,
  OverlayKeyboard = 4,
  OverlayQAM = 5,
  OverlayKeyboard1 = 6,
}

const discoveredUIComposition =
  findModuleChild((m:any) => {
    if (typeof m !== "object") return undefined;
    for (const prop in m) {
      const fn = m[prop];
      if (typeof fn === "function") {
        const s = fn.toString();
        if (s.includes("AddMinimumCompositionStateRequest") &&
            s.includes("ChangeMinimumCompositionStateRequest") &&
            s.includes("RemoveMinimumCompositionStateRequest") &&
            !s.includes("m_mapCompositionStateRequests")) {
          return fn;
        }
      }
    }
    return undefined;
  }) as any;
const useUIComposition: (composition:UIComposition)=>void =
  (typeof discoveredUIComposition === "function" ? discoveredUIComposition : (()=>{})) as any;

let current:Snapshot = {hudVersion:2,version:0,visible:false,layers:{},connected:false};
const subscribers = new Set<(s:Snapshot)=>void>();
function publish(s:Snapshot){ current=s; subscribers.forEach(fn=>fn(s)); }

function useSnapshot(){
  const [snap,setSnap] = useState<Snapshot>(current);
  useEffect(()=>{ subscribers.add(setSnap); return ()=>{subscribers.delete(setSnap)}; },[]);
  return snap;
}

type Viewport={w:number;h:number};
function useViewport():Viewport{
  const read=()=>({w:Math.max(1,window.innerWidth||1280),h:Math.max(1,window.innerHeight||800)});
  const [vp,setVp]=useState<Viewport>(read);
  useEffect(()=>{const fn=()=>setVp(read());window.addEventListener("resize",fn);return()=>window.removeEventListener("resize",fn)},[]);
  return vp;
}
function sx(v:number|undefined, scene:Scene, vp:Viewport){
  const n=v||0; return scene.space==="pixels" ? n : n*vp.w;
}
function sy(v:number|undefined, scene:Scene, vp:Viewport){
  const n=v||0; return scene.space==="pixels" ? n : n*vp.h;
}
function sw(v:number|undefined, scene:Scene, vp:Viewport){ return sx(v,scene,vp); }
function sh(v:number|undefined, scene:Scene, vp:Viewport){ return sy(v,scene,vp); }
function sr(v:number|undefined, scene:Scene, vp:Viewport){
  const n=v||0; return scene.space==="pixels" ? n : n*Math.min(vp.w,vp.h);
}
function dash(v?:string, width=1){
  switch(v){
    case "dash": return `${6*width} ${4*width}`;
    case "dot": return `${width} ${3*width}`;
    case "dashdot": return `${6*width} ${3*width} ${width} ${3*width}`;
    default: return undefined;
  }
}
function fx(i:Item){
  const f:string[]=[];
  if((i.shadowX||i.shadowY) && i.shadowColor)
    f.push(`drop-shadow(${i.shadowX||0}px ${i.shadowY||0}px 0px ${i.shadowColor})`);
  if((i.glow||0)>0)
    f.push(`drop-shadow(0 0 ${i.glow}px ${i.glowColor||i.stroke||i.color||"#ffffff"})`);
  return f.length?f.join(" "):undefined;
}
function common(i:Item){
  return {
    opacity:i.opacity??1,
    filter:fx(i),
    strokeLinecap:"round" as const,
    strokeLinejoin:"round" as const,
  };
}
function renderItem(i:Item, scene:Scene, key:string, vp:Viewport){
  const lw=i.lineWidth||1;
  const stroke=i.stroke||i.color||"none";
  const fill=i.fill||"none";
  const c=common(i);
  if(i.type==="line") return <line key={key} x1={sx(i.x,scene,vp)} y1={sy(i.y,scene,vp)} x2={sx(i.x2,scene,vp)} y2={sy(i.y2,scene,vp)}
    stroke={stroke} strokeWidth={lw} strokeDasharray={dash(i.dash,lw)} style={c}/>;
  if(i.type==="polyline"||i.type==="polygon"){
    const pts=(i.points||[]).map(p=>`${sx(p.x,scene,vp)},${sy(p.y,scene,vp)}`).join(" ");
    return i.type==="polygon"
      ? <polygon key={key} points={pts} stroke={stroke} fill={fill} strokeWidth={lw} strokeDasharray={dash(i.dash,lw)} style={c}/>
      : <polyline key={key} points={pts} stroke={stroke} fill="none" strokeWidth={lw} strokeDasharray={dash(i.dash,lw)} style={c}/>;
  }
  if(i.type==="rect") return <rect key={key} x={sx(i.x,scene,vp)} y={sy(i.y,scene,vp)} width={sw(i.w,scene,vp)} height={sh(i.h,scene,vp)}
    rx={sr(i.cornerRadius,scene,vp)} stroke={stroke} fill={fill} strokeWidth={lw} strokeDasharray={dash(i.dash,lw)} style={c}/>;
  if(i.type==="circle") return <circle key={key} cx={sx(i.x,scene,vp)} cy={sy(i.y,scene,vp)} r={sr(i.r,scene,vp)}
    stroke={stroke} fill={fill} strokeWidth={lw} strokeDasharray={dash(i.dash,lw)} style={c}/>;
  if(i.type==="text"){
    const anchor=i.align==="center"?"middle":i.align==="right"?"end":"start";
    const color=i.color||i.fill||"#fff";
    const outline=i.outlineWidth||0;
    return <text key={key} x={sx(i.x,scene,vp)} y={sy(i.y,scene,vp)}
      fill={color} fontSize={i.fontSize||18} fontFamily={`${i.fontFamily||"sans-serif"}, "Noto Sans", sans-serif`}
      fontWeight={i.fontWeight||400} fontStyle={i.italic?"italic":"normal"}
      textAnchor={anchor} dominantBaseline="text-before-edge"
      stroke={outline>0?(i.outlineColor||"#000000"):"none"} strokeWidth={outline*2}
      paintOrder="stroke fill" style={c}>{i.text||""}</text>;
  }
  return null;
}

function HUDOverlay(){
  const snap=useSnapshot();
  const vp=useViewport();
  const scenes=useMemo(()=>Object.entries(snap.layers||{}).sort((a,b)=>(a[1].z||0)-(b[1].z||0)),[snap]);
  const active=!!snap.connected && !!snap.visible && scenes.length>0;
  useUIComposition(active?UIComposition.Notification:UIComposition.Hidden);
  if(!active) return null;
  return <svg viewBox={`0 0 ${vp.w} ${vp.h}`} preserveAspectRatio="none" aria-hidden="true"
    style={{position:"fixed",left:0,top:0,width:"100vw",height:"100vh",pointerEvents:"none",zIndex:2147483000,overflow:"visible"}}>
    {scenes.flatMap(([name,scene])=>(scene.items||[]).map((item,idx)=>renderItem(item,scene,`${name}:${idx}`,vp)))}
  </svg>;
}

function Content(){
  const snap=useSnapshot();
  return <PanelSection title="HUD Renderer">
    <PanelSectionRow>
      <div style={{padding:"8px 0",fontSize:13}}>
        <div>Core: {snap.connected?"connected":"waiting"}</div>
        <div>HUD2: v{snap.hudVersion||2}</div>
        <div>Layers: {Object.keys(snap.layers||{}).length}</div>
        <div>Scene version: {snap.version||0}</div>
      </div>
    </PanelSectionRow>
  </PanelSection>;
}

export default definePlugin(()=>{
  const listener=addEventListener<[Snapshot]>("jacob_hud_snapshot",(snap)=>publish(snap));
  const name="JACoBHUDOverlay";
  routerHook.addGlobalComponent(name,HUDOverlay);
  return {
    name:"JACoB HUD",
    titleView:<div className={staticClasses.Title}>JACoB HUD Bridge</div>,
    content:<Content/>,
    icon:<FaCrosshairs/>,
    alwaysRender:true,
    onDismount(){
      removeEventListener("jacob_hud_snapshot",listener);
      routerHook.removeGlobalComponent(name);
      publish({hudVersion:2,version:0,visible:false,layers:{},connected:false});
    }
  };
});
