#!/bin/sh
# Baize local launcher (POSIX). Rebuilds embedded Chat UI when web/chat is
# newer than internal/ui/dist, then builds and runs baize.
# Usage (from repo root): ./scripts/serve.sh [-config path/to.yaml]
#   --skip-ui / BAIZE_SKIP_UI=1   skip npm build
#   --force-ui / BAIZE_FORCE_UI=1 always npm run build
set -e

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$ROOT"

SKIP_UI=0
FORCE_UI=0
# Drop launcher flags while preserving remaining args (incl. paths with spaces).
n=$#
while [ "$n" -gt 0 ]; do
	arg=$1
	shift
	n=$((n - 1))
	case "$arg" in
		-SkipUI|--skip-ui) SKIP_UI=1 ;;
		-ForceUI|--force-ui) FORCE_UI=1 ;;
		*) set -- "$@" "$arg" ;;
	esac
done

if [ "${BAIZE_SKIP_UI:-}" = "1" ]; then
	SKIP_UI=1
fi
if [ "${BAIZE_FORCE_UI:-}" = "1" ]; then
	FORCE_UI=1
fi

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
	echo "go not found. Install Go 1.25+ or put it on PATH." >&2
	exit 1
fi

newest_mtime() {
	# Print newest mtime (epoch seconds) among files under listed paths; 0 if none.
	find "$@" -type f 2>/dev/null | while IFS= read -r f; do
		# BusyBox/GNU/BSD: prefer stat -c, then stat -f
		if t=$(stat -c %Y "$f" 2>/dev/null); then
			echo "$t"
		elif t=$(stat -f %m "$f" 2>/dev/null); then
			echo "$t"
		fi
	done | sort -n | tail -1
}

chat_ui_needs_rebuild() {
	dist_index="$ROOT/internal/ui/dist/index.html"
	if [ ! -f "$dist_index" ]; then
		return 0
	fi
	src_mtime=$(newest_mtime \
		"$ROOT/web/chat/src" \
		"$ROOT/web/chat/index.html" \
		"$ROOT/web/chat/package.json" \
		"$ROOT/web/chat/package-lock.json" \
		"$ROOT/web/chat/vite.config.ts" \
		"$ROOT/web/chat/tsconfig.json" \
		"$ROOT/web/chat/tsconfig.node.json")
	dist_mtime=$(newest_mtime "$ROOT/internal/ui/dist")
	src_mtime=${src_mtime:-0}
	dist_mtime=${dist_mtime:-0}
	[ "$src_mtime" -gt "$dist_mtime" ]
}

ensure_chat_ui() {
	chat_dir="$ROOT/web/chat"
	if [ ! -f "$chat_dir/package.json" ]; then
		echo "web/chat missing; skipping UI build"
		return 0
	fi
	if [ "$FORCE_UI" != "1" ] && ! chat_ui_needs_rebuild; then
		echo "chat UI up to date (internal/ui/dist newer than web/chat sources)"
		return 0
	fi
	if ! command -v npm >/dev/null 2>&1; then
		echo "npm not found, but the Chat UI needs a rebuild (web/chat is newer than internal/ui/dist)." >&2
		echo "Install Node.js 20+ and put npm on PATH, or use --skip-ui / BAIZE_SKIP_UI=1." >&2
		exit 1
	fi
	echo "npm run build  (cwd=$chat_dir) -> internal/ui/dist"
	(cd "$chat_dir" && npm run build)
}

if [ "$SKIP_UI" = "1" ]; then
	echo "skipping chat UI build (--skip-ui / BAIZE_SKIP_UI=1)"
else
	ensure_chat_ui
fi

# Build then exec the binary directly (not `go run`): go run spawns a child
# process, which makes signal forwarding for graceful shutdown unreliable.
mkdir -p "$ROOT/bin"
echo "go build -o bin/baize ./cmd/baize  (cwd=$ROOT  go=$(command -v go))"
go build -o "$ROOT/bin/baize" ./cmd/baize

echo "baize serve"
exec "$ROOT/bin/baize" serve "$@"
