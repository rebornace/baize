# Eval regression gate CLI

> Applies to `baize eval-gate`. It opens the configured store directly; no HTTP server is required.

## Usage

```bash
baize eval-gate -config configs/config.yaml
```

| Flag | Meaning |
|------|---------|
| `-config` | Config file used for the store driver/path (default `configs/config.yaml`) |
| `-min` | Minimum pass rate, 0–1; when omitted the UI setting is used, then 0.9 |
| `-limit` | Max confirmed cases to score (default 500, max 500) |
| `-json` | Print the result as JSON |
| `-report` | Also write a Markdown report |

Exit codes:

- `0`: the pass rate meets the threshold.
- `1`: below threshold, or no confirmed cases.
- Other: config/store/flag errors.

Scoring is deterministic: no model calls and no cost.

## Example

```bash
baize eval-gate -config configs/config.yaml -min 0.9 -report eval-gate.md
```

GitHub Actions step (self-hosted runner with access to the deployment's database):

```yaml
- name: Eval regression gate
  run: |
    go build -o baize ./cmd/baize
    ./baize eval-gate -config configs/config.yaml -min 0.9 -report eval-gate.md
- name: Upload gate report
  if: always()
  uses: actions/upload-artifact@v4
  with:
    name: eval-gate
    path: eval-gate.md
```

## Semantics

- Only `confirmed` cases are scored; drafts are ignored.
- The UI gate and the CLI share the same scoring code and threshold semantics.
- An empty suite does not pass.
- At most 500 cases per run.
