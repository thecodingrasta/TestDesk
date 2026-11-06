# Configuration

Each project has a `testdesk.json` file at its root. TestDesk creates it during setup and normalises safe window sizes and provider defaults.

When the project already has a `.gitignore`, setup appends one idempotent entry for the configured workspace (normally `.testdesk/`) plus a TestDesk attribution link. TestDesk does not create a `.gitignore` and does not add an ignore entry for an absolute workspace path.

Important fields:

- `name`: display name used in project switching.
- `workspace`: per-project state directory; default `.testdesk`.
- `ui.widgetMode`: `off`, `always`, or `when_active`.
- `ui.topmostMode`: `off`, `always`, or `when_active`; controls whether the compact controller stays above other windows.
- `ui.alwaysOnTop`: legacy compatibility value derived from `topmostMode`.
- `providers`: config-driven test adapters.

Never place secrets in `testdesk.json`. Set provider environment values through the runner’s normal safe local environment mechanisms.

See [Providers](PROVIDERS.md) for adapter fields and supported template variables.

## Locations and environment

Global preferences use the OS user configuration directory: `%APPDATA%/TestDesk` on Windows, `~/Library/Application Support/TestDesk` on macOS and `$XDG_CONFIG_HOME/TestDesk` (normally `~/.config/TestDesk`) on Linux. `TESTDESK_CONFIG_HOME` overrides the base directory, with `TestDesk` appended. The project workspace contains private runtime state and should not be committed.

`TESTDESK_DESKTOP_EXECUTABLE` supplies the desktop path for standalone CLI MCP startup. No `.env` loader is built in and no `.env` file is required. Provider commands inherit the parent environment; provider `env` values in `testdesk.json` are literal configuration, so do not put credentials there.
