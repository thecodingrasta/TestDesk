# Agent integration

Use the desktop's **Settings** screen to install a detected MCP target or copy its manual configuration. Installation is explicit and may write client configuration plus project instructions. Restart the client to load it. Any stdio MCP host can launch the desktop executable with `mcp`; use an absolute executable path.

```json
{
  "mcpServers": {
    "testdesk": {
      "command": "/absolute/path/to/testdesk",
      "args": ["mcp"]
    }
  }
}
```

Client schemas differ. Use the [Codex example](../examples/codex-mcp-config.toml) for TOML clients and the target's generated manual snippet for other clients. TestDesk does not authenticate you to a model provider.

## Harness coverage

| Harness | Installed project guidance | MCP setup |
|---|---|---|
| Codex | `.agents/skills/testdesk-*/SKILL.md`, `AGENTS.md` block | Codex target or TOML snippet |
| Claude Code | `.claude/skills/testdesk-*/SKILL.md`, `CLAUDE.md` block | Claude target |
| Gemini / Antigravity | `.gemini/skills/testdesk/SKILL.md`, `GEMINI.md` block | Antigravity target or host-specific manual stdio setup |
| Cursor | `.cursor/rules/testdesk.mdc` | Cursor target |
| GitHub Copilot | `.github/copilot-instructions.md` block; shared skills can be read explicitly | Configure MCP in the host; no dedicated automatic target |
| Windsurf, Cline, OpenCode | Shared guide and `AGENTS.md` where the host supports it | Named targets; guidance auto-loading depends on the host |
| Other MCP hosts | Read this guide or `testdesk://guide` | Manual stdio setup |

Codex's repository skill path follows [official Codex skill discovery](https://developers.openai.com/codex/skills). MCP connectivity and automatic instruction loading are separate capabilities. A generated configuration is not proof that every version of a client has been exercised.

Call `testdesk_skills_install` with an explicit `project` to provision guidance in another trusted repository. MCP target installation also provisions it when a project root is supplied. The four skill topics are setup, running, debugging and multiple projects. Install preserves text outside TestDesk's delimited blocks and backs up differing skill files as `.testdesk-backup-*`. Backups may contain private instructions; keep them out of Git. A later provisioning error is reported even if the MCP configuration was already written.

## Workflow

1. Inspect `testdesk_capabilities` and compact `testdesk_snapshot` state.
2. Find IDs through `testdesk_tests` with narrow filters. Specify `project` when working across repositories.
3. Preview broad scopes with `testdesk_plan`; inspect provider-wide items and overlap warnings.
4. Queue the intended IDs or filters with `testdesk_run`, then use `testdesk_wait` and `testdesk_run_status`.
5. On failure, request diagnostics with the run ID and `maxBytes: 4096`. Use `testdesk_test_details` with a test ID for ambiguous names.
6. Fix the cause and call `testdesk_rerun_failed` for that project and run.

A wait timeout leaves work running. Project controls affect the selected project's queue, so do not interrupt unrelated work. Pause holds the queue. Stop and kill currently share cancellation behavior; kill is not a separate stronger termination path.

## MCP reference

[MCP.md](MCP.md) documents routing, discovery progress, cancellation, pagination, combined selectors and result identity. Use `tools/list` for the current tool names and input schemas, rather than relying on a copied fixed tool count.

Resources: `testdesk://guide`, `testdesk://capabilities`, `testdesk://projects`.

Prompts: `testdesk-verify-changes`, `testdesk-debug-failure`, `testdesk-plan-suite`.

Provider commands run with local user permissions. Review configuration before executing an untrusted repository, and redact diagnostics before sharing them. See [security](../SECURITY.md).
