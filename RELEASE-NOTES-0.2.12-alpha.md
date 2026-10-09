# JACoB 0.2.12-alpha — SDK 10

SDK 10 promotes the Vision work into the normal JACoB tab contract.

## Vision

- Added `Elite.vision.inspect()` with derived region operations: color presence/coverage, vertical color fill, luma, contrast, edge density, OCR text, and structured OCR lines/words.
- Added shared `Elite.vision.configure()` / `calibrate()` utilities so tabs can use baked-in defaults or invoke one standard JACoB drag-to-select/color-pick workflow.
- Added `Elite.vision.debug.show()` / `clear()` to draw effective Vision regions through JACoB's existing native overlay.
- Calibration captures one verified Elite frame while Elite is foreground, then freezes it for the host-owned setup UI. The requesting custom tab receives only the resulting normalized region/color values.
- Windows OCR remains local to the JACoB core and returns text/geometry only.
- Existing `vision.info()` and `vision.sample()` remain available.

## Privacy build

The former no-recorder Windows flavor is replaced by No Capture behavior. It is compiled without the keyboard recorder and without the Windows screen-capture driver. Game View, Vision, OCR, and Vision calibration are unavailable in that build.

Existing installations that previously chose no recorder retain the privacy choice when updating.

## Compatibility

- SDK/API version: 10.
- JACoB version: 0.2.12-alpha.
- Existing SDK 9 tabs remain compatible.
- Vision regions in SDK 10 are screen-relative. Anchor-relative tracking is deferred to SDK 10.x.

- Vision calibration now uses an automatic frozen-frame workflow on Windows: JACoB briefly focuses Elite, captures the verified client frame, restores the prior window, then calibrates on that screenshot.
- Added generic SDK 10 example tabs for text recognition, vertical color-fill recognition, and simple color detection.

- Vision calibration now uses a dedicated verified Elite-client-only snapshot path; monitor/desktop pixels and Game View privacy-matte geometry cannot enter calibration.

- Game View adds an optional privacy matte (enabled by default in the built-in viewer) that blacks the monitor area outside a windowed Elite client without changing Vision coordinates.

- Capture internals now use an OS-neutral canonical Elite-client frame shared by Vision, OCR and calibration; unsafe desktop fallback is forbidden by the backend contract.

- Windows capture now uses Windows Graphics Capture CreateForWindow on the verified Elite HWND. Canonical capture has no desktop/monitor fallback and is cropped to the Elite client area before Vision, OCR, calibration or Game View receive pixels.
