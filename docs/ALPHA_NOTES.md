# JACoB Alpha 0.2.8 Dispatch

**Journal Aligned Control Bridge**

Alpha 0.2.8 is the recovered and validated baseline for the work developed across the interrupted 0.2.5–0.2.7 sequence.

## Core recovery

- Restores journal-context recovery for current system, ship/loadout, maximum jump range, and Fleet Carrier state.
- Adds read-only Elite companion JSON access through `Elite.data`, including known aliases and future JSON-file discovery.
- Adds bounded/paged journal history through `Elite.journal.files()` and `Elite.journal.read()`.
- Adds isolated persistent saved-tab storage through `Elite.store`.
- Advances the custom-tab SDK / WebSocket protocol to version 5.
- SDK v5 adds `Elite.bindings.down()` / `Elite.bindings.up()` so tabs can perform one continuous semantic hold while still releasing immediately on Abort.
- SDK v6 adds host-wide localization with English, Russian, German, French, Simplified Chinese, and Spanish plus `Elite.locale` for locale-aware custom tabs.
- Retains the Spansh-backed Neutron Highway Control tab.
- Fixes the Action Recorder HTML parser issue caused by a literal closing-script token in generated replay code.

## Networking and large-tab hardening

- Retains the first-LAN-client pairing fix: remote browsers render navigation immediately, open Settings when no pair token is stored, and focus the pairing field before attempting the WebSocket connection.
- Saved-tab HTML is stored as individual `custom-tabs/<id>.html` files; `custom-tabs.json` is now a small schema-3 metadata/navigation manifest.
- Existing schema-2 inline saved tabs migrate automatically on startup with their tab IDs preserved, so isolated `Elite.store` data remains associated with the same tabs.
- `tabs.list` and `tabs.changed` carry metadata only. Tab HTML is fetched with `tabs.get` only when a tab is opened or edited.
- Tab Manager caches loaded tab bodies locally and gives `tabs.save` / `tabs.get` a longer request window for legitimate large transfers.
- Saved-tab size limit increases from 1 MiB to 4 MiB.
- WebSocket reads now reassemble continuation frames, and writes use deadlines so a stale LAN browser cannot indefinitely block another client's save path.
- Tab save/layout/delete responses are sent before asynchronous metadata change broadcasts.

## Fleet Carrier Market Orders

The included Carrier Market Orders tab is stamped `CMO-0.2.8-BASELINE-20261004.6`.

- Built-in 398-commodity / 15-category Fleet Carrier navigation catalog.
- BUY/SELL manifests, load planning, capacity awareness, dry-run mode, pause/resume/abort, and `CarrierTradeOrder` journal verification.
- Uses `Market.json` `meanPrice` (with Frontier `MeanPrice` spelling accepted) as the market reference for percentage pricing.
- Carrier price range is 5%–10,000%.
- Price control is modeled in the carrier UI's 5-Cr ticks rather than treating a click as a fixed percentage change.
- Requested percentages are converted to the nearest reachable credit price and verified against journal-reported `CarrierTradeOrder.Price`.
- Targets from 50% through 150% are always entered with discrete price taps; default price-tap spacing is 300 ms and is configurable.
- Quantity and price errors within the tap cleanup region are corrected using discrete taps.
- Large quantity/price changes use field-derived piecewise terminal-speed models measured from real carrier runs.
- The shipped baseline models are active immediately; calibration is optional rather than a startup requirement.
- If a live run requires repeated held corrections or fails to converge cleanly, the tab recommends calibration after the run/halt.
- Optional personalized calibration is reduced to 8 journal-confirmed trials per stage: 800, 1200, 1800, and 5200 ms, RIGHT and LEFT.
- Quantity and Price calibration are independent and saved independently through `Elite.store`.
- Abort interrupts waits, navigation, tap loops, journal waits, correction loops, calibration, and active continuous holds.
- A configurable post-Apply settle interval prevents correction input from racing the Fleet Carrier UI transition.

## Validation

Release validation includes:

- `go test ./...`
- `go vet ./...`
- JavaScript syntax parsing for example/embedded HTML script blocks
- carrier catalog consistency validation: 398 unique commodities / 15 categories
- Windows x86-64 installer, portable, and no-recorder builds
- Linux x86-64 Steam Deck build

This remains an alpha field build. Reports should include the JACoB version, custom-tab build stamp, operating system, attempted action, and the relevant control/journal log lines.
