# TestDesk Diagnostics, Result Evidence, and Reruns

## Bounded diagnostics and artifacts

- `testdesk_diagnostics` defaults to 32,768 bytes (`maxBytes`) and enforces a hard upper cap of 512,000 bytes. Start with `maxBytes: 4096` and set `includeRun` / `includeLog` as needed.
- `runId` must be a single local path component (path traversal segments are rejected).
- `testdesk_artifacts` returns paths to the diagnostics file and the run directory under `.testdesk/runs/<runId>`; inspect generated XML/TRX/JSON files directly only when summarized diagnostics are insufficient.
- Stdout and stderr captures are bounded at 32 MiB per stream during execution. Exceeding 32 MiB cancels the run with an explicit capture-limit error.

## Test identity and `unknown` outcomes

- `testdesk_test_details` accepts `testId` or unique `name`. If `name` matches more than one test in the project, the call rejects ambiguity and requires `testId`. If both `testId` and `name` are provided, they must refer to the same test.
- TestDesk combines process exit status with structured report evidence (`go-json`, `vitest-json`, `junit`, `trx`, `trx-dir`, `cargo-text`, `junit-dir`, `none`). If a multi-target or custom run omits or duplicates identity records so a case cannot be matched unambiguously, that test remains `unknown` rather than falsely turning green.

## `testdesk_rerun_failed` behavior

- `testdesk_rerun_failed` re-queues both failed/skipped individual test items and provider-wide failed items from the specified `runId` (or latest failed run when `runId` is omitted).
- When `runId` is explicitly supplied and does not exist in history, the call returns an error instead of falling back to another run.
- After `testdesk_rerun_failed` returns a new `runId`, wait for completion with `testdesk_wait` and inspect `testdesk_diagnostics`.
