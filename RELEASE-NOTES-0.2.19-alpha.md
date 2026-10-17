# JACoB 0.2.19 Alpha · SDK 15

## Native clipboard

- Adds `Elite.clipboard.info()` and `Elite.clipboard.writeText(text)` for host-native text copying while Elite remains focused.
- Clipboard access is write-only, bounded to 16 KiB, local-host-only, and protected by its own per-tab permission. Existing clipboard contents are never exposed to tabs.
- Windows uses Win32 Unicode clipboard APIs. Linux uses the active `wl-copy`, `xclip`, or `xsel` session backend when available.
- Includes an importable native clipboard diagnostics/example tab and SDK tests.

## One-scan startup pairing

- During a LAN-enabled JACoB startup, the HUD waits for an Elite-sized surface and then shows a **SCAN TO PAIR JACoB** QR for a full 30 seconds.
- The QR contains a random 192-bit, one-time token. It never contains the reusable pairing key.
- Scanning it enrolls an ordinary non-admin, non-remembered browser credential and immediately redirects into JACoB. The QR disappears after a successful scan and the token expires after first use or 30 seconds.
- The startup QR uses the same Windows and Steam Deck/Linux HUD layer system as custom-tab overlays and stays isolated on an internal JACoB layer.

## SDK

- SDK/protocol version is now **15**.
- Browser, installed Markdown reference, packaged reference, and security notes are refreshed for 0.2.19.
