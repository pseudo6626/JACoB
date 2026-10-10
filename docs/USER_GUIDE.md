# JACoB User Guide

## Start

1. Start Elite Dangerous.
2. Start JACoB.
3. JACoB opens `http://127.0.0.1:6626/` in the default browser on Windows.
4. Home reports **READY**, **WARNING**, or **BLOCKED** with the current operational note.

## Add a custom tab

Open **Tab Manager**.

1. Paste HTML or choose **Upload HTML**.
2. Give the tab a name.
3. Choose **Preview**.
4. Choose **Save tab**.

The saved tab joins the navigation manifest and persists across JACoB restarts.

## Arrange the navigation

Tab Manager includes **Navigation manifest**.

- Use the arrow controls to move any default or custom tab.
- Any default or custom tab may be hidden from the navigation bar.
- Tab Manager remains visible as the recovery point.
- Hidden tabs remain installed; hidden custom tabs may keep running in the background and can be restored at any time.

The manifest is stored with the saved-tab data and is shared by browsers connected to the same JACoB instance.

## Updates

Open **Settings → Software updates** and choose **Check for updates**.

JACoB reads the public releases from the project GitHub repository. If a newer release has a matching package, **Install update** becomes available on the host browser.

On Windows, JACoB downloads the newer setup package, closes the running core, updates the existing installation, and relaunches. A manually downloaded newer setup also detects the installed release and offers an in-place update.

On Linux / Steam Deck, JACoB can replace the running binary when the executable location is writable. If the location cannot be changed by the current user, use the published release asset manually.

## Bindings

Most tabs should use Elite action names such as `UI_Select` or `GalaxyMapOpen` rather than hard-coded physical keys.

If a needed command has no binding, close Elite and open **Settings → Elite bindings → Clone / fill unbound**. JACoB copies the current preset into a JACoB user preset and fills only commands where both Primary and Secondary are empty.

JACoB prefers Secondary keyboard bindings when executing commands and falls back to Primary when no injectable Secondary exists.

## Phone or tablet

Open **Settings → LAN access** on the computer running JACoB. The **Connect URL** is the preferred address for the home network. Scan **Open JACoB** or type that URL on the phone/tablet, then scan or copy the separate **Pair key**.

If the mobile device cannot connect, run **Network Doctor**. On Windows it checks the effective listener, active network category, preferred LAN address, and the managed `JACoB Local Network` firewall rule. **Repair Windows access** requests elevation only when the firewall rule needs to be created or corrected. The managed rule allows TCP 6626 only for `LocalSubnet` on Private/Domain profiles and does not open JACoB on Public networks.

If Network Doctor reports the listener and firewall as ready but the mobile device still cannot load the page, check for guest Wi-Fi/client isolation, separate VLANs, or VPN software that blocks local-network traffic.

Software installation, network repair, and other host mutations are restricted to a browser running on the host computer.

## HUD overlays

A custom tab can draw text, lines, polylines, polygons, rectangles, and circles over Elite. The overlay logic belongs in the tab. JACoB supplies the native transparent overlay surface.

## File exports

Custom tabs can hand generated files to the browser. Track recordings, JSON exports and other tab-owned data use the ordinary browser download destination. The sandbox does not grant general filesystem access.

## Public API access

Custom tabs can make bounded `GET` and `POST` requests to public HTTP/HTTPS APIs through JACoB. This is useful for services such as Spansh where browser CORS rules may otherwise block a sandboxed tab.

Localhost, LAN/private addresses, link-local addresses, local hostnames and non-standard ports are refused by the network bridge.

## Action Recorder

The Windows installer offers the Action Recorder as an optional component. If installed, a custom recorder tab can capture keyboard actions and timing while recording is explicitly active and Elite is foreground.

## Documentation

- **Custom Tab Developer Reference:** complete SDK, schemas, runtime rules, overlays, input, events, video, recorder, downloads, public API requests, WebSocket protocol, and examples.
- **Theming:** host appearance file format.
- **Compatibility:** current Windows and Steam Deck/Linux behavior.

## Closing JACoB

Use **Quit JACoB** in the top-right corner of the web app on the host computer. JACoB stops the local service cleanly and leaves saved tabs and settings untouched. The quit control is unavailable to LAN clients.
