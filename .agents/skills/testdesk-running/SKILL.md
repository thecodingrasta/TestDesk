---
name: testdesk-running
description: Plan, queue, wait for and control TestDesk test runs using explicit test IDs or narrow filters.
---

# Run tests

1. Inspect the selected project's state with `testdesk_snapshot`.
2. Find IDs with `testdesk_tests`, scoped by `project`, `providerIds`, `layers`, `modules` or `query` as needed.
3. Use `testdesk_plan` before broad scopes. Inspect provider-wide items and overlap warnings.
4. Call `testdesk_run` with IDs returned by discovery. Combined filters constrain every requested ID; contradictory selections are rejected.
5. Call `testdesk_wait` with `timeoutMs: 30000`, then check `testdesk_run_status`. A wait timeout does not cancel execution.
6. Request bounded diagnostics for a failing run and rerun the failed subset after fixing it.

`testdesk_control` accepts `pause`, `resume`, `stop`, `kill` and `skip`. Pause holds the queue; stop cancels current work and queued runs. Stop and kill currently share the manager cancellation path, so kill is not an independent escalation mechanism. Scope controls to the intended project and do not interrupt unrelated work.
