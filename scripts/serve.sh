#!/bin/sh
# Baize local launcher (POSIX). Runs `baize serve` with configs/config.yaml.
# Usage (from repo root): ./scripts/serve.sh [-config path/to.yaml]
set -e

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$ROOT"

# Load .env for BAIZE_API_KEY etc. when present.
if [ -f "$ROOT/.env" ]; then
    set -a
    # shellcheck disable=SC1091
    . "$ROOT/.env"
    set +a
fi

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

# Build then exec the binary directly (not `go run`): go run spawns a child
# process, which makes signal forwarding for graceful shutdown unreliable.
mkdir -p "$ROOT/bin"
echo "go build -o bin/baize ./cmd/baize  (cwd=$ROOT  go=$(command -v go))"
go build -o "$ROOT/bin/baize" ./cmd/baize

echo "baize serve"
exec "$ROOT/bin/baize" serve "$@"
