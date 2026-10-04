# JACoB — Journal Aligned Control Bridge

> Current field build: **0.2.8 Alpha**

JACoB is a local companion bridge for **Elite Dangerous**. It reads the live journal and Elite companion JSON snapshots, resolves control bindings, sends game input, hosts persistent custom HTML tabs, supplies a game-view stream, renders native HUD overlays, exports tab-generated files, and provides bounded access to public web APIs.

The core carries the common interfaces. User tabs carry route queues, race systems, mining utilities, control panels, data lookups, and other commander tools.

## Installation

Windows commanders should use the installer attached to the current [GitHub release](https://github.com/pseudo6626/JACoB/releases). It installs for the current Windows account, creates Start Menu entries, and registers an uninstaller. The Action Recorder is optional on the first installation.

A newer setup detects the existing JACoB installation and offers an in-place update. Saved tabs, navigation settings, appearance files, and the existing Action Recorder choice are retained.

Installed user data is kept under:

```text
%APPDATA%\JACoB
```

Uninstalling the program leaves that data in place.

Steam Deck and Linux builds use the same tab SDK and web interface. Platform-specific capability is reported by the relevant API before use.

## Interface

The standard console carries:

- **Home** — bridge status and current Elite connection
- **Custom tabs** — installed user tools
- **Tab Manager** — install, edit, preview, remove, reorder, and manage navigation visibility
- **Tutorial** — first-run operating notes
- **Settings** — language, appearance, LAN access, software updates, bindings, capture, and diagnostics

Home, Tutorial, and Settings may be removed from the navigation bar through Tab Manager. Tab Manager remains present as the recovery point.

## Release channel

**Settings → Software updates** checks published releases from this repository. While JACoB remains in alpha, prereleases are included in version comparison.

On Windows, a matching newer setup package can be downloaded and staged from the host browser. JACoB closes, the existing installation is updated, and the new build is launched. Linux can replace the running executable when its directory is writable by the current user.

## Custom-tab interfaces

The full custom-tab contract is recorded in:

[`docs/JACOB_CUSTOM_TAB_REFERENCE.md`](docs/JACOB_CUSTOM_TAB_REFERENCE.md)

SDK / protocol version **6** provides:

- `Elite.files.download()` and `Elite.files.json()` for browser file exports
- `Elite.net.fetch()` for bounded `GET` and `POST` requests to public HTTP/HTTPS APIs
- `Elite.data` for read-only access to Elite companion snapshots such as cargo, plotted route, modules, station services, Odyssey inventories, and Fleet Carrier material listings
- `Elite.journal.files()` / `Elite.journal.read()` for bounded, paged access to journal-session history
- `Elite.bindings.down()` / `Elite.bindings.up()` for cancelable continuous semantic holds without resetting Elite key-repeat acceleration
- `Elite.locale` for the host-wide language setting, locale-change events, translation fallback, and interpolation in locale-aware custom tabs

The network bridge refuses loopback, LAN/private, link-local and local-name targets. It is suitable for public services such as Spansh without requiring those services to permit the sandboxed browser origin through CORS.

The companion-file bridge watches JSON files inside the detected Elite journal directory only. Known files receive stable SDK names, and newly introduced Elite JSON snapshots are surfaced automatically without granting custom tabs arbitrary filesystem access.

The reference also covers the event model, state shapes, input calls, overlay scene schema, recorder interface, video interface, storage behavior, WebSocket protocol, platform notes, error codes, and working examples.

## Technical library

- [`docs/USER_GUIDE.md`](docs/USER_GUIDE.md)
- [`docs/INSTALLATION.md`](docs/INSTALLATION.md)
- [`docs/COMPATIBILITY.md`](docs/COMPATIBILITY.md)
- [`docs/THEMING.md`](docs/THEMING.md)
- [`docs/ALPHA_TEST_PLAN.md`](docs/ALPHA_TEST_PLAN.md)

## Source layout

```text
cmd/                 application and installer entry points
internal/core/       local service and WebSocket bridge
internal/journal/    journal context plus Elite companion-file watch
internal/bindings/   Elite binding parser and resolver
internal/platform/   Windows/Linux input, capture, overlay, recorder
internal/updater/    GitHub release check and update staging
internal/webfetch/   guarded public HTTP/HTTPS bridge
internal/webui/      embedded browser console and local documentation
examples/            custom tab and HUD examples
docs/                field manuals, reference, and JSON schemas
scripts/             build and recovery utilities
assets/              JACoB icons
```

## Alpha notice

This build remains under field evaluation. Reports should include the JACoB version, operating system, the action being attempted, and the relevant section of `jacob.log` where available.

JACoB is an independent community project for Elite Dangerous.
