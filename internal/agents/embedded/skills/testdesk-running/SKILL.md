---
name: testdesk-running
description: Plan, queue, wait for, and control TestDesk test runs using catalog IDs or narrow filters. Use when refreshing discovery, previewing broad test scope, executing tests, monitoring progress, or pausing/stopping/skipping an active run.
license: GPL-3.0
---

# Plan, run, and control tests

1. Inspect state with `testdesk_snapshot` (`includeTests: false`). Refresh with `testdesk_discover` only if tests are missing or newly added.
2. Locate IDs with `testdesk_tests`, narrowed by `project`, `providerIds`, `layers`, `modules`, `statuses`, `tags`, or `query`.
3. Call `testdesk_plan` before any provider-wide or multi-provider filter run. Check total execution items, provider-wide fallbacks, and `.NET` overlap warnings.
4. Queue execution with `testdesk_run` using exact `testIds` when possible. Do not combine `testIds` with contradictory filters.
5. Call `testdesk_wait` (`timeoutMs: 30000`) and inspect `testdesk_run_status`. A timeout does not cancel the run.
6. Control runs only for the intended `project` using `testdesk_control` (`pause`, `resume`, `skip`, `stop`, `kill`). Do not stop another client's active work without authorization.

Read [selection and controls](references/selection-and-controls.md) for filter rejection rules, discovery monitoring (`testdesk_discovery_status`), progress tokens, and cancellation semantics.
