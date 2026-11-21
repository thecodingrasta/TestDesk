# TestDesk Selection, Planning, and Run Controls

## Discovery and stall checks

- Call `testdesk_discover` to refresh enabled providers. Pass `_meta.progressToken` (string or integer, <= 256 bytes) to receive `notifications/progress` as providers finish.
- If a discovery takes longer than expected, poll `testdesk_discovery_status` to check active provider, phase, start timestamp, and `quietMillis` since the last stdout/stderr line. Output buffering in external test runners means `quietMillis` alone does not prove a hang.
- Discovery is serialized per project while independent across different projects.

## Selection and `testdesk_plan` behavior

- Combine filters across `providerIds`, `providerKinds`, `layers`, `modules`, `statuses`, `tags`, and `query`. Passing `testIds` alongside other filters requires every requested ID to satisfy those constraints; mismatched combinations are rejected rather than partially run.
- A provider-only request runs discovered cases for providers that have them and one provider-wide item for enabled providers without discovered cases. Always inspect `testdesk_plan` items first so you know whether an execution item represents a single case, a file/target, or an entire provider.
- Unknown or disabled provider IDs fail validation even in mixed lists.
- Mixed or multi-target `.NET` providers can emit duplicate case-name warnings; inspect provider IDs explicitly instead of guessing.

## Controls and cancellation

- `testdesk_control` actions:
  - `pause` / `resume`: pauses future queued items (does not suspend an already running subprocess) or resumes the queue.
  - `skip`: skips the active execution item cleanly.
  - `stop` / `kill`: cancel the active process tree and queued runs. Currently `stop` and `kill` share the same manager cancellation path; `kill` is not a separate escalated signal.
- Use `notifications/cancelled` with your JSON-RPC `requestId` to abort an in-flight `testdesk_discover` or `testdesk_wait`. Disconnecting the MCP client also cancels its owned long requests.
- Each connection permits up to 16 concurrent pending wait/discovery requests.
