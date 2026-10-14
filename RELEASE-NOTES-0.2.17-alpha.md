# JACoB 0.2.17-alpha — Steam Deck recovery build

This release repairs the Steam Deck/Linux package path and adds the first working Deck Vision capture backend.

## Steam Deck

- Linux release binaries are now built natively on Linux with CGO enabled.
- Restores the existing X11/XWayland + Gamescope HUD backend that previous release builds accidentally excluded.
- Adds verified Elite-window capture through X11/XComposite for Game View and Vision on Steam Deck/Linux.
- Capture reuses JACoB's Elite foreground verification and never falls back to desktop/monitor capture.
- Adds a `.flatpak` Steam Deck installer bundle. Open it in Discover, install, and launch once; it installs the native core and configures Elite's Steam launch options automatically.
- The native JACoB core still runs outside the Flatpak sandbox so uinput, Gamescope overlays, journal discovery and in-app updates retain their normal host access.

## Notes

- OCR remains platform-dependent; this release targets canonical frame capture and Vision analysis first.
- The X11/XComposite capture path is intended for Steam Deck Gaming Mode's Gamescope/XWayland stack. It should fail closed if Elite cannot be positively identified.
