# Releases

Releases are built by GitHub Actions on real Windows, Linux and macOS runners and published by a maintainer. Nothing publishes automatically.

## What runs

**[Native validation](../.github/workflows/ci.yml)** runs on every pull request and push to `main`, and again at the start of every release.

| Job | What it checks |
| --- | --- |
| Validate (Windows, Linux, macOS Intel, macOS Apple silicon) | Formatting, `go vet`, Go and frontend tests, documentation links, agent-asset drift and version strings. Then it builds the native desktop package, extracts it and exercises it. Windows and macOS launch the packaged desktop and drive it over MCP: tray readiness, discovery, exact pass and fail selections, cancellation, project isolation and reconnect. macOS also runs the Go race detector and checks the app's native architecture. Linux verifies archive contents and versions; it has no display for the desktop. |
| Real providers (same four platforms) | Real Cargo, .NET 8/10, Jest and Vitest fixtures with version-pinned tooling. |
| Vulnerabilities (Linux, Windows, macOS) | `govulncheck` against production imports. |
| Workflow syntax | `actionlint` on the workflow files. |

On pushes to `main` it also uploads the four native packages as temporary downloads (14 days, GitHub sign-in required), so anyone can try the newest build.

**[Draft release](../.github/workflows/release.yml)** is started by hand from the Actions tab.

```text
verify tag ─▶ validate ─▶ package ×4 ─▶ smoke ×4 fresh runners ─▶ draft
```

1. **Verify** that the tag exists, points at the commit being built and matches `VERSION`.
2. **Validate** runs Native validation in full.
3. **Package** builds each native package with `python scripts/package.py`.
4. **Smoke** downloads each target's files on a clean runner and checks every checksum. It confirms the standalone files are byte-identical to the ones in the package, then runs them from a folder that holds nothing else. Where there is a display (Windows and macOS) it also starts the standalone CLI and the app from one folder and drives the app over MCP. Linux installs only the runtime libraries, so the packages named in [INSTALL.md](INSTALL.md) are tested too.
5. **Draft** adds the matching source archive and one `SHA256SUMS` covering every file, then creates a draft release. A version containing a hyphen (for example `v1.1.0-rc.1`) becomes a pre-release.

## Cut a release

1. Check that `VERSION`, `main.go`, `cmd/testdesk/main.go` and `internal/mcp/server.go` agree (`python scripts/check_version.py`).
2. If dependencies changed, run `python scripts/collect_licenses.py` and review the licence texts.
3. Make sure Native validation is green on the exact commit.
4. Create and push a tag: `v` plus the contents of `VERSION`.
5. In **Actions**, run **Draft release**, select that tag as the ref and enter the same tag as `version`. It fails if the tag, commit and `VERSION` differ.
6. Open the draft. Check the four packages, `SHA256SUMS`, the source archive and the generated notes. State in the notes that packages aren't code-signed.
7. Try a package by hand on any platform you can reach, then publish the draft.

Automated checks don't judge visual layout, hand-driven tray menus, signing or notarisation, so do that testing before publishing, or invite testers with a pre-release.

## Release files

Each of the four targets produces these, named `<version>-<system>-<chip>` with `.exe` on Windows:

| File | What it is |
| --- | --- |
| `TestDesk-<version>-<system>-<chip>` | The desktop app as one executable. Windows and Linux only; a macOS app is a bundle folder. It is also the MCP server (`testdesk mcp`). |
| `testdesk-cli-<version>-<system>-<chip>` | The command-line tool as one executable, on every target. |
| `TestDesk-<version>-<system>-<chip>.zip` or `.tar.gz` | The full package: the app (a `.app` bundle on macOS), `testdesk-cli`, `LICENSE`, `THIRD_PARTY_NOTICES.md` and `third_party/licenses`, plus an inner `SHA256SUMS`. `.zip` on Windows, `.tar.gz` elsewhere. |

The release also includes `TestDesk-<tag>-source.tar.gz`, which satisfies GPL-3.0's corresponding-source requirement, and `SHA256SUMS`. Never swap in a source archive from another commit. The single executables carry no licence texts of their own; the full package, the source archive and the repository hold them.

Build one target locally with `python scripts/package.py`. Output goes to `dist/`, with one `TestDesk-<version>-<system>-<chip>.sha256` file listing every download for the target. `python scripts/check_package.py` verifies them the way the release smoke job does.

## Maintenance

Review Go and dependency advisories, update only what's needed, run the checks and regenerate third-party notices. Never put local caches, preferences, test output or credentials in a release. See [repository automation](AUTOMATION.md) for required GitHub settings.
