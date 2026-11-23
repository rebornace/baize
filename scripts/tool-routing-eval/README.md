# Tool-routing evaluation harness

Regression benchmark for **tool-candidate narrowing** (decision layer +
prefilter). Candidates are ranked by tool retrieval: when an embedder is
wired (Settings → Matching & decisions / enhanced mode), dense embeddings +
BM25 (RRF); otherwise lexical BM25. Product overview: root README section
“decision layer and matching”. System routing is a soft boost, not a hard
connector gate. The harness drives a running baize server over a fixed set of
realistic requests and measures whether the correct tool is reachable and
selected, plus the token cost of doing so.

For “was required evidence established before a write?”, see the sibling
harness [`../evidence-eval`](../evidence-eval).

The harness talks to baize over its HTTP API; it does not import Go code.

## What the numbers mean

| Layer | What you prove | Typical command |
|---|---|---|
| **A. Main-model prompt** | Narrowing + prefilter cut turn-0 prompt tokens while runs still succeed | `sweep.ps1` / `stability.ps1` |
| **B. Judgment-service cost** | System One vs standby model for the same judgments (cheaper judgment, same success) | Manual A/B below |

Published README width tables are **layer A** (already measured; re-run only if
the corpus, connectors, or narrowing logic change). **Layer B** shows that
wiring a decision-model API (or local `tev1`) is a real latency/budget lever
versus using an LLM for the same judgments.

### Layer B result (2026-10-09, this machine)

Same 37 requests, `decide_tool_pre_topk=16`, agent `default-agent`, one pass each:

| Arm | `systems_source` | Success | Expected hit | End-to-end |
|---|---|---|---|---|
| System One (`tev1`) | `systemone` ×126 | 37/37 | 31/37 | ~376 s |
| LLM profile (`deepseek-flash`) | `remote` ×124 | 37/37 | 29/37 | ~556 s |

LLM path ≈ **1.48×** System One wall time; main-model prompt averages stayed comparable.
Artifacts: `out/layerB_systemone/`, `out/layerB_chat/`, `out/layerB_compare.json`.

## Requirements

- Windows PowerShell 5.1+ (scripts are `.ps1`).
- A running baize server with the target connectors enabled
  (`pets-admin` / `mall` / `notion`).
- An API key for that server.

## SSR1 tool-pick soft-gate (archived — feature vetoed 2026-10-21)

`KindToolPick` / `decide_tool_pick_mode` **product code was discarded** after
soft-gate FAIL. Keep `compare_tool_pick.ps1` +
`docs/superpowers/notes/eval-artifacts/ssr1-tool-pick/` as the veto record
(same pattern as TD discovery). Do not expect the runtime knob to exist.

Historical driver (for replaying old notes only):

```powershell
.\compare_tool_pick.ps1 -AgentId default-agent
```

- Arm A: `decide_tool_pick_mode=off` / Arm B: `live`
- Soft gate was: PASS Δ≥1; TIE Δ=0 → retest (MaxTies 5); FAIL → human discard

## Configuration

All scripts accept `-BaseUrl` and `-ApiKey`, or read them from the
environment (`BAIZE_BASE_URL`, `BAIZE_API_KEY`), or from the repository
`.env` file when present. Default base URL is `http://127.0.0.1:8080`.

```powershell
$env:BAIZE_API_KEY = '<your key>'
```

## Usage

Run the dataset once (does not change any runtime knobs):

```powershell
.\run_batch.ps1
```

Sweep `decide_tool_pre_topk` across several widths (enabling tool narrowing)
and aggregate success rate / average sent tools / average prompt tokens:

```powershell
.\sweep.ps1 -TopKs 32,24,16,12,8
```

Run the dataset repeatedly at one fixed width to measure stability:

```powershell
.\stability.ps1 -Rounds 5 -TopK 16
```

Analyse an existing sweep offline (no network calls):

```powershell
.\analyze.ps1
```

`sweep.ps1` and `stability.ps1` enable tool narrowing and restore the
previously effective `decide_enabled` / `decide_tool_routing_enabled` /
`decide_tool_pre_topk` values when they finish (including on a trapped
error). Generated artifacts go under `out/` and `artifacts/`, which are
git-ignored.

## Optional: System One vs standby model (layer B)

Same corpus and TopK; only the judgment service changes. Goal: show that
decision-model judgments keep run success / tool reachability while latency
(and billed judgment tokens, if any) drop vs an LLM.

1. Enable Smart speed-up master + tool narrowing (and any other sub-features
   you want in the “full decision layer” story: memory gate, prune, route).
2. **Run A — System One:** Matching & decisions → enable System One
   (`tev1` or API). Leave `decide_profile_id` **empty**. Run
   `.\stability.ps1 -Rounds 5 -TopK 16` (or `.\run_batch.ps1`).
3. **Run B — standby model:** Disable System One knobs; set
   `decide_profile_id` to a low-cost LLM profile. Repeat the same script.
4. Compare: run success, `expected_called` / `prefilter_recall`, turn-0
   `prompt_tokens` (should stay in the same band if narrowing width is
   fixed), plus wall time and—if your provider reports it—Ask-side usage
   from run events (`source=systemone` vs `source=remote`).

Memory gate / prune / route are not covered by this harness’s prompt table;
for those, sample runs with the corresponding toggles on and count skipped
generative calls (e.g. memory extract skipped) from events.

## Dataset

`requests.json` is a list of `{ id, input, expect_any }` entries.
`expect_any` lists the operation id(s) that would demonstrate correct
routing; a request passes if the run succeeds and at least one expected
tool is called. Add cases to widen coverage and keep `expect_any` focused
on a tool that proves the right system/action was reached.

## Failure → corpus → re-run

When a request **FAIL**s (run did not succeed) or routing regresses, treat it
like evidence-eval’s offline corpus loop:

1. **Capture** — add or extend an entry in `requests.json` (stable `id`,
   realistic `input`, `expect_any` that proves the right tool was reached).
2. **Re-run** — `.\run_batch.ps1` (or `sweep.ps1` / `stability.ps1` at your
   fixed TopK) until the case passes again or you accept a documented alt.
3. **Track** — keep harness artifacts under `out/` for before/after comparison.

**Write-before-evidence** failures are **not** stored in this harness. Use Admin
**Settings → Eval** (EvalCase in the baize store): filter failed/slow runs
(EV1a), ingest event snapshots (EV1), deterministic re-score without model
tokens, export JSON for `scripts/evidence-eval`. Spec:
[`docs/superpowers/specs/2026-10-17-eval-failure-loop-design.md`](../../docs/superpowers/specs/2026-10-17-eval-failure-loop-design.md).

Re-invoking the model after a fix still uses `run_batch.ps1` here or
`scripts/evidence-eval/run_batch.ps1` from exported cases — not the settings
page.

## Metrics

For each request the harness records:

- `status` — final run status (`succeeded` / `failed` / `cancelled`).
- `expected_called` — an `expect_any` tool was called.
- `prefilter_recall` — every called tool was inside that turn's prefilter.
- `sent_count` — tools actually sent on turn 0.
- `prompt_tokens` / `completion_tokens` — turn-0 token usage.

An `alt` row in `analyze.ps1` means the run succeeded via a
semantically-equivalent tool that was not listed in `expect_any` (e.g. a
`listAll` instead of `getList`); it is not a hard failure. A `FAIL` row is
a run that did not succeed.
