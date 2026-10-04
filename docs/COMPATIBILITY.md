# JACoB compatibility

JACoB keeps platform-specific input, capture, recorder, overlay, and update work behind adapters so custom tabs can use one SDK on Windows and Steam Deck/Linux.

## Windows

The normal alpha build targets current Windows systems capable of running the current Elite client. Input uses scan-code `SendInput`; the overlay uses a native layered window; video uses a compatibility capture path.

The installer is per-user and supports in-place updates. The in-app updater stages the published Windows setup asset and hands control to that installer after the running JACoB process closes.

## Steam Deck / Linux

The Linux build shares the same web UI, custom-tab runtime, journal/status handling, binding parser, network bridge, file-export helpers, and overlay scene protocol. Input uses `/dev/uinput` when available. The overlay uses the Gamescope/XWayland external-overlay path when available.

The Linux updater can replace the current executable only when the containing directory is writable by the current user.

## Public web APIs

`Elite.net.fetch` behaves the same on supported platforms. Requests leave from the computer running JACoB rather than from the browser. Public HTTP/HTTPS targets on standard ports are available; local/private network targets are refused.

Optional platform features report capability through the SDK rather than preventing the rest of JACoB from starting.
