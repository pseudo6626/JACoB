const manifest = {"name":"JACoB HUD Bridge"};
const API_VERSION = 2;
const internalAPIConnection = window.__DECKY_SECRET_INTERNALS_DO_NOT_USE_OR_YOU_WILL_BE_FIRED_deckyLoaderAPIInit;
if (!internalAPIConnection) {
    throw new Error('[@decky/api]: Failed to connect to the loader as as the loader API was not initialized. This is likely a bug in Decky Loader.');
}
let api;
try {
    api = internalAPIConnection.connect(API_VERSION, manifest.name);
}
catch {
    api = internalAPIConnection.connect(1, manifest.name);
    console.warn(`[@decky/api] Requested API version ${API_VERSION} but the running loader only supports version 1. Some features may not work.`);
}
if (api._version != API_VERSION) {
    console.warn(`[@decky/api] Requested API version ${API_VERSION} but the running loader only supports version ${api._version}. Some features may not work.`);
}
const routerHook = api.routerHook;
const definePlugin = (fn) => {
    return (...args) => {
        return fn(...args);
    };
};

var DefaultContext = {
  color: undefined,
  size: undefined,
  className: undefined,
  style: undefined,
  attr: undefined
};
var IconContext = SP_REACT.createContext && /*#__PURE__*/SP_REACT.createContext(DefaultContext);

var _excluded = ["attr", "size", "title"];
function _objectWithoutProperties(e, t) { if (null == e) return {}; var o, r, i = _objectWithoutPropertiesLoose(e, t); if (Object.getOwnPropertySymbols) { var n = Object.getOwnPropertySymbols(e); for (r = 0; r < n.length; r++) o = n[r], -1 === t.indexOf(o) && {}.propertyIsEnumerable.call(e, o) && (i[o] = e[o]); } return i; }
function _objectWithoutPropertiesLoose(r, e) { if (null == r) return {}; var t = {}; for (var n in r) if ({}.hasOwnProperty.call(r, n)) { if (-1 !== e.indexOf(n)) continue; t[n] = r[n]; } return t; }
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
function ownKeys(e, r) { var t = Object.keys(e); if (Object.getOwnPropertySymbols) { var o = Object.getOwnPropertySymbols(e); r && (o = o.filter(function (r) { return Object.getOwnPropertyDescriptor(e, r).enumerable; })), t.push.apply(t, o); } return t; }
function _objectSpread(e) { for (var r = 1; r < arguments.length; r++) { var t = null != arguments[r] ? arguments[r] : {}; r % 2 ? ownKeys(Object(t), true).forEach(function (r) { _defineProperty(e, r, t[r]); }) : Object.getOwnPropertyDescriptors ? Object.defineProperties(e, Object.getOwnPropertyDescriptors(t)) : ownKeys(Object(t)).forEach(function (r) { Object.defineProperty(e, r, Object.getOwnPropertyDescriptor(t, r)); }); } return e; }
function _defineProperty(e, r, t) { return (r = _toPropertyKey(r)) in e ? Object.defineProperty(e, r, { value: t, enumerable: true, configurable: true, writable: true }) : e[r] = t, e; }
function _toPropertyKey(t) { var i = _toPrimitive(t, "string"); return "symbol" == typeof i ? i : i + ""; }
function _toPrimitive(t, r) { if ("object" != typeof t || !t) return t; var e = t[Symbol.toPrimitive]; if (void 0 !== e) { var i = e.call(t, r); if ("object" != typeof i) return i; throw new TypeError("@@toPrimitive must return a primitive value."); } return ("string" === r ? String : Number)(t); }
function Tree2Element(tree) {
  return tree && tree.map((node, i) => /*#__PURE__*/SP_REACT.createElement(node.tag, _objectSpread({
    key: i
  }, node.attr), Tree2Element(node.child)));
}
function GenIcon(data) {
  return props => /*#__PURE__*/SP_REACT.createElement(IconBase, _extends({
    attr: _objectSpread({}, data.attr)
  }, props), Tree2Element(data.child));
}
function IconBase(props) {
  var elem = conf => {
    var attr = props.attr,
      size = props.size,
      title = props.title,
      svgProps = _objectWithoutProperties(props, _excluded);
    var computedSize = size || conf.size || "1em";
    var className;
    if (conf.className) className = conf.className;
    if (props.className) className = (className ? className + " " : "") + props.className;
    return /*#__PURE__*/SP_REACT.createElement("svg", _extends({
      stroke: "currentColor",
      fill: "currentColor",
      strokeWidth: "0"
    }, conf.attr, attr, svgProps, {
      className: className,
      style: _objectSpread(_objectSpread({
        color: props.color || conf.color
      }, conf.style), props.style),
      height: computedSize,
      width: computedSize,
      xmlns: "http://www.w3.org/2000/svg"
    }), title && /*#__PURE__*/SP_REACT.createElement("title", null, title), props.children);
  };
  return IconContext !== undefined ? /*#__PURE__*/SP_REACT.createElement(IconContext.Consumer, null, conf => elem(conf)) : elem(DefaultContext);
}

// THIS FILE IS AUTO GENERATED
function FaCrosshairs (props) {
  return GenIcon({"attr":{"viewBox":"0 0 512 512"},"child":[{"tag":"path","attr":{"d":"M500 224h-30.364C455.724 130.325 381.675 56.276 288 42.364V12c0-6.627-5.373-12-12-12h-40c-6.627 0-12 5.373-12 12v30.364C130.325 56.276 56.276 130.325 42.364 224H12c-6.627 0-12 5.373-12 12v40c0 6.627 5.373 12 12 12h30.364C56.276 381.675 130.325 455.724 224 469.636V500c0 6.627 5.373 12 12 12h40c6.627 0 12-5.373 12-12v-30.364C381.675 455.724 455.724 381.675 469.636 288H500c6.627 0 12-5.373 12-12v-40c0-6.627-5.373-12-12-12zM288 404.634V364c0-6.627-5.373-12-12-12h-40c-6.627 0-12 5.373-12 12v40.634C165.826 392.232 119.783 346.243 107.366 288H148c6.627 0 12-5.373 12-12v-40c0-6.627-5.373-12-12-12h-40.634C119.768 165.826 165.757 119.783 224 107.366V148c0 6.627 5.373 12 12 12h40c6.627 0 12-5.373 12-12v-40.634C346.174 119.768 392.217 165.757 404.634 224H364c-6.627 0-12 5.373-12 12v40c0 6.627 5.373 12 12 12h40.634C392.232 346.174 346.243 392.217 288 404.634zM288 256c0 17.673-14.327 32-32 32s-32-14.327-32-32c0-17.673 14.327-32 32-32s32 14.327 32 32z"},"child":[]}]})(props);
}

var UIComposition;
(function (UIComposition) {
    UIComposition[UIComposition["Hidden"] = 0] = "Hidden";
    UIComposition[UIComposition["Notification"] = 1] = "Notification";
    UIComposition[UIComposition["Overlay"] = 2] = "Overlay";
    UIComposition[UIComposition["Opaque"] = 3] = "Opaque";
    UIComposition[UIComposition["OverlayKeyboard"] = 4] = "OverlayKeyboard";
    UIComposition[UIComposition["OverlayQAM"] = 5] = "OverlayQAM";
    UIComposition[UIComposition["OverlayKeyboard1"] = 6] = "OverlayKeyboard1";
})(UIComposition || (UIComposition = {}));
const discoveredUIComposition = DFL.findModuleChild((m) => {
    if (typeof m !== "object")
        return undefined;
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
});
const useUIComposition = (typeof discoveredUIComposition === "function" ? discoveredUIComposition : (() => { }));
let current = { hudVersion: 2, version: 0, visible: false, layers: {}, connected: false };
const subscribers = new Set();
function publish(s) { current = s; subscribers.forEach(fn => fn(s)); }
const HUD_URL = "http://127.0.0.1:6626/api/deck/hud/state";
let transportStopped = false;
let transportVersion = 0;
function sleep(ms) { return new Promise(resolve => setTimeout(resolve, ms)); }
async function hudTransportLoop() {
    transportStopped = false;
    while (!transportStopped) {
        try {
            const url = `${HUD_URL}?since=${transportVersion}&wait=1000`;
            const res = await fetch(url, { method: "GET", cache: "no-store", mode: "cors" });
            if (!res.ok)
                throw new Error(`HUD state HTTP ${res.status}`);
            const snap = await res.json();
            transportVersion = Number(snap.version || transportVersion);
            publish({ ...snap, connected: true });
        }
        catch (_err) {
            publish({
                hudVersion: current.hudVersion || 2,
                version: transportVersion,
                visible: false,
                layers: {},
                connected: false
            });
            await sleep(500);
        }
    }
}
function useSnapshot() {
    const [snap, setSnap] = SP_REACT.useState(current);
    SP_REACT.useEffect(() => { subscribers.add(setSnap); return () => { subscribers.delete(setSnap); }; }, []);
    return snap;
}
function useViewport() {
    const read = () => ({ w: Math.max(1, window.innerWidth || 1280), h: Math.max(1, window.innerHeight || 800) });
    const [vp, setVp] = SP_REACT.useState(read);
    SP_REACT.useEffect(() => { const fn = () => setVp(read()); window.addEventListener("resize", fn); return () => window.removeEventListener("resize", fn); }, []);
    return vp;
}
function sx(v, scene, vp) {
    const n = v || 0;
    return scene.space === "pixels" ? n : n * vp.w;
}
function sy(v, scene, vp) {
    const n = v || 0;
    return scene.space === "pixels" ? n : n * vp.h;
}
function sw(v, scene, vp) { return sx(v, scene, vp); }
function sh(v, scene, vp) { return sy(v, scene, vp); }
function sr(v, scene, vp) {
    const n = v || 0;
    return scene.space === "pixels" ? n : n * Math.min(vp.w, vp.h);
}
function dash(v, width = 1) {
    switch (v) {
        case "dash": return `${6 * width} ${4 * width}`;
        case "dot": return `${width} ${3 * width}`;
        case "dashdot": return `${6 * width} ${3 * width} ${width} ${3 * width}`;
        default: return undefined;
    }
}
function fx(i) {
    const f = [];
    if ((i.shadowX || i.shadowY) && i.shadowColor)
        f.push(`drop-shadow(${i.shadowX || 0}px ${i.shadowY || 0}px 0px ${i.shadowColor})`);
    if ((i.glow || 0) > 0)
        f.push(`drop-shadow(0 0 ${i.glow}px ${i.glowColor || i.stroke || i.color || "#ffffff"})`);
    return f.length ? f.join(" ") : undefined;
}
function common(i) {
    return {
        opacity: i.opacity ?? 1,
        filter: fx(i),
        strokeLinecap: "round",
        strokeLinejoin: "round",
    };
}
function renderItem(i, scene, key, vp) {
    const lw = i.lineWidth || 1;
    const stroke = i.stroke || i.color || "none";
    const fill = i.fill || "none";
    const c = common(i);
    if (i.type === "line")
        return SP_JSX.jsx("line", { x1: sx(i.x, scene, vp), y1: sy(i.y, scene, vp), x2: sx(i.x2, scene, vp), y2: sy(i.y2, scene, vp), stroke: stroke, strokeWidth: lw, strokeDasharray: dash(i.dash, lw), style: c }, key);
    if (i.type === "polyline" || i.type === "polygon") {
        const pts = (i.points || []).map(p => `${sx(p.x, scene, vp)},${sy(p.y, scene, vp)}`).join(" ");
        return i.type === "polygon"
            ? SP_JSX.jsx("polygon", { points: pts, stroke: stroke, fill: fill, strokeWidth: lw, strokeDasharray: dash(i.dash, lw), style: c }, key)
            : SP_JSX.jsx("polyline", { points: pts, stroke: stroke, fill: "none", strokeWidth: lw, strokeDasharray: dash(i.dash, lw), style: c }, key);
    }
    if (i.type === "rect")
        return SP_JSX.jsx("rect", { x: sx(i.x, scene, vp), y: sy(i.y, scene, vp), width: sw(i.w, scene, vp), height: sh(i.h, scene, vp), rx: sr(i.cornerRadius, scene, vp), stroke: stroke, fill: fill, strokeWidth: lw, strokeDasharray: dash(i.dash, lw), style: c }, key);
    if (i.type === "circle")
        return SP_JSX.jsx("circle", { cx: sx(i.x, scene, vp), cy: sy(i.y, scene, vp), r: sr(i.r, scene, vp), stroke: stroke, fill: fill, strokeWidth: lw, strokeDasharray: dash(i.dash, lw), style: c }, key);
    if (i.type === "text") {
        const anchor = i.align === "center" ? "middle" : i.align === "right" ? "end" : "start";
        const color = i.color || i.fill || "#fff";
        const outline = i.outlineWidth || 0;
        return SP_JSX.jsx("text", { x: sx(i.x, scene, vp), y: sy(i.y, scene, vp), fill: color, fontSize: i.fontSize || 18, fontFamily: `${i.fontFamily || "sans-serif"}, "Noto Sans", sans-serif`, fontWeight: i.fontWeight || 400, fontStyle: i.italic ? "italic" : "normal", textAnchor: anchor, dominantBaseline: "text-before-edge", stroke: outline > 0 ? (i.outlineColor || "#000000") : "none", strokeWidth: outline * 2, paintOrder: "stroke fill", style: c, children: i.text || "" }, key);
    }
    return null;
}
function HUDSurface({ scenes }) {
    useUIComposition(UIComposition.Notification);
    const vp = useViewport();
    return SP_JSX.jsx("div", { "aria-hidden": "true", style: {
            position: "fixed",
            left: 0,
            top: 0,
            width: "100vw",
            height: "100vh",
            pointerEvents: "none",
            zIndex: 2147483000,
            overflow: "hidden",
            background: "transparent",
            backgroundColor: "transparent"
        }, children: SP_JSX.jsx("svg", { viewBox: `0 0 ${vp.w} ${vp.h}`, preserveAspectRatio: "none", "aria-hidden": "true", fill: "none", style: {
                position: "absolute",
                left: 0,
                top: 0,
                width: "100%",
                height: "100%",
                display: "block",
                pointerEvents: "none",
                overflow: "visible",
                background: "transparent",
                backgroundColor: "transparent"
            }, children: scenes.map(([name, scene]) => SP_JSX.jsx("g", { opacity: scene.opacity ?? 1, children: (scene.items || []).map((item, idx) => renderItem(item, scene, `${name}:${idx}`, vp)) }, name)) }) });
}
function HUDOverlay() {
    const snap = useSnapshot();
    const scenes = SP_REACT.useMemo(() => Object.entries(snap.layers || {})
        .sort((a, b) => (a[1].z || 0) - (b[1].z || 0)), [snap]);
    const active = !!snap.connected && !!snap.visible && scenes.length > 0;
    if (!active)
        return null;
    return SP_JSX.jsx(HUDSurface, { scenes: scenes });
}
function Content() {
    const snap = useSnapshot();
    return SP_JSX.jsx(DFL.PanelSection, { title: "HUD Renderer", children: SP_JSX.jsx(DFL.PanelSectionRow, { children: SP_JSX.jsxs("div", { style: { padding: "8px 0", fontSize: 13 }, children: [SP_JSX.jsxs("div", { children: ["Core: ", snap.connected ? "connected" : "waiting"] }), SP_JSX.jsxs("div", { children: ["HUD2: v", snap.hudVersion || 2] }), SP_JSX.jsxs("div", { children: ["Layers: ", Object.keys(snap.layers || {}).length] }), SP_JSX.jsxs("div", { children: ["Scene version: ", snap.version || 0] })] }) }) });
}
var index = definePlugin(() => {
    const name = "JACoBHUDOverlay";
    transportStopped = false;
    void hudTransportLoop();
    routerHook.addGlobalComponent(name, HUDOverlay);
    return {
        name: "JACoB HUD",
        titleView: SP_JSX.jsx("div", { className: DFL.staticClasses.Title, children: "JACoB HUD Bridge" }),
        content: SP_JSX.jsx(Content, {}),
        icon: SP_JSX.jsx(FaCrosshairs, {}),
        alwaysRender: true,
        onDismount() {
            transportStopped = true;
            routerHook.removeGlobalComponent(name);
            publish({ hudVersion: 2, version: transportVersion, visible: false, layers: {}, connected: false });
        }
    };
});

export { index as default };
//# sourceMappingURL=index.js.map
