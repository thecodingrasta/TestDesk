#!/usr/bin/env bash
# Build the desktop app and CLI on Linux or macOS. Output goes to $TESTDESK_OUTPUT (default: dist).
set -euo pipefail
cd "$(dirname "$0")/.."
output="${TESTDESK_OUTPUT:-dist}"
command -v go >/dev/null
command -v wails >/dev/null
expected=$(go list -m -f '{{.Version}}' github.com/wailsapp/wails/v2)
wails version | grep -q "$expected" || { echo "Install Wails $expected: go install github.com/wailsapp/wails/v2/cmd/wails@$expected" >&2; exit 1; }
go mod download
case "$(uname -s)" in
  Linux) wails build -clean -trimpath -skipbindings -skipembedcreate -m -nosyncgomod -tags webkit2_41 -o testdesk ;;
  Darwin) wails build -clean -trimpath -skipbindings -skipembedcreate -m -nosyncgomod ;;
  *) echo 'Use scripts/windows/Build-TestDesk.ps1 on Windows.' >&2; exit 1 ;;
esac
mkdir -p "$output"
go build -trimpath -o "$output/testdesk-cli" ./cmd/testdesk
if [[ "$(uname -s)" == Linux ]]; then
  cp build/bin/testdesk "$output/testdesk"
else
  app=$(find build/bin -maxdepth 1 -name '*.app' -type d -print -quit)
  test -n "$app" || { echo 'Wails did not produce an application bundle.' >&2; exit 1; }
  ditto "$app" "$output/$(basename "$app")"
fi
cp LICENSE THIRD_PARTY_NOTICES.md "$output/"
mkdir -p "$output/third_party"
cp -R third_party/licenses "$output/third_party/"
echo "Native package and CLI are in $output/."
