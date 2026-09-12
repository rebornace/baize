#!/bin/sh
# Trial stack launcher (mock LLM + demo HTTP).
set -e
ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$ROOT"
exec go run ./cmd/baize demo "$@"
