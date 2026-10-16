import { findModuleChild, PanelSection, PanelSectionRow, staticClasses } from "@decky/ui";
import { definePlugin, routerHook } from "@decky/api";
import { useEffect, useMemo, useRef, useState } from "react";
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
  hudVersion:number;
  version:number;
  visible:boolean;
  layers:Record<string,Scene>;
  connected?:boolean;
  rendererConnected?:boolean;
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
        if (
          s.includes("AddMinimumCompositionStateRequest") &&
          s.includes("ChangeMinimumCompositionStateRequest") &&
          s.includes("RemoveMinimumCompositionStateRequest") &&
          !s.includes("m_mapCompositionStateRequests")
        ) {
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
  const [snap,setSnap]=useState<Snapshot>(current);

  useEffect(()=>{
    subscribers.add(setSnap);
    return ()=>{ subscribers.delete(setSnap); };
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

function sx(v:number|undefined,scene:Scene){
  const n=Number(v||0);
  return scene.space==="pixels" ? n*pixelScaleX(scene) : n*CANVAS_W;
}

function sy(v:number|undefined,scene:Scene){
  const n=Number(v||0);
  return scene.space==="pixels" ? n*pixelScaleY(scene) : n*CANVAS_H;
}

function sw(v:number|undefined,scene:Scene){
  return sx(v,scene);
}

function sh(v:number|undefined,scene:Scene){
  return sy(v,scene);
}

function sr(v:number|undefined,scene:Scene){
  const n=Number(v||0);
  return scene.space==="pixels"
    ? n*metricScale(scene)
    : n*Math.min(CANVAS_W,CANVAS_H);
}

function setDash(ctx:CanvasRenderingContext2D,dash:string|undefined,width:number){
  switch(dash){
    case "dash":
      ctx.setLineDash([6*width,4*width]);
      break;
    case "dot":
      ctx.setLineDash([width,3*width]);
      break;
    case "dashdot":
      ctx.setLineDash([6*width,3*width,width,3*width]);
      break;
    default:
      ctx.setLineDash([]);
      break;
  }
}

function hasPaint(v:string|undefined){
  return !!v && v!=="none" && v!=="transparent";
}

function roundedRectPath(
  ctx:CanvasRenderingContext2D,
  x:number,
  y:number,
  w:number,
  h:number,
  radius:number
){
  let r=Math.max(0,Math.min(radius,Math.abs(w)/2,Math.abs(h)/2));

  ctx.beginPath();

  if(r<=0){
    ctx.rect(x,y,w,h);
    return;
  }

  ctx.moveTo(x+r,y);
  ctx.lineTo(x+w-r,y);
  ctx.quadraticCurveTo(x+w,y,x+w,y+r);
  ctx.lineTo(x+w,y+h-r);
  ctx.quadraticCurveTo(x+w,y+h,x+w-r,y+h);
  ctx.lineTo(x+r,y+h);
  ctx.quadraticCurveTo(x,y+h,x,y+h-r);
  ctx.lineTo(x,y+r);
  ctx.quadraticCurveTo(x,y,x+r,y);
  ctx.closePath();
}

function beginItem(ctx:CanvasRenderingContext2D,item:Item,scene:Scene){
  const ms=metricScale(scene);

  ctx.save();
  ctx.globalAlpha=item.opacity??1;
  ctx.lineWidth=Math.max(.5,(item.lineWidth||1)*ms);
  ctx.lineCap="round";
  ctx.lineJoin="round";
  setDash(ctx,item.dash,ctx.lineWidth);

  ctx.shadowBlur=0;
  ctx.shadowOffsetX=0;
  ctx.shadowOffsetY=0;
  ctx.shadowColor="transparent";

  if((item.glow||0)>0){
    ctx.shadowBlur=(item.glow||0)*ms;
    ctx.shadowColor=item.glowColor||item.stroke||item.color||"#ffffff";
  }else if((item.shadowX||0)!==0 || (item.shadowY||0)!==0){
    ctx.shadowOffsetX=(item.shadowX||0)*ms;
    ctx.shadowOffsetY=(item.shadowY||0)*ms;
    ctx.shadowColor=item.shadowColor||"#000000AA";
  }
}

function strokeAndFill(
  ctx:CanvasRenderingContext2D,
  item:Item,
  defaultStroke="none"
){
  const fill=item.fill||"none";
  const stroke=item.stroke||item.color||defaultStroke;

  if(hasPaint(fill)){
    ctx.fillStyle=fill;
    ctx.fill();
  }

  if(hasPaint(stroke)){
    ctx.strokeStyle=stroke;
    ctx.stroke();
  }
}

function drawItem(ctx:CanvasRenderingContext2D,item:Item,scene:Scene){
  beginItem(ctx,item,scene);

  try{
    if(item.type==="line"){
      ctx.beginPath();
      ctx.moveTo(sx(item.x,scene),sy(item.y,scene));
      ctx.lineTo(sx(item.x2,scene),sy(item.y2,scene));

      const stroke=item.stroke||item.color||"#ffffff";
      if(hasPaint(stroke)){
        ctx.strokeStyle=stroke;
        ctx.stroke();
      }
      return;
    }

    if(item.type==="polyline" || item.type==="polygon"){
      const points=item.points||[];
      if(points.length<2) return;

      ctx.beginPath();
      ctx.moveTo(sx(points[0].x,scene),sy(points[0].y,scene));

      for(let n=1;n<points.length;n++){
        ctx.lineTo(sx(points[n].x,scene),sy(points[n].y,scene));
      }

      if(item.type==="polygon") ctx.closePath();
      strokeAndFill(ctx,item,item.type==="polyline"?"#ffffff":"none");
      return;
    }

    if(item.type==="rect"){
      roundedRectPath(
        ctx,
        sx(item.x,scene),
        sy(item.y,scene),
        sw(item.w,scene),
        sh(item.h,scene),
        sr(item.cornerRadius,scene)
      );
      strokeAndFill(ctx,item);
      return;
    }

    if(item.type==="circle"){
      const r=sr(item.r,scene);
      if(r<=0) return;

      ctx.beginPath();
      ctx.arc(sx(item.x,scene),sy(item.y,scene),r,0,Math.PI*2);
      strokeAndFill(ctx,item);
      return;
    }

    if(item.type==="text"){
      const ms=metricScale(scene);
      const size=Math.max(1,(item.fontSize||18)*ms);
      const family=item.fontFamily||"sans-serif";
      const weight=item.fontWeight||400;
      const italic=item.italic?"italic ":"";

      ctx.font=`${italic}${weight} ${size}px ${family}, "Noto Sans", sans-serif`;
      ctx.textBaseline="top";
      ctx.textAlign=
        item.align==="center" ? "center" :
        item.align==="right" ? "right" :
        "left";

      const x=sx(item.x,scene);
      const y=sy(item.y,scene);
      const text=item.text||"";
      const color=item.color||item.fill||"#ffffff";
      const outline=(item.outlineWidth||0)*ms;

      if(outline>0){
        ctx.lineWidth=Math.max(.5,outline*2);
        ctx.strokeStyle=item.outlineColor||"#000000";
        ctx.strokeText(text,x,y);
      }

      ctx.fillStyle=color;
      ctx.fillText(text,x,y);
      return;
    }
  }finally{
    ctx.restore();
  }
}

function HUDSurface({scenes}:{scenes:NamedScene[]}){
  useUIComposition(UIComposition.Notification);
  const canvasRef=useRef<HTMLCanvasElement>(null);

  useEffect(()=>{
    const canvas=canvasRef.current;
    if(!canvas) return;

    const ctx=canvas.getContext("2d",{alpha:true});
    if(!ctx) return;

    ctx.setTransform(1,0,0,1,0,0);
    ctx.globalCompositeOperation="source-over";
    ctx.clearRect(0,0,CANVAS_W,CANVAS_H);

    for(const [,scene] of scenes){
      for(const item of scene.items||[]){
        drawItem(ctx,item,scene);
      }
    }
  },[scenes]);

  return <canvas
    ref={canvasRef}
    width={CANVAS_W}
    height={CANVAS_H}
    aria-hidden="true"
    style={{
      position:"fixed",
      left:0,
      top:0,
      width:"100vw",
      height:"100vh",
      display:"block",
      pointerEvents:"none",
      zIndex:2147483000,
      background:"transparent",
      backgroundColor:"transparent"
    }}
  />;
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
        <div>Renderer: Canvas 2D / 1280×800</div>
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
