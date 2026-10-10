# JACoB 0.2.14-alpha — Spatial Mapper / SDK 11

## Spatial core

JACoB now has a generic local spatial-mapping service built on the existing Elite-only Vision capture boundary.

- Persistent visual feature tracks are generated inside the core. Tabs never receive frame pixels or descriptors.
- Scene-local right-handed coordinates use metres once a range-bearing observation establishes scale.
- `spatial.observe` accepts a normalized screen bearing plus an optional metric range and turns semantic targets into 3-D landmarks.
- Repeated range-bearing observations correct camera drift.
- Visual tracks are triangulated into anonymous 3-D map points once enough metric baseline exists.
- Imported scenes can re-associate low-information internal feature signatures and relocalize against their saved map.
- `spatial.project` projects a saved 3-D landmark back into canonical Elite-frame coordinates for HUD sighting cues.
- Scene export/import is JSON-safe so a custom tab can persist a map with `Elite.store`.

## SDK 11

New tab API:

```js
const scene = await Elite.spatial.begin({ horizontalFovDeg: 80, maxWidth: 480 });
await Elite.spatial.update(scene.scene);

await Elite.spatial.observe(scene.scene, {
  landmark: 'rock-17',
  screen: { x: 0.61, y: 0.44 },
  rangeM: 1284,
  confidence: 0.96
});

const pose = await Elite.spatial.pose(scene.scene);
const aim = await Elite.spatial.project(scene.scene, { landmark: 'rock-17' });
const saved = await Elite.spatial.export(scene.scene);
```

Spatial methods use the existing **Vision** custom-tab permission. No new capture permission or desktop capture path is introduced.

## Important behavior

The first release is intentionally confidence-aware. It does not claim centimetre-level SLAM. Metric scale is established by range observations, visual odometry carries the scene between anchors, and repeated known landmarks tighten the solution. Tabs should use the returned `pose.confidence`, `trackingQuality`, and scene `state` before drawing strong guidance cues.
