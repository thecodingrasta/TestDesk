# Install TestDesk

Download a prebuilt binary or package from [GitHub Releases](https://github.com/thecodingrasta/TestDesk/releases). No Go toolchain, Wails CLI, or build tools are required unless building from source.

## Release downloads

Each release builds four platform targets and publishes one combined `SHA256SUMS` file:

| System | App (single file) | Optional CLI | Full package (with licence texts) |
| --- | --- | --- | --- |
| Windows, 64-bit (`amd64`) | `TestDesk-<version>-windows-amd64.exe` | `testdesk-cli-<version>-windows-amd64.exe` | `TestDesk-<version>-windows-amd64.zip` |
| Linux, 64-bit (`amd64`) | `TestDesk-<version>-linux-amd64` | `testdesk-cli-<version>-linux-amd64` | `TestDesk-<version>-linux-amd64.tar.gz` |
| macOS, Apple silicon (`arm64`) | Inside full package (`.app`) | `testdesk-cli-<version>-macos-arm64` | `TestDesk-<version>-macos-arm64.tar.gz` |
| macOS, Intel (`amd64`) | Inside full package (`.app`) | `testdesk-cli-<version>-macos-amd64` | `TestDesk-<version>-macos-amd64.tar.gz` |

The desktop app also runs the local MCP server (`testdesk mcp`), so it works standalone for agent integrations. A full package includes the desktop app (a `.app` bundle on macOS), `testdesk-cli`, `LICENSE`, `THIRD_PARTY_NOTICES.md`, `third_party/licenses`, and an inner `SHA256SUMS`. Agent host plugin ZIPs (`testdesk_agent_*.zip` and `AGENT_PLUGIN_SHA256SUMS`) are published alongside the release when using standalone plugin packages.

Verify downloads against `SHA256SUMS` from the same release before opening:

- **Windows (PowerShell):** `Get-FileHash .\<file> -Algorithm SHA256` and compare with the matching line in `SHA256SUMS`.
- **Linux:** `sha256sum -c SHA256SUMS --ignore-missing`
- **macOS:** `shasum -a 256 -c SHA256SUMS --ignore-missing`

## Windows

1. Download `TestDesk-<version>-windows-amd64.exe` (or extract `TestDesk-<version>-windows-amd64.zip`).
2. Double-click `TestDesk.exe` anywhere, or place both `testdesk.exe` and `testdesk-cli.exe` in a user folder on `PATH`:

```powershell
New-Item -ItemType Directory -Force "$HOME\bin" | Out-Null
Move-Item .\TestDesk-*-windows-amd64.exe "$HOME\bin\testdesk.exe"
Move-Item .\testdesk-cli-*-windows-amd64.exe "$HOME\bin\testdesk-cli.exe"
[Environment]::SetEnvironmentVariable('Path', "$([Environment]::GetEnvironmentVariable('Path', 'User'));$HOME\bin", 'User')
```

Open a new terminal and run `testdesk`. Keep both executables together so `testdesk-cli mcp` finds the desktop app without extra environment variables.

If Windows SmartScreen warns you, choose **More info**, then **Run anyway** (builds are not code-signed yet). Windows 11 includes Microsoft Edge WebView2; on Windows 10, install the [Evergreen WebView2 Runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/) if the window fails to open.

To create a Start Menu shortcut without administrator rights from a source checkout, point the helper script at the extracted binary:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\windows\Install-TestDesk.ps1 -Source "C:\path\to\TestDesk.exe"
```

Add `-DesktopShortcut` for a desktop shortcut or `-NoLaunch` to install without opening the window. Files install under `%LOCALAPPDATA%\Programs\TestDesk`.

## Linux

Install GTK 3, WebKitGTK 4.1, and AppIndicator libraries first (Ubuntu 24.04 package names shown):

```bash
sudo apt-get install libgtk-3-0t64 libwebkit2gtk-4.1-0 libayatana-appindicator3-1
install -D -m 755 TestDesk-*-linux-amd64 ~/.local/bin/testdesk
install -D -m 755 testdesk-cli-*-linux-amd64 ~/.local/bin/testdesk-cli
testdesk
```

Add `~/.local/bin` to `PATH` if your shell does not already include it, or run `./TestDesk-*-linux-amd64` directly after `chmod +x`. Keep `testdesk` and `testdesk-cli` in the same folder. Tray icon visibility depends on your desktop environment and may require an AppIndicator extension on GNOME.

## macOS

1. Extract the `.tar.gz` matching your chip (check Apple menu, **About This Mac**) and drag `TestDesk.app` into **Applications**.
2. Launch it. Because binaries are not signed or notarised yet, open **System Settings**, then **Privacy & Security**, and choose **Open Anyway**. Never disable Gatekeeper globally.

For the optional `testdesk-cli`, mark it executable and place it in `/usr/local/bin/testdesk-cli` or `~/.local/bin/testdesk-cli`. If macOS blocks the CLI after checksum verification, run `xattr -d com.apple.quarantine <file>`. Point `TESTDESK_DESKTOP_EXECUTABLE` to `/Applications/TestDesk.app/Contents/MacOS/testdesk` if invoking `testdesk-cli mcp`.

Automated checks start the macOS app and drive it over MCP on every CI run, while hands-on desktop testing on physical Mac hardware has not yet been performed. Share results with a [compatibility report](https://github.com/thecodingrasta/TestDesk/issues/new?template=compatibility_report.yml).

## The CLI

`testdesk-cli` runs standalone commands (`version`, `init`, `doctor`, `discover`, `list`, `run`, `status`, `control`, `diagnostics`, `mcp-targets`, `mcp-install`, `mcp-smoke`) as well as `mcp` (which connects to or launches the desktop owner beside it or via `TESTDESK_DESKTOP_EXECUTABLE`). Run `testdesk-cli` with no arguments for full usage. Use MCP or the desktop app for persistent queue/compact-widget sessions.

## Update and uninstall

Quit TestDesk and any connected MCP clients, replace the executables (or `.app` bundle) with the new release, launch TestDesk, and restart the clients. Project settings in `testdesk.json` and `.testdesk/` workspaces stay intact in each repository.

To uninstall, quit the app, delete the executable folder (or `.app` bundle) and shortcuts, remove TestDesk entries from your MCP client configs, and optionally remove the OS user config folder documented in [configuration](CONFIGURATION.md#locations-and-environment).

## Build from source (advanced)

Needed only when modifying TestDesk or running an unreleased commit.

Prerequisites:
- Go matching `go.mod`
- Node.js 22+ (runs plain-JS tests and checks; the shipped frontend uses zero npm packages)
- Python 3.12+ (for repository checks and packaging scripts)
- [Wails v2 CLI](https://wails.io) pinned to `go.mod` (`python scripts/package.py` installs the pinned version automatically if missing)
- Platform WebView/C libraries (`wails doctor` reports gaps)

```powershell
# Windows
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\windows\Build-TestDesk.ps1
.\dist\TestDesk.exe
```

```bash
# Linux (install build-essential, pkg-config, libgtk-3-dev, libwebkit2gtk-4.1-dev, libayatana-appindicator3-dev)
bash scripts/build.sh
./dist/testdesk
```

```bash
# macOS (install Xcode Command Line Tools: xcode-select --install)
bash scripts/build.sh
open dist/*.app
```

Run `python scripts/package.py` to build the full release archive and `.sha256` file in `dist/`. Run `scripts/validate.ps1` (Windows) or `bash scripts/validate.sh` (Linux/macOS) before committing changes.
