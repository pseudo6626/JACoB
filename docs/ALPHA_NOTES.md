# JACoB Alpha 0.2.4

## Alpha 0.2.4

- Tab Manager now carries a persistent navigation manifest. Default and custom tabs can be reordered; Home, Tutorial, and Settings can be hidden from the navigation bar. Tab Manager remains visible as the recovery point.
- Settings now includes a GitHub release check. Newer published alpha or stable releases can be staged from the host browser. Windows uses the release installer; Linux replaces the running binary when its location is writable.
- The Windows installer detects an existing JACoB installation. A newer setup offers an in-place update and preserves saved tabs, navigation settings, appearance data, and the Action Recorder choice.
- Custom tabs now receive `Elite.files.download()` and `Elite.files.json()` for browser file exports. Saved and preview tabs retain the sandbox download permission added in Alpha 0.2.3.
- Custom tabs now receive `Elite.net.fetch()` for bounded public API requests. JACoB handles the request outside the browser sandbox, avoiding ordinary CORS failures while blocking loopback, LAN/private, link-local, local-name, and non-standard-port targets.
- SDK / protocol version is now 2.
