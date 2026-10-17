# Native clipboard API (SDK 15)

Adds `Elite.clipboard.info()` and `Elite.clipboard.writeText(text)` to the existing
tab SDK. A survey tab can copy its next target while Elite remains focused,
without browser clipboard permission, Python, a polling helper, or game input.
This API ships in JACoB Alpha 0.2.19 / SDK 15. Use feature detection when a tab also supports older JACoB hosts.

## Use from a tab

```js
if (!Elite.clipboard?.writeText) {
  // Older host: keep your existing clipboard helper or a manual Copy button.
} else {
  const info = await Elite.clipboard.info();
  if (info.available) {
    const result = await Elite.clipboard.writeText('Phroea Phio UK-W c18-0');
    // Only announce success after result.written === true.
  }
}
```

The first write prompts through JACoB's existing per-tab permission mechanism.
Grant **Write the host clipboard** once from a normal setup/Copy button before
enabling automatic copies. Grants are per browser and saved-tab revision;
updating the tab or using **Reset permissions** requires a fresh grant. A denied
permission is remembered. Previews have separate, session-only grants.

The clipboard belongs to the computer running JACoB. Writes are restricted to
its local browser connection, paired remote administrator devices are also denied.
This intentionally does not overwrite a tablet/phone browser's clipboard.
Clipboard access is independent of JACoB's game-input setting.

## Results and errors

`info()` does not write or request permission:

```json
{"available":true,"backend":"win32","writeOnly":true,"hostOnly":true,"maxTextBytes":16384}
```

`available` means an applicable backend was detected, not that a desktop session
will necessarily accept the next operation. An unavailable result includes a
`reason`. The same backend information is included in the core capability map.
The standalone `info()` call does not perform focus checks or touch game input.

`writeText(text)` accepts a string of at most 16,384 UTF-8 bytes, with no NUL.
Unicode and newlines are preserved. Empty text deliberately replaces/clears the
text clipboard. Numbers and objects are rejected, not silently stringified.

```json
{"written":true,"backend":"win32","bytes":22}
```

`bytes` is the UTF-8 size of the submitted text. The response does not echo it.
Success means the native write/clipboard utility completed successfully; it
does not promise the game has pasted it or that another application will not
subsequently replace it. There is no clipboard-read SDK method.

| Code | Meaning |
| --- | --- |
| `PERMISSION_DENIED` | This tab's clipboard permission was declined. |
| `HOST_ONLY` | A remote connection attempted to write the host clipboard. |
| `BAD_PARAMS` | Missing/non-string text, invalid UTF-8, NUL or oversized input. |
| `CLIPBOARD_UNAVAILABLE` | No usable platform backend was detected. |
| `CLIPBOARD_BUSY` | Another JACoB write or Windows clipboard owner is busy. |
| `CLIPBOARD_TIMEOUT` | The backend did not finish within two seconds. |
| `CLIPBOARD_FAILED` | Native write failed (for example, display access denied). |

Concurrent JACoB writes are rejected rather than queued. Windows retries an OS
clipboard lock for up to 250 ms. Command backends have a two-second deadline.
No automatic retry or alternate backend is launched after a failed write. Tabs
should retry only their still-current target; do not queue stale target names.

## Platforms

**Windows:** Win32 `CF_UNICODETEXT`, with a temporary message-only owner window
on a locked OS thread. No console, focus activation, simulated keys, or extra
runtime. Global memory is handed to Windows only after a successful write.

**Linux:** prefer `wl-copy` when `WAYLAND_DISPLAY` is set. Otherwise, or if
`wl-copy` is missing, use `xclip`, then `xsel`, when `DISPLAY` is set. These are
standard desktop utilities; a clipboard utility must be installed and visible
on the host PATH. The program receives text over stdin, never shell arguments.
Clipboard-owner child processes are allowed to remain alive after the utility's
parent completes, as required by Wayland/X11 selection ownership. Output pipes
are deliberately avoided so an owner child cannot hold JACoB's request open.

Run JACoB in the same graphical session as Elite. A headless service, inaccessible
display, compositor without the required clipboard protocol, or isolated sandbox
can prevent copying even when a utility is installed. There is no runtime
fallback to a different display after a backend reports a write error.

**Steam Deck:** the Linux backend is included, but Desktop Mode and Gaming Mode
need real-device validation, particularly gamescope/XWayland clipboard sharing
with Elite under Proton. This patch does not modify Flatpak packaging or claim
clipboard tools are preinstalled. A supported utility and session access are
release prerequisites. Other operating systems report unsupported.

## Adopting this in Systematic Boxel Survey

Keep helper support for old hosts; choose a single transport for each copy:

```js
async function copyTarget(text) {
  if (!Elite.clipboard?.writeText) return existingHelperCopy(text);
  const info = await Elite.clipboard.info();
  if (!info.available) return existingHelperCopy(text);
  return Elite.clipboard.writeText(text);
}
```

`existingHelperCopy` above stands for the tab's existing helper implementation.
Do not send a helper request as well as a native request. Do not fall back on
permission denial or a native write failure: show the error and retain manual
copy instead of bypassing consent or risking a late duplicate write. Apply the
tab-side change after a compatible JACoB host is available. This core patch does
not modify installed survey tabs, helper pairing, or user survey state.

## Validation

```text
go test ./...
node --test internal/webui/clipboard.test.cjs
```

Normal tests use fakes and a child test process, not the user's clipboard. They
cover input validation, Unicode, backend discovery, command failures/timeouts,
concurrent writes, core local-only enforcement, permission isolation/denial,
and acknowledgement/error propagation through the actual injected SDK.

Optional Windows native check, **which replaces the current clipboard**:

```powershell
$env:JACOB_TEST_NATIVE_CLIPBOARD = '1'
go test ./internal/clipboard -run '^TestNativeWindowsClipboard$' -count=1 -v
Remove-Item Env:JACOB_TEST_NATIVE_CLIPBOARD
```

The native test checks Unicode/empty text, persistence after closing the
temporary owner, and unchanged foreground window. It leaves a sample system
name on the clipboard; it does not save or restore private clipboard contents.

For an end-to-end check, import `examples/native-clipboard-tab.html`:

1. Click **Copy now / grant permission** and allow the new capability.
2. Choose **Copy after 10 seconds**, then focus Elite. After ten seconds, paste
   manually into GalMap and verify the exact text and absence of focus changes.
3. Repeat with Unicode in a text editor, with an empty string, and while the
   survey tab is hidden but JACoB's browser page is still running.
4. Reset permissions, decline the request, and verify the clipboard stays put.
5. Test on Linux/X11, Wayland, and Steam Deck Desktop/Gaming Mode before claiming
   those environments are field-verified. Check useful errors with no utility
   installed or no display access. A successful cross-build is not a field test.

Platform references:
[Win32 clipboard ownership](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setclipboarddata),
[wl-clipboard documentation](https://github.com/bugaevc/wl-clipboard/blob/master/data/wl-clipboard.1).
