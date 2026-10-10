# SDK 11 utility additions

This is the non-spatial half of JACoB SDK 11. These methods are generic core services intended to keep custom tabs small, deterministic, and polite to shared external APIs.

## Video lifecycle

```js
await Elite.video.attach(img, {width: 960, fps: 10, quality: 65});
Elite.video.close(img);
```

`close()` removes the media source from the element and causes the browser to close the active MJPEG request. It does not disable JACoB capture for other tabs.

## Queued public API access

```js
const r = await Elite.net.fetchQueued(url, {
  group: 'spansh',
  minIntervalMs: 350,
  retry: 2,
  cacheTtlMs: 300000
});

const batch = await Elite.net.fetchBatch(requests, {
  group: 'spansh',
  concurrency: 2,
  minIntervalMs: 350,
  pauseEvery: 10,
  pauseMs: 3000
});
```

Queue groups are shared by every tab using the same JACoB core. GET cache entries support ETag and Last-Modified revalidation when the provider supplies them. The existing public-network restrictions still apply.

## Procedural galaxy helpers

ID64 values are decimal strings in SDK 11. Do not convert them to JavaScript `Number`; valid Elite system addresses can exceed JavaScript's exact-integer range.

```js
const parsed = await Elite.galaxy.parseSystemName('Synuefe EN-H d11-96');
const decoded = await Elite.galaxy.decodeAddress('3309179996515');
const next = await Elite.galaxy.addressForSequence(decoded.id64, 97);
const same = await Elite.galaxy.encodeAddress(decoded);
const box = await Elite.galaxy.boxel(decoded.id64);
const hierarchy = await Elite.galaxy.boxelHierarchy(decoded.id64, {
  position: {x: 735.1, y: -184.7, z: -104.6}
});
```

`boxelHierarchy()` always knows the ID64's base boxel and all coarser parent cells. Finer child cells require a physical system position and are returned as unresolved if one is not supplied.

## Atomic tab state

```js
await Elite.store.update('statistics', current => ({
  ...(current || {}),
  runs: Number(current?.runs || 0) + 1
}));

await Elite.store.batch([
  {op:'set', key:'route', value:route},
  {op:'set', key:'settings', value:settings},
  {op:'delete', key:'old-cache'}
]);
```

`update()` uses compare-and-set retries so two browser views do not silently overwrite each other's state. `batch()` applies all operations with one store lock and one disk persist; a validation or write failure rolls the batch back.

## Shared system catalog

```js
const system = await Elite.catalog.system.get({
  id64: '3309179996515',
  provider: 'spansh',
  detail: 'summary'
});
```

The provider is explicit. SDK 11 initially supports `spansh`. Catalog calls use the shared queued network service and preserve ID64 as a decimal string.
