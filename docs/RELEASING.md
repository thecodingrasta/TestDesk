# Releases

Releases are built by GitHub Actions on Windows, Linux, and macOS runners and published by a maintainer. Nothing publishes automatically.

## What runs

**[Native validation](../.github/workflows/ci.yml)** runs on every pull request and push to `main`, and at the start of every draft release build.

| Job | What it checks |
| --- | --- |
| Validate (Windows `amd64`, Linux `amd64`, macOS Intel `amd64`, macOS Apple silicon `arm64`) | Formatting (`gofmt`), `go vet`, Go and JavaScript test suites, documentation links and anchors (`scripts/check_repository.py`), skill asset drift (`sync-agent-assets -check`), `tools/agentdist validate --release-version`, version agreement (`scripts/check_version.py`), script unit tests, and native packaging (`scripts/package.py` + `scripts/check_package.py`). Windows and macOS also run `--desktop-smoke` against the packaged executable over MCP. macOS also runs the CLI pairing check from the extracted package and checks native Mach-O architecture and bundle version. Linux and macOS run `go test -race`; Windows race coverage is a documented gap until its runner toolchain is verified. |
| Real providers (same four platforms) | Real Cargo, .NET 8/10, Jest, and Vitest 4 fixtures with version-pinned tooling. |
| Vulnerabilities (Linux, Windows, macOS) | `govulncheck -tags=production,webkit2_41 ./...` against production imports. |
| Workflow syntax | `actionlint` validation on `.github/workflows/`. |

On pushes to `main`, Native validation uploads temporary candidate artifacts (retained 14 days, GitHub sign-in required).

**[Draft release](../.github/workflows/release.yml)** is triggered manually from the Actions tab against a tag matching `VERSION`:

```text
verify tag -> validate -> package x4 -> smoke x4 fresh runners -> package agent plugins + SHA256SUMS + source -> draft
```

1. **Verify** runs `python3 scripts/release_contract.py verify`. It checks the tag grammar (see [Version policy](#version-policy)), that the tag equals `v<VERSION>`, and that the tag resolves to the exact commit being built. (If the tag already points at an earlier commit, reconcile the tag or bump `VERSION` before running the workflow.)
2. **Validate** runs Native validation in full.
3. **Package** builds each platform archive and standalone executable via `python scripts/package.py`.
4. **Smoke** downloads each platform artifact on a clean runner, verifies `.sha256` checksums, checks standalone binary parity, installs Linux runtime libs (`--runtime`) or ensures WebView2, and executes `--desktop-smoke` on Windows and macOS.
5. **Draft** runs `go run ./tools/agentdist package` to create the nine `testdesk_agent_*_v<VERSION>.zip` plugin packages and `AGENT_PLUGIN_SHA256SUMS`, exports `TestDesk-<tag>-source.tar.gz` via `git archive` for GPL-3.0 corresponding-source compliance, writes one combined `SHA256SUMS` covering all assets, and creates a GitHub **draft** release. `python3 scripts/release_contract.py draft-flags` supplies the flags. The draft is marked pre-release only for hyphenated tags such as `v1.0.0-rc.1`. Stable and single-letter tags such as `v1.21a` are not. The step refuses to run if `--draft` is missing, because `gh release create` publishes without it.

## Cut a release

1. Verify `VERSION`, `main.go`, `cmd/testdesk/main.go`, `internal/mcp/server.go`, `agent-distribution/testdesk/agentdist.json`, the in-repo plugin manifests, and `wails.json` `info.productVersion` agree (`python scripts/check_version.py`).
2. Confirm copyright/licence notices (including resolving `GPL-3.0-only` vs `GPL-3.0-or-later` and third-party notices via `python scripts/collect_licenses.py`).
3. Confirm Native validation is green on the exact release commit.
4. Create and push the matching `v<VERSION>` tag pointing to that commit.
5. Run **Draft release** in GitHub Actions for that tag.
6. Inspect the draft assets (platform ZIP/tarballs, standalone executables, nine agent plugin ZIPs, `AGENT_PLUGIN_SHA256SUMS`, `TestDesk-<tag>-source.tar.gz`, combined `SHA256SUMS`) and confirm release notes state that builds are unsigned/unnotarised.
7. Run hands-on checks on available platforms, then publish the draft manually.

## Version policy

<a id="version-policy"></a>

The external tag is `v<VERSION>`, and `VERSION` holds the exact tag text. The desktop app, the CLI and the MCP server print that text. Three forms are accepted:

| Form | Example tag | Manifest version | Bundle version (`Info.plist`, Windows) | Draft |
| --- | --- | --- | --- | --- |
| Stable | `v1.0.0` | `1.0.0` | `1.0.0` | Not a pre-release |
| Prerelease | `v1.0.0-rc.1` | `1.0.0-rc.1` | `1.0.0` | Pre-release |
| Single-letter revision | `v1.21a`, `v1.21b` | `1.21.1`, `1.21.2` | `1.21.1`, `1.21.2` | Not a pre-release |

Plugin manifests need semantic versions, and macOS bundle keys accept only numeric dotted versions. `scripts/release_contract.py` and `tools/agentdist/version.go` derive those forms, and their golden tables are tested on both sides. Plugin archives keep the exact tag text in their file names, such as `testdesk_agent_plugin_v1.21a.zip`.

**Single-letter revisions.** A letter after the numbered line is a patch revision: `a` is patch 1, `b` is patch 2, through `z` for patch 26. So `v1.21a` is version 1.21.1. Letter tags are not prereleases. Only a hyphenated tag such as `v1.0.0-rc.1` is marked as a pre-release on the draft.

## Release files

Each platform target `<version>-<system>-<chip>` (`.exe` on Windows) produces:

| File | Contents |
| --- | --- |
| `TestDesk-<version>-<system>-<chip>` | Standalone desktop app (Windows and Linux; on macOS, `.app` sits inside the tarball). Also runs `testdesk mcp`. |
| `testdesk-cli-<version>-<system>-<chip>` | Standalone CLI executable on all four targets. |
| `TestDesk-<version>-<system>-<chip>.zip` / `.tar.gz` | Full package with desktop app (or `.app`), CLI, `LICENSE`, `THIRD_PARTY_NOTICES.md`, `third_party/licenses`, and inner `SHA256SUMS`. |
| `testdesk_agent_<target>_v<version>.zip` + `AGENT_PLUGIN_SHA256SUMS` | Canonical Agent Plugin and native adapter ZIP packages (`plugin`, `kiro-power`, `claude-code`, `antigravity`, `cursor`, `openai-skills-only`, `opencode`, `kiro`, `vscode-copilot`). Binary installed separately. |
| `TestDesk-<tag>-source.tar.gz` + `SHA256SUMS` | Corresponding source tarball from `$GITHUB_SHA` plus combined release checksums. |

## Maintenance

Review Dependabot and `govulncheck` advisories, keep `THIRD_PARTY_NOTICES.md` synced with `python scripts/collect_licenses.py`, and keep `dist/`, caches, and personal paths out of Git. See [Repository automation](AUTOMATION.md).
