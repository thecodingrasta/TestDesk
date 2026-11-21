---
name: testdesk-debugging
description: Diagnose failed or cancelled TestDesk runs using bounded logs, test identity, artifact paths, and focused reruns after code edits. Use when investigating test failures, unknown outcomes, or verifying fixes.
license: GPL-3.0
---

# Diagnose failures and rerun failed items

1. Locate recent failures with `testdesk_history` (`status: "failed"`, small `limit`).
2. Call `testdesk_diagnostics` with `project`, `runId`, and `maxBytes: 4096` before expanding log limits.
3. For ambiguous test names or case-level history, call `testdesk_test_details` with the catalog's exact `testId`.
4. Call `testdesk_artifacts` with `runId` when report paths or the `.testdesk/runs/<runId>` directory are needed. Artifact contents are never inlined.
5. Never treat missing, incomplete, or `unknown` report evidence as a passing test. Fix the underlying cause first.
6. Verify the fix with `testdesk_rerun_failed` for the same `project` and `runId`, then call `testdesk_wait`.

Redact tokens, secrets, and private filesystem paths before sharing diagnostics. Read [diagnostics and evidence](references/diagnostics-and-evidence.md) for ID/name matching, size limits, and mixed-scope rerun guarantees.
