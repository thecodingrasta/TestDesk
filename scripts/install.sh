#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
case "$(uname -s)" in
  Linux)
    target="${XDG_BIN_HOME:-$HOME/.local/bin}"
    install -d "$target"
    install -m 755 dist/testdesk "$target/testdesk"
    install -m 755 dist/testdesk-cli "$target/testdesk-cli"
    notice_dir="${XDG_DATA_HOME:-$HOME/.local/share}/TestDesk"
    install -d "$notice_dir"
    cp LICENSE THIRD_PARTY_NOTICES.md "$notice_dir/"
    mkdir -p "$notice_dir/third_party"
    cp -R third_party/licenses "$notice_dir/third_party/"
    echo "Installed to $target. Ensure it is in PATH."
    ;;
  Darwin)
    app=$(find dist -maxdepth 1 -name '*.app' -type d -print -quit)
    test -n "$app" || { echo 'Build the macOS package first.' >&2; exit 1; }
    mkdir -p "$HOME/Applications"
    ditto "$app" "$HOME/Applications/$(basename "$app")"
    target="${XDG_BIN_HOME:-$HOME/.local/bin}"
    install -d "$target"
    install -m 755 dist/testdesk-cli "$target/testdesk-cli"
    notice_dir="$HOME/Library/Application Support/TestDesk"
    mkdir -p "$notice_dir/third_party"
    cp LICENSE THIRD_PARTY_NOTICES.md "$notice_dir/"
    cp -R third_party/licenses "$notice_dir/third_party/"
    echo "Installed $HOME/Applications/$(basename "$app")"
    echo "CLI installed to $target. Set TESTDESK_DESKTOP_EXECUTABLE to the app's Contents/MacOS/testdesk executable for MCP."
    ;;
  *) echo 'Use scripts/windows/Install-TestDesk.ps1 on Windows.' >&2; exit 1 ;;
esac
