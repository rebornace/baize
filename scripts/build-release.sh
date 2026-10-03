#!/bin/sh
# Cross-compile baize + weixin-adapter for common OS/arch pairs.
# Usage (repo root): ./scripts/build-release.sh v0.1.0
set -e

VERSION="${1:?usage: $0 vX.Y.Z}"
ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
OUT="${2:-$ROOT/dist/release/$VERSION}"

TARGETS="
linux amd64 tar.gz
linux arm64 tar.gz
darwin amd64 tar.gz
darwin arm64 tar.gz
windows amd64 zip
windows arm64 zip
"

rm -rf "$OUT"
mkdir -p "$OUT"
export CGO_ENABLED=0
LDFLAGS="-s -w"

cd "$ROOT"
echo "$TARGETS" | while read -r GOOS GOARCH ARCHIVE; do
	[ -n "$GOOS" ] || continue
	name="baize_${VERSION}_${GOOS}_${GOARCH}"
	stage="$OUT/$name"
	mkdir -p "$stage/configs"
	ext=""
	[ "$GOOS" = windows ] && ext=".exe"
	echo "Building $name ..."
	GOOS=$GOOS GOARCH=$GOARCH go build -trimpath -ldflags "$LDFLAGS" -o "$stage/baize$ext" ./cmd/baize
	GOOS=$GOOS GOARCH=$GOARCH go build -trimpath -ldflags "$LDFLAGS" -o "$stage/weixin-adapter$ext" ./cmd/weixin-adapter
	cp configs/config.yaml "$stage/configs/config.yaml"
	cp configs/config.local.yaml.example "$stage/configs/config.local.yaml.example"
	cp .env.example "$stage/.env.example"
	cat >"$stage/README.txt" <<EOF
Baize $VERSION ($GOOS/$GOARCH)

1. Copy .env.example to .env and set BAIZE_API_KEY / BAIZE_SETTINGS_KEY.
2. Optionally edit configs/config.yaml (or create configs/config.local.yaml).
3. Run: ./baize serve   (Windows: .\\baize.exe serve)
4. Open http://127.0.0.1:8080/ui

weixin-adapter is only needed for the Weixin channel.
Docs: https://github.com/rebornace/baize
EOF
	if [ "$ARCHIVE" = zip ]; then
		( cd "$OUT" && zip -qr "${name}.zip" "$name" )
	else
		tar -czf "$OUT/${name}.tar.gz" -C "$OUT" "$name"
	fi
	rm -rf "$stage"
done

(
	cd "$OUT"
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum *.tar.gz *.zip 2>/dev/null >SHA256SUMS.txt || true
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 *.tar.gz *.zip 2>/dev/null >SHA256SUMS.txt || true
	fi
)

echo "Done: $OUT"
ls -la "$OUT"
