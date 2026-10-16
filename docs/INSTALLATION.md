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

The Flatpak is only the installer/bootstrap. The actual JACoB core runs natively on the Steam Deck host so Gamescope capture, Linux input, journal/status access and the in-app updater are not trapped inside a Flatpak sandbox.

### Decky Loader requirement for HUD overlays

JACoB's normal web UI, journal/status tools, input bridge, Game View, Vision and OCR do not require Decky Loader. **The in-game HUD overlay does.** JACoB ships its own Decky HUD bridge, but Decky Loader must be installed so Steam's Gaming Mode UI can load that bridge.

JACoB Steam Deck Setup checks for Decky. If Decky is missing, it offers to open the official Decky Loader page. To install Decky manually:

1. Switch the Steam Deck to **Desktop Mode**.
2. Open the official [Decky Loader repository](https://github.com/SteamDeckHomebrew/decky-loader).
3. Download the official `decky_installer.desktop` installer. If Firefox saves it as `decky_installer.desktop.download`, rename it to `decky_installer.desktop`.
4. Move the installer to the Desktop and double-click it.
5. Authorize the installer when prompted and choose **Latest Release**.
6. Return to Gaming Mode.
7. Launch **JACoB Steam Deck Setup** once more. JACoB copies its bundled HUD bridge into Decky's plugin directory and requests a Decky reload. A reboot also loads the bridge if the reload is declined.

Decky is maintained by the SteamDeckHomebrew project and is separate from JACoB. JACoB does not silently pipe a remote privileged installer into a shell.

### Steam Deck capture and HUD

The full Linux build is compiled natively on Linux with CGO enabled. Game View, Vision and OCR use JACoB's Elite-only Gamescope/PipeWire capture path. The capture helper identifies Elite Dangerous (Steam app `359320`) and does not fall back to monitor or desktop capture.

The HUD uses the bundled JACoB Decky renderer and Steam's CEF/Gamescope notification composition layer. The renderer consumes the same HUD2 `OverlayScene` contract as Windows. The HUD layer is composited after the Elite frame, so JACoB Vision does not ingest JACoB's own overlay.

OCR is capability-checked separately and uses the bundled Tesseract runtime in the Steam Deck package.

## Other Linux systems

The release also contains the standalone `JACoB-<version>-linux-amd64` binary. It can be run directly from a writable location. X11/XWayland-dependent features report their capability at runtime.

The in-app updater can replace the current Linux executable when the directory containing it is writable by the current user. A `.previous` binary is left beside the updated executable for recovery.
