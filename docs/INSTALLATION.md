# JACoB installation

## Windows

Run `JACoB-Alpha-0.2.8-Setup.exe`.

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

## Steam Deck / Linux

The release package includes `JACoB-Alpha-0.2.8-SteamDeck-linux-amd64`.

The browser UI, custom tabs, journal/status reader, bindings layer, public API bridge and core SDK are shared with Windows. Platform-specific input, capture, recorder, and overlay capability can vary; query the relevant SDK capability before depending on an optional feature.

The in-app updater can replace the current Linux executable when the directory containing it is writable by the current user. A `.previous` binary is left beside the updated executable for recovery. If the executable location is read-only, use the published release asset manually.
