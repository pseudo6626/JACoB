# JACoB — Journal Aligned Control Bridge


> Current field build: **0.2.2 Alpha**

JACoB is a local companion bridge for **Elite Dangerous**. It reads the live journal and `Status.json`, resolves control bindings, sends game input, hosts persistent custom HTML tabs, supplies a game-view stream, and renders native HUD overlays.

The core carries the interfaces. User tabs carry the tools. Route queues, race HUDs, mining utilities, control panels, and other workflows live in ordinary HTML loaded through Tab Manager.

## Installation

[Windows commanders should use the release installer found here](https://github.com/pseudo6626/JACoB/releases/tag/v0.2.2-alpha) . It installs for the current Windows account, creates Start Menu entries, and registers an uninstaller. The Action Recorder is optional during setup.

Installed data is kept under:

```text
%APPDATA%\JACoB
```

Uninstalling the program leaves saved tabs and appearance settings in place.

[Steam Deck and Linux builds use the same tab SDK and web interface](https://github.com/pseudo6626/JACoB/releases/tag/v0.2.2-alpha). Platform-specific capability is reported by the relevant API before use.

## Interface

The standard console keeps routine controls in view:

- **Home** — bridge status and current Elite connection
- **Custom tabs** — installed user tools
- **Tab Manager** — install, edit, preview, or remove tools
- **Tutorial** — first-run operating notes
- **Settings** — appearance, LAN access, bindings, capture, and diagnostics

JACoB is by design fairly limited in features by default. It is the creation of user added tabs via custom HTML files that bring the app to full life. Examples of possible tab ideas are provided in [Examples](examples)

The full custom-tab contract is recorded in:

[`docs/JACOB_CUSTOM_TAB_REFERENCE.md`](docs/JACOB_CUSTOM_TAB_REFERENCE.md)

It includes the browser SDK, event model, state shapes, input calls, overlay scene schema, recorder interface, video interface, storage behavior, WebSocket protocol, platform notes, error codes, and working examples.


## Technical library

Additional notices:

- [`docs/USER_GUIDE.md`](docs/USER_GUIDE.md)
- [`docs/INSTALLATION.md`](docs/INSTALLATION.md)
- [`docs/COMPATIBILITY.md`](docs/COMPATIBILITY.md)
- [`docs/THEMING.md`](docs/THEMING.md)
- [`docs/ALPHA_TEST_PLAN.md`](docs/ALPHA_TEST_PLAN.md)

## Source layout

```text
cmd/                 application and installer entry points
internal/core/       local service and WebSocket bridge
internal/journal/    Elite journal and Status.json watch
internal/bindings/   Elite binding parser and resolver
internal/platform/   Windows/Linux input, capture, overlay, recorder
internal/webui/      embedded browser console and local documentation
examples/            custom tab and HUD examples
docs/                field manuals, reference, and JSON schemas
scripts/             build and recovery utilities
assets/              JACoB icons
```

## Alpha notice

This build is under field evaluation. Reports should include the JACoB version, operating system, the action being attempted, and the relevant section of `jacob.log` where available.

JACoB is an independent community project for Elite Dangerous.
