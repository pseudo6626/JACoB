# JACoB SDK14 — Linux / Steam Deck platform contract

SDK14 consolidates the Steam Deck fixes into the core so custom tabs do not need their own OS-specific workarounds.

## Input safety

`binding.down` records the chord JACoB injected. A later `binding.up` for that action is always allowed to release that recorded chord, even if Steam/Gamescope changed focus after the key-down.

New SDK calls:

```js
await Elite.bindings.releaseAll();
await Elite.input.releaseAll();
```

JACoB also releases tracked holds when a WebSocket client disconnects and during core shutdown.

## Platform capability profile

```js
const caps = await Elite.system.capabilities();
const perf = await Elite.system.performance();
```

`system.capabilities()` reports input, capture, Vision/OCR, Spatial, HUD, recorder and capture-cache diagnostics. `system.performance()` provides platform-aware recommended rates. Steam Deck defaults are 5 Hz Spatial/Game View, 4 Hz color Vision, 2 Hz OCR and 10 Hz HUD.

## Linux capture / Vision

Linux capture resolves the actual Elite client across Gamescope's XWayland displays, excludes JACoB's own HUD process/window, and prefers XComposite. Live captures are cached for 100 ms so Game View, Spatial and Vision can reuse the same verified frame. Calibration requests always capture fresh.

Vision inspection no longer fails merely because Spatial sampled a frame immediately beforehand.

## Linux OCR

The Steam Deck package includes a pinned Tesseract runtime. Set `JACOB_OCR_LANG` to a Tesseract language code to override the default `eng` model. Tabs still receive only derived text/geometry; raw images remain inside the core.

## Linux Action Recorder

The Linux recorder reads keyboard `EV_KEY` events from permitted `/dev/input/event*` devices, excludes `JACoB Virtual Keyboard`, accepts new key-down events only while Elite is verified foreground, and completes an already-started key-up after a focus transition.

The Steam Deck bootstrap installs a udev/uaccess rule on first Desktop-mode setup so the active Deck user can access `/dev/uinput` and physical keyboard event nodes without running JACoB as root.

## HUD

The Linux HUD is created lazily and can migrate to the XWayland display containing the verified Elite client. It is grouped as a no-input transient of Elite and stays mapped after first use; hiding it clears the ARGB surface instead of repeatedly unmapping/remapping it.

`overlay.info()` now includes target display, surface-created and mapped state. Gamescope composition verification remains `false` until JACoB has a compositor-level confirmation mechanism.

## Phone calibration

Vision calibration canvases claim pointer input with `touch-action:none`, `preventDefault()`, pointer capture and pointer-cancel handling, so region/color selection works from a paired phone.
