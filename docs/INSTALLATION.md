# JACoB Installation Notice

> **GALNET SERVICE BULLETIN // LOCAL SYSTEMS**  
> Installation procedure for JACoB Alpha 0.2.2.

## Windows

Run the JACoB Alpha installer from the release page.

The current-user installation path is:

```text
%LOCALAPPDATA%\Programs\JACoB
```

Setup creates Start Menu entries and registers an uninstaller.

The **Action Recorder** is optional. Clearing that option installs the Windows build without the recorder hook.

Commander data is normally stored at:

```text
%APPDATA%\JACoB
```

Uninstall leaves that data in place. Saved tabs and appearance settings can therefore return after reinstall.

## Steam Deck / Linux

The Linux release uses the same browser console, custom-tab runtime, journal/status reader, binding parser, and SDK. Input, capture, recorder, and overlay support depend on the host session. Check the corresponding capability call before enabling an optional feature.

For Steam Deck, Gaming Mode support is expected to pass through Gamescope/XWayland where available.
