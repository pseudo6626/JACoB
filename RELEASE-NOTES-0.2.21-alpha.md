# JACoB 0.2.21-alpha

SDK 16 adds a separate large local storage service for custom tabs.

- `Elite.storage` uses the saved tab ID as its private namespace.
- 128 MiB default quota; 256/512/1024 MiB increases require explicit user approval.
- 32 MiB maximum object size with chunked binary transfer hidden behind `Blob`/ArrayBuffer APIs.
- `putJSON`, `getJSON`, `putMany`, paged `list`, usage reporting, delete and clear helpers.
- Large storage is a new Community Tab capability and permission.
- Tab updates retain local storage; copies/remixes start empty; removing a tab removes its large storage.
- Community submissions never include local stored data.
- `Elite.store` remains unchanged at 512 KiB/value and 2 MiB/tab for small JSON state.
