# SDK 12: `Elite.macros`

JACoB 0.2.15 adds a host-managed macro library for short quality-of-life sequences. The shipped defaults are deliberately limited to logistics and interface/menu navigation. They use current Elite bindings where possible so they follow the player's active controls.

```js
const all = await Elite.macros.list();
const route = await Elite.macros.get('plot_route');
await Elite.macros.run('plot_route', { system: 'Sol' });
```

Convenience calls are provided for the bundled defaults:

```js
await Elite.macros.exitMenus();
await Elite.macros.openGalaxyMap();
await Elite.macros.openSystemMap();
await Elite.macros.openNavigationPanel();
await Elite.macros.openCommsPanel();
await Elite.macros.openRolePanel();
await Elite.macros.openInternalPanel();
await Elite.macros.openQuickComms();
await Elite.macros.openCodexDiscovery();
await Elite.macros.plotRoute('Sol');
```

`Elite.macros.list()` returns the shared definitions. `Elite.macros.get(name)` accepts a macro ID or display name. `Elite.macros.run(name, args)` executes the stored definition host-side and requires the custom tab's normal control permission.

## Responsible use

Frontier's Code of Conduct prohibits external automation used to generate a player advantage. JACoB macros are intended for accessibility, menu navigation, and repetitive logistical/quality-of-life tasks. JACoB does not condone macros used to automate combat, competitive interactions, unattended gameplay, or otherwise cheat or generate a prohibited player advantage. Users are responsible for following the current Elite Dangerous EULA and Frontier Code of Conduct and their spirit.

The macro editor is locked until the installation's physical host accepts the Macro Use Agreement in **Settings → Macros**. Acceptance is versioned and persisted; a future policy revision can require a fresh acknowledgement. Shipped built-ins can still be run before acceptance, but custom macros and all create/edit/delete/reset operations remain locked.

Custom macros are limited to **12 linear steps**, have no loops or conditionals, and are capped at a 60-second static timeline and a 90-second execution window. Text steps are limited to 256 characters with at most 250 ms between characters. Built-ins have a separate internal step allowance because `plot_route` needs a longer menu sequence, but edited built-ins remain restricted to a logistics/menu-navigation action allowlist, unmodified Escape for raw-key steps, and declared parameter placeholders for text.

Macro definitions are edited under **Settings → Macros** after agreement. The Action Recorder can record a sequence directly into the editor; recordings longer than 12 logical steps are truncated to the first 12 before review. Unambiguous recorded bindings are saved as Elite binding actions, while ambiguous/unmapped presses retain their physical scan-code key.

Step types are `binding`, `raw`, and `text`. `delayMs` is the wait before the step. `holdMs` is optional for binding/raw steps; zero means a normal tap. Text steps support `intervalMs` and declared `{{parameter}}` placeholders. JACoB blocks known OS-global shortcuts for raw steps and serializes macro execution so two macros cannot interleave.

Current Frontier terms: https://www.frontier.co.uk/legal/eula and https://www.frontier.co.uk/legal/frontier-code-of-conduct
