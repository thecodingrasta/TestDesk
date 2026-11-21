# TestDesk Host Setup and Binary Prerequisites

TestDesk MCP plugins and skills connect over stdio to a local TestDesk executable (`testdesk mcp` on Linux/macOS, `TestDesk.exe mcp` or `testdesk.exe mcp` on Windows). The plugin packages do not bundle the desktop binary.

## Runtime prerequisites

1. Install the matching TestDesk desktop release on the host where the agent runs (see `docs/INSTALL.md`).
2. Confirm the version matches the plugin version (`testdesk version` or `testdesk-cli version`).
3. Ensure `testdesk` (or `TestDesk.exe`) is on `PATH` when using portable `plugin.json` / `mcp.json`, or use an explicit absolute executable path in client configuration.
4. When invoking `testdesk-cli mcp`, keep the desktop executable in the same directory or set `TESTDESK_DESKTOP_EXECUTABLE` to its absolute path.

## Host configuration formats

| Host | Config path | Format / key | Install / backup behavior |
| --- | --- | --- | --- |
| Claude Code | `.mcp.json` (project) | `mcpServers.testdesk` | `testdesk mcp-install --target claude-code` writes `.testdesk-backup-*` first |
| Codex | `~/.codex/config.toml` | `[mcp_servers.testdesk]` TOML | `testdesk mcp-install --target codex` replaces only its table after backup |
| Antigravity / Gemini | `~/.gemini/config/mcp_config.json` | `mcpServers.testdesk` | `testdesk mcp-install --target antigravity` |
| Cursor | `.cursor/mcp.json` | `mcpServers.testdesk` | `testdesk mcp-install --target cursor` |
| VS Code / Copilot | `.vscode/mcp.json` | `servers.testdesk` (`type: "stdio"`) | `testdesk mcp-install --target cline` or repository marketplace |
| OpenCode | `opencode.json` | `mcp.testdesk` (`type: "local"`) | `testdesk mcp-install --target opencode` |
| Windsurf | `~/.codeium/windsurf/mcp_config.json` | `mcpServers.testdesk` | `testdesk mcp-install --target windsurf` |

Restart the client host after modifying MCP configuration or installing a plugin.

## Standalone CLI subcommands

`testdesk-cli` executes commands directly without requiring a persistent UI session (`mcp` still connects to or starts the desktop owner):

- `testdesk-cli version`
- `testdesk-cli init --project <path>`
- `testdesk-cli doctor --project <path>`
- `testdesk-cli discover --project <path> [--json]`
- `testdesk-cli list --project <path> [--provider ...] [--layer ...] [--status ...] [--query ...] [--limit 200] [--json]`
- `testdesk-cli run --project <path> [--tests id1,id2 | --providers ... | --query ...] [--wait=true] [--json]`
- `testdesk-cli status --project <path> [--tests] [--json]`
- `testdesk-cli control --project <path> --action pause|resume|stop|kill|skip`
- `testdesk-cli diagnostics --project <path> [--run id] [--max-bytes 4096] [--json]`
- `testdesk-cli mcp [--project <path>]`
- `testdesk-cli mcp-targets [--project <path>]`
- `testdesk-cli mcp-install --project <path> --target <id>`
- `testdesk-cli mcp-smoke [--project <path>]`

Use `TESTDESK_CONFIG_HOME` to isolate global preferences and desktop lifecycle files during automated checks or multi-environment verification.
