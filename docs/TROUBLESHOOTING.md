# Troubleshooting

## Build fails

Confirm `go version` meets `go.mod`, `wails version` matches the Wails version in `go.mod` (`go list -m github.com/wailsapp/wails/v2`) and `wails doctor` finds the native prerequisites. The frontend needs no npm install. On Linux, install WebKitGTK 4.1 development packages and use `-tags webkit2_41` for native tests/builds. A headless host cannot exercise desktop interaction.

Use the repository build scripts. They use checked-in assets and bindings and bypass Wails' older package-analysis step, which cannot read the patched Go toolchain's export data. They also prevent Wails from editing `go.mod`. If Windows cannot replace an executable in `dist`, exit that copy first or build with `-OutputDirectory` pointing to another folder.

## Windows app will not open

Install WebView2 if it's missing, and check that the executable matches your CPU architecture. Download the latest release instead of running an old binary.

## macOS says the app can't be opened

Packages aren't notarised. Open **System Settings**, then **Privacy & Security**, and choose **Open Anyway** for TestDesk. Make sure you downloaded the package for your chip (Apple silicon is `arm64`, Intel is `amd64`).

## Tray or MCP startup is unavailable

The desktop's tray-backed service must become ready before MCP is admitted. Check platform tray support and leave TestDesk running. Linux environments may require an AppIndicator extension.

For the standalone CLI, set `TESTDESK_DESKTOP_EXECUTABLE` if the desktop binary is not beside it. Use an absolute path in MCP configuration and restart the client after editing it. **Exit TestDesk** ends its service and connected bridges.

## No tests, ambiguous selection or an unexpected unknown result

Inspect provider settings and run discovery again after upgrading. Ensure the actual runner and any required reporter are installed in the project. Use IDs from the catalog; names may be ambiguous. Missing, invalid or unmatched reports do not prove that a test passed. See [provider limitations](PROVIDERS.md).

On Windows, npm launchers depend on `cmd.exe`. An inherited PATH longer than its 8,191-character limit can disappear inside that shell, producing misleading “not recognized” errors for installed tools. Try a fresh terminal with a shorter PATH containing the required tools, then restart TestDesk from that environment. Do not remove unrelated tool installations to diagnose this.

## Run is stuck or output is excessive

Pause holds future queue work; it does not suspend a running process. Stop cancels current work and queued runs. Kill currently uses the same manager cancellation path. A process that escapes its group/job may require separate cleanup.

Each stdout/stderr capture is limited to 32 MiB. If exceeded, TestDesk cancels the command and rejects its incomplete output. Reduce runner verbosity or divide the scope, then rediscover or rerun. Logs and runner-created report files still consume disk; configure retention and review disk usage.

## Undo an agent installation

Restore the relevant `.testdesk-backup-*` configuration or skill file and restart the client. For an installation that created a new configuration entry, remove that TestDesk entry manually. Remove only the TestDesk block from shared instruction files; preserve other project guidance.

## Window or compact controller position is wrong

Use **Reset widget position** in Settings. Management views use native window chrome, while the compact controller is frameless. Rebuild before diagnosing a version-specific window issue. Platform behavior needs native reproduction.

## Collect useful diagnostics

Include the version/commit, OS, architecture, runner version, selected project/provider and minimal reproduction. Request bounded MCP diagnostics or copy relevant output. Redact credentials, command arguments and personal paths. See [support](../SUPPORT.md).
