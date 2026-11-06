#!/usr/bin/env bash
# Install the GTK and WebKit packages that the Linux desktop build needs (Ubuntu 24.04 package names).
set -euo pipefail
sudo apt-get update
sudo apt-get install -y build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev libayatana-appindicator3-dev
