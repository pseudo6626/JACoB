# JACoB 0.2.16-alpha — SDK13 / HUD2

This release replaces the Windows HUD's legacy color-key renderer with a per-pixel alpha GDI+ renderer and adds the first HUD2 semantic scene system.

## HUD2

- True per-pixel transparency on Windows.
- Antialiased text and vector geometry.
- Cached render surface to avoid reallocating the HUD every update.
- Fractional opacity, glow, text outline, drop shadow, font weight, italics, dashed lines, and rounded rectangles.
- Scene themes: `legacy`, `cockpit`, `clean`, and `highcontrast`.
- Semantic roles for consistent tab styling.
- Semantic primitives: metric, panel, progress, reticle, bracket, chevron, arc, and ring.
- Existing SDK12 overlay scenes remain valid.
- New HUD Studio tab for visual verification and tuning.

## Cleanup and validation

- Overlay validation is centralized before platform rendering.
- Common semantic primitives are expanded in shared core code so platform renderers stay smaller.
- Windows surface allocation is cached and only rebuilt when the Elite client size changes.
- Added focused tests for color parsing, backward compatibility, semantic expansion, theme validation, and HUD2 capability metadata.
- Hotpatch workflow validates formatting, JavaScript syntax, `go vet`, full tests, Windows builds, installer build, and `git diff --check` before committing.

This is still an alpha release. Use HUD Studio first before converting production tabs to heavier glow/effect use.
