---
name: testdesk-debugging
description: Diagnose a failed TestDesk run using bounded logs, test identity, result evidence and a focused rerun after a fix.
---

# Diagnose failures

Find the failing run with `testdesk_history` using `status: "failed"` and a small `limit`. Request `testdesk_diagnostics` with its `runId`, the intended `project` and `maxBytes: 4096`.

Use `testdesk_test_details` with the catalog's `testId` when names are ambiguous. Use `testdesk_artifacts` for report references. Missing or malformed result evidence can make a run fail while the selected case remains unknown; do not turn unknown into a passing result.

Identify the cause before editing. After a fix, call `testdesk_rerun_failed` with the same project and run ID. Wait for its result. Keep the checks focused, and redact logs and environment values before sharing them.
