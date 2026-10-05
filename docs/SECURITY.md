# JACoB security model

JACoB intentionally has privileges that ordinary web pages do not: it can read Elite Dangerous journals, send input to Elite, render overlays, proxy selected public HTTP requests, and on supported platforms capture the Elite window. The security boundary therefore lives in the JACoB core, not in custom-tab HTML.

## Core guarantees

- **Elite-targeted input.** Input methods focus the verified `EliteDangerous64.exe` / `EliteDangerous.exe` window before injecting keys. Custom tabs cannot ask JACoB to target another process.
- **Elite-only video.** Windows capture reads the verified Elite client window, never a rectangle from the desktop framebuffer. If Elite is absent, hidden, minimized, or not foreground, capture returns a freshly generated black frame.
- **Derived vision only.** SDK 7 exposes `Elite.vision.info()` and `Elite.vision.sample()`. Samples contain feature coordinates, coarse frame motion, brightness and contrast. Raw pixels are not exposed to custom tabs.
- **Same-origin WebSockets and DNS-rebinding defense.** Browser WebSocket connections must originate from the same JACoB host and port. Cross-site and sandbox-`null` origins are rejected, and HTTP Host values are limited to `localhost`, loopback literals, or IP addresses actually assigned to this computer. This prevents a hostile website or rebinding domain from silently reaching localhost JACoB.
- **Strong LAN pairing.** Automatically generated pairing tokens are 128-bit cryptographically random values. Configured LAN tokens shorter than 16 characters disable LAN access rather than silently weakening authentication.
- **Remote mutation limits.** Installing/editing/removing custom tabs, changing Appearance HTML, and modifying Elite binding files are host-computer-only operations.
- **Authenticated diagnostics/media.** LAN health access requires the pairing token. Video uses a different cryptographically random media-only token on both loopback and LAN; a paired browser receives that token only after WebSocket authentication. Custom tabs granted Video permission therefore never receive the higher-privilege LAN pairing credential. Remote system info omits host paths and detailed binding diagnostics.
- **Custom-tab sandbox.** Tabs retain an opaque sandboxed origin. SDK 7 injects a restrictive CSP that blocks direct external scripts, frames, fetch/WebSocket connections, forms, workers and external image loads, and injects a navigation guard that blocks ordinary external iframe navigations and popups. Sensitive bridge capabilities (input, network, recorder, overlay, video and vision) require a per-tab browser permission prompt. The network prompt explicitly warns that a network-enabled tab can transmit journal/state data it can read. Denials are remembered until permissions are reset, preventing prompt-spam loops.
- **Public-network proxy only.** `Elite.net.fetch()` blocks loopback, private/LAN, carrier-grade NAT, protocol-benchmark, link-local, multicast and local hostnames, re-checks DNS at dial time, permits only HTTP/HTTPS on standard ports, limits redirects, and bounds request/response sizes.
- **Bounded Elite file access.** Journal history reads accept only `Journal*.log` basenames from the detected Elite journal directory, reject traversal and symlinks, and cap results. Companion JSON access is limited to files already indexed from that directory. The host also rate-limits high-frequency custom-tab calls, including journal scans, vision samples, network requests and persistent-state writes.
- **Verified automatic updates.** Automatic installation is refused unless GitHub publishes a SHA-256 digest for the selected release asset and the downloaded bytes match it.
- **Appearance sanitization.** Appearance HTML is presentation-only: shell-slot markup is reduced to a small safe tag/attribute allow-list, event handlers/embedded media are removed, external URLs are blocked, and theme CSS has external `url()` / `@import` sources stripped.
- **Clickjacking/browser-permission hardening.** The main UI denies framing by other sites, blocks external image loads, and disables camera, microphone, geolocation and browser display-capture permissions.

## Custom-tab permissions

Permissions are granted per saved-tab version and per browser. A tab asks the first time it attempts a sensitive capability. Editing or replacing the tab changes its version and therefore invalidates the old grants, including on browsers that were offline during the update. Grants can also be cleared manually from Tab Manager with **Reset permissions**.

The capabilities are:

- **Control Elite Dangerous** — raw or semantic input actions.
- **Network** — `Elite.net.fetch()` to public internet services.
- **Recorder** — Elite-foreground keyboard recording.
- **Overlay** — game overlay layers.
- **Video** — display of the focus-gated Elite video stream.
- **Vision** — derived feature/motion telemetry from Elite-only frames.

Read-only Elite journal/state access remains available to custom tabs because most JACoB tools require it. The injected CSP and network permission boundary are intended to prevent that data from being silently sent elsewhere.

## Remaining boundaries

- **LAN mode is for trusted networks.** JACoB currently serves its browser UI and WebSocket over HTTP/WS rather than TLS. The strong pairing token prevents unauthenticated control, but an attacker who can actively intercept or modify traffic on the local network is outside the current protection boundary. Keep LAN access disabled on public/untrusted networks.
- **Imported HTML is still code.** The opaque iframe, CSP, capability prompts and navigation guard substantially reduce what a custom tab can do, but JACoB does not claim a formally non-exfiltrating JavaScript sandbox across every old browser engine. Current browsers with the Navigation API get the strongest navigation guard. Install tabs from sources you trust, especially when granting Network or Control.

## Threat-model boundary

JACoB protects against hostile web pages, untrusted custom-tab HTML, accidental desktop capture, weak LAN authentication, common SSRF targets, path traversal, and clickjacking. It does not attempt to defend against malware or another local process already running with the user's operating-system privileges.

The Linux/Steam Deck capture backend remains unavailable until a PipeWire/Gamescope adapter can implement the same Elite-only/focus-gated contract. Linux input now refuses to emit keys unless JACoB can positively verify Elite as the focused X11/Gamescope window; if focus cannot be verified, input fails closed.

## LAN transport limitation

LAN mode currently uses plain HTTP/WebSocket transport. The 128-bit pairing token prevents unauthenticated clients from connecting, but it does **not** provide confidentiality against a device that can sniff or actively intercept traffic on the local network. Do not expose or port-forward JACoB to the public internet, and use LAN control only on a network you trust. A future TLS/authenticated-encryption transport should be used before claiming protection on hostile Wi-Fi or other untrusted networks.

## Installed-tab trust boundary

Custom tabs remain executable code chosen by the user. The sandbox and capability prompts block common direct network/control channels and protect host secrets, but they are not a formal information-flow sandbox. In particular, browser navigation behavior should not be relied on as a guarantee that a malicious tab can never disclose read-only game data. Only install custom tabs you trust with the journal/state data they are designed to consume.
