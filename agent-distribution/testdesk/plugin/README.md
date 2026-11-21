# TestDesk Agent Plugin

Run, monitor, and diagnose tests across configured local projects from your AI agent host. This plugin pairs four progressive-disclosure Agent Skills (`testdesk-guide`, `testdesk-running`, `testdesk-debugging`, `testdesk-projects`) with TestDesk's local stdio MCP server (`testdesk mcp`).

## Prerequisites

- Install a matching TestDesk desktop release (`testdesk` / `TestDesk.exe`) on your host `PATH`, or update client config to point at the executable's absolute path.
- Keep plugin and desktop versions aligned (`testdesk version`).
- On Windows, if using `testdesk-cli mcp` separately from `TestDesk.exe`, place them in the same folder or set `TESTDESK_DESKTOP_EXECUTABLE`.

## Safety and scope

- Configured provider commands in `testdesk.json` execute with the local user's permissions. Only register and test trusted repositories.
- Request `testdesk_plan` before broad filter runs and bound failure inspection with `testdesk_diagnostics(maxBytes: 4096)`.
- Each MCP connection maintains its own session default and cancels its own long waits/discovery calls on disconnect without interrupting other projects.
