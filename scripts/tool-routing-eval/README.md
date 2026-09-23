# Tool-routing evaluation harness

Regression benchmark for the DP-2a tool-candidate narrowing (the
`decide` layer). It drives a running baize server over a fixed set of
realistic requests and measures whether the correct tool is reachable
and selected, plus the token cost of doing so.

The harness talks to baize over its HTTP API; it does not import Go code.

## Requirements

- Windows PowerShell 5.1+ (scripts are `.ps1`).
- A running baize server with the target connectors enabled
  (`doctor-miao` / `mall` / `notion`).
- An API key for that server.

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

## Dataset

`requests.json` is a list of `{ id, input, expect_any }` entries.
`expect_any` lists the operation id(s) that would demonstrate correct
routing; a request passes if the run succeeds and at least one expected
tool is called. Add cases to widen coverage and keep `expect_any` focused
on a tool that proves the right system/action was reached.

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
