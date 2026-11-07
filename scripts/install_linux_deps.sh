#!/usr/bin/env bash
# Install the GTK and WebKit packages for the Linux desktop (Ubuntu 24.04 package names).
#   install_linux_deps.sh            build dependencies (headers and compilers)
#   install_linux_deps.sh --runtime  only the libraries a downloaded package needs to run
#
# A hosted-runner mirror stall once held a bare `apt-get` for 24 minutes and consumed the whole
# job budget. Each apt call is therefore bounded, and the pair is retried a few times.
set -euo pipefail

case "${1:-}" in
  '') packages=(build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev libayatana-appindicator3-dev) ;;
  --runtime) packages=(libgtk-3-0t64 libwebkit2gtk-4.1-0 libayatana-appindicator3-1) ;;
  *) echo "usage: $0 [--runtime]" >&2; exit 2 ;;
esac

export DEBIAN_FRONTEND=noninteractive
attempts=3
# A normal run of each command takes under a minute. The override exists for the stall test.
limit_seconds="${TESTDESK_APT_LIMIT_SECONDS:-120}"

# timeout runs under sudo so that it can signal apt-get, which runs as root.
apt() {
  sudo timeout "$limit_seconds" apt-get -o Acquire::Retries=3 -o Acquire::http::Timeout=30 \
    -o Acquire::https::Timeout=30 -o Dpkg::Lock::Timeout=60 "$@"
}

for ((attempt = 1; attempt <= attempts; attempt++)); do
  if apt update && apt install -y "${packages[@]}"; then
    exit 0
  fi
  echo "apt attempt $attempt of $attempts failed" >&2
  if ((attempt < attempts)); then
    # A timeout can interrupt dpkg part-way, which blocks the next install until it is repaired.
    sudo dpkg --configure -a || true
    sleep $((attempt * 10))
  fi
done
echo "Could not install: ${packages[*]}" >&2
exit 1
