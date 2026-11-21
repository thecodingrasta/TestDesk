# Agent Distribution Toolkit

`go run ./tools/agentdist` validates, synchronizes, and packages TestDesk's canonical Agent Plugin (`agent-distribution/testdesk/plugin/`) and native host adapters from one canonical definition (`agent-distribution/testdesk/agentdist.json`).

## Commands

```sh
go run ./tools/agentdist validate --release-version "$(cat VERSION)"
go run ./tools/agentdist sync-skills --overwrite
go run ./tools/agentdist package --output dist
```

- `validate` checks Agent Plugins 1.0 manifests, native adapter layouts, Agent Skills frontmatter and `references/` links, marketplace catalogs (`.github/plugin/`, `.claude-plugin/`, `.cursor-plugin/`), MCP tool contracts against `internal/mcp/server.go` and `internal/mcp/v1.go`, CLI help commands against `cmd/testdesk/main.go`, synchronized skill trees, and license notices.
- `sync-skills` writes canonical skills and notice files to declared repository publication sources without overwriting modified copies unless `--overwrite` is supplied. Pair with `go run ./scripts/sync-agent-assets` to keep embedded binary assets (`internal/agents/embedded/skills`) and repository skill copies aligned.
- `package` writes nine deterministic ZIP archives (`testdesk_agent_*.zip`) and `AGENT_PLUGIN_SHA256SUMS` into a fresh output directory without overwriting existing files. The ZIP archives bundle manifests, skills, README, and notice files; install the matching native `TestDesk` / `testdesk` release executable separately on `PATH` or with an explicit absolute path.
