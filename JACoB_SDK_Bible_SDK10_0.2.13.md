# JACoB SDK Bible

**Custom tab developer reference**  
**Alpha 0.2.13 | SDK / Protocol 10**

The practical reference for building JACoB tabs. Every public SDK method includes a worked example, plus the event shapes, Elite data, permissions, limits, and patterns you need to build tools that behave well.

Journal Aligned Control Bridge • 10 October 2026  
Source baseline: `pseudo6626/JACoB` `main` @ `54a6ef432bcedcb8c7f62be0ccdb8847a2aff942`

<span id="contents"></span>

# Contents

Use the contents or the SDK index to jump straight to what you need. The PDF bookmarks follow the same major sections.

> **First tab?** Read sections 1-4 once, build the starter in section 2, then use section 6 as your day-to-day reference. If you already know JACoB, the SDK index is the fastest way in.

1.  [1. How JACoB tabs work](#architecture)
2.  [2. Build a tab in five minutes](#starter)
3.  [3. Requests, responses, and events](#message-model)
4.  [4. Permissions, sandbox, and limits](#security-model)
5.  [5. SDK index](#quick-index)
6.  [6. SDK reference](#method-reference)
7.  [Elite.api](#ns-elite-api)
8.  [Elite.core](#ns-elite-core)
9.  [Elite.system](#ns-elite-system)
10. [Elite.state](#ns-elite-state)
11. [Elite.data](#ns-elite-data)
12. [Elite.journal](#ns-elite-journal)
13. [Elite.events](#ns-elite-events)
14. [Elite.bindings](#ns-elite-bindings)
15. [Elite.input](#ns-elite-input)
16. [Elite.recorder](#ns-elite-recorder)
17. [Elite.overlay](#ns-elite-overlay)
18. [Elite.video](#ns-elite-video)
19. [Elite.vision](#ns-elite-vision)
20. [Elite.net](#ns-elite-net)
21. [Elite.store](#ns-elite-store)
22. [Elite.actions](#ns-elite-actions)
23. [Elite.tabs](#ns-elite-tabs)
24. [Elite.locale](#ns-elite-locale)
25. [Elite.files](#ns-elite-files)
26. [7. Elite data available to tabs](#data-reference)
27. [8. HUD overlay scenes](#overlay-reference)
28. [9. Automation patterns that hold up](#patterns)
29. [10. Direct WebSocket and host protocol](#direct-protocol)
30. [11. Error handling](#errors)
31. [12. Platform support](#platforms)
32. [13. Full tab examples](#complete-examples)
33. [14. Glossary](#glossary)

<span id="architecture"></span>

## 1. How JACoB tabs work <a href="#contents" class="toplink">Back to contents</a>

A JACoB tab is a self-contained HTML page that runs inside a sandboxed iframe. Use HTML for structure, CSS for the interface, and JavaScript for the tool logic. JACoB injects one controlled global object, `Elite`. That object is the public SDK your tab uses to read Elite data and ask the core to do privileged work.

**Your tab**  
HTML + CSS + JavaScript  
Mining, routing, exploration, carrier tools, Touch Deck, race HUDs…↓ Elite.\* SDK requests / subscriptions**JACoB host + Go core**  
Sandbox bridge • WebSocket • journal/status watchers • binding resolver • native input • overlay • capture • persistence • action broker • network proxy↕**Elite Dangerous + operating system**  
Journal files • Status.json • companion JSON •.binds XML • keyboard/input APIs • native windows

> **Keep the split simple:** the Go core owns trusted capabilities. Your tab owns the workflow. Put sequencing, timers, state machines, route logic, statistics, and game-specific decisions in the tab.

### Sandbox boundaries

- Tabs cannot read arbitrary files on the computer.
- Tabs cannot call Windows, Linux, or other native OS APIs directly.
- Tabs do not connect directly to the privileged core WebSocket.
- Normal browser networking from the iframe is blocked by the tab CSP. Use `Elite.net.fetch()` for public HTTP/HTTPS requests.
- Each saved tab gets its own HUD layer and persistent-state namespace.
- Saved tabs stay loaded when you switch away from them. Subscriptions, timers, and background logic can keep running.

### What SDK 10 adds

SDK 10 turns Vision into a normal tab-facing capability. Tabs can inspect a normalized region of the verified Elite client frame, run derived measurements or OCR, and use one host-owned calibration flow. Raw captured pixels still stay inside JACoB. SDK 9 cross-tab actions and tab navigation remain part of the contract.

- Use `Elite.vision.inspect()` for color coverage/presence, vertical fill, luma, contrast, edge density, OCR text, or structured OCR lines and words.
- Use `Elite.vision.configure()` when a tool needs a saved region or sampled color. JACoB owns the drag-to-select/color-pick UI and stores the result in the calling tab's state.
- Use `Elite.vision.debug.show()` while developing or troubleshooting a calibrated region. Clear it with `Elite.vision.debug.clear()`.
- Vision, OCR, calibration, and Game View all use the same canonical Elite-client frame. Moving the Elite window on the desktop does not change normalized Vision coordinates.
- The No Capture build deliberately removes capture, Vision, OCR, Vision calibration, and the keyboard recorder.

### Physical keyboard IDs (SDK 8+)

Physical key IDs such as `SC:29` and `SC:E0:38` are layout-independent. Windows recorder events also include richer physical-key identity. Use `Elite.input.text()` for text a person would type. Use physical tokens when you need the same physical key position across keyboard layouts.

### Saved-tab storage in 0.2.11

JACoB 0.2.11 uses the schema-3 tab store. `custom-tabs.json` keeps tab metadata and navigation state; the HTML body for each saved tab lives separately under `custom-tabs/<tab-id>.html`. `tabs.list` therefore returns metadata without hauling every tab body across the bridge, and JACoB loads HTML only when it needs that tab.

- Maximum saved HTML size: 4 MiB per tab.
- Existing schema-2 stores with inline HTML migrate without changing tab IDs or `Elite.store` state.
- 0.2.11 can recover surviving schema-3 HTML bodies that the 0.2.10 regression left orphaned from the manifest.
- Hiding a tab does not delete it. An already-loaded hidden tab can keep subscriptions, timers, overlays, and published actions running.
- **Tab Manager** remains visible as the recovery page even when other default or custom navigation items are hidden.

<span id="starter"></span>

## 2. Build a tab in five minutes <a href="#contents" class="toplink">Back to contents</a>

This is a complete minimal tab. Paste it into Tab Manager, preview it, save it, and open the saved tab. If this works, the basic tab → host → core path is working.

    <!doctype html>
    <html>
    <head>
      <meta charset="utf-8">
      <style>
        body { margin:0; padding:16px; background:#090a0b; color:#eee; font:14px system-ui; }
        button { padding:8px 10px; margin-right:6px; }
        pre { white-space:pre-wrap; }
      </style>
    </head>
    <body>
      <button id="map">Open Galaxy Map</button>
      <button id="state">Read state</button>
      <pre id="out">Ready.</pre>
      <script>
        const out = document.querySelector('#out');

        document.querySelector('#map').onclick = async () => {
          try {
            await Elite.bindings.press('GalaxyMapOpen');
            out.textContent = 'Galaxy Map command sent.';
          } catch (err) {
            out.textContent = `${err.code || 'ERROR'}: ${err.message}`;
          }
        };

        document.querySelector('#state').onclick = async () => {
          const s = await Elite.state.get();
          out.textContent = JSON.stringify(s.journalContext, null, 2);
        };

        Elite.journal.subscribe('FSDJump', event => {
          out.textContent = `Jumped to ${event.StarSystem || '?'}`;
        });
      </script>
    </body>
    </html>

> **Good default:** use `Elite.bindings.*` when Elite already has an action name. Use `Elite.input.text()` for text. Drop to raw key tap/hold only when you actually care which physical key gets pressed.

<span id="message-model"></span>

## 3. Requests, responses, and events <a href="#contents" class="toplink">Back to contents</a>

### Requests return Promises

Most SDK commands return a JavaScript `Promise`. Use `await` when the next step depends on the result. Failed requests reject with an `Error` that includes a machine-readable `code`.

    try {
      const result = await Elite.bindings.press('UI_Select');
      console.log(result);
    } catch (err) {
      if (err.code === 'PERMISSION_DENIED') {
        // The user declined this permission for this tab/browser.
      } else {
        console.error(err.code, err.message);
      }
    }

### Subscriptions

Subscriptions register a JavaScript callback for events JACoB already emits. Each subscription returns an unsubscribe function. Keep it if the tool can start, stop, or re-arm more than once.

    const off = Elite.journal.subscribe('FSDJump', event => {
      console.log(event.StarSystem);
    });

    // Stop the subscription when you are done:
    off();

### What happens when a tab calls the SDK

Under the hood, the iframe sends a structured request to the trusted host page. The host checks the SDK allowlist, permission grants, rate limits, and the calling tab ID, then forwards a JSON request over its privileged WebSocket to the Go core. The result comes back through the same path. Normal tabs should stay on `Elite.*`; the direct protocol is documented later for debugging and external clients.

    // The underlying request is conceptually:
    {
      "type": "request",
      "id": "req-1",
      "method": "binding.press",
      "params": { "action": "UI_Select" }
    }

    // Success:
    { "type":"response", "id":"req-1", "ok":true, "result":{} }

    // Failure:
    { "type":"response", "id":"req-1", "ok":false,
      "error": { "code":"BINDING_NOT_FOUND", "message":"..." } }

### Cross-tab actions are brokered

SDK 9 cross-tab calls use the trusted host as a broker. A provider registers a named handler; a caller invokes that name and waits for the handler result. The caller never receives the provider iframe, JavaScript scope, or DOM. If the provider reloads or disappears during an invocation, JACoB rejects the pending call instead of leaving it hung.

    // Provider
    await Elite.actions.register('carrier.runLoad', async payload => {
      return runLoad(payload);
    });

    // Caller
    const result = await Elite.actions.invoke('carrier.runLoad', { load: 0 });

<span id="security-model"></span>

## 4. Permissions, sandbox, and limits <a href="#contents" class="toplink">Back to contents</a>

Capabilities that can control Elite, access the public network, record keys, use native display/capture features, or invoke another tab require a per-tab permission grant in that browser. When saved tab code changes, JACoB changes its permission version so updated code does not quietly inherit every old grant. A denial is remembered too, which prevents a broken tab from prompt-spamming the user.

| Capability        | Triggered by                                         | What the user is granting                                                                                                                                           |
|-------------------|------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Control Elite     | `Elite.input.*`, `Elite.bindings.press/down/up/hold` | Allows the tab to send keyboard/control input to the Elite window.                                                                                                  |
| Network           | `Elite.net.fetch()`                                  | Allows the tab to call public HTTP/HTTPS services through JACoB.                                                                                                    |
| Recorder          | `Elite.recorder.start/stop()`                        | Allows the tab to record physical keyboard input while Elite is foreground.                                                                                         |
| Overlay           | `Elite.overlay.set/clear()`                          | Allows the tab to draw on JACoB's native Elite overlay.                                                                                                             |
| Video             | `Elite.video.url/attach()`                           | Allows the tab to display JACoB's focus-gated Elite capture stream.                                                                                                 |
| Vision            | `Elite.vision.sample()`                              | Allows the tab to receive derived visual features and motion data. This API does not return raw pixels.                                                             |
| Inter-tab control | `Elite.actions.invoke()`                             | Allows the caller to invoke actions that other saved JACoB tabs explicitly published. The provider still needs its own permissions for anything privileged it does. |

Users can clear remembered grants and denials for a saved tab from Tab Manager with **Reset permissions**. Preview-tab permissions last only for that preview session.

### Rate limits

- General cap: about 120 SDK requests per second for each tab frame.
- `vision.sample()` and `journal.read()`: leave about 75 ms or more between calls.
- `net.fetch()`: leave about 100 ms or more between calls.
- Persistent store writes, deletes, and clears: leave about 50 ms or more between calls.
- If JACoB emits the data as an event, subscribe to it. Do not poll `state.get()` just to recreate an existing event stream.

> **Before you grant permissions:** a tab with Network access can send data it can read to a public service. If you did not write the tab, treat it like a browser extension: read it first, then grant only the capabilities you are comfortable giving it.

<span id="quick-index"></span>

## 5. SDK index <a href="#contents" class="toplink">Back to contents</a>

The SDK 10 tab surface has 59 public properties, methods, and helpers in this build. Use the table as the fast path: click a method to jump to its call, parameters, return data, errors, notes, and example.

| Namespace      | Method / property                                                                                  | Type                     | What it does                                                                                                                                                                |
|----------------|----------------------------------------------------------------------------------------------------|--------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Elite.api      | [`Elite.api.version`](#m-elite-api-version)                                                        | local property           | Returns the SDK version exposed to this tab. Check it before using a feature that was added in a newer SDK.                                                                 |
| Elite.core     | [`Elite.core.ping()`](#m-elite-core-ping)                                                          | request                  | Checks that the tab can reach the JACoB core. Use it for a basic connection test or diagnostics.                                                                            |
| Elite.system   | [`Elite.system.health()`](#m-elite-system-health)                                                  | request                  | Returns JACoB's current automation health: game state, bindings, devices, input path, blockers, and warnings. Check this before starting a control-heavy workflow.          |
| Elite.state    | [`Elite.state.get()`](#m-elite-state-get)                                                          | request                  | Returns the current consolidated Elite snapshot: Status.json, the latest journal event, recovered session context, Market compatibility data, and companion JSON snapshots. |
| Elite.state    | [`Elite.state.subscribe(callback)`](#m-elite-state-subscribe-callback)                             | local event subscription | Runs your callback when Status.json changes. The callback receives the Status object.                                                                                       |
| Elite.data     | [`Elite.data.list()`](#m-elite-data-list)                                                          | request                  | Returns the Elite companion JSON files JACoB currently sees in the detected journal directory.                                                                              |
| Elite.data     | [`Elite.data.get(name)`](#m-elite-data-get-name)                                                   | request                  | Returns one companion JSON snapshot by canonical name, such as `cargo`, or by filename, such as `NavRoute.json`.                                                            |
| Elite.data     | [`Elite.data.subscribe(name, callback)`](#m-elite-data-subscribe-name-callback)                    | local event subscription | Runs your callback when a companion JSON snapshot changes. Pass `*` to listen to every companion-file update.                                                               |
| Elite.journal  | [`Elite.journal.files()`](#m-elite-journal-files)                                                  | request                  | Returns the journal session files JACoB allows tabs to read.                                                                                                                |
| Elite.journal  | [`Elite.journal.read(options)`](#m-elite-journal-read-options)                                     | request                  | Reads a bounded page of historical journal events. Use it for startup recovery, backfill, or analysis. Use subscriptions for new events as they happen.                     |
| Elite.journal  | [`Elite.journal.subscribe(eventName, callback)`](#m-elite-journal-subscribe-eventname-callback)    | local event subscription | Runs your callback for live journal events. Pass an event name such as `FSDJump`, or `*` for every journal event.                                                           |
| Elite.events   | [`Elite.events.subscribe(name, callback)`](#m-elite-events-subscribe-name-callback)                | local event subscription | Subscribes to JACoB's generic event bus. Use this for host events outside the journal helpers, or pass `*` while debugging.                                                 |
| Elite.bindings | [`Elite.bindings.list()`](#m-elite-bindings-list)                                                  | request                  | Returns the active Elite binding set plus binding diagnostics.                                                                                                              |
| Elite.bindings | [`Elite.bindings.diagnostics()`](#m-elite-bindings-diagnostics)                                    | request                  | Returns binding-loader diagnostics without the full action list.                                                                                                            |
| Elite.bindings | [`Elite.bindings.get(name)`](#m-elite-bindings-get-name)                                           | request                  | Returns the primary and secondary binding slots for one Elite action.                                                                                                       |
| Elite.bindings | [`Elite.bindings.reload()`](#m-elite-bindings-reload)                                              | request                  | Reloads Elite's binding files from disk. Use it after the user changes controls.                                                                                            |
| Elite.bindings | [`Elite.bindings.press(action)`](#m-elite-bindings-press-action)                                   | request                  | Presses one Elite action using the user's actual binding. JACoB tries an injectable keyboard Secondary binding first, then Primary.                                         |
| Elite.bindings | [`Elite.bindings.down(action)`](#m-elite-bindings-down-action)                                     | request                  | Presses an Elite action and leaves it held. Pair it with `bindings.up()` for native repeat acceleration or continuous controls.                                             |
| Elite.bindings | [`Elite.bindings.up(action)`](#m-elite-bindings-up-action)                                         | request                  | Releases an action previously held with `bindings.down()`.                                                                                                                  |
| Elite.bindings | [`Elite.bindings.hold(action, durationMs)`](#m-elite-bindings-hold-action-durationms)              | request                  | Holds an Elite action for a fixed number of milliseconds, then releases it.                                                                                                 |
| Elite.input    | [`Elite.input.tap(key, options)`](#m-elite-input-tap-key-options)                                  | request                  | Sends a specific key or chord to Elite. Use raw input only when the physical key itself matters.                                                                            |
| Elite.input    | [`Elite.input.hold(key, durationMs, options)`](#m-elite-input-hold-key-durationms-options)         | request                  | Holds a specific raw key or chord for a fixed duration.                                                                                                                     |
| Elite.input    | [`Elite.input.text(text, options)`](#m-elite-input-text-text-options)                              | request                  | Types text into Elite through the host text-input path. Use it for system names, search boxes, labels, and other human text.                                                |
| Elite.recorder | [`Elite.recorder.status()`](#m-elite-recorder-status)                                              | request                  | Returns whether the optional Action Recorder is available and whether it is currently recording.                                                                            |
| Elite.recorder | [`Elite.recorder.start()`](#m-elite-recorder-start)                                                | request                  | Starts an explicit physical-key recording session. On Windows, recording only runs while Elite is foreground and ignores JACoB-injected input.                              |
| Elite.recorder | [`Elite.recorder.subscribe(callback)`](#m-elite-recorder-subscribe-callback)                       | local event subscription | Runs your callback for recorder key-down and key-up events during an active session.                                                                                        |
| Elite.recorder | [`Elite.recorder.stop()`](#m-elite-recorder-stop)                                                  | request                  | Stops recording and returns the complete enriched event list captured by the core.                                                                                          |
| Elite.overlay  | [`Elite.overlay.info()`](#m-elite-overlay-info)                                                    | request                  | Returns HUD-overlay availability, dimensions, driver, and supported rendering features. Check it before showing HUD controls.                                               |
| Elite.overlay  | [`Elite.overlay.set(scene)`](#m-elite-overlay-set-scene)                                           | request                  | Replaces this tab's HUD layer with the scene you provide. Send the complete current scene each time.                                                                        |
| Elite.overlay  | [`Elite.overlay.clear()`](#m-elite-overlay-clear)                                                  | request                  | Clears this tab's HUD layer without touching overlays owned by other tabs.                                                                                                  |
| Elite.video    | [`Elite.video.info()`](#m-elite-video-info)                                                        | request                  | Returns whether JACoB can capture the Elite window for Game View.                                                                                                           |
| Elite.video    | [`Elite.video.url(options)`](#m-elite-video-url-options)                                           | request                  | Returns an authenticated MJPEG stream URL for the current browser.                                                                                                          |
| Elite.video    | [`Elite.video.attach(element, options)`](#m-elite-video-attach-element-options)                    | request                  | Gets a Game View stream URL and assigns it to the supplied element's `src`.                                                                                                 |
| Elite.vision   | [`Elite.vision.info()`](#m-elite-vision-info)                                                      | request                  | Returns availability and capabilities for JACoB's derived-vision service. The service returns features and motion data, not raw frames.                                     |
| Elite.vision   | [`Elite.vision.sample(options)`](#m-elite-vision-sample-options)                                   | request                  | Returns one derived visual sample with feature points, luminance/contrast, and coarse frame-to-frame motion.                                                                |
| Elite.vision   | [`Elite.vision.inspect(spec)`](#m-elite-vision-inspect-spec)                                       | request                  | Runs up to eight derived visual operations against one normalized Elite-client region, including color tests, fill, image statistics, and OCR.                              |
| Elite.vision   | [`Elite.vision.calibrate(options)`](#m-elite-vision-calibrate-options)                             | host-owned setup request | Runs JACoB’s standard frozen-frame region/color calibration and returns normalized configuration data without exposing the image to the tab.                                |
| Elite.vision   | [`Elite.vision.configure(id, options)`](#m-elite-vision-configure-id-options)                      | persistent SDK helper    | Loads, defaults, or calibrates a saved Vision configuration scoped to the calling tab.                                                                                      |
| Elite.vision   | [`Elite.vision.reset(id)`](#m-elite-vision-reset-id)                                               | persistent SDK helper    | Deletes one saved Vision configuration so the tab can default or calibrate it again.                                                                                        |
| Elite.vision   | [`Elite.vision.debug.show(options)`](#m-elite-vision-debug-show-options)                           | request                  | Shows Vision region/subregion bounds in a reserved native HUD debug layer.                                                                                                  |
| Elite.vision   | [`Elite.vision.debug.clear()`](#m-elite-vision-debug-clear)                                        | request                  | Clears the calling tab’s Vision debug layer.                                                                                                                                |
| Elite.net      | [`Elite.net.fetch(url, options)`](#m-elite-net-fetch-url-options)                                  | request                  | Makes a bounded public HTTP/HTTPS request through JACoB. Use this for APIs such as Spansh; direct browser networking is blocked in sandboxed tabs.                          |
| Elite.store    | [`Elite.store.get(key, fallback)`](#m-elite-store-get-key-fallback)                                | request                  | Reads one value from this saved tab's persistent state. Returns the fallback when the key does not exist.                                                                   |
| Elite.store    | [`Elite.store.set(key, value)`](#m-elite-store-set-key-value)                                      | request                  | Saves a JSON-serializable value in this tab's private persistent state.                                                                                                     |
| Elite.store    | [`Elite.store.delete(key)`](#m-elite-store-delete-key)                                             | request                  | Deletes one key from this tab's persistent state.                                                                                                                           |
| Elite.store    | [`Elite.store.clear()`](#m-elite-store-clear)                                                      | request                  | Deletes all persistent state owned by this tab.                                                                                                                             |
| Elite.actions  | [`Elite.actions.register(name, handler, options)`](#m-elite-actions-register-name-handler-options) | broker registration      | Publishes a named callback that other saved JACoB tabs can invoke. Providers must be saved tabs.                                                                            |
| Elite.actions  | [`Elite.actions.unregister(name)`](#m-elite-actions-unregister-name)                               | broker registration      | Removes one action published by the calling tab.                                                                                                                            |
| Elite.actions  | [`Elite.actions.list(options)`](#m-elite-actions-list-options)                                     | host request             | Returns actions currently published by saved tabs. Discovery is enabled by default.                                                                                         |
| Elite.actions  | [`Elite.actions.invoke(name, payload, options)`](#m-elite-actions-invoke-name-payload-options)     | host request             | Invokes a named action in another saved tab and waits for its result.                                                                                                       |
| Elite.tabs     | [`Elite.tabs.list()`](#m-elite-tabs-list)                                                          | host request             | Returns JACoB navigation entries, including hidden/active/loaded state for custom tabs.                                                                                     |
| Elite.tabs     | [`Elite.tabs.activate(target)`](#m-elite-tabs-activate-target)                                     | host request             | Displays a JACoB page by saved-tab ID, navigation ID, tab name, or default-page name.                                                                                       |
| Elite.locale   | [`Elite.locale.language`](#m-elite-locale-language)                                                | local property           | Returns the tab's currently cached JACoB language code immediately.                                                                                                         |
| Elite.locale   | [`Elite.locale.supported`](#m-elite-locale-supported)                                              | local property           | Returns a copy of the languages currently advertised by the host.                                                                                                           |
| Elite.locale   | [`Elite.locale.get()`](#m-elite-locale-get)                                                        | request                  | Fetches the current host language and supported-language list directly from JACoB.                                                                                          |
| Elite.locale   | [`Elite.locale.t(dictionary, key, vars)`](#m-elite-locale-t-dictionary-key-vars)                   | local helper             | Resolves a tab-owned string for the active JACoB language, with English fallback and `{name}`-style interpolation.                                                          |
| Elite.locale   | [`Elite.locale.subscribe(callback)`](#m-elite-locale-subscribe-callback)                           | local event subscription | Runs your callback once with the current locale, then again whenever JACoB's language changes.                                                                              |
| Elite.files    | [`Elite.files.download(name, data, options)`](#m-elite-files-download-name-data-options)           | local browser helper     | Starts a browser download for generated content. It does not give the tab general filesystem access.                                                                        |
| Elite.files    | [`Elite.files.json(name, value, compact)`](#m-elite-files-json-name-value-compact)                 | local browser helper     | Downloads a JavaScript value as `application/json`.                                                                                                                         |

<span id="method-reference"></span>

## 6. SDK reference <a href="#contents" class="toplink">Back to contents</a>

> **Scope of this reference:** this section documents the `Elite` object actually injected into sandboxed tabs. Host-console methods stay out of the public tab API and are listed separately in the direct-protocol section.

<span id="ns-elite-api"></span>

## Elite.api <a href="#contents" class="toplink">Back to contents</a>

Check which SDK contract the host exposes.

<span id="m-elite-api-version"></span>

### `Elite.api.version`

local propertyPermission: None

Returns the SDK version exposed to this tab. Check it before using SDK 10 features such as Vision inspection and calibration.

#### Call

    const sdk = Elite.api.version;

#### Returns

Number. JACoB Alpha 0.2.13 reports `10`.

#### Example

    if (Elite.api.version < 10) {
      document.body.innerHTML = '<h2>This tab requires JACoB SDK 10.</h2>';
      throw new Error('SDK_TOO_OLD');
    }
    console.log('SDK', Elite.api.version);

<span id="ns-elite-core"></span>

## Elite.core <a href="#contents" class="toplink">Back to contents</a>

Check that the tab bridge can reach the JACoB core.

<span id="m-elite-core-ping"></span>

### `Elite.core.ping()`

requestPermission: None

Checks that the tab can reach the JACoB core. Use it for a basic connection test or diagnostics.

#### Call

    const result = await Elite.core.ping();

#### Returns

`{ pong: true, time: "...RFC3339..." }`

#### Errors

- `OFFLINE`
- `TIMEOUT`

#### Example

    try {
      const { pong, time } = await Elite.core.ping();
      console.log(pong ? `Core alive at ${time}` : 'Unexpected reply');
    } catch (err) {
      console.error(err.code, err.message);
    }

<span id="ns-elite-system"></span>

## Elite.system <a href="#contents" class="toplink">Back to contents</a>

Check whether the host is ready for automation.

<span id="m-elite-system-health"></span>

### `Elite.system.health()`

requestPermission: None

Returns JACoB's current automation health: game state, bindings, devices, input path, blockers, and warnings. Check this before starting a control-heavy workflow.

#### Call

    const health = await Elite.system.health();

#### Returns

Object with `status` (`ok`, `warning`, or `blocked`), plus blockers, warnings, game/device information, and binding diagnostics.

#### Errors

- `OFFLINE`
- `TIMEOUT`

#### Example

    const health = await Elite.system.health();
    if (health.status === 'blocked') {
      alert('Cannot automate safely: ' + health.blockers.join('; '));
    } else if (health.status === 'warning') {
      console.warn(...health.warnings);
    } else {
      console.log('JACoB is ready');
    }

<span id="ns-elite-state"></span>

## Elite.state <a href="#contents" class="toplink">Back to contents</a>

Read current Elite state and subscribe to live Status.json updates.

<span id="m-elite-state-get"></span>

### `Elite.state.get()`

requestPermission: None

Returns the current consolidated Elite snapshot: Status.json, the latest journal event, recovered session context, Market compatibility data, and companion JSON snapshots.

#### Call

    const snapshot = await Elite.state.get();

#### Returns

Snapshot with fields such as `status`, `lastJournalEvent`, `journalContext`, `market`, `eliteFiles`, filenames, timestamps, and detected journal paths.

#### Errors

- `OFFLINE`
- `TIMEOUT`

#### Example

    const s = await Elite.state.get();
    const system = s.journalContext?.currentSystem ?? 'unknown';
    const range = s.journalContext?.maxJumpRange;
    console.log(`System: ${system}`);
    if (Number.isFinite(range)) console.log(`Jump range: ${range.toFixed(2)} ly`);

<span id="m-elite-state-subscribe-callback"></span>

### `Elite.state.subscribe(callback)`

local event subscriptionPermission: None

Runs your callback when Status.json changes. The callback receives the Status object.

#### Call

    const off = Elite.state.subscribe(status => { ... });

#### Parameters

| Name       | Type     | What it means                                         |
|------------|----------|-------------------------------------------------------|
| `callback` | function | Called whenever JACoB relays a changed `Status.json`. |

#### Returns

An unsubscribe function. Call it when you no longer need the stream.

#### Notes

- Status updates are file-change driven. Do not assume a fixed update rate.
- Use `state.get()` once for startup context, then subscribe for live changes.

#### Example

    const stopStatus = Elite.state.subscribe(status => {
      const h = Number(status.Heading);
      if (Number.isFinite(h)) document.querySelector('#heading').textContent = Math.round(h) + '°';
    });

    // Later:
    // stopStatus();

<span id="ns-elite-data"></span>

## Elite.data <a href="#contents" class="toplink">Back to contents</a>

Read and subscribe to Elite companion JSON snapshots.

<span id="m-elite-data-list"></span>

### `Elite.data.list()`

requestPermission: None

Returns the Elite companion JSON files JACoB currently sees in the detected journal directory.

#### Call

    const index = await Elite.data.list();

#### Returns

`{ files: [...] }`. Each entry includes its normalized name, source filename, availability, update metadata, and related fields.

#### Errors

- `OFFLINE`
- `TIMEOUT`

#### Example

    const index = await Elite.data.list();
    for (const file of index.files) {
      console.log(file.name, file.file, file.available);
    }

<span id="m-elite-data-get-name"></span>

### `Elite.data.get(name)`

requestPermission: None

Returns one companion JSON snapshot by canonical name, such as `cargo`, or by filename, such as `NavRoute.json`.

#### Call

    const result = await Elite.data.get(name);

#### Parameters

| Name   | Type   | What it means                                    |
|--------|--------|--------------------------------------------------|
| `name` | string | Canonical JACoB name or companion JSON filename. |

#### Returns

`{ found, name, file, updated, data }`. When found, `data` is the parsed Elite JSON object.

#### Errors

- `OFFLINE`
- `TIMEOUT`

#### Example

    const cargo = await Elite.data.get('cargo');
    if (!cargo.found) {
      console.log('Cargo.json has not been written yet');
    } else {
      console.table(cargo.data.Inventory ?? []);
    }

<span id="m-elite-data-subscribe-name-callback"></span>

### `Elite.data.subscribe(name, callback)`

local event subscriptionPermission: None

Runs your callback when a companion JSON snapshot changes. Pass `*` to listen to every companion-file update.

#### Call

    const off = Elite.data.subscribe(name, update => { ... });

#### Parameters

| Name       | Type     | What it means                               |
|------------|----------|---------------------------------------------|
| `name`     | string   | Canonical name, filename, or `*`.           |
| `callback` | function | Receives the companion-file update payload. |

#### Returns

An unsubscribe function.

#### Example

    const stopRoute = Elite.data.subscribe('navRoute', update => {
      if (!update.available) return;
      const route = update.data?.Route ?? [];
      console.log(`Route now has ${route.length} jumps`);
    });

    // stopRoute();

<span id="ns-elite-journal"></span>

## Elite.journal <a href="#contents" class="toplink">Back to contents</a>

Read historical journal records and subscribe to new ones.

<span id="m-elite-journal-files"></span>

### `Elite.journal.files()`

requestPermission: None

Returns the journal session files JACoB allows tabs to read.

#### Call

    const index = await Elite.journal.files();

#### Returns

`{ files: [...] }` with journal basenames and metadata you can pass back to `journal.read()`.

#### Errors

- `OFFLINE`
- `TIMEOUT`

#### Example

    const { files } = await Elite.journal.files();
    const newest = files[0];
    console.log('Newest exposed journal:', newest?.file ?? newest?.name);

<span id="m-elite-journal-read-options"></span>

### `Elite.journal.read(options)`

requestPermission: None

Reads a bounded page of historical journal events. Use it for startup recovery, backfill, or analysis. Use subscriptions for new events as they happen.

#### Call

    const page = await Elite.journal.read(options);

#### Parameters

| Name             | Type   | What it means                                                              |
|------------------|--------|----------------------------------------------------------------------------|
| `options.file`   | string | Optional exposed journal basename. Leave blank to use the current journal. |
| `options.event`  | string | Optional exact Elite event filter, for example `FSDJump`.                  |
| `options.offset` | number | Parsed-record offset. Default: `0`.                                        |
| `options.limit`  | number | Records per request. Default: 1,000. Core maximum: 5,000.                  |

#### Returns

`{ events, info }`. If `info.truncated` is true, continue from `info.nextOffset`.

#### Errors

- `JOURNAL_READ_FAILED`
- `RATE_LIMITED`
- `OFFLINE`
- `TIMEOUT`

#### Notes

- The sandbox bridge requires about 75 ms between `journal.read()` calls.
- JACoB rejects arbitrary paths and path traversal.

#### Example

    let offset = 0;
    const allJumps = [];
    for (;;) {
      const page = await Elite.journal.read({ event: 'FSDJump', offset, limit: 1000 });
      allJumps.push(...page.events);
      if (!page.info?.truncated) break;
      offset = page.info.nextOffset;
      await new Promise(r => setTimeout(r, 80));
    }
    console.log(`Recovered ${allJumps.length} jumps`);

<span id="m-elite-journal-subscribe-eventname-callback"></span>

### `Elite.journal.subscribe(eventName, callback)`

local event subscriptionPermission: None

Runs your callback for live journal events. Pass an event name such as `FSDJump`, or `*` for every journal event.

#### Call

    const off = Elite.journal.subscribe(eventName, event => { ... });

#### Parameters

| Name        | Type     | What it means                                           |
|-------------|----------|---------------------------------------------------------|
| `eventName` | string   | Exact Elite journal event, such as `FSDJump`, or `*`.   |
| `callback`  | function | Receives the parsed journal object as written by Elite. |

#### Returns

An unsubscribe function.

#### Example

    const off = Elite.journal.subscribe('FSDJump', event => {
      console.log(`Jumped to ${event.StarSystem}; ${event.JumpDist ?? '?'} ly`);
    });

    // Later: off();

<span id="ns-elite-events"></span>

## Elite.events <a href="#contents" class="toplink">Back to contents</a>

Subscribe to JACoB host events outside the journal-specific helpers.

<span id="m-elite-events-subscribe-name-callback"></span>

### `Elite.events.subscribe(name, callback)`

local event subscriptionPermission: None

Subscribes to JACoB's generic event bus. Use this for host events outside the journal helpers, or pass `*` while debugging.

#### Call

    const off = Elite.events.subscribe(name, callback);

#### Parameters

| Name       | Type     | What it means                                                                           |
|------------|----------|-----------------------------------------------------------------------------------------|
| `name`     | string   | Event name such as `status`, `eliteFile`, `recorder.input`, or `*`.                     |
| `callback` | function | Named subscriptions receive `data`. Wildcard subscriptions receive `(eventName, data)`. |

#### Returns

An unsubscribe function.

#### Example

    const off = Elite.events.subscribe('*', (name, data) => {
      console.log('[JACoB event]', name, data);
    });

    // Useful while debugging. Remove noisy wildcard logging before shipping.
    // off();

<span id="ns-elite-bindings"></span>

## Elite.bindings <a href="#contents" class="toplink">Back to contents</a>

Work in Elite action names instead of hard-coded keys whenever possible.

<span id="m-elite-bindings-list"></span>

### `Elite.bindings.list()`

requestPermission: None

Returns the active Elite binding set plus binding diagnostics.

#### Call

    const result = await Elite.bindings.list();

#### Returns

Object with the bindings directory, active preset file(s), source, available files, parsed actions, autofill metadata, and diagnostics.

#### Errors

- `OFFLINE`
- `TIMEOUT`

#### Example

    const b = await Elite.bindings.list();
    console.log(`${b.actions.length} parsed Elite actions`);
    const missing = b.actions.filter(a => !a.primary?.key && !a.secondary?.key);
    console.log(`${missing.length} actions have no keyboard slot`);

<span id="m-elite-bindings-diagnostics"></span>

### `Elite.bindings.diagnostics()`

requestPermission: None

Returns binding-loader diagnostics without the full action list.

#### Call

    const d = await Elite.bindings.diagnostics();

#### Returns

Preset diagnostics including the active source, selector state, required devices, duplicate elements, parse/loader errors, and read-only state.

#### Errors

- `OFFLINE`
- `TIMEOUT`

#### Example

    const d = await Elite.bindings.diagnostics();
    if (d.parseError) console.error('Binding parse error:', d.parseError);
    if (d.loader?.hasErrors) console.warn('Elite loader reported errors', d.loader.lines);
    if (d.duplicateElements?.length) console.log('Duplicates:', d.duplicateElements);

<span id="m-elite-bindings-get-name"></span>

### `Elite.bindings.get(name)`

requestPermission: None

Returns the primary and secondary binding slots for one Elite action.

#### Call

    const action = await Elite.bindings.get(name);

#### Parameters

| Name   | Type   | What it means                                                  |
|--------|--------|----------------------------------------------------------------|
| `name` | string | Elite action name, for example `GalaxyMapOpen` or `UI_Select`. |

#### Returns

Binding action with `name`, `primary`, and `secondary` slots.

#### Errors

- `BINDING_NOT_FOUND`
- `BAD_PARAMS`
- `OFFLINE`
- `TIMEOUT`

#### Example

    const select = await Elite.bindings.get('UI_Select');
    console.log('Primary:', select.primary);
    console.log('Secondary:', select.secondary);

<span id="m-elite-bindings-reload"></span>

### `Elite.bindings.reload()`

requestPermission: None

Reloads Elite's binding files from disk. Use it after the user changes controls.

#### Call

    const result = await Elite.bindings.reload();

#### Returns

`{ activeFile, actionCount }`.

#### Errors

- `BINDINGS_RELOAD_FAILED`
- `OFFLINE`
- `TIMEOUT`

#### Example

    document.querySelector('#reload').onclick = async () => {
      const result = await Elite.bindings.reload();
      console.log(`Reloaded ${result.actionCount} actions from ${result.activeFile}`);
    };

<span id="m-elite-bindings-press-action"></span>

### `Elite.bindings.press(action)`

requestPermission: Control Elite Dangerous

Presses one Elite action using the user's actual binding. JACoB tries an injectable keyboard Secondary binding first, then Primary.

#### Call

    const result = await Elite.bindings.press(action);

#### Parameters

| Name     | Type   | What it means                            |
|----------|--------|------------------------------------------|
| `action` | string | Elite action name from the binding file. |

#### Returns

Control result with the action, selected binding slot, resolved key/modifiers, and elapsed duration.

#### Errors

- `PERMISSION_DENIED`
- `INPUT_DISABLED`
- `INPUT_UNAVAILABLE`
- `BINDING_NOT_FOUND`
- `BINDING_PRESS_FAILED`

#### Example

    try {
      await Elite.bindings.press('GalaxyMapOpen');
      console.log('Galaxy Map command sent');
    } catch (err) {
      console.error(`${err.code}: ${err.message}`);
    }

<span id="m-elite-bindings-down-action"></span>

### `Elite.bindings.down(action)`

requestPermission: Control Elite Dangerous

Presses an Elite action and leaves it held. Pair it with `bindings.up()` for native repeat acceleration or continuous controls.

#### Call

    const result = await Elite.bindings.down(action);

#### Parameters

| Name     | Type   | What it means                            |
|----------|--------|------------------------------------------|
| `action` | string | Elite action name from the binding file. |

#### Returns

Control result for the resolved semantic binding.

#### Errors

- `PERMISSION_DENIED`
- `BINDING_DOWN_FAILED`
- `INPUT_DISABLED`
- `INPUT_UNAVAILABLE`

#### Notes

- Pair `down()` with `up()` in `try/finally`.
- The core tracks held semantic bindings and attempts to release them if the browser disconnects.

#### Example

    await Elite.bindings.down('UI_Right');
    try {
      await new Promise(r => setTimeout(r, 1200));
    } finally {
      await Elite.bindings.up('UI_Right');
    }

<span id="m-elite-bindings-up-action"></span>

### `Elite.bindings.up(action)`

requestPermission: Control Elite Dangerous

Releases an action previously held with `bindings.down()`.

#### Call

    const result = await Elite.bindings.up(action);

#### Parameters

| Name     | Type   | What it means                                           |
|----------|--------|---------------------------------------------------------|
| `action` | string | Same semantic action passed to \<code\>down()\</code\>. |

#### Returns

A control result for the release.

#### Errors

- `PERMISSION_DENIED`
- `BINDING_UP_FAILED`
- `INPUT_DISABLED`
- `INPUT_UNAVAILABLE`

#### Example

    let holding = false;
    async function startScroll() {
      if (holding) return;
      await Elite.bindings.down('UI_Down');
      holding = true;
    }
    async function stopScroll() {
      if (!holding) return;
      await Elite.bindings.up('UI_Down');
      holding = false;
    }

<span id="m-elite-bindings-hold-action-durationms"></span>

### `Elite.bindings.hold(action, durationMs)`

requestPermission: Control Elite Dangerous

Holds an Elite action for a fixed number of milliseconds, then releases it.

#### Call

    const result = await Elite.bindings.hold(action, durationMs);

#### Parameters

| Name         | Type   | What it means                                                |
|--------------|--------|--------------------------------------------------------------|
| `action`     | string | Elite action name from the binding file.                     |
| `durationMs` | number | Hold duration in milliseconds. Accepted range: 20-10,000 ms. |

#### Returns

Control result with the action, slot, key/modifiers, and duration.

#### Errors

- `PERMISSION_DENIED`
- `BAD_PARAMS`
- `BINDING_HOLD_FAILED`
- `INPUT_DISABLED`
- `INPUT_UNAVAILABLE`

#### Example

    // Nudge the camera using the Elite action, not a hard-coded user key.
    await Elite.bindings.hold('MoveFreeCamRight', 240);

<span id="ns-elite-input"></span>

## Elite.input <a href="#contents" class="toplink">Back to contents</a>

Send raw keys, chords, and human text when semantic bindings are not the right tool.

<span id="m-elite-input-tap-key-options"></span>

### `Elite.input.tap(key, options)`

requestPermission: Control Elite Dangerous

Sends a specific key or chord to Elite. Use raw input only when the physical key itself matters.

#### Call

    const result = await Elite.input.tap(key, options);

#### Parameters

| Name                | Type       | What it means                                                                            |
|---------------------|------------|------------------------------------------------------------------------------------------|
| `key`               | string     | Normalized key name, such as `ENTER` or `A`, or an SDK 8 physical token such as `SC:29`. |
| `options.modifiers` | string\[\] | Optional modifiers such as `CTRL`, `SHIFT`, `RIGHTALT`, or physical scan-code tokens.    |
| `options.delayMs`   | number     | Optional delay before sending input. Range: 0-5,000 ms.                                  |

#### Returns

`{ sent: true, key, modifiers, delayMs }`.

#### Errors

- `PERMISSION_DENIED`
- `BAD_PARAMS`
- `GAME_FOCUS_FAILED`
- `INPUT_FAILED`
- `INPUT_DISABLED`
- `INPUT_UNAVAILABLE`

#### Notes

- SDK 8+ physical scan-code tokens are layout-independent.
- JACoB blocks OS-global escape chords such as Alt+Tab and Alt+F4.

#### Example

    // Logical key name:
    await Elite.input.tap('ENTER');

    // Layout-independent physical key (PC/AT set-1 scan code 0x29):
    await Elite.input.tap('SC:29');

    // Chord with modifiers:
    await Elite.input.tap('K', { modifiers: ['CTRL', 'SHIFT'] });

<span id="m-elite-input-hold-key-durationms-options"></span>

### `Elite.input.hold(key, durationMs, options)`

requestPermission: Control Elite Dangerous

Holds a specific raw key or chord for a fixed duration.

#### Call

    const result = await Elite.input.hold(key, durationMs, options);

#### Parameters

| Name                | Type       | What it means                                          |
|---------------------|------------|--------------------------------------------------------|
| `key`               | string     | Normalized key name or SDK 8 physical scan-code token. |
| `durationMs`        | number     | 20-10000 ms.                                           |
| `options.modifiers` | string\[\] | Optional normalized or physical modifier keys.         |

#### Returns

`{ sent: true, key, modifiers, durationMs }`.

#### Errors

- `PERMISSION_DENIED`
- `BAD_PARAMS`
- `GAME_FOCUS_FAILED`
- `INPUT_FAILED`
- `INPUT_DISABLED`
- `INPUT_UNAVAILABLE`

#### Example

    // Hold D for 350 ms:
    await Elite.input.hold('D', 350);

    // Hold a physical key with physical Right Alt / AltGr:
    await Elite.input.hold('SC:10', 500, { modifiers: ['SC:E0:38'] });

<span id="m-elite-input-text-text-options"></span>

### `Elite.input.text(text, options)`

requestPermission: Control Elite Dangerous

Types text into Elite through the host text-input path. Use it for system names, search boxes, labels, and other human text.

#### Call

    const result = await Elite.input.text(text, options);

#### Parameters

| Name                 | Type   | What it means                                   |
|----------------------|--------|-------------------------------------------------|
| `text`               | string | Text to type.                                   |
| `options.intervalMs` | number | Delay between characters; SDK default is 15 ms. |

#### Returns

A control result identifying `input.text` and total duration.

#### Errors

- `PERMISSION_DENIED`
- `BAD_PARAMS`
- `TEXT_INPUT_FAILED`
- `INPUT_DISABLED`
- `INPUT_UNAVAILABLE`

#### Notes

- SDK 8+ resolves characters against the active host keyboard layout and preserves Unicode where the platform path supports it.
- Do not use raw tap sequences to type human text on international keyboards.

#### Example

    const destination = 'Hegoo SB-L b35-0';
    await Elite.input.text(destination, { intervalMs: 18 });

    // Text entry follows the host layout and supports international input paths.
    await Elite.input.text('München');

<span id="ns-elite-recorder"></span>

## Elite.recorder <a href="#contents" class="toplink">Back to contents</a>

Capture physical keyboard input for user-recorded control sequences.

<span id="m-elite-recorder-status"></span>

### `Elite.recorder.status()`

requestPermission: None

Returns whether the optional Action Recorder is available and whether it is currently recording.

#### Call

    const status = await Elite.recorder.status();

#### Returns

`{ available, driver, recording, eventCount, scope }`.

#### Errors

- `OFFLINE`
- `TIMEOUT`

#### Example

    const r = await Elite.recorder.status();
    if (!r.available) {
      document.querySelector('#record').disabled = true;
      console.log('Recorder unavailable on this build/platform');
    }

<span id="m-elite-recorder-start"></span>

### `Elite.recorder.start()`

requestPermission: Record Elite keyboard input

Starts an explicit physical-key recording session. On Windows, recording only runs while Elite is foreground and ignores JACoB-injected input.

#### Call

    const status = await Elite.recorder.start();

#### Returns

The updated recorder status object.

#### Errors

- `PERMISSION_DENIED`
- `RECORDER_UNAVAILABLE`
- `RECORDER_START_FAILED`

#### Example

    const before = await Elite.recorder.status();
    if (before.available && !before.recording) {
      await Elite.recorder.start();
      console.log('Recording armed - perform the action in Elite');
    }

<span id="m-elite-recorder-subscribe-callback"></span>

### `Elite.recorder.subscribe(callback)`

local event subscriptionPermission: None

Runs your callback for recorder key-down and key-up events during an active session.

#### Call

    const off = Elite.recorder.subscribe(event => { ... });

#### Parameters

| Name       | Type     | What it means                            |
|------------|----------|------------------------------------------|
| `callback` | function | Receives enriched recorder.input events. |

#### Returns

An unsubscribe function.

#### Notes

- SDK 8 events retain legacy `key`/`modifiers` and add physical identity such as `physical`, `scanCode`, `virtualKey`, `extended`, and `localizedName`.
- Prefer storing `physical` when present; use `localizedName` for display.

#### Example

    const events = [];
    const off = Elite.recorder.subscribe(ev => {
      events.push(ev);
      const label = ev.localizedName || ev.key || ev.physical;
      console.log(ev.type, label, ev.durationMs ?? '');
    });

    // Later: off();

<span id="m-elite-recorder-stop"></span>

### `Elite.recorder.stop()`

requestPermission: Record Elite keyboard input

Stops recording and returns the complete enriched event list captured by the core.

#### Call

    const result = await Elite.recorder.stop();

#### Returns

`{ status, events }`.

#### Errors

- `PERMISSION_DENIED`
- `RECORDER_STOP_FAILED`

#### Example

    const result = await Elite.recorder.stop();
    const completedPresses = result.events.filter(e => e.type === 'up' && !e.isModifier);
    console.log(`Captured ${completedPresses.length} completed key presses`);

<span id="ns-elite-overlay"></span>

## Elite.overlay <a href="#contents" class="toplink">Back to contents</a>

Draw a native HUD layer owned by this tab.

<span id="m-elite-overlay-info"></span>

### `Elite.overlay.info()`

requestPermission: None

Returns HUD-overlay availability, dimensions, driver, and supported rendering features. Check it before showing HUD controls.

#### Call

    const info = await Elite.overlay.info();

#### Returns

Overlay capability data such as `available`, driver, dimensions, visible/layer state, alpha/text support, supported shapes, and coordinate spaces.

#### Errors

- `OFFLINE`
- `TIMEOUT`

#### Example

    const info = await Elite.overlay.info();
    if (!info.available) {
      document.querySelector('#hud-toggle').disabled = true;
    } else {
      console.log(`HUD: ${info.driver} ${info.width}x${info.height}`);
    }

<span id="m-elite-overlay-set-scene"></span>

### `Elite.overlay.set(scene)`

requestPermission: Draw an Elite overlay

Replaces this tab's HUD layer with the scene you provide. Send the complete current scene each time.

#### Call

    const result = await Elite.overlay.set(scene);

#### Parameters

| Name    | Type   | What it means                                                                                   |
|---------|--------|-------------------------------------------------------------------------------------------------|
| `scene` | object | Scene with \<code\>space\</code\>, optional \<code\>z\</code\>, and \<code\>items\[\]\</code\>. |

#### Returns

Result with the tab layer ID, rendered item count, and current overlay info.

#### Errors

- `PERMISSION_DENIED`
- `OVERLAY_UNAVAILABLE`
- `OVERLAY_SCENE_INVALID`
- `OVERLAY_RENDER_FAILED`
- `BAD_PARAMS`

#### Notes

- Supported items: `text`, `line`, `polyline`, `polygon`, `rect`, and `circle`.
- Limits: 2,000 items per scene and 10,000 points in one polyline or polygon.
- Each saved tab owns its own overlay layer.

#### Example

    await Elite.overlay.set({
      z: 20,
      space: 'normalized',
      items: [
        { type: 'text', x: 0.5, y: 0.08, text: 'TARGET 42.7%', color: '#ff7b00', fontSize: 24, align: 'center' },
        { type: 'circle', x: 0.5, y: 0.5, r: 0.01, stroke: '#ffffff', lineWidth: 2 }
      ]
    });

<span id="m-elite-overlay-clear"></span>

### `Elite.overlay.clear()`

requestPermission: Draw an Elite overlay

Clears this tab's HUD layer without touching overlays owned by other tabs.

#### Call

    const result = await Elite.overlay.clear();

#### Returns

`{ layer, cleared: true, overlay }` when the overlay exists; clearing an unavailable overlay is treated safely.

#### Errors

- `PERMISSION_DENIED`
- `OVERLAY_CLEAR_FAILED`

#### Example

    document.querySelector('#stop').onclick = async () => {
      running = false;
      await Elite.overlay.clear();
    };

<span id="ns-elite-video"></span>

## Elite.video <a href="#contents" class="toplink">Back to contents</a>

Display a utility-rate live view of the Elite window.

<span id="m-elite-video-info"></span>

### `Elite.video.info()`

requestPermission: None

Returns whether JACoB can capture the Elite window for Game View.

#### Call

    const info = await Elite.video.info();

#### Returns

Capability data including `available`, capture driver, `eliteOnly`, `foregroundOnly`, and `rawFramesExposed: false`.

#### Errors

- `OFFLINE`
- `TIMEOUT`

#### Example

    const info = await Elite.video.info();
    console.log(info.available ? `Capture via ${info.driver}` : 'Capture unavailable');

<span id="m-elite-video-url-options"></span>

### `Elite.video.url(options)`

requestPermission: View the Elite video feed

Returns an authenticated MJPEG stream URL for the current browser.

#### Call

    const url = await Elite.video.url(options);

#### Parameters

| Name              | Type    | What it means                                                                                                                                                |
|-------------------|---------|--------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `options.width`   | number  | Requested width. Supported range: 160-1,920.                                                                                                                 |
| `options.fps`     | number  | Requested frame rate. Range: 1-15 FPS.                                                                                                                       |
| `options.quality` | number  | JPEG quality. Range: 20-95.                                                                                                                                  |
| `options.matte`   | boolean | When true, place the verified Elite client frame on a black monitor-sized privacy canvas. The built-in Game View enables this by default for windowed Elite. |

#### Returns

URL string with the current media authentication token when the browser needs one.

#### Errors

- `PERMISSION_DENIED`
- `RATE_LIMITED`

#### Example

    const url = await Elite.video.url({ width: 800, fps: 4, quality: 55 });
    document.querySelector('#game').src = url;

<span id="m-elite-video-attach-element-options"></span>

### `Elite.video.attach(element, options)`

requestPermission: View the Elite video feed

Gets a Game View stream URL and assigns it to the supplied element's `src`.

#### Call

    const url = await Elite.video.attach(element, options);

#### Parameters

| Name      | Type         | What it means                                    |
|-----------|--------------|--------------------------------------------------|
| `element` | HTML element | Usually an `<img>` element.                      |
| `options` | object       | Same width/FPS/quality options as `video.url()`. |

#### Returns

The URL assigned to the element.

#### Errors

- `PERMISSION_DENIED`
- `RATE_LIMITED`

#### Example

    const img = document.querySelector('#game');
    await Elite.video.attach(img, { width: 640, fps: 5, quality: 55 });
    img.alt = 'Live Elite utility preview';

<span id="ns-elite-vision"></span>

## Elite.vision <a href="#contents" class="toplink">Back to contents</a>

Inspect privacy-bounded regions of the verified Elite frame, run OCR and visual measurements, and use JACoB-owned calibration without exposing captured pixels to the tab.

<span id="m-elite-vision-info"></span>

### `Elite.vision.info()`

requestPermission: Use derived game vision

Returns availability and capabilities for JACoB's derived-vision service. SDK 10 Vision uses the canonical Elite-client frame and never returns raw captured pixels.

#### Call

    const info = await Elite.vision.info();

#### Returns

Capability data including availability, capture driver, Elite/foreground gating, `rawFramesExposed: false`, and the available derived feature classes.

#### Errors

- `PERMISSION_DENIED`
- `OFFLINE`
- `TIMEOUT`

#### Example

    const info = await Elite.vision.info();
    if (info.available) console.log('Derived vision:', info.derived.join(', '));

<span id="m-elite-vision-sample-options"></span>

### `Elite.vision.sample(options)`

requestPermission: Use derived game vision

Returns one derived visual sample with feature points, luminance/contrast, and coarse frame-to-frame motion.

#### Call

    const sample = await Elite.vision.sample(options);

#### Parameters

| Name            | Type   | What it means                                                  |
|-----------------|--------|----------------------------------------------------------------|
| `options.width` | number | Analysis width. Core range is roughly 160-640; default is 320. |

#### Returns

Sample with timestamp, availability/blanking state, dimensions, normalized `features[]`, `meanLuma`, `contrast`, and `motion`.

#### Errors

- `PERMISSION_DENIED`
- `VISION_UNAVAILABLE`
- `VISION_SAMPLE_FAILED`
- `RATE_LIMITED`

#### Notes

- Leave about 75 ms or more between samples.
- If Elite loses focus, JACoB blanks samples and resets temporal tracking.

#### Example

    const sample = await Elite.vision.sample({ width: 320 });
    if (!sample.blanked && sample.motion?.compared) {
      console.log(`motion dx=${sample.motion.dx.toFixed(3)} dy=${sample.motion.dy.toFixed(3)}`);
    }
    for (const f of sample.features.slice(0, 5)) {
      console.log('feature', f.x, f.y, f.score);
    }

<span id="m-elite-vision-inspect-spec"></span>

### `Elite.vision.inspect(spec)`

requestPermission: Use derived game vision

Runs one bounded Vision inspection against a normalized region of the verified Elite client frame. Use this when you care about a specific UI region or visual condition rather than general feature points.

#### Call

    const result = await Elite.vision.inspect(spec);

#### Parameters

| Name                        | Type   | What it means                                                                                                     |
|-----------------------------|--------|-------------------------------------------------------------------------------------------------------------------|
| `spec.region`               | object | Required normalized `{x,y,width,height}` region in the canonical Elite client frame.                              |
| `spec.operations`           | object | Required map of 1-8 named operations. Each name becomes a key in `result.results`.                                |
| `spec.maxWidth`             | number | Optional analysis width, 480-1600. Core default is 1280.                                                          |
| `operation.type`            | string | `colorPresent`, `colorCoverage`, `colorVerticalFill`, `luma`, `contrast`, `edgeDensity`, `text`, or `textLines`.  |
| `operation.subregion`       | object | Optional normalized region inside the parent Vision region.                                                       |
| `operation.color`           | string | Required `#RRGGBB` target for color operations.                                                                   |
| `operation.tolerance`       | number | Color tolerance 0-255. A zero/omitted value uses the core default of 55.                                          |
| `operation.minimumCoverage` | number | 0-1 threshold. Defaults depend on the color operation.                                                            |
| `operation.fullThreshold`   | number | Optional 0-1 "full" threshold for `colorVerticalFill`.                                                            |
| `operation.preprocess`      | object | Optional in-core OCR cleanup: scale, grayscale, auto-contrast, auto-threshold, invert, or isolate-color settings. |

#### Returns

`{ at, available, blanked, reason?, region, results }`. Each named result reports its operation `type` and whichever fields apply: `matched`, `number`, `text`, `lines`, `confidence`, and `samples`. `textLines` returns normalized line and word bounds.

#### Errors

- `PERMISSION_DENIED`
- `VISION_UNAVAILABLE`
- `VISION_INSPECT_FAILED`
- `RATE_LIMITED`

#### Notes

- The tab receives derived observations only. OCR source pixels and preprocessed images never leave the core.
- Keep at least about 75 ms between bridge calls; the core also applies a tighter live-inspection capture guard.
- Regions are normalized to the Elite client area, not the monitor or desktop.

#### Example

    const result = await Elite.vision.inspect({
      region: { x: 0.30, y: 0.50, width: 0.60, height: 0.40 },
      operations: {
        target: {
          type: 'colorPresent',
          color: '#4aa8ff',
          tolerance: 70,
          minimumCoverage: 0.03
        },
        label: {
          type: 'textLines',
          subregion: { x: 0.08, y: 0.05, width: 0.60, height: 0.45 },
          preprocess: { scale: 2, grayscale: true, autoContrast: true }
        }
      }
    });

    if (result.results.target?.matched) console.log('Target is present');
    console.log(result.results.label?.text || '');

<span id="m-elite-vision-calibrate-options"></span>

### `Elite.vision.calibrate(options)`

host-owned setup requestPermission: Use derived game vision

Runs JACoB's standard Vision calibration flow. JACoB briefly focuses Elite, captures one verified Elite-client frame, restores the previous window, and opens a host-owned frozen-frame picker. The calling tab receives only the normalized result.

#### Call

    const calibration = await Elite.vision.calibrate(options);

#### Parameters

| Name                  | Type    | What it means                                                             |
|-----------------------|---------|---------------------------------------------------------------------------|
| `options.label`       | string  | Human-readable label shown by the calibration UI.                         |
| `options.defaults`    | object  | Optional default region/color data used as the starting configuration.    |
| `options.pickColor`   | boolean | Also ask the user to sample a target color.                               |
| `options.averageSize` | number  | When color picking, sample an averaged square instead of one exact pixel. |

#### Returns

Normalized calibration data. Region-only calibration includes `region`. Color calibration also returns the sampled color in useful host-generated representations such as hex/RGB/HSV.

#### Errors

- `PERMISSION_DENIED`
- `VISION_UNAVAILABLE`
- `VISION_CALIBRATION_BUSY`
- `VISION_CALIBRATION_FAILED`
- `VISION_CALIBRATION_TIMEOUT`

#### Notes

- The frozen frame is displayed by JACoB itself. It is never sent into the custom-tab iframe.
- Calibration uses the same normalized Elite-client coordinate space as `vision.inspect()`.
- The captured calibration frame expires inside the host and is cleared after use.

#### Example

    const cfg = await Elite.vision.calibrate({
      label: 'Mining prospector result',
      pickColor: true,
      averageSize: 11
    });

    console.log('Region', cfg.region);
    console.log('Color', cfg.color);

<span id="m-elite-vision-configure-id-options"></span>

### `Elite.vision.configure(id, options)`

persistent SDK helperPermission: Vision only when calibration runs

Gets or creates a saved Vision configuration for the calling tab. Use this instead of inventing your own region-picker or calibration-storage system.

#### Call

    const cfg = await Elite.vision.configure(id, options);

#### Parameters

| Name                  | Type    | What it means                                                                               |
|-----------------------|---------|---------------------------------------------------------------------------------------------|
| `id`                  | string  | Required stable configuration ID. It is stored under the calling saved tab's private state. |
| `options.defaults`    | object  | Default configuration to save/use when no calibration is required.                          |
| `options.calibrate`   | boolean | Run the standard host calibration flow when no saved value exists.                          |
| `options.force`       | boolean | Ignore a saved value and create a fresh configuration.                                      |
| `options.pickColor`   | boolean | Ask calibration to capture a target color as well as the region.                            |
| `options.averageSize` | number  | Color sample averaging size passed to calibration.                                          |
| `options.label`       | string  | Label passed to JACoB's calibration UI.                                                     |

#### Returns

The saved/default/calibrated configuration object. A saved value is returned immediately unless `force` is true.

#### Errors

- `BAD_PARAMS` when `id` is empty
- Any calibration/storage error raised by the underlying request

#### Example

    const cfg = await Elite.vision.configure('carrier-market-price', {
      defaults: {
        region: { x: 0.40, y: 0.28, width: 0.30, height: 0.12 }
      },
      calibrate: true,
      label: 'Carrier market price'
    });

    const scan = await Elite.vision.inspect({
      region: cfg.region,
      operations: { price: { type: 'text', preprocess: { scale: 2 } } }
    });

<span id="m-elite-vision-reset-id"></span>

### `Elite.vision.reset(id)`

persistent SDK helperPermission: None

Deletes one saved Vision configuration from the calling tab's private state. The next `configure()` call can use defaults or run calibration again.

#### Call

    await Elite.vision.reset(id);

#### Parameters

| Name | Type   | What it means                                            |
|------|--------|----------------------------------------------------------|
| `id` | string | The same stable ID passed to `Elite.vision.configure()`. |

#### Returns

The normal tab-state deletion result.

#### Example

    await Elite.vision.reset('carrier-market-price');
    const cfg = await Elite.vision.configure('carrier-market-price', {
      calibrate: true,
      label: 'Carrier market price'
    });

<span id="m-elite-vision-debug-show-options"></span>

### `Elite.vision.debug.show(options)`

requestPermission: Use derived game vision

Draws the effective Vision region and optional operation subregions through JACoB's native HUD. Use it to verify calibration and coordinate math while building a tab.

#### Call

    const result = await Elite.vision.debug.show(options);

#### Parameters

| Name                 | Type   | What it means                                                         |
|----------------------|--------|-----------------------------------------------------------------------|
| `options.region`     | object | Normalized parent Vision region.                                      |
| `options.operations` | object | Optional operation definitions whose subregions should also be shown. |
| `options.label`      | string | Optional debug label.                                                 |

#### Returns

Host result confirming the debug overlay update.

#### Notes

- Debug bounds live in a reserved child layer under the saved tab's HUD namespace.
- Reloading or deleting the saved tab clears its owned overlay namespace.

#### Example

    await Elite.vision.debug.show({
      region: cfg.region,
      operations: {
        text: { type: 'textLines', subregion: { x: .1, y: .06, width: .57, height: .62 } }
      },
      label: 'NAV OCR'
    });

<span id="m-elite-vision-debug-clear"></span>

### `Elite.vision.debug.clear()`

requestPermission: Use derived game vision

Clears the calling tab's SDK 10 Vision debug bounds without clearing the tab's normal overlay scene.

#### Call

    await Elite.vision.debug.clear();

#### Returns

Host result confirming the debug layer was cleared.

#### Example

    try {
      await Elite.vision.debug.show({ region: cfg.region, label: 'CHECK' });
      // inspect or let the user verify the box
    } finally {
      await Elite.vision.debug.clear();
    }

<span id="ns-elite-net"></span>

## Elite.net <a href="#contents" class="toplink">Back to contents</a>

Call bounded public HTTP/HTTPS APIs through the trusted core.

<span id="m-elite-net-fetch-url-options"></span>

### `Elite.net.fetch(url, options)`

requestPermission: Use JACoB network access

Makes a bounded public HTTP/HTTPS request through JACoB. Use this for APIs such as Spansh; direct browser networking is blocked in sandboxed tabs.

#### Call

    const response = await Elite.net.fetch(url, options);

#### Parameters

| Name              | Type             | What it means                                                                     |
|-------------------|------------------|-----------------------------------------------------------------------------------|
| `url`             | string           | Public HTTP/HTTPS URL. JACoB blocks private/local targets and non-standard ports. |
| `options.method`  | string           | `GET` or `POST`.                                                                  |
| `options.headers` | object           | Headers from the SDK allowlist only.                                              |
| `options.body`    | string \| object | String body or object. Objects are JSON-serialized by the SDK wrapper.            |

#### Returns

`{ url, status, statusText, headers, contentType, body, json, bytes }`. JACoB also fills `json` when the response body parses as JSON.

#### Errors

- `PERMISSION_DENIED`
- `NET_FETCH_UNAVAILABLE`
- `NET_FETCH_FAILED`
- `RATE_LIMITED`

#### Notes

- Limits: 1 MiB request body, 8 MiB response, 20 s timeout, and 5 redirects.
- JACoB blocks localhost, private/LAN/link-local addresses, `.local`, bare local hostnames, and non-standard ports.
- Allowed headers include `Accept`, `Content-Type`, `Authorization`, `X-API-Key`, `If-None-Match`, and `If-Modified-Since`.

#### Example

    const res = await Elite.net.fetch('https://example.org/api/query', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: { system: 'Sol' }
    });
    if (res.status < 200 || res.status >= 300)
      throw new Error(`HTTP ${res.status}: ${res.statusText}`);
    console.log(res.json ?? JSON.parse(res.body));

<span id="ns-elite-store"></span>

## Elite.store <a href="#contents" class="toplink">Back to contents</a>

Keep persistent JSON state for this saved tab.

<span id="m-elite-store-get-key-fallback"></span>

### `Elite.store.get(key, fallback)`

requestPermission: None

Reads one value from this saved tab's persistent state. Returns the fallback when the key does not exist.

#### Call

    const value = await Elite.store.get(key, fallback);

#### Parameters

| Name       | Type   | What it means                                          |
|------------|--------|--------------------------------------------------------|
| `key`      | string | State key. Maximum: 128 bytes.                         |
| `fallback` | any    | Returned when the key does not exist. Default: `null`. |

#### Returns

Stored JSON value, or the supplied fallback if the key is missing.

#### Errors

- `TAB_STATE_PREVIEW`
- `TAB_STATE_UNAVAILABLE`
- `OFFLINE`
- `TIMEOUT`

#### Example

    const settings = await Elite.store.get('settings', {
      threshold: 30,
      showHud: true
    });
    console.log(settings);

<span id="m-elite-store-set-key-value"></span>

### `Elite.store.set(key, value)`

requestPermission: None

Saves a JSON-serializable value in this tab's private persistent state.

#### Call

    const result = await Elite.store.set(key, value);

#### Parameters

| Name    | Type              | What it means                        |
|---------|-------------------|--------------------------------------|
| `key`   | string            | State key. Maximum: 128 bytes.       |
| `value` | JSON-serializable | Maximum encoded value size: 512 KiB. |

#### Returns

Success result from the core.

#### Errors

- `TAB_STATE_PREVIEW`
- `TAB_STATE_SAVE_FAILED`
- `TAB_STATE_UNAVAILABLE`
- `RATE_LIMITED`

#### Notes

- Each saved tab can store up to 2 MiB total.
- The sandbox bridge spaces write/delete/clear calls by about 50 ms. Debounce frequent saves.

#### Example

    let saveTimer;
    function scheduleSave(settings) {
      clearTimeout(saveTimer);
      saveTimer = setTimeout(() => {
        Elite.store.set('settings', settings).catch(console.error);
      }, 300);
    }

<span id="m-elite-store-delete-key"></span>

### `Elite.store.delete(key)`

requestPermission: None

Deletes one key from this tab's persistent state.

#### Call

    const result = await Elite.store.delete(key);

#### Parameters

| Name  | Type   | What it means                        |
|-------|--------|--------------------------------------|
| `key` | string | State key. It may already be absent. |

#### Returns

Success result. Deleting a missing key is safe.

#### Errors

- `TAB_STATE_PREVIEW`
- `TAB_STATE_DELETE_FAILED`
- `TAB_STATE_UNAVAILABLE`
- `RATE_LIMITED`

#### Example

    if (confirm('Reset only the calibration data?')) {
      await Elite.store.delete('calibration');
    }

<span id="m-elite-store-clear"></span>

### `Elite.store.clear()`

requestPermission: None

Deletes all persistent state owned by this tab.

#### Call

    const result = await Elite.store.clear();

#### Returns

A success result.

#### Errors

- `TAB_STATE_PREVIEW`
- `TAB_STATE_CLEAR_FAILED`
- `TAB_STATE_UNAVAILABLE`
- `RATE_LIMITED`

#### Example

    document.querySelector('#factory-reset').onclick = async () => {
      if (!confirm('Erase all saved data for this tab?')) return;
      await Elite.store.clear();
      location.reload();
    };

<span id="ns-elite-actions"></span>

## Elite.actions <a href="#contents" class="toplink">Back to contents</a>

Publish and invoke explicit capabilities across saved JACoB tabs without giving tabs direct access to each other.

<span id="m-elite-actions-register-name-handler-options"></span>

### `Elite.actions.register(name, handler, options)`

host-brokered registrationPermission: None

Publishes a named handler that another saved JACoB tab can call. Use this to expose small, deliberate capabilities instead of trying to reach into another iframe.

#### Call

    const unregister = await Elite.actions.register(name, handler, options);

#### Parameters

| Name                  | Type     | What it means                                                                                                                                                                 |
|-----------------------|----------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `name`                | string   | Global action name for this JACoB browser session. 1-96 characters; letters, numbers, dots, colons, underscores, and hyphens. The first character must be a letter or number. |
| `handler`             | function | Called as `handler(payload, context)`. It may return a value or a Promise. `context` contains `{ action, caller }`; caller identifies the invoking tab.                       |
| `options.label`       | string   | Optional human label. JACoB keeps at most 120 characters.                                                                                                                     |
| `options.description` | string   | Optional description. JACoB keeps at most 300 characters.                                                                                                                     |

#### Returns

An async unregister function. Call it to remove the action. The handler result is returned later to callers of `Elite.actions.invoke()`.

#### Errors

- `BAD_PARAMS`
- `ACTION_PREVIEW`
- `ACTION_CONFLICT`
- `RATE_LIMITED`

#### Notes

- Only saved tabs can publish actions. Tab Manager previews cannot register as providers.
- Use a stable namespace such as `carrier.*`, `mining.*`, `neutron.*`, or `race.*`. Action names are global within the browser session.
- Actions are session registrations. Register them again when the tab loads. JACoB clears a tab's registrations when that iframe reloads or is deleted.

#### Example

    const unregister = await Elite.actions.register(
      'carrier.runLoad',
      async ({ load = 0 } = {}, context) => {
        console.log('Called by', context.caller?.tabName);
        await prepareCarrierRun(load);
        return { prepared: true, load };
      },
      {
        label: 'Run carrier load',
        description: 'Prepare and run one Fleet Carrier market load.'
      }
    );

    // Later, if the tool no longer wants to expose the action:
    await unregister();

<span id="m-elite-actions-unregister-name"></span>

### `Elite.actions.unregister(name)`

host-brokered registrationPermission: None

Removes one action published by the calling tab.

#### Call

    const result = await Elite.actions.unregister(name);

#### Parameters

| Name   | Type   | What it means                           |
|--------|--------|-----------------------------------------|
| `name` | string | Name previously registered by this tab. |

#### Returns

`{ unregistered: true, name }`.

#### Errors

- `ACTION_NOT_FOUND`
- `ACTION_NOT_OWNER`
- `RATE_LIMITED`

#### Notes

- A tab may unregister only its own action. The async function returned by `register()` is usually the cleanest way to tear an action down.

#### Example

    await Elite.actions.register('mining.toggleHud', toggleHud);

    // ...later
    await Elite.actions.unregister('mining.toggleHud');

<span id="m-elite-actions-list-options"></span>

### `Elite.actions.list(options)`

host requestPermission: None

Returns the actions currently published by saved JACoB tabs. By default JACoB loads saved tabs as needed so they get a chance to register their actions first.

#### Call

    const result = await Elite.actions.list(options);

#### Parameters

| Name               | Type    | What it means                                                                                                                                  |
|--------------------|---------|------------------------------------------------------------------------------------------------------------------------------------------------|
| `options.discover` | boolean | Default `true`. When true, JACoB performs action discovery before returning. Set false when you only want the registry that is already loaded. |

#### Returns

`{ actions }`, where each action is `{ name, label, description, tabId, tabName }`.

#### Errors

- `RATE_LIMITED`

#### Notes

- Discovery can load hidden saved tabs. Do not call discovery in a fast polling loop; cache the result or refresh it on user request.

#### Example

    const { actions } = await Elite.actions.list();
    for (const action of actions) {
      console.log(action.name, 'from', action.tabName);
    }

    // Fast snapshot of only the actions already registered:
    const current = await Elite.actions.list({ discover: false });

<span id="m-elite-actions-invoke-name-payload-options"></span>

### `Elite.actions.invoke(name, payload, options)`

host requestPermission: Inter-tab control

Invokes one published action and waits for the provider handler to finish. This is the supported way for Touch Deck, dashboards, or one tool tab to ask another tool tab to do work.

#### Call

    const result = await Elite.actions.invoke(name, payload, options);

#### Parameters

| Name                | Type    | What it means                                                                                                        |
|---------------------|---------|----------------------------------------------------------------------------------------------------------------------|
| `name`              | string  | Published action name.                                                                                               |
| `payload`           | any     | Optional structured data passed to the provider handler. Keep this JSON-style/structured-cloneable. Default: `null`. |
| `options.timeoutMs` | number  | Default: 15,000 ms. JACoB clamps the effective timeout to 500-60,000 ms.                                             |
| `options.discover`  | boolean | Default `true`. If the action is not already registered, JACoB can load saved tabs and retry discovery.              |

#### Returns

Whatever the provider handler returns. If the handler returns a Promise, JACoB waits for it.

#### Errors

- `PERMISSION_DENIED`
- `BAD_PARAMS`
- `ACTION_NOT_FOUND`
- `ACTION_TARGET_RELOADED`
- `ACTION_TIMEOUT`
- `ACTION_FAILED`
- `RATE_LIMITED`

#### Notes

- The caller needs the **Control other JACoB tabs** permission. The provider still needs its own normal permissions for input, overlay, network, recorder, video, or vision work.
- If the provider throws an error carrying its own `code`, that code can be forwarded to the caller. Otherwise the broker uses `ACTION_FAILED`.
- Reloading or deleting the provider rejects outstanding calls with `ACTION_TARGET_RELOADED`.

#### Example

    try {
      const result = await Elite.actions.invoke(
        'carrier.runLoad',
        { load: 0 },
        { timeoutMs: 30000 }
      );
      console.log('Carrier tab returned', result);
    } catch (err) {
      if (err.code === 'ACTION_NOT_FOUND') {
        console.log('Carrier tool is not installed or did not publish the action.');
      } else {
        throw err;
      }
    }

<span id="ns-elite-tabs"></span>

## Elite.tabs <a href="#contents" class="toplink">Back to contents</a>

Inspect JACoB navigation and bring a page to the foreground without bypassing the tab sandbox.

<span id="m-elite-tabs-list"></span>

### `Elite.tabs.list()`

host requestPermission: None

Returns the navigation entries available in the current JACoB browser, including default pages and saved custom tabs.

#### Call

    const result = await Elite.tabs.list();

#### Returns

`{ tabs }`. Each record contains `{ id, navId, name, type, hidden, active, loaded }`. For custom tabs, `id` is the saved-tab ID and `navId` is the shell navigation ID.

#### Errors

- `RATE_LIMITED`

#### Notes

- `type` is `custom` or `default`. `loaded` tells you whether a custom tab iframe is currently SDK-ready.
- Hidden tabs are still returned. Hiding a tab does not delete its state or prevent an already-loaded tab from running in the background.

#### Example

    const { tabs } = await Elite.tabs.list();
    const carrier = tabs.find(tab => tab.name === 'Carrier Market Orders');
    if (carrier) {
      console.log(carrier.id, carrier.hidden, carrier.loaded);
    }

<span id="m-elite-tabs-activate-target"></span>

### `Elite.tabs.activate(target)`

host requestPermission: None

Displays a JACoB page in the current browser. Use this when a controller wants to bring the user to the tool that owns an action or dataset.

#### Call

    const result = await Elite.tabs.activate(target);

#### Parameters

| Name     | Type   | What it means                                                                                                             |
|----------|--------|---------------------------------------------------------------------------------------------------------------------------|
| `target` | string | A saved-tab ID, navigation ID, saved-tab name, default-page ID, or default-page label. Name matching is case-insensitive. |

#### Returns

`{ id, name, hidden }`, where `id` is the resolved shell navigation ID.

#### Errors

- `TAB_NOT_FOUND`
- `RATE_LIMITED`

#### Notes

- JACoB loads a saved custom tab before activating it.
- Activating a hidden tab displays it but does not unhide its navigation item.
- Activation only changes the page shown in this browser. Use `Elite.actions.invoke()` when you need the target tab to perform work.

#### Example

    const { tabs } = await Elite.tabs.list();
    const mining = tabs.find(tab => tab.name === 'Ring Mining Companion');

    if (mining) {
      await Elite.tabs.activate(mining.id);
    } else {
      await Elite.tabs.activate('Tab Manager');
    }

<span id="ns-elite-locale"></span>

## Elite.locale <a href="#contents" class="toplink">Back to contents</a>

Read the host language and translate strings owned by your tab.

<span id="m-elite-locale-language"></span>

### `Elite.locale.language`

local propertyPermission: None

Returns the tab's currently cached JACoB language code immediately.

#### Call

    const language = Elite.locale.language;

#### Returns

A locale code such as `en`, `de`, `fr`, `ru`, `zh-CN`, or `es`.

#### Example

    document.documentElement.lang = Elite.locale.language;
    console.log('Current JACoB language:', Elite.locale.language);

<span id="m-elite-locale-supported"></span>

### `Elite.locale.supported`

local propertyPermission: None

Returns a copy of the languages currently advertised by the host.

#### Call

    const languages = Elite.locale.supported;

#### Returns

An array of objects such as `{ code, name, nativeName }`.

#### Example

    for (const lang of Elite.locale.supported) {
      console.log(`${lang.code}: ${lang.nativeName || lang.name}`);
    }

<span id="m-elite-locale-get"></span>

### `Elite.locale.get()`

requestPermission: None

Fetches the current host language and supported-language list directly from JACoB.

#### Call

    const info = await Elite.locale.get();

#### Returns

`{ language, supported }`.

#### Errors

- `OFFLINE`
- `TIMEOUT`

#### Example

    const info = await Elite.locale.get();
    document.documentElement.lang = info.language;
    console.log(info.supported);

<span id="m-elite-locale-t-dictionary-key-vars"></span>

### `Elite.locale.t(dictionary, key, vars)`

local helperPermission: None

Resolves a tab-owned string for the active JACoB language, with English fallback and `{name}`-style interpolation.

#### Call

    const text = Elite.locale.t(dictionary, key, vars);

#### Parameters

| Name         | Type   | What it means                                                     |
|--------------|--------|-------------------------------------------------------------------|
| `dictionary` | object | Nested dictionary keyed first by locale code, then by string key. |
| `key`        | string | String key to resolve.                                            |
| `vars`       | object | Optional interpolation values.                                    |

#### Returns

Translated/interpolated string. Fallback order is active language → English → the key itself.

#### Example

    const strings = {
      en: { load: 'Load {count} orders' },
      de: { load: '{count} Aufträge laden' },
      es: { load: 'Cargar {count} órdenes' }
    };
    button.textContent = Elite.locale.t(strings, 'load', { count: 12 });

<span id="m-elite-locale-subscribe-callback"></span>

### `Elite.locale.subscribe(callback)`

local event subscriptionPermission: None

Runs your callback once with the current locale, then again whenever JACoB's language changes.

#### Call

    const off = Elite.locale.subscribe(info => { ... });

#### Parameters

| Name       | Type     | What it means                       |
|------------|----------|-------------------------------------|
| `callback` | function | Receives `{ language, supported }`. |

#### Returns

An unsubscribe function.

#### Example

    const off = Elite.locale.subscribe(info => {
      document.documentElement.lang = info.language;
      renderTranslatedUI();
    });

    // off();

<span id="ns-elite-files"></span>

## Elite.files <a href="#contents" class="toplink">Back to contents</a>

Export generated files through the browser download flow.

<span id="m-elite-files-download-name-data-options"></span>

### `Elite.files.download(name, data, options)`

local browser helperPermission: None

Starts a browser download for generated content. It does not give the tab general filesystem access.

#### Call

    const info = Elite.files.download(name, data, options);

#### Parameters

| Name              | Type                    | What it means                                                                  |
|-------------------|-------------------------|--------------------------------------------------------------------------------|
| `name`            | string                  | Filename to suggest to the browser. JACoB replaces unsafe filename characters. |
| `data`            | string \| Blob \| value | Strings and Blobs are used directly. Other values are JSON-serialized.         |
| `options.type`    | string                  | Optional MIME type. Default: UTF-8 `text/plain`.                               |
| `options.compact` | boolean                 | For JSON output, omit pretty indentation when true.                            |

#### Returns

`{ name, bytes, type }`. This helper returns synchronously in the tab.

#### Example

    const csv = 'system,jumps\nSol,1\nAchenar,2\n';
    const info = Elite.files.download('route.csv', csv, { type: 'text/csv;charset=utf-8' });
    console.log(`Prepared ${info.bytes} bytes`);

<span id="m-elite-files-json-name-value-compact"></span>

### `Elite.files.json(name, value, compact)`

local browser helperPermission: None

Downloads a JavaScript value as `application/json`.

#### Call

    const info = Elite.files.json(name, value, compact);

#### Parameters

| Name      | Type              | What it means                              |
|-----------|-------------------|--------------------------------------------|
| `name`    | string            | Filename to suggest for the JSON download. |
| `value`   | JSON-serializable | JSON-serializable value to encode.         |
| `compact` | boolean           | When true, omit pretty indentation.        |

#### Returns

Same `{ name, bytes, type }` result as `files.download()`.

#### Example

    Elite.files.json('mining-session.json', {
      exportedAt: new Date().toISOString(),
      rocks: session.rocks,
      tons: session.tons
    });

<span id="data-reference"></span>

## 7. Elite data available to tabs <a href="#contents" class="toplink">Back to contents</a>

### 7.1 State snapshot

`Elite.state.get()` is the broad startup snapshot. Read the fields your tool needs and ignore fields you do not recognize; Elite and JACoB can add fields without breaking a well-behaved tab.

    {
      "journalDir": "...",
      "journalFile": "Journal....log",
      "status": { "event":"Status", "Latitude":12.345, "Longitude":-67.89, "Heading":142 },
      "market": { "event":"Market", "MarketID":3700005632, "Items":[] },
      "eliteFiles": {
        "cargo": { "event":"Cargo", "Inventory":[] },
        "navRoute": { "event":"Route", "Route":[] },
        "modulesInfo": { "event":"ModuleInfo", "Modules":[] }
      },
      "lastJournalEvent": { "event":"FSDJump", "StarSystem":"Sol" },
      "journalContext": {
        "currentSystem":"Sol",
        "maxJumpRange":65.43,
        "ship":"krait_light",
        "shipName":"Wayfarer",
        "shipIdent":"NH-01",
        "carrierCallsign":"ABC-123",
        "carrierFreeSpace":8800,
        "carrierAvailableBalance":123456789
      }
    }

### 7.2 Recovered journal context

When JACoB attaches, it rebuilds useful context from the current journal session. Your tab does not need to wait for another login, loadout, or jump just to learn where the player is or what ship/carrier context is active. JACoB keeps that context current from events including `Location`, `FSDJump`, `CarrierJump`, `Loadout`, and `CarrierStats`.

### 7.3 Companion JSON names

Stable names include `status`, `market`, `outfitting`, `shipyard`, `modulesInfo`, `cargo`, `navRoute`, `backpack`, `shipLocker`, and `fcMaterials`. If Elite adds another JSON snapshot in the journal directory, JACoB can surface it automatically under a normalized name.

### 7.4 Events JACoB relays to tabs

| Event                | Typical payload / use                                                                                                                                                                                                                                                                        |
|----------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `core.hello`         | Sanitized core identity, build/API version, platform/capabilities, LAN status, health summary, locale, update/network availability, and optional-service capability data. Pair/media tokens, host name, journal paths, and binding paths are stripped before the event reaches a custom tab. |
| `core.journalFile`   | Metadata for a journal-file transition.                                                                                                                                                                                                                                                      |
| `state`              | Initial or current broad watcher snapshot.                                                                                                                                                                                                                                                   |
| `status`             | Parsed `Status.json` object.                                                                                                                                                                                                                                                                 |
| `market`             | `Market.json` compatibility payload.                                                                                                                                                                                                                                                         |
| `eliteFile`          | Companion-file change with name, filename, updated time, availability, and data.                                                                                                                                                                                                             |
| `journal`            | One parsed Elite journal event.                                                                                                                                                                                                                                                              |
| `tabs.changed`       | The host custom-tab manifest or layout changed.                                                                                                                                                                                                                                              |
| `appearance.changed` | The host appearance changed.                                                                                                                                                                                                                                                                 |
| `locale.changed`     | The host language changed.                                                                                                                                                                                                                                                                   |
| `recorder.input`     | One enriched physical keyboard transition.                                                                                                                                                                                                                                                   |
| `core.update`        | Host update lifecycle information.                                                                                                                                                                                                                                                           |

### 7.5 Recorder event shape

    {
      "pressId": 12,
      "type": "up",
      "key": "GRAVE",
      "physical": "SC:29",
      "scanCode": 41,
      "virtualKey": 192,
      "extended": false,
      "localizedName": "`",
      "modifiers": [],
      "atMs": 1835,
      "deltaMs": 87,
      "durationMs": 142,
      "isModifier": false,
      "matches": [ { "action":"MoveFreeCamRight", "slot":"primary" } ]
    }

For recorder data that needs to survive different keyboard layouts, store `physical` when it is present. Keep `key` for legacy/semantic handling and use `localizedName` for the label shown to the user.

### 7.6 core.hello in SDK 10

The host sanitizes `core.hello` before it crosses into a custom tab. Treat the object as capability metadata, not as a filesystem or authentication API.

    {
      "prototype": "Alpha 0.2.13 SDK10",
      "version": "0.2.13-alpha",
      "product": "JACoB",
      "name": "Journal Aligned Control Bridge",
      "apiVersion": 10,
      "os": "windows",
      "arch": "amd64",
      "goRuntime": "go1.x",
      "uptimeSeconds": 120,
      "localClient": false,
      "input": { "enabled": true, "available": true, "driver": "windows-sendinput-scancode" },
      "recorder": {},
      "capture": {},
      "vision": {},
      "overlay": {},
      "lan": {},
      "health": { "status": "ok", "blockers": [], "warnings": [], "gameRunning": true },
      "locale": { "language": "en", "supported": [] },
      "updates": { "repository": "pseudo6626/JACoB", "checkAvailable": true, "installAvailable": true },
      "network": { "fetchAvailable": true, "publicHTTPOnly": true },
      "customTabs": { "available": true }
    }

Fields can grow. Read only the fields your tab needs and tolerate additional fields.

<span id="overlay-reference"></span>

## 8. HUD overlay scenes <a href="#contents" class="toplink">Back to contents</a>

`Elite.overlay.set(scene)` replaces this tab's entire HUD layer. Build the scene you want on screen right now and send that complete scene. Overlay items are not persistent DOM nodes.

| Scene field       | What it means                                    |
|-------------------|--------------------------------------------------|
| `z`               | Layer ordering hint.                             |
| `space`           | `normalized` (default) or `pixels`.              |
| `width`, `height` | Optional scene dimensions for pixel-space logic. |
| `items`           | Array of up to 2000 draw items.                  |

| Item type | Useful fields                      |
|-----------|------------------------------------|
| `text`    | x, y, text, color, fontSize, align |
| line      | x, y, x2, y2, stroke, lineWidth    |
| polyline  | points\[\], stroke, lineWidth      |
| polygon   | points\[\], stroke/fill, lineWidth |
| rect      | x, y, w, h, stroke/fill, lineWidth |
| circle    | x, y, r, stroke/fill, lineWidth    |

Colors accept `#RGB`, `#RRGGBB`, `#RRGGBBAA`, `transparent`, and `none`. Default line width is 2 (maximum 64). Default font size is 18 (maximum 256). Text alignment is left, center, or right. One polyline or polygon can contain up to 10,000 points.

    function miningHud(percent, tph, crh) {
      const hot = percent >= 40;
      return {
        space: 'normalized',
        z: 30,
        items: [
          { type:'rect', x:0.015, y:0.77, w:0.18, h:0.115,
            fill:'#050607CC', stroke: hot ? '#ff7b00' : '#777777', lineWidth:2 },
          { type:'text', x:0.03, y:0.80, text:`ROCK ${percent.toFixed(1)}%`,
            color: hot ? '#ffb36b' : '#dddddd', fontSize:22 },
          { type:'text', x:0.03, y:0.84, text:`${tph.toFixed(1)} t/h  ${Math.round(crh/1e6)} MCr/h`,
            color:'#ffffff', fontSize:16 }
        ]
      };
    }

    await Elite.overlay.set(miningHud(42.7, 189.2, 241_000_000));

<span id="patterns"></span>

## 9. Automation patterns that hold up <a href="#contents" class="toplink">Back to contents</a>

### 9.1 Initialize once, then subscribe

    const initial = await Elite.state.get();
    let currentStatus = initial.status;

    const offStatus = Elite.state.subscribe(status => {
      currentStatus = status;
      render();
    });

### 9.2 Wait for journal events with a timeout

    function waitForJournal(name, predicate = () => true, timeoutMs = 15000) {
      return new Promise((resolve, reject) => {
        let done = false;
        const off = Elite.journal.subscribe(name, event => {
          if (done || !predicate(event)) return;
          done = true;
          clearTimeout(timer);
          off();
          resolve(event);
        });
        const timer = setTimeout(() => {
          if (done) return;
          done = true;
          off();
          reject(new Error(`Timed out waiting for ${name}`));
        }, timeoutMs);
      });
    }

    await Elite.bindings.press('GalaxyMapOpen');
    const jump = await waitForJournal('FSDJump');
    console.log('Confirmed by Elite:', jump.StarSystem);

### 9.3 Make long runs cancelable

    let runToken = 0;

    async function startRun() {
      const token = ++runToken;
      for (const target of targets) {
        if (token !== runToken) return;        // This run was canceled.
        await plot(target);
        if (token !== runToken) return;
        await waitForJournal('FSDJump', e => e.StarSystem === target, 120000);
      }
    }

    function abortRun() {
      runToken++;                              // Cancel the current sequence.
      Elite.overlay.clear().catch(() => {});
    }

### 9.4 Use closed-loop control

When Elite tells you what happened, use that feedback. Do not guess from timing alone. Carrier Market Orders is the clean example: send an input, wait for the relevant journal or companion-file update, measure the actual result, then decide the next input.

    async function adjustUntil(target) {
      for (let attempt = 0; attempt < 20; attempt++) {
        const before = latestValue;
        await Elite.bindings.press(before < target ? 'UI_Right' : 'UI_Left');
        await waitForJournal('CarrierTradeOrder', e => e.Price !== before, 5000);
        if (latestValue === target) return;
      }
      throw new Error('Did not converge');
    }

### 9.5 Always release long holds

    await Elite.bindings.down('UI_Right');
    try {
      await doCancelableWork();
    } finally {
      await Elite.bindings.up('UI_Right').catch(() => {});
    }

### 9.6 Debounce storage and HUD updates

Do not save state or redraw the HUD for every tiny intermediate change. Debounce writes, save meaningful checkpoints, and update the overlay when something the player can see actually changed.

### 9.7 Publish capabilities, not internals

When tabs need to cooperate, expose one stable action for the job instead of mirroring another tab's UI state. Keep the payload small, return a clear result, and let the provider own its own permissions and automation details.

    // Provider owns the workflow.
    await Elite.actions.register('neutron.plotNext', async ({ system }) => {
      await plotNextJump(system);
      return { plotted: true, system };
    });

    // Controller only knows the published contract.
    await Elite.actions.invoke('neutron.plotNext', { system: 'Jackson's Lighthouse' });

<span id="direct-protocol"></span>

## 10. Direct WebSocket and host protocol <a href="#contents" class="toplink">Back to contents</a>

> **If you are writing a normal tab, stay on `Elite.*`.** The direct protocol is here for debugging, external clients, and host tooling. Some core methods are intentionally unavailable inside sandboxed tabs.

JACoB 0.2.13 binds new instances to `6626` by default. The WebSocket endpoint is `ws://HOST:6626/ws`. Loopback clients do not need a pairing token. LAN clients include the pairing token in the query string. `JACOB_BIND` can override the bind address/port. Ordinary tabs should never hard-code a port because they use the injected SDK.

    // Request
    {"type":"request","id":"req-1","method":"binding.press","params":{"action":"UI_Select"}}

    // Success
    {"type":"response","id":"req-1","ok":true,"result":{}}

    // Error
    {"type":"response","id":"req-1","ok":false,"error":{"code":"...","message":"..."}}

    // Event
    {"type":"event","event":"journal","data":{"event":"FSDJump","StarSystem":"Sol"}}

### Core request methods in 0.2.13

The Go core currently recognizes these direct WebSocket request methods. Some require loopback access or parameters that the trusted host normally supplies for a custom tab.

    core.ping
    core.shutdown
    state.get
    system.health
    network.diagnostics
    network.repair
    locale.get
    locale.save
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
    vision.info
    vision.sample
    vision.inspect
    vision.calibration.arm
    vision.calibration.status
    net.fetch
    elitefiles.list
    elitefiles.get
    journal.files
    journal.read
    tabstate.get
    tabstate.set
    tabstate.delete
    tabstate.clear

`network.diagnostics` and `network.repair` are host-only Network Doctor operations. The calibration arm/status methods are implementation plumbing for JACoB’s host-owned Vision picker; normal tabs call `Elite.vision.calibrate()` or `configure()` instead.

> **SDK 9 host-brokered calls:** `Elite.actions.*`, `Elite.tabs.list()`, `Elite.tabs.activate()`, and `Elite.video.url()` are handled by the trusted browser host rather than sent straight through as core WebSocket methods. `Elite.files.*`, locale translation, and subscription helpers are implemented locally in the injected SDK.

### Host-only core methods

| Method family                           | Why it is host-only / special                                                                                                                                                                                   |
|-----------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `core.shutdown`                         | Only the loopback host UI can shut down the core.                                                                                                                                                               |
| `locale.save`                           | JACoB Settings owns the global language. Tabs get read-only locale access.                                                                                                                                      |
| `appearance.get/save/reset`             | Changes the privileged host-shell appearance.                                                                                                                                                                   |
| `tabs.list/get/save/delete/layout.save` | Installed-tab mutation and navigation layout are Tab Manager concerns. SDK 9 separately exposes read-only navigation discovery through `Elite.tabs.list()` and page activation through `Elite.tabs.activate()`. |
| `bindings.autofill`                     | Can modify Elite binding files, so host-local and game-closed safety rules apply.                                                                                                                               |
| `network.diagnostics / network.repair`  | Network Doctor is host-local. It inspects listener/bind/profile/firewall state and can request Windows firewall repair; custom tabs do not receive this as an `Elite.*` namespace.                              |
| `update.check/install`                  | Controls the application update lifecycle.                                                                                                                                                                      |
| `tabstate.*`                            | The SDK supplies the calling saved-tab ID; tabs do not call the underlying core methods directly.                                                                                                               |
| `overlay.set/clear`                     | The host supplies the calling tab's private overlay-layer ID.                                                                                                                                                   |
| `video.url`                             | The trusted host resolves this so media authentication is correct for the current browser.                                                                                                                      |

### HTTP endpoints

    GET http://HOST:6626/api/health
    GET http://HOST:6626/api/video/frame.jpg?width=960&quality=60
    GET http://HOST:6626/api/video.mjpeg?width=960&fps=5&quality=60

LAN media requests use JACoB's scoped media token. A custom tab with Video permission receives a prepared URL; it does not receive the control-capable LAN pairing token.

<span id="errors"></span>

## 11. Error handling <a href="#contents" class="toplink">Back to contents</a>

Use `err.code` for program logic. Show or log `err.message` for the human. Do not branch on message text; wording can change while the code remains stable.

### Bridge and permissions

`OFFLINE`, `TIMEOUT`, `SDK_DENIED`, `PERMISSION_DENIED`, `RATE_LIMITED`, `BAD_MESSAGE`, `BAD_JSON`, `BAD_PARAMS`, `NO_SUCH_METHOD`

### Input and bindings

`INPUT_DISABLED`, `INPUT_UNAVAILABLE`, `INPUT_FAILED`, `GAME_FOCUS_FAILED`, `TEXT_INPUT_FAILED`, `BINDING_NOT_FOUND`, `BINDING_PRESS_FAILED`, `BINDING_HOLD_FAILED`, `BINDING_DOWN_FAILED`, `BINDING_UP_FAILED`, `BINDINGS_RELOAD_FAILED`

### Recorder, HUD, and vision

`RECORDER_UNAVAILABLE`, `RECORDER_START_FAILED`, `RECORDER_STOP_FAILED`, `OVERLAY_UNAVAILABLE`, `OVERLAY_SCENE_INVALID`, `OVERLAY_RENDER_FAILED`, `OVERLAY_CLEAR_FAILED`, `VISION_UNAVAILABLE`, `VISION_SAMPLE_FAILED`

### Journal, network, and store

`JOURNAL_READ_FAILED`, `NET_FETCH_UNAVAILABLE`, `NET_FETCH_FAILED`, `TAB_STATE_PREVIEW`, `TAB_STATE_UNAVAILABLE`, `TAB_STATE_SAVE_FAILED`, `TAB_STATE_DELETE_FAILED`, `TAB_STATE_CLEAR_FAILED`

### Cross-tab actions and navigation

`ACTION_PREVIEW`, `ACTION_CONFLICT`, `ACTION_NOT_FOUND`, `ACTION_NOT_OWNER`, `ACTION_TARGET_RELOADED`, `ACTION_TIMEOUT`, `ACTION_FAILED`, `TAB_NOT_FOUND`

A provider can also throw an error with its own machine-readable `code`. The action broker forwards that code to the caller when possible.

### Host-only errors

`LOCAL_ONLY`, `GAME_RUNNING`, `BINDINGS_PREFLIGHT_FAILED`, `BINDINGS_AUTOFILL_FAILED`, `LOCALE_UNAVAILABLE`, `BAD_LANGUAGE`, `TAB_STORE_UNAVAILABLE`, `TAB_NOT_FOUND`, `TAB_SAVE_FAILED`, `TAB_DELETE_FAILED`, `TAB_LAYOUT_SAVE_FAILED`, `APPEARANCE_UNAVAILABLE`, `APPEARANCE_SAVE_FAILED`, `APPEARANCE_RESET_FAILED`, `NO_UPDATE`, `UPDATE_CHECK_FAILED`, `UPDATE_DOWNLOAD_FAILED`, `UPDATE_INSTALL_FAILED`, `UPDATE_LAUNCH_FAILED`, `UPDATE_NOT_INSTALLABLE`

### Recommended error wrapper

    async function jacobCall(label, fn) {
      try {
        return await fn();
      } catch (err) {
        const code = err?.code || 'ERROR';
        console.error(`${label}: ${code}: ${err?.message || err}`);
        if (code === 'PERMISSION_DENIED') showPermissionHelp();
        if (code === 'OFFLINE') showDisconnected();
        throw err;
      }
    }

    await jacobCall('Open Galaxy Map', () => Elite.bindings.press('GalaxyMapOpen'));
    VISION_UNAVAILABLE
    VISION_SAMPLE_FAILED
    VISION_INSPECT_FAILED
    VISION_CALIBRATION_BUSY
    VISION_CALIBRATION_FAILED
    VISION_CALIBRATION_TIMEOUT
    RATE_LIMITED
    PERMISSION_DENIED

<span id="platforms"></span>

## 12. Platform support <a href="#contents" class="toplink">Back to contents</a>

> **SDK 10 capture rule:** Game View, Vision, OCR, and calibration share one canonical Elite-client frame contract. A platform backend may change, but it may not substitute monitor/desktop capture when it cannot safely isolate Elite. The No Capture build reports these features unavailable by design and also omits the keyboard recorder.

| Capability                        | Windows                                             | Steam Deck / Linux                                                |
|-----------------------------------|-----------------------------------------------------|-------------------------------------------------------------------|
| Journal / Status / companion JSON | Yes                                                 | Yes                                                               |
| Binding parser / semantic actions | Yes                                                 | Yes                                                               |
| Raw + semantic input              | Scan-code SendInput                                 | /dev/uinput path when available                                   |
| SDK 8 physical SC tokens          | Native set-1 scan-code identity                     | Translated to evdev key codes where mapped                        |
| Text entry                        | Active layout + Unicode-aware path                  | Platform text/uinput path where supported                         |
| Recorder                          | Optional Windows component; foreground-scoped       | May report unavailable depending on validated scoped capture path |
| Overlay                           | Topmost non-activating click-through layered window | X11/XWayland / Gamescope path when available                      |
| Video / derived vision            | Elite-window capture, foreground-gated              | Platform-dependent capture capability                             |
| LAN web UI / tab model            | Yes                                                 | Yes                                                               |

Feature-detect optional native services. Check `overlay.info()`, `video.info()`, `vision.info()`, and `recorder.status()` before enabling controls that depend on them. A missing overlay or recorder should not stop the rest of the tab from working.

<span id="complete-examples"></span>

## 13. Full tab examples <a href="#contents" class="toplink">Back to contents</a>

### 13.1 Journal + state dashboard

    <!doctype html>
    <html><body>
    <h3 id="system">System ?</h3>
    <div>Heading: <span id="heading">—</span></div>
    <div>Last event: <span id="event">—</span></div>
    <script>
    (async () => {
      const system = document.querySelector('#system');
      const heading = document.querySelector('#heading');
      const eventEl = document.querySelector('#event');

      const initial = await Elite.state.get();
      system.textContent = initial.journalContext?.currentSystem || 'System ?';
      if (Number.isFinite(initial.status?.Heading)) heading.textContent = Math.round(initial.status.Heading) + '°';

      Elite.state.subscribe(s => {
        if (Number.isFinite(s.Heading)) heading.textContent = Math.round(s.Heading) + '°';
      });
      Elite.journal.subscribe('*', e => {
        eventEl.textContent = e.event || 'journal';
        if (e.event === 'FSDJump' || e.event === 'Location') system.textContent = e.StarSystem || system.textContent;
      });
    })();
    </script>
    </body></html>

### 13.2 Persistent, localized public-API tool

    const strings = {
      en: { ready:'Ready', fetching:'Fetching…', saved:'Saved {count} systems' },
      de: { ready:'Bereit', fetching:'Lade…', saved:'{count} Systeme gespeichert' }
    };
    let systems = await Elite.store.get('systems', []);

    function tr(key, vars={}) { return Elite.locale.t(strings, key, vars); }
    function render() { status.textContent = tr('saved', { count: systems.length }); }
    Elite.locale.subscribe(render);

    async function fetchSystem(name) {
      status.textContent = tr('fetching');
      const r = await Elite.net.fetch(`https://example.org/api/system?q=${encodeURIComponent(name)}`);
      if (r.status !== 200) throw new Error(`HTTP ${r.status}`);
      systems.push(r.json);
      await Elite.store.set('systems', systems);
      render();
    }

    exportButton.onclick = () => Elite.files.json('systems.json', systems);

### 13.3 Live telemetry HUD with clean shutdown

    let enabled = false;
    let lastStatus = null;

    const off = Elite.state.subscribe(status => {
      lastStatus = status;
      if (!enabled) return;
      drawHud().catch(console.error);
    });

    async function drawHud() {
      if (!lastStatus) return;
      await Elite.overlay.set({
        space:'normalized',
        items:[
          {type:'text', x:0.03, y:0.08,
           text:`HDG ${Math.round(lastStatus.Heading || 0)}`,
           color:'#ff7b00', fontSize:20}
        ]
      });
    }

    async function start() {
      const info = await Elite.overlay.info();
      if (!info.available) throw new Error('Overlay unavailable');
      enabled = true;
      await drawHud();
    }

    async function stop() {
      enabled = false;
      await Elite.overlay.clear();
    }

    // If the tab has an explicit cleanup path:
    // off();

### 13.4 Closed-loop Galaxy Map plotter

    const sleep = ms => new Promise(r => setTimeout(r, ms));

    async function plotSystem(name) {
      const health = await Elite.system.health();
      if (health.status === 'blocked') throw new Error(health.blockers.join('; '));

      // Clear any stray Elite submenus first.
      for (let i = 0; i < 5; i++) {
        await Elite.input.tap('ESC');
        await sleep(80);
      }

      await Elite.bindings.press('GalaxyMapOpen');
      await sleep(700);
      await Elite.input.text(name, { intervalMs:18 });
      await sleep(250);
      await Elite.bindings.press('UI_Select');
    }

    function waitForJump(target, timeoutMs=120000) {
      return new Promise((resolve, reject) => {
        let done = false;
        const off = Elite.journal.subscribe('FSDJump', e => {
          if (done || e.StarSystem !== target) return;
          done = true; clearTimeout(timer); off(); resolve(e);
        });
        const timer = setTimeout(() => {
          if (done) return;
          done = true; off(); reject(new Error(`No jump to ${target}`));
        }, timeoutMs);
      });
    }

    await plotSystem('Shinrarta Dezhra');
    const confirmed = await waitForJump('Shinrarta Dezhra');
    console.log('Game confirmed arrival:', confirmed.timestamp);

### 13.5 Cross-tab provider + controller

The provider owns the actual workflow. The controller discovers the public action, invokes it, then optionally brings the provider tab to the foreground.

    // PROVIDER TAB: Carrier Market Orders
    const stopRunLoad = await Elite.actions.register(
      'carrier.runLoad',
      async ({ load = 0 } = {}, context) => {
        console.log('Request from', context.caller?.tabName);
        const result = await runCarrierLoad(load);
        return { ok: true, result };
      },
      {
        label: 'Run carrier load',
        description: 'Run one configured carrier market load.'
      }
    );

    // CONTROLLER TAB: Touch Deck
    async function runCarrier() {
      const { actions } = await Elite.actions.list();
      const action = actions.find(a => a.name === 'carrier.runLoad');
      if (!action) throw new Error('Carrier tab did not publish carrier.runLoad');

      const result = await Elite.actions.invoke(
        action.name,
        { load: 0 },
        { timeoutMs: 30000 }
      );

      await Elite.tabs.activate(action.tabId);
      return result;
    }

> **Keep the contract narrow:** Touch Deck does not need to know which keys the carrier tab presses, how it reads journal feedback, or which permissions it uses. It only needs the action name, payload, and returned result.

<span id="example-sdk10-vision"></span>

### SDK 10 Vision: calibrate once, inspect repeatedly

This pattern is the normal starting point for a tab that needs to recognize one Elite UI region. JACoB owns the calibration screenshot and the tab stores only the normalized configuration.

    const CONFIG_ID = 'prospector-result-v1';
    let cfg;

    async function setupVision(force = false) {
      cfg = await Elite.vision.configure(CONFIG_ID, {
        force,
        calibrate: true,
        pickColor: true,
        averageSize: 11,
        label: 'Prospector result region'
      });
      return cfg;
    }

    async function inspectProspector() {
      if (!cfg) await setupVision();

      const r = await Elite.vision.inspect({
        region: cfg.region,
        maxWidth: 1280,
        operations: {
          highlight: {
            type: 'colorPresent',
            color: cfg.color,
            tolerance: 60,
            minimumCoverage: 0.02
          },
          text: {
            type: 'textLines',
            preprocess: { scale: 2, grayscale: true, autoContrast: true }
          }
        }
      });

      if (r.blanked) return null;
      return {
        highlighted: !!r.results.highlight?.matched,
        text: r.results.text?.text || '',
        lines: r.results.text?.lines || []
      };
    }

    async function showCalibration() {
      if (!cfg) await setupVision();
      await Elite.vision.debug.show({ region: cfg.region, label: 'PROSPECTOR' });
    }

    async function recalibrate() {
      await Elite.vision.reset(CONFIG_ID);
      return setupVision(true);
    }

> **Do not poll as fast as JavaScript can run.** Vision is real capture/OCR work. Pick an interval that matches the game UI you are watching, handle `RATE_LIMITED`, and stop polling when the feature is not active.

<span id="glossary"></span>

## 14. Glossary <a href="#contents" class="toplink">Back to contents</a>

| Term              | Plain-language meaning                                                                                                                 |
|-------------------|----------------------------------------------------------------------------------------------------------------------------------------|
| SDK               | The JavaScript surface JACoB injects into a tab: `Elite.*`.                                                                            |
| API               | A defined contract of calls, inputs, outputs, events, and errors.                                                                      |
| Promise           | A JavaScript object for a result that will arrive later. Most tab code consumes it with `await`.                                       |
| Event             | A message JACoB pushes to the tab when something happens.                                                                              |
| Subscription      | A callback registered for future events. JACoB subscriptions return a function that stops the subscription.                            |
| Journal           | Elite Dangerous append-only event log. Use it for discrete facts such as jumps, scans, docking, mining events, and trade orders.       |
| Status.json       | Frequently updated Elite state snapshot. Use it for current heading, flags, latitude/longitude, and mode/state.                        |
| Companion JSON    | Other Elite snapshot files such as `Cargo.json`, `NavRoute.json`, `Market.json`, and `ModulesInfo.json`.                               |
| Semantic binding  | An Elite action name such as `UI_Select`, resolved through the user's active `.binds` preset.                                          |
| Raw input         | A specific keyboard key or chord, independent of Elite action names.                                                                   |
| Physical SC token | SDK 8+ layout-independent PC/AT set-1 scan-code identity such as `SC:29`.                                                              |
| Sandbox           | Browser isolation that lets tab JavaScript run without direct filesystem, OS, or privileged-core access.                               |
| WebSocket         | Persistent two-way connection used by the trusted host page and Go core for requests and events.                                       |
| Overlay layer     | One tab's independent set of native HUD draw items.                                                                                    |
| Closed loop       | Automation that reads Elite feedback after an action and chooses the next step from what actually happened.                            |
| Action broker     | SDK 9 host service that lets a saved tab publish named actions and lets another tab invoke those actions without direct iframe access. |

Source baseline: `pseudo6626/JACoB` `main` at commit `54a6ef432bce`, JACoB Alpha 0.2.13 / SDK 10, reviewed 10 October 2026. This guide checks the public tab injection and host broker against the Go core, not only the shipped Markdown reference. That matters because the source is authoritative when a reference example or namespace list lags the implementation.
