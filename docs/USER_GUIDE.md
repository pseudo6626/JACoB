# JACoB Field Guide

  
> Operating notes for JACoB Alpha 0.2.2.

## Start sequence

1. Start Elite Dangerous.
2. Start JACoB.
3. On Windows, JACoB opens `http://127.0.0.1:4510/` in the default browser.
4. Check **Home**. The bridge reports **READY**, **WARNING**, or **BLOCKED** with the current condition.

## Install a custom tab

Open **Tab Manager**.

1. Paste HTML or choose **Upload HTML**.
2. Enter a tab name.
3. Choose **Preview**.
4. Choose **Save tab**.

Saved tabs join the main navigation and return after JACoB restarts.

## Control bindings

Custom tabs should call Elite action names such as `UI_Select` and `GalaxyMapOpen`. Physical-key calls remain available for cases that require a specific key.

If an action has no usable binding, close Elite and open **Settings → Elite bindings → Clone / fill unbound**. JACoB copies the current preset to a JACoB user preset, then fills commands with empty Primary and Secondary slots. The source preset remains unchanged.

During execution, JACoB uses an injectable Secondary keyboard binding first. Primary is used when no suitable Secondary exists.

## Phone or tablet console

Open **Settings → LAN access**. Use one of the listed LAN addresses on the second device, enter the pairing token, and reconnect.

## HUD overlays

Custom tabs may draw text, lines, polylines, polygons, rectangles, and circles over Elite. Each tab receives its own overlay layer. Track maps, race markers, navigation cues, and other HUD logic stay inside the tab.

## Action Recorder

The Windows installer offers the Action Recorder as an optional component. When installed, a recorder tab may capture keyboard actions and timing while recording is armed and Elite is the foreground window.

## Technical library

- **Custom Tab Developer Reference** — complete SDK, schemas, events, input, overlays, video, recorder, storage, WebSocket protocol, and examples.
- **Theming** — host appearance file format.
- **Compatibility** — current Windows and Steam Deck/Linux capability notes.

## Shutdown

Use **Quit JACoB** at the upper-right of the host console. The local service closes cleanly. Saved tabs and settings remain on disk. LAN clients do not receive the quit control.
