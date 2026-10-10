# JACoB 0.2.13-alpha — LAN Network Doctor

## Mobile/LAN reliability

- Adds a host-only Network Doctor for listener, bind, LAN-address, Windows network-profile, and firewall diagnostics.
- Windows installs and managed updates reconcile a scoped inbound rule for `JACoB.exe`: TCP 6626, `LocalSubnet`, Private/Domain profiles only, with Public networks excluded.
- Firewall repair is UAC-assisted and can also be run from Settings → LAN access.
- JACoB now ranks the default-route/private IPv4 address first instead of presenting VPN/WSL/virtual adapters as equally useful mobile addresses.
- LAN diagnostics warn when `JACOB_LAN` and the effective bind contradict each other.
- Link-local IPv4 addresses are no longer advertised as mobile connection candidates.

## Pairing UI

- Replaces the oversized single QR with two compact local QRs: one for the mobile Connect URL and one for the separate pairing key.
- Keeps the pairing token out of the URL.
- Fixes the static Copy button wiring regression in the LAN card.
- Mobile browsers now distinguish a page that is reachable but unpaired from a fully offline connection.

SDK/API version remains 10.
