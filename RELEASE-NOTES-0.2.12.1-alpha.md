# JACoB 0.2.12.1-alpha — SDK 10 reliability hotfix

## Automatic update handoff

- The automatic installer now requires the old JACoB process to actually stop before replacing files.
- If graceful shutdown stalls, it terminates the requesting PID and then removes any stale JACoB.exe instance.
- The installer verifies that the JACoB port is free before launching the replacement core.
- The replacement core must report the expected version and its exact process ID.
- Failed startup restores the previous executable and verifies that the previous version comes back online.
- Automatic updates preserve the active data directory and capture/privacy choice.
- Managed automatic updates refuse portable copies when source-path information is available.
- `%APPDATA%\JACoB\update.log` records each handoff step.
- The installer remains compatible with 0.2.11/0.2.12 cores that use the older update argument set.

## Tab HUD ownership

- Saved-tab overlays are owned by the `tab:<id>` namespace.
- Deleting/removing a tab clears the parent layer and every child layer, including Vision debug HUDs.
- Reloading an edited saved tab clears its previous HUD namespace before the new iframe starts.
- Navigating away from a tab does not clear its HUD, so persistent companion tabs still work.

SDK/API version remains 10.
