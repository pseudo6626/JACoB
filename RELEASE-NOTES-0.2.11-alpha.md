# JACoB 0.2.11 Alpha — Tab Recovery + Port 6626

- Restores the schema-3 custom-tab store used by 0.2.8: metadata in `custom-tabs.json`, HTML bodies in `custom-tabs/<id>.html`, lazy `tabs.get`, 4 MiB per-tab limit, atomic writes, and ID/path validation.
- Recovers surviving split HTML files if the 0.2.10 regression left them orphaned from the manifest. Existing metadata is preserved whenever it is still present; otherwise a title is recovered from the HTML when possible.
- Retains SDK 9 cross-tab actions and the ability to hide any custom/default navigation tab except Tab Manager.
- Moves the default JACoB service port from 4510 to 6626. The launcher can still detect an already-running legacy 4510 instance, but new instances bind 6626.
- Updates the Windows updater matcher for canonical `*-win-x64.exe` installer names while preserving compatibility with older alpha/setup names.
- Restores schema-3 regression tests and corrects the SDK documentation.

Before installing this build after 0.2.10, back up `%APPDATA%\JACoB` if possible. The repair is designed to recover the existing split tab bodies in place.
