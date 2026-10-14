# JACoB installation

## Windows

Download the current `JACoB-<version>-win-x64.exe` installer from the release **Assets** section and run it.

The installer uses the current Windows account and installs under:

```text
%LOCALAPPDATA%\Programs\JACoB
```

It creates Start Menu shortcuts and registers an uninstaller.

On a first installation, setup asks whether to include the optional Action Recorder. Choosing **No** installs the Windows build that does not contain the recorder hook.

If JACoB is already installed, setup reads the installed version. A newer setup offers an in-place update, closes the running JACoB process if necessary, preserves the existing Action Recorder choice, and keeps user data.

JACoB user data is normally stored under:

```text
%APPDATA%\JACoB
```

Uninstall keeps user data so saved tabs, navigation settings and appearance files can survive a reinstall.

### In-app update check

Open **Settings → Software updates** and choose **Check for updates**. JACoB reads published releases from `pseudo6626/JACoB`, including prereleases while the project remains in alpha.

When a newer Windows release contains a matching setup asset, **Install update** downloads that setup package, closes JACoB, performs the in-place update, and relaunches the application. Update installation is available only from a browser running on the host computer.

## Steam Deck

Use the release asset named:

```text
JACoB-<version>-SteamDeck.flatpak
```

Installation is designed for Steam Deck Desktop Mode:

1. Download the `.flatpak` asset.
2. Open it with Discover.
3. Choose **Install**.
4. Launch **JACoB Steam Deck Setup** once.

The setup app copies the native JACoB core into:

```text
~/.local/share/JACoB
```

It then finds the Steam account containing Elite Dangerous, backs up `localconfig.vdf`, preserves existing launch options, adds JACoB immediately before `%command%`, and restarts Steam. No terminal command or executable permission change is required.

After setup, launch Elite Dangerous normally from Gaming Mode. JACoB starts with Elite and its browser UI can be selected as the second window through the Steam window switcher.

The Flatpak is only the installer/bootstrap. The actual JACoB core runs natively on the Steam Deck host so Gamescope overlays, Linux input, journal/status access and the in-app updater are not trapped inside a Flatpak sandbox.

### Steam Deck capture and HUD

The full Linux build is compiled natively on Linux with CGO enabled. The Steam Deck HUD uses JACoB's X11/XWayland Gamescope overlay backend.

Game View and base Vision use a verified-window X11/XWayland capture backend. JACoB accepts a frame only when the focused Gamescope app is Elite Dangerous (Steam app `359320`) or the focused X11 process can be traced to Elite. It does not fall back to monitor or desktop capture.

OCR remains platform-dependent and should be capability-checked separately.

## Other Linux systems

The release also contains the standalone `JACoB-<version>-linux-amd64` binary. It can be run directly from a writable location. X11/XWayland-dependent features report their capability at runtime.

The in-app updater can replace the current Linux executable when the directory containing it is writable by the current user. A `.previous` binary is left beside the updated executable for recovery.
