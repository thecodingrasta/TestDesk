# Architecture

TestDesk has a Wails desktop entry point at `main.go` and a standalone CLI at `cmd/testdesk`. The frontend is embedded HTML, CSS and JavaScript under `internal/desktop/assets`. Generated Wails bindings are tracked so packaging can use `-skipbindings`.

## Runtime ownership

The desktop owns project managers and the local MCP service. A stdio MCP bridge starts or connects to that desktop through the lifecycle package's authenticated loopback connection. Client disconnects cancel work owned by that client. Managers are shared within the desktop, while each MCP session has its own default project.

Each project manager owns its queue, catalog and current execution state. Manager mutexes protect shared state; discovery is serialized per project. Different projects can execute concurrently. Returned run snapshots are copied to avoid exposing state that the queue is mutating.

## Execution and results

Configuration selects provider commands and parsers. Discovery records identities used by targeted execution; identity requirements vary across Go packages, .NET cases, Cargo targets and JavaScript projects. Missing or ambiguous identities fail selection rather than broadening a requested run.

The runner captures stdout/stderr, streams events and enforces timeouts. Each stream has a 32 MiB capture limit, including unterminated lines. Exceeding it cancels execution and reports an error; incomplete output is discarded. Windows uses kill-on-close job objects. Unix uses process groups and bounded output-pipe waits. Processes that escape those boundaries are not guaranteed to be contained.

The manager combines command status with parsed result evidence. A failed command makes the run fail; conclusive per-test report evidence can still retain that case's outcome. Missing or malformed reports do not establish a pass. Exit-only custom providers intentionally use a different contract. See [providers](PROVIDERS.md).

## Storage

Each project's workspace defaults to `.testdesk/`. It contains a catalog, JSON-lines history, run logs, reports and diagnostics. OS file locks coordinate catalog/history updates and active run leases across processes. Catalog, history and preference replacement uses temporary files and rename; this is not a transactional database or a guarantee of power-loss durability. Diagnostics are written separately from history.

Run IDs are checked before diagnostics reads. New runtime directories and store files use private Unix permissions; existing files and externally generated report permissions are not rewritten. Retention removes eligible old run data while preserving known active or unrecognized ownership state.

Global preferences and lifecycle files are under the OS user configuration directory's `TestDesk` folder. `TESTDESK_CONFIG_HOME` overrides the base directory for isolated runs. See [configuration](CONFIGURATION.md) and [security](../SECURITY.md).

## Agent assets

`internal/agents` detects MCP targets, writes client configuration and provisions instruction blocks and skills. Existing MCP configurations and differing skill files receive backups. Content outside delimited TestDesk instruction blocks is retained. `scripts/sync-agent-assets` keeps the published harness assets aligned with the application's templates.

Windows and Linux use systray's platform loops. macOS adds a native status item to Wails' existing AppKit loop without replacing its application delegate.
