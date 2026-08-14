#!/bin/sh
# Baize local one-shot launcher (POSIX).
# Usage (from repo root): ./scripts/start.sh
set -e

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$ROOT"

if [ -z "${GOPROXY:-}" ]; then
	GOPROXY=https://goproxy.cn,direct
	export GOPROXY
fi
if [ -z "${GOSUMDB:-}" ]; then
	GOSUMDB=sum.golang.google.cn
	export GOSUMDB
fi

if ! command -v go >/dev/null 2>&1; then
	echo "go not found. Install Go 1.22+ or put it on PATH." >&2
	exit 1
fi

echo "baize start  (cwd=$ROOT  go=$(command -v go))"
exec go run ./cmd/baize start "$@"
