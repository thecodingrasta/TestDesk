---
name: testdesk-guide
description: Inspect TestDesk capabilities, discover test IDs, set up projects, and find MCP tools, resources, prompts, or CLI commands. Use when starting a TestDesk session, connecting an agent host, or checking available operations on demand.
license: GPL-3.0
---

# TestDesk workflow

Use TestDesk MCP for configured project tests. If a single test selector is already known and needs no queuing, run history, or diagnostics, the native runner can execute directly.

1. Call `testdesk_capabilities` (or `testdesk_guide`) and `testdesk_snapshot` with `includeTests: false` to inspect state without loading large catalogs.
2. Call `testdesk_tests` (alias `testdesk_catalog`) with narrow filters (`project`, `providerIds`, `layers`, `modules`, `statuses`, `tags`, `query`, `limit`, `offset`) and keep exact returned test IDs.
3. Call `testdesk_plan` before broad provider or filter runs to inspect execution items and overlap warnings.
4. Call `testdesk_run`, then `testdesk_wait` (`timeoutMs: 30000`) and `testdesk_run_status`.
5. On failure, call `testdesk_diagnostics` with `runId` and `maxBytes: 4096`, check `testdesk_test_details`, fix the cause, and call `testdesk_rerun_failed`.

Discover additional capabilities on demand: call MCP `tools/list`, `resources/list` (`testdesk://guide`, `testdesk://capabilities`, `testdesk://projects`), and `prompts/list` (`testdesk-verify-changes`, `testdesk-debug-failure`, `testdesk-plan-suite`).

Read `testdesk_settings` to see actual configured commands. Test commands run with your user permissions. Review untrusted `testdesk.json` files before running discovery or tests.

For full tool arguments, timeout/disconnect behavior, and CLI flags, read [tool contracts](references/tool-contracts.md). For host MCP setup and binary prerequisites, read [installation](references/installation.md).
