# TestDesk MCP Tool Contracts and Semantics

TestDesk exposes 24 MCP tools over stdio JSON-RPC (`2025-06-18`), plus 3 resources and 3 prompts. Call `tools/list` on the live server whenever you need exact JSON Schemas.

## Tool inventory and arguments

| Tool | Purpose | Key arguments |
| --- | --- | --- |
| `testdesk_capabilities` / `testdesk_guide` | Workflow, filters, controls, safety rules | none |
| `testdesk_projects` | Page registered projects and active root | `limit` (default 50, max 100), `offset` |
| `testdesk_project_add` | Register folder; creates `testdesk.json` if missing | `projectRoot` (abs), `name`, `activate` (default true) |
| `testdesk_project_activate` | Change this session's default project | `project` or `projectRoot` |
| `testdesk_project_configure` | Detect/merge providers on an idle project | `project`, `replaceProviders` (bool) |
| `testdesk_project_forget_missing` | Remove one missing, unloaded root from registry | `projectRoot` (abs) |
| `testdesk_snapshot` | Compact lifecycle state (`tests` omitted by default) | `project`, `includeTests`, filters, `limit`, `offset` |
| `testdesk_tests` / `testdesk_catalog` | Paged compact catalog (`id`, provider, layer, status) | `project`, `query`, `providerIds`, `providerKinds`, `statuses`, `layers`, `modules`, `tags`, `limit`, `offset` |
| `testdesk_discover` | Run enabled discovery commands; supports `_meta.progressToken` | `project` |
| `testdesk_discovery_status` | Latest discovery phase, timestamps, quiet duration | `project` |
| `testdesk_plan` | Dry-run items and overlap warnings without executing | `project`, `testIds`, filters, `limit` |
| `testdesk_run` | Queue items; rejects contradictory IDs plus filters | `project`, `testIds`, filters, `limit` |
| `testdesk_run_status` | Compact active and recent run summary | `project` |
| `testdesk_control` | Control active/queued run | `project`, `action` (`pause`, `resume`, `stop`, `kill`, `skip`) |
| `testdesk_wait` | Wait until idle or timeout; returns snapshot | `project`, `timeoutMs` (default 30000) |
| `testdesk_diagnostics` | Bounded run log and summary | `project`, `runId`, `maxBytes` (default 32768, cap 512000), `includeRun`, `includeLog` |
| `testdesk_artifacts` | Diagnostics and run directory paths (never inlines logs) | `project`, `runId` |
| `testdesk_history` | Paged past runs | `project`, `limit` (default 20, max 100), `offset`, `status` |
| `testdesk_settings` | Active project name, providers, and UI config | `project` |
| `testdesk_test_details` | Definition, latest duration, matching failure | `project`, `testId`, `name` |
| `testdesk_rerun_failed` | Re-run failed/skipped items from run or latest failure | `project`, `runId` |
| `testdesk_skills_install` | Install/update project instruction blocks and skills | `project` |

## Resources and prompts

- Resources (`resources/list`, `resources/read`): `testdesk://guide`, `testdesk://capabilities`, `testdesk://projects`.
- Prompts (`prompts/list`, `prompts/get`): `testdesk-verify-changes`, `testdesk-debug-failure`, `testdesk-plan-suite`.

## Concurrency, timeouts, and disconnects

- **Project scoping:** Every project-scoped tool accepts an optional `project` (unique display name or absolute root). Supplying `project` targets that call without altering another client's or your own session default.
- **Timeouts:** If `testdesk_wait` returns after `timeoutMs` with `running` still set, execution continues in the background. Call `testdesk_wait` again or inspect `testdesk_run_status`.
- **Progress and cancellation:** Include a string or integer `_meta.progressToken` (up to 256 encoded bytes) on `testdesk_discover` to receive `notifications/progress`. Send `notifications/cancelled` with `requestId` to cancel your pending discovery or wait (up to 16 concurrent long requests per connection).
- **Disconnects:** Closing the stdio MCP client cancels work started/awaited on that client's context without draining unrelated sessions sharing the desktop owner.
