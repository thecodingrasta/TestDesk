# Install TestDesk

Download a prebuilt file from the [releases page](https://github.com/thecodingrasta/testdesk/releases/latest). You don't need Go, Wails or any build tools. To build from source instead, jump to [Build from source](#build-from-source-advanced).

Each release has these downloads, and a `SHA256SUMS` file covering all of them:

| System | App (a single file) | Command-line tool | Full package |
| --- | --- | --- | --- |
| Windows, 64-bit | `TestDesk-<version>-windows-amd64.exe` | `testdesk-cli-<version>-windows-amd64.exe` | `TestDesk-<version>-windows-amd64.zip` |
| Linux, 64-bit | `TestDesk-<version>-linux-amd64` | `testdesk-cli-<version>-linux-amd64` | `TestDesk-<version>-linux-amd64.tar.gz` |
| macOS, Apple silicon | in the full package | `testdesk-cli-<version>-macos-arm64` | `TestDesk-<version>-macos-arm64.tar.gz` |
| macOS, Intel | in the full package | `testdesk-cli-<version>-macos-amd64` | `TestDesk-<version>-macos-amd64.tar.gz` |

The single files are the quickest way in: download, mark executable where needed, put on your PATH and run. The desktop app is the MCP server too (`testdesk mcp`), so it works on its own. A full package holds the same programs plus `LICENSE`, `THIRD_PARTY_NOTICES.md` and the licence texts. A macOS app is a folder, so it only comes inside the full package. Packages aren't code-signed or notarised, so Windows and macOS ask you to confirm the first launch.

To verify a download, compare its SHA-256 hash with the matching line in `SHA256SUMS`. PowerShell: `Get-FileHash .\<file>`. macOS: `shasum -a 256 <file>`. Linux: `sha256sum -c SHA256SUMS --ignore-missing`.

## Windows

1. Download `TestDesk-<version>-windows-amd64.exe`, and optionally the CLI.
2. Put them in a folder on your PATH under the names `testdesk.exe` and `testdesk-cli.exe`, for example in PowerShell:

```powershell
New-Item -ItemType Directory -Force "$HOME\bin" | Out-Null
Move-Item .\TestDesk-[0-9]*-windows-amd64.exe "$HOME\bin\testdesk.exe"
Move-Item .\testdesk-cli-*-windows-amd64.exe "$HOME\bin\testdesk-cli.exe"
[Environment]::SetEnvironmentVariable('Path', "$([Environment]::GetEnvironmentVariable('Path', 'User'));$HOME\bin", 'User')
```

3. Open a new terminal and run `testdesk`, or double-click the file wherever it is.

If SmartScreen blocks it, choose **More info**, then **Run anyway**.

TestDesk needs the Microsoft Edge WebView2 runtime. Windows 11 includes it; on Windows 10 install the [Evergreen runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/) if the app won't open. TestDesk doesn't bundle or install it.

The CLI starts the desktop it finds beside it, so keep the two files together. To add a Start Menu entry, unzip the full package and run the installer script from a clone of this repository, pointing it at the extracted app:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\windows\Install-TestDesk.ps1 -Source "C:\path\to\TestDesk.exe"
```

It copies the app, CLI and notices to `%LOCALAPPDATA%\Programs\TestDesk` and needs no administrator rights. Add `-DesktopShortcut` for a desktop shortcut or `-NoLaunch` to skip starting the app.

## Linux

Install the runtime libraries, then put the files on your PATH. On Ubuntu 24.04:

```bash
sudo apt-get install libgtk-3-0t64 libwebkit2gtk-4.1-0 libayatana-appindicator3-1
install -D -m 755 TestDesk-*-linux-amd64 ~/.local/bin/testdesk
install -D -m 755 testdesk-cli-*-linux-amd64 ~/.local/bin/testdesk-cli
testdesk
```

`~/.local/bin` is on the PATH of most distributions; add it if yours isn't. Keep `testdesk` and `testdesk-cli` in the same folder so the CLI can start the desktop. Other distributions package these libraries under different names. The tray icon depends on your desktop environment and may need an AppIndicator extension (GNOME, for example).

## macOS

1. Unpack the `.tar.gz` and drag the `.app` into **Applications**.
2. Open it. If macOS says it can't check the app for malicious software, open **System Settings**, then **Privacy & Security**, and choose **Open Anyway**. Only do this for a download you got from the releases page, and don't turn Gatekeeper off.

Use the package that matches your chip. Apple menu, then **About This Mac** shows it. TestDesk uses the system WKWebView and shows a menu bar item.

Automated checks start the macOS app and drive it over MCP on every change, but nobody has used it by hand on a Mac yet. If something looks or behaves wrong, please [report it](https://github.com/thecodingrasta/testdesk/issues/new?template=compatibility_report.yml).

For the command-line tool, run `chmod +x testdesk-cli-*-macos-*` and move it to a folder on your PATH, such as `/usr/local/bin/testdesk-cli`. If macOS blocks it, `xattr -d com.apple.quarantine <file>` removes the flag macOS adds to downloads. To use the CLI or MCP from the terminal, point `TESTDESK_DESKTOP_EXECUTABLE` at the executable inside the app bundle, in `Contents/MacOS`.

## The CLI

`testdesk-cli` runs without the desktop: `init`, `doctor`, `discover`, `list`, `run`, `status`, `control`, `diagnostics` and the MCP setup commands. Run it with no arguments for usage. Each CLI call is a separate process, so use MCP for ongoing control of the desktop.

Its `mcp` command needs the desktop executable. On Windows and Linux, keep `testdesk-cli` and the app (`testdesk` or `TestDesk.exe`) in the same folder and the CLI finds it. Otherwise set `TESTDESK_DESKTOP_EXECUTABLE` to the desktop's absolute path. The app's own `testdesk mcp` command needs no CLI at all.

## Update and uninstall

Quit TestDesk and any connected MCP clients, replace the files with the new package, then start the app and restart the clients. Project settings and `.testdesk/` folders stay in each project. Back up anything important first.

To uninstall, quit the app, delete the install folder (or app bundle) and shortcuts, and remove any MCP entries you added. Project state and user preferences are separate; see [configuration locations](CONFIGURATION.md).

## Build from source (advanced)

You only need this to change TestDesk or run something that hasn't been released.

**You need**
- Go at the version in `go.mod` (Go downloads the exact toolchain if yours is older).
- Node.js 22 or newer, used only to check the frontend. The frontend itself has no npm dependencies.
- Python 3.12 or newer, used by the repository checks and packaging.
- The [Wails v2 CLI](https://wails.io) at the version `go.mod` pins. Find it with `go list -m -f '{{.Version}}' github.com/wailsapp/wails/v2`, then `go install github.com/wailsapp/wails/v2/cmd/wails@<that version>` and put the Go `bin` directory on your PATH. `python scripts/package.py` installs the right version for you if it's missing.
- The platform libraries below. `wails doctor` helps diagnose gaps.

Run everything from the repository root. The scripts download Go modules but don't edit `go.mod` or install system packages.

**Windows**

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\windows\Build-TestDesk.ps1
.\dist\TestDesk.exe
```

This builds `dist\TestDesk.exe` and `dist\testdesk-cli.exe` for Windows amd64. Pass `-Platform windows/arm64` for ARM64; that architecture hasn't been tested on native hardware. Pass `-OutputDirectory path` to build elsewhere, for example if Windows can't replace a running `dist\TestDesk.exe`.

**Linux**

```bash
sudo apt-get install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev libayatana-appindicator3-dev
bash scripts/build.sh
./dist/testdesk
```

These are Ubuntu 24.04 package names. The build and tests use the `webkit2_41` tag, because Wails v2 otherwise selects WebKitGTK 4.0. See the [Wails Linux notes](https://v2.wails.io/docs/gettingstarted/installation/). `bash scripts/install.sh` then copies the app and CLI to `~/.local/bin` (or `XDG_BIN_HOME`) and the notices to your user data directory.

**macOS**

```bash
xcode-select --install
bash scripts/build.sh
open dist/*.app
```

Build on the architecture you want to run. `bash scripts/install.sh` copies the app to `~/Applications` and the CLI to `~/.local/bin`.

**Package like a release.** `python scripts/package.py` builds the native app and writes the archive and checksum to `dist/`, the same way a release does. `bash scripts/validate.sh` (or `scripts/validate.ps1` on Windows) runs the checks first.
