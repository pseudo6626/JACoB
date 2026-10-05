# JACoB Alpha 0.2.8 test plan

## Installer / updater

- Install with Action Recorder enabled.
- Run the same installer again and confirm it offers a reinstall.
- Install an older field build, then run the 0.2.8 setup and confirm it offers an in-place update.
- Confirm the Action Recorder choice is retained during update.
- Confirm Start Menu shortcuts and the uninstall entry remain valid.
- Confirm saved user data survives update, reinstall, and uninstall.
- From the host browser, run **Settings → Software updates → Check for updates**.
- After a newer test release is published, confirm **Install update** downloads, closes, updates, and relaunches JACoB.
- Confirm a LAN browser may check the channel but cannot install an update.

## Home / navigation

- Home reaches READY/WARNING/BLOCKED correctly.
- Saved custom tabs appear in navigation.
- Reorder default and custom tabs in Tab Manager and restart JACoB; confirm the order persists.
- Hide Home, Tutorial, and Settings individually; confirm they leave the navigation bar and can be restored.
- Confirm Tab Manager cannot be hidden.
- Confirm a restart does not open a default page that is currently hidden.
- Technical sections remain collapsed under Settings by default.

## Tab Manager

- Paste HTML, preview, save, reopen, edit, rename, and remove a tab.
- Restart JACoB and confirm saved tabs return.
- Confirm a non-active saved tab keeps journal subscriptions running.
- Export a text file and JSON file from a saved tab using `Elite.files` and confirm the browser receives both downloads.

## Public API bridge

- Call `Elite.net.fetch()` against a public JSON API such as Spansh and confirm a parsed `json` result is returned.
- Confirm `GET` and `POST` work with accepted headers.
- Confirm localhost, RFC1918/private, link-local, `.local`, single-label hostnames, and non-standard ports are refused.
- Confirm an oversized response is refused cleanly.
- Confirm an unreachable public host times out without affecting the JACoB core.

## Appearance

- Upload a theme HTML file.
- Confirm CSS and documented shell templates apply.
- Restart JACoB and confirm the theme persists.
- Reset to default.

## Input / bindings

- Verify Secondary-first binding execution.
- Verify Primary fallback when no injectable Secondary exists.
- Close Elite and test Clone / fill unbound on a preset with an unbound discrete action.
- Confirm the source preset is unchanged.

## Journal / status

- Generate journal events and confirm subscriptions update.
- Confirm Status.json updates are relayed.

## Overlay

- Run HUD Diagnostics at realistic and heavy update rates.
- Confirm overlay clears per-tab without affecting other saved tab layers.

## LAN

- Open the interface from a second device.
- Pair with the token.
- Open a saved tab and confirm SDK calls work.

## Documentation

- Open `/docs/` from the app.
- Open the Custom Tab Developer Reference.
- Verify `Elite.files`, `Elite.net.fetch`, SDK version 7, update behavior, and navigation layout are documented.
- Verify the plain-text reference and schema links load.


## Alpha 0.2.8 carrier-data checks

- Open a commodity market and confirm `Elite.state.get().market.Items` reflects the latest `Market.json`.
- Open Carrier Management and confirm `journalContext.carrierFreeSpace`, callsign, and available balance update from `CarrierStats`.
- Save a custom tab, set a value through `Elite.store`, reload JACoB, and confirm the value persists.
- Confirm preview tabs receive `TAB_STATE_PREVIEW` rather than writing persistent state.
- Delete a saved tab and confirm its tab-state namespace is removed.

## Alpha 0.2.8 companion-file checks

- Start JACoB with an existing journal whose latest `Location`, `Loadout`, and `CarrierStats` records are already in the file; confirm `journalContext` is populated immediately.
- Confirm `Elite.data.list()` reports every JSON snapshot currently present in the journal directory.
- Open or change game screens that write `Cargo.json`, `NavRoute.json`, `ModulesInfo.json`, `Market.json`, `Outfitting.json`, and `Shipyard.json`; confirm the corresponding `eliteFile` event arrives with the updated object.
- On Odyssey, change backpack/locker contents and inspect a Fleet Carrier bartender; confirm `backpack`, `shipLocker`, and `fcMaterials` update when Elite writes those files.
- Place a valid test JSON snapshot with an unfamiliar filename in a test journal directory and confirm it is exposed under a normalized key without adding code for that filename.
- Remove a test snapshot and confirm the next `eliteFile` event reports `available:false`.
- Confirm `Elite.data.get()` cannot accept an arbitrary filesystem path outside the detected journal directory.

## Alpha 0.2.8 Carrier Market Orders catalog

- Open the example tab with no saved catalog and confirm 398 commodities / 15 categories are available immediately.
- Verify Tritium resolves to Chemicals row 18 and the last catalog entry resolves to Weapons row 10.
- Import a replacement catalog, restart JACoB, and confirm the replacement persists.
- Use **Restore built-in** and confirm the supplied 398-row catalog returns.
