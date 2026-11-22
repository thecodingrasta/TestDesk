# Changelog

This file records user-visible changes and release-process changes. Release steps are in [docs/RELEASING.md](docs/RELEASING.md).

## 1.21a

Not yet tagged. The maintainer creates the `v1.21a` tag after review.

- The release version is `1.21a`, the exact tag text. The desktop app, CLI and MCP server report it.
- The tag `v1.21a` is version 1.21.1 in plugin manifests, the macOS bundle and the Windows version resources. A single letter is a patch number (`a` is 1). The mapping is documented in [docs/RELEASING.md](docs/RELEASING.md).
- The Go toolchain is 1.27.4, which fixes GO-2026-6859 (an HTTP/1 server crash in `net/http`) reported by `govulncheck`.
- The VS Code and GitHub Copilot package includes `.vscode/mcp.json`.
- The draft release workflow verifies the tag, creates only drafts, and marks hyphenated prerelease tags such as `v1.0.0-rc.1` as prereleases.
- `scripts/validate.sh` finds a working Python on Windows and runs every gate, including the documentation and version checks.

## Earlier commits

These commits have no tagged releases.

- `1.21`: hardened tray and MCP startup, polished the desktop interface, and prepared agent distribution and documentation.
- `1.02`: made the project release-ready.
- `1.01`: fixed continuous integration issues.
- `1`: first commit of TestDesk, a desktop test runner for AI agents.
