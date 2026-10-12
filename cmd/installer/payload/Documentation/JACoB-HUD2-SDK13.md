# JACoB HUD2 — SDK13

HUD2 upgrades the tab overlay system while keeping existing `Elite.overlay.set(scene)` scenes valid.

## What changed

Windows now uses a per-pixel alpha GDI+ renderer instead of the legacy color-keyed GDI surface. The renderer uses antialiased vector drawing and text, cached render surfaces, fractional alpha, rounded geometry, line dashes, and text effects.

Existing primitive item types still work:

- `text`
- `line`
- `polyline`
- `polygon`
- `rect`
- `circle`

HUD2 adds semantic item types that are expanded by core before rendering:

- `metric`
- `panel`
- `progress`
- `reticle`
- `bracket`
- `chevron`
- `arc`
- `ring`

## Scene options

```js
await Elite.overlay.set({
  z: 20,
  space: 'normalized',
  theme: 'cockpit',
  opacity: 0.92,
  scale: 1,
  items: []
});
```

`theme` accepts `legacy`, `cockpit`, `clean`, or `highcontrast`. Omitted themes use `legacy` so old tabs keep their previous color behavior. `opacity` and `scale` apply to the full scene.

## Semantic roles

Items can use `role` instead of hard-coding a color. Supported roles are:

`primary`, `secondary`, `muted`, `accent`, `metric`, `label`, `heading`, `success`, `warning`, `danger`, and `panel`.

Explicit `color`, `stroke`, and `fill` values still win over role defaults.

## Text controls

Text supports:

- `fontFamily`
- `fontWeight` from 100 through 900
- `italic`
- `align`
- `opacity`
- `glow` and `glowColor`
- `outlineWidth` and `outlineColor`
- `shadowX`, `shadowY`, and `shadowColor`

Example:

```js
{
  type: 'text',
  x: 0.5,
  y: 0.2,
  text: 'COURSE CLEAR',
  align: 'center',
  role: 'success',
  fontSize: 20,
  fontWeight: 600,
  glow: 2,
  shadowX: 1,
  shadowY: 1,
  shadowColor: '#000000AA'
}
```

## Geometry controls

Primitive geometry supports `opacity`, `glow`, `glowColor`, and `dash`. Rectangles also support `cornerRadius` on the Windows HUD2 renderer. `dash` accepts `solid`, `dash`, `dot`, or `dashdot`.

## Metric

```js
{
  type: 'metric',
  x: 0.06,
  y: 0.08,
  label: 'SPEED',
  value: '487',
  unit: 'm/s',
  size: 28,
  role: 'primary'
}
```

## Progress

```js
{
  type: 'progress',
  x: 0.06,
  y: 0.22,
  w: 0.18,
  h: 0.008,
  label: 'COURSE INTEGRITY',
  progress: 0.73,
  role: 'success'
}
```

`progress` is normally `0..1`. When `min` and `max` are supplied, core normalizes the value.

## HUD Studio

`examples/hud-studio-tab.html` is the visual acceptance test for HUD2. Run it with Elite foreground and check text edges, panel transparency, line quality, glow restraint, rounded geometry, and theme consistency.

## Compatibility

Tabs written for SDK12 do not need to change. Old scenes are normalized to the `legacy` theme and rendered through the upgraded backend on Windows.

Steam Deck/Linux continues to use the existing 32-bit X11/Gamescope overlay. Semantic items are expanded in shared core code, so the new higher-level HUD vocabulary works there too. Windows-only visual effects degrade gracefully on renderers that do not implement them.
