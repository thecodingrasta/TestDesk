#!/usr/bin/env bash
# Validation gate for Linux and macOS. macOS ships bash 3.2, which treats an
# empty array as unset under `set -u`, so build tags use a plain string instead.
set -euo pipefail
cd "$(dirname "$0")/.."

build_tags=""
if [[ "$(uname -s)" == Linux ]]; then build_tags="webkit2_41"; fi
# Expands to nothing, or to the single word `-tags=webkit2_41`.
tags_flag=${build_tags:+-tags=$build_tags}

unformatted=$(gofmt -l cmd internal scripts/sync-agent-assets main.go)
if [[ -n "$unformatted" ]]; then printf '%s\n' "$unformatted"; exit 1; fi
go mod verify
go test ${tags_flag} ./... -count=1 -timeout=10m
go vet ${tags_flag} ./...
node --check internal/desktop/assets/app.js
node --test internal/desktop/testdata/*.test.cjs
go run ./scripts/sync-agent-assets -check
python3 scripts/check_repository.py
python3 scripts/check_version.py
python3 -m unittest discover -s scripts -p 'test_*.py'
if [[ "${1:-}" == --build ]]; then bash scripts/build.sh; fi
