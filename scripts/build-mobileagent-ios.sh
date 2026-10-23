#!/usr/bin/env bash
# Rebuild MOBILE-RT iOS XCFramework (requires macOS, Xcode, gomobile).
# Usage (from repo root):
#   ./scripts/build-mobileagent-ios.sh
#
# Env overrides:
#   GOPROXY, GOSUMDB, GOMODCACHE

set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export PATH="${HOME}/go/bin:${PATH}"
export GOPROXY="${GOPROXY:-https://goproxy.io,direct}"
export GOSUMDB="${GOSUMDB:-sum.golang.google.cn}"

if ! command -v gomobile >/dev/null 2>&1; then
  echo "gomobile not found; install with:" >&2
  echo "  go install golang.org/x/mobile/cmd/gomobile@latest" >&2
  echo "  go install golang.org/x/mobile/cmd/gobind@latest" >&2
  echo "  gomobile init" >&2
  exit 1
fi
if ! command -v xcodebuild >/dev/null 2>&1; then
  echo "xcodebuild not found (need Xcode on macOS)" >&2
  exit 1
fi

OUT_DIR="${ROOT}/mobile/rt-shell-ios/Frameworks"
OUT="${OUT_DIR}/Mobileagent.xcframework"
mkdir -p "${OUT_DIR}"
rm -rf "${OUT}"

echo "Binding ./mobileagent -> ${OUT}"
gomobile bind -target=ios,iossimulator -o "${OUT}" ./mobileagent
ls -la "${OUT}"
du -sh "${OUT}"
echo "OK"
