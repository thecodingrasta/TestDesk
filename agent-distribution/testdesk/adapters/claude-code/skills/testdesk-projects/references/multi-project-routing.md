# Multi-Project Routing and Workspace Configuration

## Per-project isolation

- Each registered project maintains its own manager, queue, `.testdesk/` catalog, run history JSONL, logs, and artifacts. Multiple projects can discover and run tests concurrently.
- Each MCP connection maintains its own session default project. Calling `testdesk_project_activate` updates only that connection's default (and user preferences), not other connected agents or GUI windows.
- If two registered projects share the same display name, pass the full absolute root path in `project` to prevent ambiguity errors.

## Adding, configuring, and removing projects

- **`testdesk_project_add`:** Requires an existing directory `projectRoot`. Creates `testdesk.json` with auto-detected built-in providers only if absent; existing `testdesk.json` files are loaded without overwriting custom configuration.
- **`testdesk_project_configure`:** Runs built-in provider detection and merges missing built-ins without touching custom providers. Pass `replaceProviders: true` only for an intentional reset. Rejected while work is active or queued in that project.
- **`testdesk_project_forget_missing`:** Removes one registered `projectRoot` from global preferences only after verifying the directory no longer exists on disk and no active session manager holds it loaded. Never deletes files from disk.
- **`testdesk_skills_install`:** Writes delimited `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `.github/copilot-instructions.md` blocks (preserving content outside `<!-- testdesk:start/end -->` tags) plus `.agents/skills/`, `.claude/skills/`, `.gemini/skills/testdesk/`, and `.cursor/rules/testdesk.mdc`, creating `.testdesk-backup-*` backups before overwriting differing skill files.
