#!/usr/bin/env bash
# Validation gate for Linux and macOS. macOS ships bash 3.2, which treats an
# empty array as unset under `set -u`, so build tags use a plain string instead.
set -euo pipefail
cd "$(dirname "$0")/.."

build_tags=""
if [[ "$(uname -s)" == Linux ]]; then build_tags="webkit2_41"; fi
# Expands to nothing, or to the single word `-tags=webkit2_41`.
tags_flag=${build_tags:+-tags=$build_tags}

unformatted=$(gofmt -l cmd internal scripts/sync-agent-assets tools/agentdist main.go)
if [[ -n "$unformatted" ]]; then printf '%s\n' "$unformatted"; exit 1; fi
go mod verify
go test ${tags_flag} ./... -count=1 -timeout=10m
go vet ${tags_flag} ./...
node --check internal/desktop/assets/app.js
node --test internal/desktop/testdata/*.test.cjs
go run ./scripts/sync-agent-assets -check
go run ./tools/agentdist validate --release-version "$(tr -d '\r\n' < VERSION)"
python_bin=""
# Windows ships a python3 App Execution Alias that exits non-zero, so run each candidate before choosing it.
for candidate in python3 python; do
  if "$candidate" -c 'import sys; raise SystemExit(sys.version_info < (3, 9))' >/dev/null 2>&1; then
    python_bin=$candidate
    break
  fi
done
[[ -n "$python_bin" ]] || { echo 'Python 3.9 or later is required.' >&2; exit 1; }
"$python_bin" scripts/check_repository.py
"$python_bin" scripts/check_version.py
"$python_bin" -m unittest discover -s scripts -p 'test_*.py'
if [[ "${1:-}" == --build ]]; then bash scripts/build.sh; fi
