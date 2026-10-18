# JACoB 0.2.20 Alpha · SDK 15

## Steam Deck Vulkan HUD orientation hotfix

- Corrects the Steam Deck Vulkan HUD Y-axis transform so JACoB's top-left overlay coordinate system maps to the screen correctly.
- Fixes vertically inverted text, QR codes, and other HUD geometry on the native Steam Deck Vulkan renderer.
- Restores the startup pairing QR to its intended upper-right position.
- Windows HUD behavior is unchanged.
- Adds a regression test that fails if the inverted Vulkan shader transform returns.

## SDK

- SDK/protocol version remains **15**.
- No custom-tab API changes are required.
