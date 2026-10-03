# JACoB Compatibility Report



JACoB keeps host-specific input, capture, recorder, and overlay code behind platform adapters. Custom tabs use the same SDK on supported systems.

## Windows

The standard alpha build targets current Windows systems capable of running the current Elite Dangerous client. Input uses scan-code `SendInput`. HUD output uses a native layered window. Game View uses a compatibility capture path.

A separate Windows build can be produced without the Action Recorder component.

## Steam Deck / Linux

The Linux build shares the browser console, tab runtime, journal/status handling, binding parser, storage model, and overlay scene protocol. Input uses `/dev/uinput` when available. HUD output uses the Gamescope/XWayland external-overlay path when available.

Optional platform features report availability through their SDK capability calls. An unavailable optional driver does not stop the remaining bridge services.
