# JACoB Custom Tab Developer Reference

Version: Alpha 0.2.8  
SDK version: 7

This is the canonical reference for building JACoB custom HTML tabs. It describes the tab runtime, the browser SDK, the direct WebSocket protocol, state and event shapes, input behavior, overlays, video, recorder support, persistence, security boundaries, and platform limitations.

JACoB deliberately exposes primitives. Multi-step behavior, timers, state machines, route queues, HUD logic, and other tool-specific behavior belong in custom HTML tabs.

---

## 1. Smallest useful tab

A custom tab is ordinary self-contained HTML. JACoB injects the `Elite` JavaScript object into the tab at runtime.

```html
<!doctype html>
<html>
<head>
<meta charset="utf-8">
<style>
body { margin: 0; padding: 16px; background: #090a0b; color: #eee; font: 14px system-ui; }
button { padding: 8px 10px; }
</style>
</head>
<body>
<button id="galmap">Galaxy Map</button>
<pre id="out"></pre>
<script>
const out = document.querySelector('#out');

document.querySelector('#galmap').onclick = async () => {
  try {
    const result = await Elite.bindings.press('GalaxyMapOpen');
    out.textContent = JSON.stringify(result, null, 2);
  } catch (error) {
    out.textContent = `${error.code || 'ERROR'}: ${error.message}`;
  }
};

Elite.journal.subscribe('FSDJump', event => {
  out.textContent = `Jumped to ${event.StarSystem || '?'}`;
});
</script>
</body>
</html>
```

Paste or upload the file in **Tab Manager**, preview it, then save it. Saved tabs become top-level JACoB tabs and persist across restarts.

---

## 2. Runtime model

### 2.1 Sandbox

Saved and previewed tabs run in sandboxed iframes with script execution enabled. User-initiated file downloads are permitted so a tab can export its own data. Tabs do not receive normal same-origin access to the JACoB host page, arbitrary filesystem access, operating-system APIs, or direct access to JACoB's privileged WebSocket.

The host injects the `Elite` SDK into each tab and forwards only allowed SDK calls.

SDK 7 also injects a restrictive Content Security Policy into custom tabs. Direct external scripts, frames, fetch/WebSocket connections, workers, forms, and external image loads are blocked. Sensitive bridge capabilities prompt the user per saved-tab version/browser before first use: game control, public-network access, recorder, overlay, video, and derived vision. A code update invalidates old grants, denials are remembered to prevent prompt-spam loops, and grants can be cleared from Tab Manager with **Reset permissions**. High-frequency tab calls are bounded by the host bridge.

Custom tabs retain an opaque sandbox origin. They cannot open a direct browser WebSocket to JACoB because sandbox `Origin: null` connections are rejected by the core.

### 2.2 Lifetime

A saved tab remains loaded while JACoB is open, even when another tab is visible. This allows a hidden tab to keep journal subscriptions, timers, queue state, and overlay updates running.

Editing and saving a tab reloads that tab's iframe. Other saved tabs stay loaded.

Deleting a tab removes its persistent record and clears that tab's overlay layer.

### 2.3 Persistence

Saved tabs are stored by the JACoB core, not browser local storage. On Windows the default data directory is normally:

```text
%APPDATA%\JACoB
```

The tab database is:

```text
custom-tabs.json
```

A saved tab record contains:

```json
{
  "id": "tab-0123456789abcdef",
  "name": "Route Queue",
  "html": "<!doctype html>...",
  "createdAt": "2026-10-03T16:00:00Z",
  "updatedAt": "2026-10-03T16:10:00Z"
}
```

Maximum saved HTML size is 1 MiB per tab.

### 2.4 Overlay isolation

Each saved tab receives a separate overlay layer. `Elite.overlay.clear()` clears only the calling tab's layer. A preview in Tab Manager uses a temporary preview layer.

---

## 3. SDK overview

The injected object is:

```js
Elite
```

Current top-level namespaces:

```text
Elite.api
Elite.core
Elite.system
Elite.state
Elite.bindings
Elite.input
Elite.recorder
Elite.overlay
Elite.video
Elite.net
Elite.data
Elite.store
Elite.files
Elite.events
Elite.journal
```

The current SDK version is:

```js
Elite.api.version === 7
```

All command methods return Promises.

Errors reject with an `Error` object carrying a machine-readable `code` when one is available:

```js
try {
  await Elite.bindings.press('UI_Select');
} catch (error) {
  console.log(error.code);
  console.log(error.message);
}
```

---

## 4. Core and system health

### `Elite.core.ping()`

```js
const result = await Elite.core.ping();
```

Result:

```json
{
  "pong": true,
  "time": "2026-10-03T20:15:12.123456Z"
}
```

### `Elite.system.health()`

```js
const health = await Elite.system.health();
```

Result shape:

```json
{
  "status": "ok",
  "blockers": [],
  "warnings": [],
  "gameRunning": true,
  "devices": {
    "source": "windows-raw-input",
    "keyboardCount": 1,
    "mouseCount": 1,
    "hidCount": 0,
    "error": ""
  },
  "bindings": {}
}
```

`status` is one of:

```text
ok
warning
blocked
```

A blocked health result does not mean every JACoB feature is unusable. It means the current binding/device state has a condition that can make input automation unreliable or unsafe.

---

## 5. State and Elite event data

JACoB intentionally keeps Elite's journal and `Status.json` data close to the game's original structure. New game fields are passed through rather than normalized away.

### `Elite.state.get()`

```js
const snapshot = await Elite.state.get();
```

Result:

```json
{
  "journalDir": "C:\\Users\\name\\Saved Games\\Frontier Developments\\Elite Dangerous",
  "journalFile": "Journal.2026-10-03T160000.01.log",
  "status": {
    "timestamp": "2026-10-03T20:00:00Z",
    "event": "Status",
    "Flags": 0,
    "Latitude": 12.345,
    "Longitude": -67.89,
    "Heading": 142
  },
  "market": {
    "event": "Market",
    "MarketID": 3700005632,
    "StationName": "Fleet Carrier",
    "StarSystem": "Sol",
    "Items": []
  },
  "eliteFiles": {
    "cargo": {"event":"Cargo","Inventory":[]},
    "navRoute": {"event":"Route","Route":[]},
    "modulesInfo": {"event":"ModuleInfo","Modules":[]}
  },
  "eliteFileNames": {
    "cargo": "Cargo.json",
    "navRoute": "NavRoute.json",
    "modulesInfo": "ModulesInfo.json"
  },
  "eliteFileUpdated": {
    "cargo": "2026-10-04T10:15:00Z"
  },
  "lastJournalEvent": {
    "timestamp": "2026-10-03T19:59:58Z",
    "event": "FSDJump",
    "StarSystem": "Sol"
  },
  "journalContext": {
    "currentSystem": "Sol",
    "maxJumpRange": 65.43,
    "ship": "krait_light",
    "shipName": "Wayfarer",
    "shipIdent": "NH-01",
    "carrierCallsign": "ABC-123",
    "carrierFreeSpace": 8800,
    "carrierAvailableBalance": 123456789
  }
}
```

`status`, `market`, and `lastJournalEvent` may be empty when Elite has not produced those files or events yet. `market` remains the compatibility shortcut for the latest parsed `Market.json`. `journalContext` is recovered from the current journal when JACoB attaches and is maintained as new `Location`, `FSDJump`, `CarrierJump`, `Loadout`, and `CarrierStats` records arrive. Carrier context includes callsign/name plus capacity and available-balance fields when Elite has written them.

`eliteFiles` contains every valid JSON snapshot currently present in Elite's journal directory. JACoB provides stable names for the documented files: `status`, `market`, `outfitting`, `shipyard`, `modulesInfo`, `cargo`, `navRoute`, `backpack`, `shipLocker`, and `fcMaterials`. Any future JSON file written into that directory is exposed automatically with a normalized filename key. JACoB reads only the detected Elite journal directory; this interface cannot browse arbitrary filesystem paths.

### Elite companion-file access

List available snapshots:

```js
const index = await Elite.data.list();
console.log(index.files);
```

Read one snapshot by canonical name or filename:

```js
const cargo = await Elite.data.get('cargo');
const route = await Elite.data.get('NavRoute.json');

if (cargo.found) {
  console.log(cargo.data.Inventory);
}
```

Subscribe to one snapshot:

```js
const off = Elite.data.subscribe('navRoute', update => {
  if (update.available) console.log(update.data.Route);
});

off();
```

Subscribe to all companion-file changes:

```js
Elite.data.subscribe('*', update => {
  console.log(update.name, update.file, update.data);
});
```

Each change is also available through `Elite.events.subscribe('eliteFile', ...)`. The event payload contains `name`, `file`, `updated`, `available`, and `data` when available. `Status.json` and `Market.json` continue to emit their existing `status` and `market` compatibility events as well.

### Status subscription

```js
const unsubscribe = Elite.state.subscribe(status => {
  console.log(status.Latitude, status.Longitude, status.Heading);
});

unsubscribe();
```

Equivalent generic event name:

```js
Elite.events.subscribe('status', status => {});
```

JACoB checks `Status.json` approximately every 250 ms and emits a status event when the file modification time changes.

### Journal subscription

Specific event:

```js
const unsubscribe = Elite.journal.subscribe('FSDJump', event => {
  console.log(event.StarSystem);
});
```

All journal events:

```js
Elite.journal.subscribe('*', event => {
  console.log(event.event, event);
});
```

Journal objects are the parsed JSON objects written by Elite.

### Journal-session history

List journal session files inside the detected Elite journal directory:

```js
const index = await Elite.journal.files();
console.log(index.files);
```

Read parsed records from the current journal, or name one of the files returned above:

```js
const page = await Elite.journal.read({offset: 0, limit: 1000});
const jumps = await Elite.journal.read({
  file: index.files[0].file,
  event: 'FSDJump',
  offset: 0,
  limit: 500
});
```

`limit` defaults to 1,000 and is capped at 5,000 parsed events per request. Use `offset` and the returned `info.nextOffset` when `info.truncated` is true to page through longer sessions. Filenames must be journal basenames from the detected Elite journal directory; path traversal and arbitrary filesystem reads are rejected. Live subscriptions remain the preferred interface when a tab only needs new events.

### Generic event subscription

```js
Elite.events.subscribe('journal', event => {});
Elite.events.subscribe('status', status => {});
Elite.events.subscribe('recorder.input', input => {});

Elite.events.subscribe('*', (eventName, data) => {
  console.log(eventName, data);
});
```

Events currently relayed to tabs can include:

```text
core.hello
core.journalFile
state
status
market
eliteFile
journal
tabs.changed
appearance.changed
recorder.input
core.update
```

---

## 6. Elite bindings

### Binding action shape

```json
{
  "name": "UI_Select",
  "primary": {
    "device": "Keyboard",
    "key": "Key_Enter",
    "modifiers": []
  },
  "secondary": {
    "device": "Keyboard",
    "key": "Key_Numpad_1",
    "modifiers": [
      {"device":"Keyboard","key":"Key_LeftControl"}
    ]
  }
}
```

A slot can be empty or use a non-keyboard device.

### Binding execution rule

When JACoB executes a semantic Elite binding:

1. use an injectable keyboard **Secondary** binding if one exists;
2. otherwise use an injectable keyboard **Primary** binding;
3. otherwise return an error.

This rule applies to both press and hold.

### `Elite.bindings.list()`

```js
const result = await Elite.bindings.list();
```

Result includes:

```json
{
  "directory": "...",
  "activeFile": "...",
  "activeFiles": ["..."],
  "activeSource": "user",
  "files": [],
  "actions": [],
  "autoBind": {},
  "diagnostics": {}
}
```

### `Elite.bindings.get(name)`

```js
const action = await Elite.bindings.get('GalaxyMapOpen');
```

Returns one binding action object.

### `Elite.bindings.diagnostics()`

Returns the active preset diagnostics, including selector state, required devices, duplicate elements, parse errors, loader errors, and whether the source preset is read-only.

Representative shape:

```json
{
  "directory": "...",
  "activeFile": "...",
  "activeFiles": ["..."],
  "activeSource": "user",
  "selectorLegacy": [],
  "selectorOdyssey": ["Custom", "Custom", "Custom", "Custom"],
  "requiredDevices": ["Keyboard", "Mouse"],
  "duplicateElements": ["MouseGUI"],
  "parseError": "",
  "readOnly": false,
  "loader": {
    "path": "...BindingLoadingErrors.log",
    "relevant": true,
    "hasErrors": false,
    "presetFiles": [],
    "missingDevices": [],
    "duplicateBindings": [],
    "lines": []
  }
}
```

`MouseGUI` is treated as a known non-fatal duplicate because Elite itself uses the first entry.

### `Elite.bindings.reload()`

```js
await Elite.bindings.reload();
```

Reloads binding files from disk.

### Binding autofill is host-only in SDK 7

`bindings.autofill` still exists as a local host-console operation, but it is intentionally no longer exposed through the custom-tab SDK. A custom tab cannot modify Elite binding files.

The host-side autofill operation remains conservative:

- Elite must be closed;
- health preflight must have no blockers;
- the active preset is never edited in place;
- JACoB clones the current preset into a user preset;
- existing Primary bindings are preserved;
- existing Secondary bindings are preserved;
- a fallback Secondary keyboard chord is assigned only when both slots are empty;
- selector files are updated transactionally;
- failures restore the selector and remove incomplete clones.

Representative report:

```json
{
  "scannedActions": 220,
  "alreadyBound": 198,
  "existingFallback": 7,
  "assigned": 15,
  "sourceFiles": ["..."],
  "createdFiles": ["..."],
  "selectorBackups": ["..."],
  "assignments": [
    {"action":"SomeAction","key":"F9","modifiers":["CTRL","ALT"]}
  ],
  "clones": [
    {
      "sourceFile":"...",
      "sourcePreset":"KeyboardMouseOnly",
      "targetFile":"...",
      "targetPreset":"JACoB - KeyboardMouseOnly",
      "assigned":15
    }
  ]
}
```

### `Elite.bindings.press(action)`

```js
await Elite.bindings.press('UI_Select');
```

JACoB focuses Elite and sends one semantic binding press.

### `Elite.bindings.down(action)` / `Elite.bindings.up(action)`

```js
await Elite.bindings.down('UI_Right');
try {
  await new Promise(resolve => setTimeout(resolve, 1200));
} finally {
  await Elite.bindings.up('UI_Right');
}
```

These calls keep one semantic Elite binding physically held across client-side work. They are useful when a tab needs to preserve Elite's native key-repeat acceleration while remaining able to release the key immediately on cancellation. Always pair `down()` with `up()` in a `finally` block.

### `Elite.bindings.hold(action, durationMs)`

```js
await Elite.bindings.hold('MoveFreeCamRight', 240);
```

`durationMs` range: 20 to 10,000 ms. This legacy convenience call blocks until the hold completes; use `down()` / `up()` when the hold must be interruptible.

---

## 7. Raw keyboard input

Use raw input when the physical key itself matters more than the Elite action name.

### `Elite.input.tap(key, options)`

```js
await Elite.input.tap('D');
await Elite.input.tap('K', { modifiers: ['CTRL', 'SHIFT'] });
await Elite.input.tap('ENTER', { delayMs: 250 });
```

Options:

```json
{
  "delayMs": 0,
  "modifiers": []
}
```

`delayMs` range: 0 to 5,000 ms.

### `Elite.input.hold(key, durationMs, options)`

```js
await Elite.input.hold('D', 350);
await Elite.input.hold('F8', 800, { modifiers: ['CTRL'] });
```

`durationMs` range: 20 to 10,000 ms.

### Accepted normalized key names

Letters and digits:

```text
A through Z
0 through 9
```

Named keys:

```text
ENTER RETURN ESC ESCAPE TAB SPACE BACKSPACE
PAGEUP PAGEDOWN END HOME
UP DOWN LEFT RIGHT INSERT DELETE
CTRL CONTROL LEFTCONTROL RIGHTCONTROL
SHIFT LEFTSHIFT RIGHTSHIFT
ALT LEFTALT RIGHTALT
NUMPAD0 through NUMPAD9
MULTIPLY ADD SUBTRACT DECIMAL DIVIDE
SEMICOLON EQUALS COMMA MINUS PERIOD SLASH GRAVE
LEFTBRACKET BACKSLASH RIGHTBRACKET APOSTROPHE
F1 through F12
```

### `Elite.input.text(text, options)`

```js
await Elite.input.text('Hegoo SB-L b35-0', { intervalMs: 18 });
```

The built-in text mapper supports:

- A-Z / a-z
- 0-9
- space, newline, tab
- `- _ = + . , / ? \\ ; : ' " [ { ] }`
- `! @ # $ % ^ & * ( )`

Unsupported characters return an error instead of silently changing the text.

---

## 8. Input recorder

Recorder support is optional in Windows installation and may be unavailable on some platform builds.

### `Elite.recorder.status()`

```js
const status = await Elite.recorder.status();
```

Shape:

```json
{
  "available": true,
  "driver": "windows-low-level-keyboard-hook",
  "recording": false,
  "eventCount": 0,
  "scope": "Elite foreground only"
}
```

### `Elite.recorder.start()`

```js
await Elite.recorder.start();
```

The recorder only captures while explicitly armed. On Windows it records keyboard events while Elite is foreground. JACoB marks its own injected input and excludes it from recorder output while still allowing remote-input tools to be recorded.

### `Elite.recorder.subscribe(callback)`

```js
const off = Elite.recorder.subscribe(event => {
  console.log(event);
});
```

Event shape:

```json
{
  "pressId": 12,
  "type": "up",
  "key": "D",
  "modifiers": [],
  "atMs": 1835,
  "deltaMs": 87,
  "durationMs": 142,
  "isModifier": false,
  "matches": [
    {"action":"MoveFreeCamRight","slot":"primary"}
  ]
}
```

Fields:

- `pressId`: identifies one key-down/key-up pair.
- `type`: `down` or `up`.
- `key`: normalized raw key name.
- `modifiers`: modifiers active for the chord.
- `atMs`: milliseconds since recorder start.
- `deltaMs`: milliseconds since the preceding recorder event.
- `durationMs`: hold duration on the `up` event.
- `isModifier`: true for modifier-only transitions.
- `matches`: Elite binding actions matching the physical chord.

### `Elite.recorder.stop()`

```js
const result = await Elite.recorder.stop();
```

Result:

```json
{
  "status": {},
  "events": []
}
```

A common replay transformation is:

```text
wait before action = current key-down atMs - previous action key-up atMs
hold duration      = current key-up atMs - current key-down atMs
```

---

## 9. Overlay HUD API

### `Elite.overlay.info()`

```js
const info = await Elite.overlay.info();
```

Representative result:

```json
{
  "available": true,
  "driver": "windows-layered-window",
  "visible": true,
  "layers": 2,
  "width": 1920,
  "height": 1080,
  "supportsAlpha": false,
  "supportsText": true,
  "supportsShapes": ["text","line","polyline","polygon","rect","circle"],
  "coordinateSpaces": ["normalized","pixels"],
  "note": ""
}
```

### `Elite.overlay.set(scene)`

```js
await Elite.overlay.set({
  z: 20,
  space: 'normalized',
  items: [
    {
      type: 'text',
      x: 0.5,
      y: 0.08,
      text: 'LAP 2 / 5',
      color: '#ff7b00',
      fontSize: 28,
      align: 'center'
    },
    {
      type: 'polyline',
      points: [
        {x:0.10,y:0.80},
        {x:0.20,y:0.70},
        {x:0.30,y:0.82}
      ],
      stroke: '#ff7b00',
      lineWidth: 3
    },
    {
      type: 'circle',
      x: 0.20,
      y: 0.70,
      r: 0.008,
      fill: '#ffffff'
    }
  ]
});
```

### `Elite.overlay.clear()`

```js
await Elite.overlay.clear();
```

Clears the calling tab's layer only.

### Scene fields

```json
{
  "z": 0,
  "space": "normalized",
  "width": 0,
  "height": 0,
  "items": []
}
```

`space`:

- `normalized`: coordinates are relative to the Elite client area; X/W use overlay width and Y/H use overlay height. Circle radius is relative to the smaller overlay dimension.
- `pixels`: coordinates are direct overlay pixels.

Supported item types:

```text
text
line
polyline
polygon
rect
circle
```

Common item fields:

```text
x y x2 y2 w h r
points[]
text
color
stroke
fill
lineWidth
fontSize
align
```

Color formats:

```text
#RGB
#RRGGBB
#RRGGBBAA
transparent
none
```

Limits and defaults:

- maximum 2,000 items per scene;
- maximum 10,000 points in one polyline/polygon;
- default `space`: `normalized`;
- default `lineWidth`: 2;
- maximum `lineWidth`: 64;
- default `fontSize`: 18;
- maximum `fontSize`: 256;
- default `align`: `left`;
- `align`: `left`, `center`, or `right`.

### Racing/minimap pattern

A racing tab can keep track control points in the HTML, subscribe to Status.json, project Elite latitude/longitude into a local track plane, and render a polyline plus ship marker.

```js
const track = [
  {lat:12.3450, lon:-67.8900},
  {lat:12.3454, lon:-67.8892},
  {lat:12.3460, lon:-67.8887}
];

Elite.state.subscribe(status => {
  if (typeof status.Latitude !== 'number' || typeof status.Longitude !== 'number') return;

  const scene = buildTrackScene(track, status.Latitude, status.Longitude, status.Heading);
  Elite.overlay.set(scene).catch(console.error);
});
```

### Windows notes

The Windows overlay is a topmost, non-activating, click-through layered window aligned to the Elite client area. It automatically hides when Elite is not foreground.

The compatibility renderer does not provide full blended per-pixel alpha on every supported Windows configuration. Basic text and shapes are the intended baseline.

### Steam Deck / Linux notes

The Linux overlay uses an X11/XWayland ARGB window and the Gamescope external-overlay path when available. If the required display path is unavailable, `Elite.overlay.info()` reports `available:false` rather than disabling the rest of JACoB.

---

## 10. Video preview

### `Elite.video.info()`

```js
const info = await Elite.video.info();
```

Result:

```json
{
  "available": true,
  "driver": "windows-gdi"
}
```

### `Elite.video.url(options)`

```js
const url = await Elite.video.url({
  width: 960,
  fps: 5,
  quality: 60
});
```

The returned URL is already prepared for the current local/paired browser.

### `Elite.video.attach(element, options)`

```html
<img id="game" alt="Elite preview">
```

```js
await Elite.video.attach(document.querySelector('#game'), {
  width: 960,
  fps: 5,
  quality: 60
});
```

Current MJPEG limits:

- width: 160 to 1920;
- FPS: 1 to 15;
- JPEG quality: 20 to 95.

This preview is for utility/remote-control views, not high-frame-rate game streaming.

On Windows the capture source is the verified Elite client window itself. If Elite is missing, hidden, minimized, or not foreground, JACoB returns a freshly generated black frame. It does not fall back to capturing the desktop rectangle.

### `Elite.vision.info()`

```js
const info = await Elite.vision.info();
```

Derived vision is processed inside the JACoB core. Custom tabs never receive raw JPEG bytes, pixel buffers, OCR output, or screenshots through this API.

### `Elite.vision.sample(options)`

```js
const sample = await Elite.vision.sample({ width: 320 });
```

A sample returns bounded, derived telemetry such as:

```json
{
  "available": true,
  "blanked": false,
  "width": 320,
  "height": 180,
  "featureCount": 87,
  "features": [
    { "x": 0.42, "y": 0.31, "score": 0.71 }
  ],
  "meanLuma": 0.18,
  "contrast": 0.12,
  "motion": {
    "dx": 0.012,
    "dy": -0.004,
    "confidence": 0.64,
    "compared": true
  }
}
```

Feature coordinates and motion are normalized to the frame dimensions. The motion estimate is deliberately coarse and intended for experimentation such as asteroid-field tracking; it is not a substitute for game telemetry. If Elite loses focus, the sample reports `blanked:true`, resets temporal tracking state, and returns no features.

---

## 10A. File exports and public web APIs

### `Elite.files.download(name, data, options)`

Custom tabs can hand a file to the browser download system. Downloads remain user-browser files; JACoB does not grant the tab general filesystem access.

```js
Elite.files.download('notes.txt', 'Survey complete.');
```

For JSON:

```js
Elite.files.json('track.jacobtrack.json', trackObject);
```

`Elite.files.download()` accepts strings, `Blob` objects, or ordinary JavaScript values. Non-string values are serialized as JSON. The optional `options.type` sets the MIME type and `options.compact` removes JSON indentation.

Saved and preview tabs run with the browser sandbox permission required for user-initiated downloads. The sandbox still withholds general filesystem access.

### `Elite.net.fetch(url, options)`

JACoB can make a bounded outbound API request on behalf of a custom tab. This avoids browser CORS restrictions that otherwise make many public APIs unreliable from sandboxed tabs.

```js
const result = await Elite.net.fetch(
  'https://spansh.co.uk/api/systems/field_values/system_names?q=Sol'
);

console.log(result.status);
console.log(result.json ?? result.body);
```

POST example:

```js
const result = await Elite.net.fetch('https://example.org/api/query', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Accept': 'application/json'
  },
  body: { system: 'Sol' }
});
```

Result shape:

```json
{
  "url": "https://example.org/api/query",
  "status": 200,
  "statusText": "200 OK",
  "headers": {"Content-Type":"application/json"},
  "contentType": "application/json",
  "body": "{...}",
  "json": {},
  "bytes": 1234
}
```

Network limits:

- `GET` and `POST` only;
- `http://` and `https://` only;
- loopback, LAN/private, link-local and local-name targets are blocked;
- non-standard ports are blocked;
- redirects are rechecked and limited;
- request bodies are limited to 1 MiB;
- responses are limited to 8 MiB;
- requests time out;
- JACoB supplies its own `User-Agent`;
- accepted request headers are limited to `Accept`, `Content-Type`, `Authorization`, `X-API-Key`, `If-None-Match`, and `If-Modified-Since`.

A custom tab with `net.fetch` can transmit information to a public service. Install tabs from sources you trust and inspect tabs that request credentials or send journal/state data away from the computer.

### `Elite.store`

Saved custom tabs receive a small persistent JSON store scoped to that saved tab. The host supplies the tab identity; one tab cannot select another tab's storage namespace.

```js
const manifest = await Elite.store.get('manifest', []);
await Elite.store.set('manifest', manifest);
await Elite.store.delete('manifest');
await Elite.store.clear();
```

`get(key, fallback)` returns the fallback when the key is absent. Values must be JSON-serializable. A single value is limited to 512 KiB and one saved tab is limited to 2 MiB total. Preview tabs do not receive persistent storage; save the tab first when testing persistence. Removing a saved tab also removes its stored state.

---

## 11. Timing and sequencing patterns

JACoB does not include a procedure engine. Build sequences inside the custom tab.

### Sleep helper

```js
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
```

### Timed sequence

```js
await Elite.bindings.press('GalaxyMapOpen');
await sleep(1200);
await Elite.bindings.press('UI_Up');
await sleep(150);
await Elite.bindings.press('UI_Select');
```

### Wait for a journal event

```js
function waitForJournal(eventName, timeoutMs = 120000) {
  return new Promise((resolve, reject) => {
    let timer;
    const off = Elite.journal.subscribe(eventName, event => {
      clearTimeout(timer);
      off();
      resolve(event);
    });
    timer = setTimeout(() => {
      off();
      reject(new Error(`Timed out waiting for ${eventName}`));
    }, timeoutMs);
  });
}

await waitForJournal('FSDJump');
```

### Wait for a Status.json condition

```js
function waitForStatus(test, timeoutMs = 10000) {
  return new Promise((resolve, reject) => {
    let timer;
    const off = Elite.state.subscribe(status => {
      if (!test(status)) return;
      clearTimeout(timer);
      off();
      resolve(status);
    });
    timer = setTimeout(() => {
      off();
      reject(new Error('Status wait timed out'));
    }, timeoutMs);
  });
}
```

### Cancellation token pattern

```js
let runId = 0;

async function run() {
  const token = ++runId;
  await Elite.bindings.press('UI_Up');
  await sleep(250);
  if (token !== runId) return;
  await Elite.bindings.press('UI_Select');
}

function stop() {
  runId++;
}
```

---

## 12. Appearance/theme files

A JACoB shell theme is an HTML file uploaded under **Settings → Appearance**.

Theme files can contain CSS:

```html
<style>
:root { --accent: #7fd5ff; }
body { font-family: monospace; }
.card { border-color: #7fd5ff; }
</style>
```

A theme can replace shell slots with `<template>` elements:

```html
<template data-jacob-slot="brand">
  <strong>MY BRIDGE</strong><span>Elite tools</span>
</template>

<template data-jacob-slot="header-extra">...</template>
<template data-jacob-slot="nav-extra">...</template>
<template data-jacob-slot="home-extra">...</template>
<template data-jacob-slot="footer">...</template>
```

Optional body class:

```html
<meta name="jacob-body-class" content="compact-theme">
```

All host CSS selectors can be overridden. Theme scripts are not executed in the privileged host page. Interactive behavior belongs in custom tabs.

The selected theme is persisted by the core and is shared by browsers opening the same JACoB instance.

---

## 13. Direct WebSocket protocol

Most custom tabs should use the injected SDK. The direct protocol is documented for external clients and debugging.

Endpoint:

```text
ws://HOST:4510/ws
```

Loopback clients do not require a pairing token. LAN clients use:

```text
ws://HOST:4510/ws?token=PAIR_TOKEN
```

### Request

```json
{
  "type": "request",
  "id": "req-1",
  "method": "binding.press",
  "params": {
    "action": "UI_Select"
  }
}
```

### Success response

```json
{
  "type": "response",
  "id": "req-1",
  "ok": true,
  "result": {}
}
```

### Error response

```json
{
  "type": "response",
  "id": "req-1",
  "ok": false,
  "error": {
    "code": "BINDING_NOT_FOUND",
    "message": "..."
  }
}
```

### Event

```json
{
  "type": "event",
  "event": "journal",
  "data": {
    "event": "FSDJump",
    "StarSystem": "Sol"
  }
}
```

### Core methods

The core currently recognizes:

```text
core.ping
core.shutdown
state.get
system.health
update.check
update.install
appearance.get
appearance.save
appearance.reset
tabs.list
tabs.layout.save
tabs.get
tabs.save
tabs.delete
bindings.list
bindings.diagnostics
bindings.autofill
bindings.reload
bindings.get
binding.press
binding.down
binding.up
binding.hold
input.tap
input.hold
input.text
recorder.status
recorder.start
recorder.stop
overlay.info
overlay.set
overlay.clear
video.info
net.fetch
elitefiles.list
elitefiles.get
journal.files
journal.read
tabstate.get
tabstate.set
tabstate.delete
tabstate.clear
```

`core.shutdown`, `update.check`, `update.install`, tab-layout storage, and appearance storage are host/UI concerns. `update.install` is additionally limited to a loopback browser.

`core.shutdown` is host-UI only and is accepted only from a loopback browser. It is not exposed through the custom-tab SDK.

The sandbox SDK exposes the game-facing primitives, `net.fetch`, read-only Elite companion-file access through `Elite.data`, and the saved-tab-scoped `Elite.store` interface. Navigation layout, update installation, shutdown, and appearance management remain host/UI concerns. `Elite.files` is implemented inside the injected SDK and does not require a WebSocket method.

---

## 14. HTTP media endpoints

Snapshot:

```text
GET /api/video/frame.jpg?width=960&quality=60
```

MJPEG:

```text
GET /api/video.mjpeg?width=960&fps=5&quality=60
```

LAN requests must include the pairing token query parameter.

---

## 15. Core hello structure

When a browser connects, the host receives `core.hello`. Custom tabs receive it through the generic event bridge.

Representative shape:

```json
{
  "prototype": "Alpha 0.2.8",
  "version": "0.2.8-alpha",
  "product": "JACoB",
  "name": "Journal Aligned Control Bridge",
  "apiVersion": 7,
  "os": "windows",
  "arch": "amd64",
  "goRuntime": "go1.x",
  "host": "PC-NAME",
  "uptimeSeconds": 120,
  "journalDir": "...",
  "bindingsDir": "...",
  "bindingsFile": "...",
  "bindingsFiles": ["..."],
  "bindingsSource": "user",
  "bindingsCount": 220,
  "input": {
    "enabled": true,
    "available": true,
    "driver": "windows-sendinput-scancode"
  },
  "recorder": {},
  "capture": {},
  "overlay": {},
  "lan": {},
  "health": {},
  "appearance": {},
  "customTabs": {}
}
```

---

## 16. Common error codes

The exact set can grow, but tabs should be prepared for at least:

```text
OFFLINE
TIMEOUT
SDK_DENIED
BAD_MESSAGE
BAD_JSON
BAD_PARAMS
NO_SUCH_METHOD
INPUT_DISABLED
INPUT_UNAVAILABLE
INPUT_FAILED
GAME_FOCUS_FAILED
TEXT_INPUT_FAILED
BINDING_NOT_FOUND
BINDING_PRESS_FAILED
BINDING_HOLD_FAILED
BINDINGS_RELOAD_FAILED
BINDINGS_AUTOFILL_FAILED
BINDINGS_PREFLIGHT_FAILED
GAME_RUNNING
RECORDER_UNAVAILABLE
RECORDER_START_FAILED
RECORDER_STOP_FAILED
OVERLAY_UNAVAILABLE
OVERLAY_SCENE_INVALID
OVERLAY_RENDER_FAILED
OVERLAY_CLEAR_FAILED
TAB_STORE_UNAVAILABLE
TAB_NOT_FOUND
TAB_SAVE_FAILED
TAB_DELETE_FAILED
APPEARANCE_UNAVAILABLE
APPEARANCE_SAVE_FAILED
APPEARANCE_RESET_FAILED
```

Do not depend on error message wording. Use `error.code` for branching.

---

## 17. Security and trust boundaries

Custom tabs:

- run in sandboxed iframes;
- do not receive direct filesystem access;
- do not receive direct native API access;
- do not connect directly to the privileged core WebSocket;
- can invoke only the SDK methods forwarded by the host;
- receive their own overlay layer;
- can continue running while hidden.

Theme files:

- can override host CSS;
- can replace documented shell template slots;
- do not execute theme scripts in the privileged host page.

Recorder:

- is optional on Windows install;
- starts only on explicit request;
- stops when requested or when the controlling browser disconnects;
- records only in its documented scope.

LAN clients require the pairing token for WebSocket authentication. After pairing, JACoB supplies a separate media-only token for video URLs so custom tabs granted Video access never receive the control-capable pairing credential.

---

### External API boundary

`Elite.data` is read-only and is restricted to JSON snapshots inside the detected Elite journal directory. `Elite.journal.files()` and `Elite.journal.read()` are similarly restricted to journal basenames from that directory and cap each historical read. Unknown future JSON filenames are exposed automatically, but custom tabs cannot supply filesystem paths.

`Elite.net.fetch` is an outbound public-network bridge and requires the tab's Network permission. JACoB rejects loopback, private/LAN, link-local and local-name destinations so a custom tab cannot use the bridge to probe services on the host or local network. Public API access still gives the tab a route to transmit data off the computer; treat third-party tabs accordingly.

## 18. Platform capability summary

### Windows

Expected capabilities:

```text
journal/status       yes
bindings             yes
semantic input       yes
raw input            yes
text input           yes
recorder              optional install component
game preview         yes, compatibility capture path
overlay               yes, native click-through window
LAN web UI           yes
```

### Steam Deck / Linux

Expected capabilities:

```text
journal/status       yes
bindings             yes
semantic input       /dev/uinput path when available
automation tabs      same HTML/SDK model
game preview         platform-dependent
recorder              currently reports unavailable when scoped capture is not supported
overlay               Gamescope/XWayland path when available
LAN web UI           yes
```

Tabs should query capability methods instead of assuming every platform exposes every optional feature:

```js
const overlay = await Elite.overlay.info();
if (overlay.available) {
  // enable HUD controls
}

const recorder = await Elite.recorder.status();
if (!recorder.available) {
  // hide recorder-specific controls
}
```

---

## 19. JSON schemas

The release package contains machine-readable schemas under:

```text
docs/schemas/
```

### Binding action

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "JACoB binding action",
  "type": "object",
  "required": ["name", "primary", "secondary"],
  "properties": {
    "name": {"type":"string"},
    "primary": {"$ref":"#/$defs/slot"},
    "secondary": {"$ref":"#/$defs/slot"}
  },
  "$defs": {
    "slot": {
      "type":"object",
      "required":["device","key"],
      "properties": {
        "device":{"type":"string"},
        "key":{"type":"string"},
        "modifiers":{
          "type":"array",
          "items":{
            "type":"object",
            "required":["device","key"],
            "properties":{"device":{"type":"string"},"key":{"type":"string"}}
          }
        }
      }
    }
  }
}
```

### State snapshot

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "JACoB State Snapshot",
  "type": "object",
  "required": ["journalDir", "journalFile", "status", "lastJournalEvent"],
  "properties": {
    "journalDir": {"type":"string"},
    "journalFile": {"type":"string"},
    "status": {"type":["object","null"],"additionalProperties":true},
    "lastJournalEvent": {"type":["object","null"],"additionalProperties":true}
  },
  "additionalProperties": true
}
```

### Overlay scene

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "JACoB Overlay Scene",
  "type": "object",
  "required": ["items"],
  "properties": {
    "z": {"type":"integer"},
    "space": {"enum":["normalized","pixels"],"default":"normalized"},
    "width": {"type":"integer","minimum":0},
    "height": {"type":"integer","minimum":0},
    "items": {
      "type":"array",
      "maxItems":2000,
      "items": {
        "type":"object",
        "required":["type"],
        "properties": {
          "type":{"enum":["text","line","polyline","polygon","rect","circle"]},
          "x":{"type":"number"}, "y":{"type":"number"},
          "x2":{"type":"number"}, "y2":{"type":"number"},
          "w":{"type":"number"}, "h":{"type":"number"}, "r":{"type":"number"},
          "text":{"type":"string"},
          "points":{"type":"array","maxItems":10000,"items":{"type":"object","required":["x","y"],"properties":{"x":{"type":"number"},"y":{"type":"number"}}}},
          "color":{"type":"string"}, "stroke":{"type":"string"}, "fill":{"type":"string"},
          "lineWidth":{"type":"number","exclusiveMinimum":0,"maximum":64},
          "fontSize":{"type":"number","exclusiveMinimum":0,"maximum":256},
          "align":{"enum":["left","center","right"]}
        }
      }
    }
  }
}
```

### Health object

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "JACoB system health",
  "type": "object",
  "required": ["status", "blockers", "warnings", "devices", "bindings", "gameRunning"],
  "properties": {
    "status": {"type":"string","enum":["ok","warning","blocked"]},
    "blockers": {"type":"array","items":{"type":"string"}},
    "warnings": {"type":"array","items":{"type":"string"}},
    "gameRunning": {"type":"boolean"},
    "devices": {
      "type":"object",
      "required":["source","keyboardCount","mouseCount"],
      "properties": {
        "source":{"type":"string"},
        "keyboardCount":{"type":"integer","minimum":0},
        "mouseCount":{"type":"integer","minimum":0},
        "hidCount":{"type":"integer","minimum":0},
        "error":{"type":"string"}
      },
      "additionalProperties":true
    },
    "bindings": {"type":"object"}
  },
  "additionalProperties":true
}
```

### WebSocket envelope

```json
{
  "request": {
    "type": "request",
    "id": "string",
    "method": "string",
    "params": {}
  },
  "response": {
    "type": "response",
    "id": "string",
    "ok": true,
    "result": {}
  },
  "errorResponse": {
    "type": "response",
    "id": "string",
    "ok": false,
    "error": {"code":"string","message":"string"}
  },
  "event": {
    "type": "event",
    "event": "string",
    "data": {}
  }
}
```

For exact machine-readable schemas, use the files in `docs/schemas/` shipped with the same release.

---

## 20. Complete custom-tab starter

```html
<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>
body {
  margin: 0;
  padding: 14px;
  background: #070809;
  color: #e8e3dc;
  font: 14px system-ui;
}
button, input {
  font: inherit;
  color: inherit;
  background: #111315;
  border: 1px solid #8a430e;
  border-radius: 0;
  padding: 8px 10px;
}
pre {
  white-space: pre-wrap;
  background: #050607;
  border: 1px solid #2a2d30;
  padding: 10px;
}
</style>
</head>
<body>

<button id="action">Run action</button>
<button id="clear">Clear HUD</button>
<pre id="log">Ready.</pre>

<script>
const log = document.querySelector('#log');
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
let stopped = false;

function write(value) {
  log.textContent = typeof value === 'string' ? value : JSON.stringify(value, null, 2);
}

document.querySelector('#action').onclick = async () => {
  stopped = false;
  try {
    const health = await Elite.system.health();
    if (health.status === 'blocked') throw new Error(health.blockers.join('; '));

    await Elite.bindings.press('UI_Up');
    await sleep(180);
    if (stopped) return;
    await Elite.bindings.press('UI_Select');
    write('Done');
  } catch (error) {
    write(`${error.code || 'ERROR'}: ${error.message}`);
  }
};

document.querySelector('#clear').onclick = async () => {
  stopped = true;
  await Elite.overlay.clear();
};

Elite.journal.subscribe('FSDJump', event => {
  write(`FSDJump → ${event.StarSystem || '?'}`);
});

Elite.state.subscribe(status => {
  if (typeof status.Latitude !== 'number') return;

  Elite.overlay.set({
    space: 'normalized',
    items: [
      {
        type: 'text',
        x: 0.03,
        y: 0.08,
        text: `HDG ${Math.round(status.Heading || 0)}`,
        color: '#ff7b00',
        fontSize: 20
      }
    ]
  }).catch(() => {});
});
</script>
</body>
</html>
```

---

## 21. Design guidance for durable tabs

- Prefer semantic Elite bindings over raw keys when the action name is known.
- Use raw keys only when the physical key itself is part of the interaction.
- Query optional capabilities before showing controls that depend on them.
- Keep long-running sequences cancellable.
- Add timeouts to waits for journal/status conditions.
- Keep overlay scenes small and update only when useful.
- Clear the overlay when a tool is stopped or disabled.
- Treat journal and Status.json objects as extensible; ignore fields you do not use.
- Do not depend on exact human-readable error messages.
- Keep platform-specific assumptions out of the tab unless the tab is intentionally platform-specific.

This document is intended to be sufficient for implementing JACoB custom tabs without reading the JACoB source tree.
