import { findModuleChild, PanelSection, PanelSectionRow, staticClasses } from "@decky/ui";
import { definePlugin, routerHook } from "@decky/api";
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
type Scene = {
  z?:number; space?:string; width?:number; height?:number; opacity?:number; items:Item[];
};
type Snapshot = {
  hudVersion:number; version:number; visible:boolean; layers:Record<string,Scene>;
  connected?:boolean; rendererConnected?:boolean;
};
type NamedScene = [string, Scene];

const CANVAS_W = 1280;
const CANVAS_H = 800;

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

let current:Snapshot = {
  hudVersion:2,
  version:0,
  visible:false,
  layers:{},
  connected:false
};

const subscribers = new Set<(s:Snapshot)=>void>();

function publish(s:Snapshot){
  current=s;
  subscribers.forEach(fn=>fn(s));
}

const HUD_URL="http://127.0.0.1:6626/api/deck/hud/state";
let transportStopped=false;
let transportVersion=0;

function sleep(ms:number){
  return new Promise(resolve=>setTimeout(resolve,ms));
}

async function hudTransportLoop(){
  transportStopped=false;
  while(!transportStopped){
    try{
      const url=`${HUD_URL}?since=${transportVersion}&wait=1000`;
      const res=await fetch(url,{method:"GET",cache:"no-store",mode:"cors"});
      if(!res.ok) throw new Error(`HUD state HTTP ${res.status}`);
      const snap=await res.json() as Snapshot;
      transportVersion=Number(snap.version||transportVersion);
      publish({...snap,connected:true});
    }catch(_err){
      publish({
        hudVersion:current.hudVersion||2,
        version:transportVersion,
        visible:false,
        layers:{},
        connected:false
      });
      await sleep(500);
    }
  }
}

function useSnapshot(){
  const [snap,setSnap] = useState<Snapshot>(current);
  useEffect(()=>{
    subscribers.add(setSnap);
    return ()=>{subscribers.delete(setSnap)};
  },[]);
  return snap;
}

function pixelScaleX(scene:Scene){
  if(scene.space!=="pixels") return 1;
  const ref=(scene.width||0)>0 ? Number(scene.width) : CANVAS_W;
  return CANVAS_W/ref;
}

function pixelScaleY(scene:Scene){
  if(scene.space!=="pixels") return 1;
  const ref=(scene.height||0)>0 ? Number(scene.height) : CANVAS_H;
  return CANVAS_H/ref;
}

function metricScale(scene:Scene){
  if(scene.space!=="pixels") return 1;
  return Math.min(pixelScaleX(scene),pixelScaleY(scene));
}

function sx(v:number|undefined, scene:Scene){
  const n=v||0;
  return scene.space==="pixels" ? n*pixelScaleX(scene) : n*CANVAS_W;
}

function sy(v:number|undefined, scene:Scene){
  const n=v||0;
  return scene.space==="pixels" ? n*pixelScaleY(scene) : n*CANVAS_H;
}

function sw(v:number|undefined, scene:Scene){
  return sx(v,scene);
}

function sh(v:number|undefined, scene:Scene){
  return sy(v,scene);
}

function sr(v:number|undefined, scene:Scene){
  const n=v||0;
  return scene.space==="pixels" ? n*metricScale(scene) : n*Math.min(CANVAS_W,CANVAS_H);
}

function dash(v:string|undefined, width:number){
  switch(v){
    case "dash": return `${6*width} ${4*width}`;
    case "dot": return `${width} ${3*width}`;
    case "dashdot": return `${6*width} ${3*width} ${width} ${3*width}`;
    default: return undefined;
  }
}

function fx(i:Item, scene:Scene){
  const f:string[]=[];
  const ms=metricScale(scene);
  if((i.shadowX||i.shadowY) && i.shadowColor){
    f.push(`drop-shadow(${(i.shadowX||0)*ms}px ${(i.shadowY||0)*ms}px 0px ${i.shadowColor})`);
  }
  if((i.glow||0)>0){
    f.push(`drop-shadow(0 0 ${(i.glow||0)*ms}px ${i.glowColor||i.stroke||i.color||"#ffffff"})`);
  }
  return f.length?f.join(" "):undefined;
}

function common(i:Item, scene:Scene){
  return {
    opacity:i.opacity??1,
    filter:fx(i,scene),
    strokeLinecap:"round" as const,
    strokeLinejoin:"round" as const,
  };
}

function renderItem(i:Item, scene:Scene, key:string){
  const ms=metricScale(scene);
  const lw=(i.lineWidth||1)*ms;
  const stroke=i.stroke||i.color||"none";
  const fill=i.fill||"none";
  const c=common(i,scene);

  if(i.type==="line"){
    return <line
      key={key}
      x1={sx(i.x,scene)}
      y1={sy(i.y,scene)}
      x2={sx(i.x2,scene)}
      y2={sy(i.y2,scene)}
      stroke={stroke}
      strokeWidth={lw}
      strokeDasharray={dash(i.dash,lw)}
      style={c}
    />;
  }

  if(i.type==="polyline"||i.type==="polygon"){
    const pts=(i.points||[])
      .map(p=>`${sx(p.x,scene)},${sy(p.y,scene)}`)
      .join(" ");

    return i.type==="polygon"
      ? <polygon
          key={key}
          points={pts}
          stroke={stroke}
          fill={fill}
          strokeWidth={lw}
          strokeDasharray={dash(i.dash,lw)}
          style={c}
        />
      : <polyline
          key={key}
          points={pts}
          stroke={stroke}
          fill="none"
          strokeWidth={lw}
          strokeDasharray={dash(i.dash,lw)}
          style={c}
        />;
  }

  if(i.type==="rect"){
    return <rect
      key={key}
      x={sx(i.x,scene)}
      y={sy(i.y,scene)}
      width={sw(i.w,scene)}
      height={sh(i.h,scene)}
      rx={sr(i.cornerRadius,scene)}
      stroke={stroke}
      fill={fill}
      strokeWidth={lw}
      strokeDasharray={dash(i.dash,lw)}
      style={c}
    />;
  }

  if(i.type==="circle"){
    return <circle
      key={key}
      cx={sx(i.x,scene)}
      cy={sy(i.y,scene)}
      r={sr(i.r,scene)}
      stroke={stroke}
      fill={fill}
      strokeWidth={lw}
      strokeDasharray={dash(i.dash,lw)}
      style={c}
    />;
  }

  if(i.type==="text"){
    const anchor=i.align==="center"?"middle":i.align==="right"?"end":"start";
    const color=i.color||i.fill||"#fff";
    const outline=(i.outlineWidth||0)*ms;

    return <text
      key={key}
      x={sx(i.x,scene)}
      y={sy(i.y,scene)}
      fill={color}
      fontSize={(i.fontSize||18)*ms}
      fontFamily={`${i.fontFamily||"sans-serif"}, "Noto Sans", sans-serif`}
      fontWeight={i.fontWeight||400}
      fontStyle={i.italic?"italic":"normal"}
      textAnchor={anchor}
      dominantBaseline="text-before-edge"
      stroke={outline>0?(i.outlineColor||"#000000"):"none"}
      strokeWidth={outline*2}
      paintOrder="stroke fill"
      style={c}
    >
      {i.text||""}
    </text>;
  }

  return null;
}

function HUDSurface({scenes}:{scenes:NamedScene[]}){
  useUIComposition(UIComposition.Notification);

  return <div
    aria-hidden="true"
    style={{
      position:"fixed",
      left:0,
      top:0,
      width:"100vw",
      height:"100vh",
      pointerEvents:"none",
      zIndex:2147483000,
      overflow:"hidden",
      background:"transparent",
      backgroundColor:"transparent"
    }}
  >
    <svg
      viewBox={`0 0 ${CANVAS_W} ${CANVAS_H}`}
      preserveAspectRatio="none"
      aria-hidden="true"
      fill="none"
      style={{
        position:"absolute",
        left:0,
        top:0,
        width:"100%",
        height:"100%",
        display:"block",
        pointerEvents:"none",
        overflow:"visible",
        background:"transparent",
        backgroundColor:"transparent"
      }}
    >
      {scenes.flatMap(([name,scene])=>
        (scene.items||[]).map((item,idx)=>
          renderItem(item,scene,`${name}:${idx}`)
        )
      )}
    </svg>
  </div>;
}

function HUDOverlay(){
  const snap=useSnapshot();
  const scenes=useMemo(
    ()=>Object.entries(snap.layers||{})
      .sort((a,b)=>(a[1].z||0)-(b[1].z||0)) as NamedScene[],
    [snap]
  );

  const active=!!snap.connected && !!snap.visible && scenes.length>0;
  if(!active) return null;

  return <HUDSurface scenes={scenes}/>;
}

function Content(){
  const snap=useSnapshot();

  return <PanelSection title="HUD Renderer">
    <PanelSectionRow>
      <div style={{padding:"8px 0",fontSize:13}}>
        <div>Core: {snap.connected?"connected":"waiting"}</div>
        <div>HUD2: v{snap.hudVersion||2}</div>
        <div>Renderer: Deck canonical canvas 1280×800</div>
        <div>Layers: {Object.keys(snap.layers||{}).length}</div>
        <div>Scene version: {snap.version||0}</div>
      </div>
    </PanelSectionRow>
  </PanelSection>;
}

export default definePlugin(()=>{
  const name="JACoBHUDOverlay";

  transportStopped=false;
  void hudTransportLoop();
  routerHook.addGlobalComponent(name,HUDOverlay);

  return {
    name:"JACoB HUD",
    titleView:<div className={staticClasses.Title}>JACoB HUD Bridge</div>,
    content:<Content/>,
    icon:<FaCrosshairs/>,
    alwaysRender:true,
    onDismount(){
      transportStopped=true;
      routerHook.removeGlobalComponent(name);
      publish({
        hudVersion:2,
        version:transportVersion,
        visible:false,
        layers:{},
        connected:false
      });
    }
  };
});
