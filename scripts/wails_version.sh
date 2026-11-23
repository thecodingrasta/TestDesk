# Shared check that the Wails CLI matches go.mod. Sourced by build.sh and exercised by test_wails_version.py.

require_wails_version() {
  local expected=$1 output
  # Capture the output before matching. Piping into `grep -q` lets grep exit at the first match, so a CLI
  # that keeps printing then gets a broken pipe, and with pipefail the check fails even when the version matches.
  output=$(wails version 2>&1) || true
  case "$output" in
    *"$expected"*) return 0 ;;
  esac
  echo "Install Wails $expected: go install github.com/wailsapp/wails/v2/cmd/wails@$expected" >&2
  return 1
}
