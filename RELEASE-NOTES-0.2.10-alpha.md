# JACoB 0.2.10 Alpha — Recovery + SDK 9

This release repairs the broken 0.29 update path and advances the custom-tab SDK to version 9.

## Recovery

- Corrects the embedded/runtime version so the updater no longer installs a build that still identifies itself as 0.2.8.
- Windows installer keeps `JACoB.exe.previous`, verifies the restarted core reports the expected version, and restores the previous executable if verification fails.
- LAN pairing tokens persist in the JACoB data directory instead of rotating every restart.
- Release builds are produced by GitHub Actions from the committed source and exact installer payload.
- Version comparison now handles additional numeric components safely.

## International keyboard support

- Physical keyboard controls can use canonical scan-code tokens such as `SC:29`, `SC:56`, and `SC:E0:38`.
- Existing key names remain supported for backward compatibility.
- Windows recorder events retain physical scan code, extended-key state, virtual key, localized key label, and the legacy `key`/`modifiers` fields.
- Unknown ISO/JIS/OEM keys are no longer discarded by the Windows recorder.
- Right Alt / AltGr, right Control, right Shift, ISO-102, JIS conversion keys, and equivalent physical positions can be learned and replayed without assuming QWERTY.
- Linux/Steam Deck accepts the same physical-key tokens and translates common extended/international set-1 scan codes to evdev.
- Text entry is separated from physical control replay. Windows uses the Elite window's active keyboard layout and Unicode fallback; Linux uses layout-aware `xdotool type` when available and retains the legacy mapper as fallback.
- Physical scan-code expressions pass through the same global-shortcut safety guard as legacy key names.

## Updates UI

- JACoB checks the published release channel shortly after connecting and every 30 minutes while the UI remains open.
- A `NEW` badge appears on Settings when a newer release is found.
- Opening Settings dismisses that badge for that specific published version even if the update is not installed.
- A later, newer release re-arms the badge.
- Background checks never install an update automatically.

## Cross-tab actions

- Saved tabs can publish named host-brokered actions with `Elite.actions.register()`.
- Other saved tabs can discover and invoke published actions with `Elite.actions.list()` and `Elite.actions.invoke()`.
- The action broker preserves iframe isolation; tabs never receive direct access to another tab's DOM or JavaScript context.
- Cross-tab invocation has its own per-tab/browser permission, conflict detection, timeouts, and reload/deletion cleanup.
- `Elite.tabs.list()` and `Elite.tabs.activate()` allow tools such as Touch Deck to open another JACoB tab without host DOM access.
- Action discovery can load hidden saved tabs so they can publish controls while remaining off the navigation bar.

## Navigation visibility

- Tab Manager can hide or restore saved custom tabs as well as default pages.
- Hidden custom tabs stay installed and may continue running in the background.
- Tab Manager remains visible as the recovery page.

