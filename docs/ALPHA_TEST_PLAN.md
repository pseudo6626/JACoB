# JACoB Alpha 0.2.2 Field Evaluation



## Installation

- Install with Action Recorder enabled.
- Uninstall and reinstall with Action Recorder disabled.
- Confirm Start Menu entries and the Windows uninstall entry.
- Confirm saved commander data survives reinstall.

## Console and navigation

- Confirm Home reaches READY, WARNING, and BLOCKED states as conditions change.
- Confirm saved custom tabs appear in navigation.
- Confirm Tutorial links reach Tab Manager and the technical library.
- Confirm technical panels remain folded under Settings at startup.

## Tab Manager

- Paste HTML, preview, save, reopen, edit, rename, and remove a tab.
- Restart JACoB and confirm saved tabs return.
- Confirm a hidden saved tab keeps journal subscriptions running.

## Appearance

- Load an appearance HTML file.
- Confirm CSS and documented shell templates apply.
- Restart JACoB and confirm the appearance persists.
- Restore the standard appearance.

## Input and bindings

- Verify Secondary-first binding execution.
- Verify Primary fallback when no injectable Secondary exists.
- Close Elite and test **Clone / fill unbound** on a preset with an unbound discrete action.
- Confirm the source preset remains unchanged.

## Journal and Status.json

- Generate journal events and confirm subscriptions update.
- Confirm Status.json updates are relayed.

## HUD overlay

- Run HUD Diagnostics at normal and heavy update rates.
- Confirm one tab can clear its HUD layer without disturbing other layers.

## LAN console

- Open JACoB from a second device.
- Pair with the displayed token.
- Open a saved tab and confirm SDK calls operate normally.

## Technical library

- Open `/docs/` from the console.
- Open the Custom Tab Developer Reference.
- Confirm the plain-text reference and schema links load.
