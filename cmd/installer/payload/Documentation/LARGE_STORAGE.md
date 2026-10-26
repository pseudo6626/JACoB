# JACoB Large Local Storage — SDK 16

`Elite.storage` is the large, tab-scoped persistence layer for data that does not belong in `Elite.store`.

Use `Elite.store` for small JSON state such as settings, UI state, queues, and current workflow state. Use `Elite.storage` for larger local archives, images, generated data, caches, and long-running research datasets.

## Security and lifecycle

- Large storage is a separate **Large local storage** tab capability.
- A saved tab receives a private namespace based on its local saved-tab ID. Tabs never receive a filesystem path.
- Preview tabs do not receive persistent large storage.
- Updating or locally editing a saved tab keeps its storage because the local saved-tab ID stays the same.
- A copy or Community remix gets a new saved-tab ID and starts with empty storage.
- Removing a saved tab removes its large-storage namespace.
- Community submissions contain the tab HTML and registry metadata only. Large local storage is never included in a submission.

## Limits

- Default quota: **128 MiB per saved tab**.
- User-approved quota tiers: **256 MiB, 512 MiB, or 1 GiB**.
- Maximum object size: **32 MiB**.
- Binary transfer is chunked internally; tab authors do not handle Base64 or transport chunks.
- `list()` is paged and returns at most 500 items per call.

## API

```js
const info = await Elite.storage.info();
// { usedBytes, quotaBytes, items, maxObjectBytes, chunkBytes }

await Elite.storage.put('images/ship.png', blob);
const blobAgain = await Elite.storage.get('images/ship.png');

await Elite.storage.putJSON('systems/1047737387.json', systemRecord);
const record = await Elite.storage.getJSON('systems/1047737387.json', null);

const page = await Elite.storage.list('systems/', { limit: 250 });
const next = page.nextCursor
  ? await Elite.storage.list('systems/', { limit: 250, cursor: page.nextCursor })
  : null;

await Elite.storage.putMany([
  { name: 'systems/1.json', data: one },
  { name: 'systems/2.json', data: two }
]);

await Elite.storage.delete('systems/1.json');
await Elite.storage.clear();

// Prompts the user before increasing the tab's allowance.
await Elite.storage.requestQuota(512 * 1024 * 1024);
```

`put()` accepts `Blob`, `ArrayBuffer`, typed-array views, or strings. Use `putJSON()` / `getJSON()` for normal JavaScript values.

Object names are logical forward-slash-separated names, not operating-system paths. Absolute paths, traversal (`..`), backslashes, JACoB-reserved names, and Windows-reserved filename components are rejected.

## Deadworlds / large research archives

The API is designed to support long-lived local archives without turning JACoB into a database service. A useful pattern is one structured object per system or logical record group:

```text
systems/<system-address>.json
investigations/<id>/snapshot.json
sectors/<sector>/profile.json
cache/artifact-thumbnails/<id>.webp
```

Keep externally shared/canonical data in the application's backend. `Elite.storage` is local commander/application data.
