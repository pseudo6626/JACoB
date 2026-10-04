# JACoB Alpha 0.2.4 test plan

## Installer / updater

- Install with Action Recorder enabled.
- Run the same installer again and confirm it offers a reinstall.
- Install an older field build, then run the 0.2.4 setup and confirm it offers an in-place update.
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
- Verify `Elite.files`, `Elite.net.fetch`, SDK version 2, update behavior, and navigation layout are documented.
- Verify the plain-text reference and schema links load.
