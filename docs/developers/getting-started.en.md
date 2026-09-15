[中文](./getting-started.md) | **English**

# Getting started: build, test, contribute

## Environment

| Component | Version | Notes |
|-----------|---------|--------|
| Go | **1.25.0** | Matches [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) |
| Node.js | 22 | Only for `web/chat` CI / local checks |

## Go

From the repo root:

```bash
go vet ./...
test -z "$(gofmt -l .)"   # On Windows PowerShell, run gofmt -l . and check for output
go build ./...
go test -count=1 ./...
```

### Lint (Go)

Local runs must match CI (full suite):

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.4.0
golangci-lint run ./... --timeout=5m
```

Integration tests that need Postgres may use `BAIZE_TEST_PG_DSN` (injected in CI).

## Frontend (`web/chat`)

```bash
cd web/chat
npm ci
npm run lint
npm test
npx tsc --noEmit
```

## Local trial

From the repo root:

- Windows: `.\demo.cmd`
- Unix: `./scripts/demo.sh`

Production-oriented start: `start` / `baize start` in the root README (needs `BAIZE_API_KEY`). More deploy detail: [Deployment](./deployment.en.md) ([中文](./deployment.md)).

## Directory map (contributors)

| Path | Role |
|------|------|
| `internal/api/` | HTTP control plane; `server.go` registers routes; handlers split by domain (incl. `server_runs.go` / `server_runs_exec.go`) |
| `web/chat/src/api/` | Browser client; `api.ts` is a barrel; settings under `api/settings/*` |
| `web/chat/src/pages/chat/` | Chat page hooks/components; entry `pages/ChatPage.tsx` |
| `web/chat/src/pages/` | Settings entry pages; Tools/Models/MCP Export split into subdirs (e.g. `pages/tools/`) |
| `internal/run/` | Run engine; `engine.go` entry/loop; step/stream helpers in `engine_*.go` |
| `internal/store/` | Persistence; `sqlite.go` open/migrate; entity SQL in `sqlite_*.go` |

## Contributing

1. Branch from `main`, open a PR into `main`.
2. Keep `gofmt` / `go vet` / `go test` and frontend lint, test, `tsc` aligned with CI.
3. Design discussion via Issue / PR; do not park undelivered roadmaps in public docs.
