# CX2 fact-retention evaluation harness

Live-service A/B for **fact retention after compaction**: baseline
`context_offload_enabled=false` vs live `true`. Measures retention rate
(hits / total questions on valid cases only). Soft-close is a **human**
verdict in `CONCLUSION.md` — the scripts never auto-discard or change
product defaults.

Related: Spec
[`docs/superpowers/specs/2026-10-21-cx2-fact-retention-design.md`](../../docs/superpowers/specs/2026-10-21-cx2-fact-retention-design.md);
CX1 offload
[`docs/superpowers/specs/2026-10-20-context-offload-design.md`](../../docs/superpowers/specs/2026-10-20-context-offload-design.md).

## Requirements

- Windows PowerShell 5.1+ (scripts are `.ps1`).
- A running baize server with an LLM wired.
- An API key (`.env` `BAIZE_API_KEY`, env, or `-ApiKey`).
- **Restart baize** after pulling the CX2 GET-knob wire if the process
  predates `context_offload_enabled` on `GET /v0/settings/runtime` —
  otherwise `Assert-OffloadKnob` will fail.

## Configuration

Defaults: `BaseUrl=http://127.0.0.1:8080`, `AgentId=default-agent`.
Scripts accept `-BaseUrl` / `-ApiKey`, or `BAIZE_BASE_URL` /
`BAIZE_API_KEY`, or repository `.env`.

```powershell
$env:BAIZE_API_KEY = '<your key>'
```

## PowerShell 5 notes

- After `ConvertFrom-Json`, wrap with `@(...)` before using `.Count` —
  a one-element JSON array becomes a bare object and `.Count` is wrong.
  Prefer `@($rows).Count` (same pattern as tool-routing-eval).

## Usage

Single arm (does not toggle offload; uses current knobs + aggressive
compact threshold for the run):

```powershell
.\run_batch.ps1 -RequestsFile .\requests.json -OutDir .\out\manual
```

Full dual-arm compare (recommended):

```powershell
.\compare_fact_retention.ps1 -AgentId default-agent
```

| Arm | Knob |
|-----|------|
| baseline | `context_offload_enabled=false` (`CX2_EXPECT_OFFLOAD=0`) |
| live | `context_offload_enabled=true` (`CX2_EXPECT_OFFLOAD=1`; bulky cases without `context.offloaded` → `invalid`) |

Each arm: `Set-RuntimeKnobs` → `Assert-OffloadKnob` → `run_batch.ps1`.
When finished, compare **restores** `context_offload_enabled=true`
(CX1 default) and clears `CX2_EXPECT_OFFLOAD`.

### What each case does

1. Plant facts over several turns (same `conversation_id`).
2. Optional **bulky** turn: ≥64KiB paste with secret at the end (preview
   cannot see it; live arm should emit `context.offloaded`).
3. Filler + compact nudge until `context.compacted` (or mark `invalid`).
4. Ask questions; substring match on assistant text → hits.

Every user turn is prefixed with an eval steer line that **forbids tool
calls** (text-only short replies) to avoid connector HITL login gates on
`default-agent`.

### Outputs

- Worktree: `out/cx2-fact-retention-<ts>/{baseline,live}/` (+ `compare.json`)
- Committable archive:
  `docs/superpowers/notes/eval-artifacts/cx2-fact-retention/<ts>/`
  (`compare.json`, `baseline-summary.json`, `live-summary.json`,
  `CONCLUSION.md` with Verdict left for humans)

## Soft-close

Read rates / Δ / invalid counts in `CONCLUSION.md`, then fill Verdict.
Do **not** treat the script as a gate that changes defaults or discards
code.
