#!/usr/bin/env python3
"""EV-PRES: pairwise preservation-aware evaluation summary.

Input is JSONL, one row per arm per case. The first arm is the baseline,
the second is the candidate. The report keeps the pass/fail outcome primary
and reports score, cost and mechanism attribution alongside it.

No third-party dependencies.
"""

from __future__ import annotations

import argparse
import json
import math
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable


@dataclass(frozen=True)
class Result:
    case_id: str
    arm: str
    passed: bool
    score: float | None = None
    tokens: float | None = None
    duration_ms: float | None = None
    mechanism: str | None = None
    notes: str = ""
    line_no: int = 0


def _number(value: object, field: str) -> float | None:
    if value is None:
        return None
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ValueError(f"{field} must be a number")
    value = float(value)
    if not math.isfinite(value):
        raise ValueError(f"{field} must be finite")
    if value < 0:
        raise ValueError(f"{field} must be >= 0")
    return value


def _optional_text(value: object, field: str) -> str | None:
    if value is None:
        return None
    if not isinstance(value, str):
        raise ValueError(f"{field} must be a string")
    value = value.strip()
    return value or None


def _parse_line(raw: str, line_no: int) -> Result:
    try:
        data = json.loads(raw)
    except json.JSONDecodeError as exc:
        raise ValueError(f"line {line_no}: invalid JSON: {exc.msg}") from exc
    if not isinstance(data, dict):
        raise ValueError(f"line {line_no}: expected a JSON object")

    for field in ("case_id", "arm", "pass"):
        if field not in data:
            raise ValueError(f"line {line_no}: missing required field: {field}")

    case_id = data["case_id"]
    if not isinstance(case_id, str) or not case_id.strip():
        raise ValueError(f"line {line_no}: case_id must be a non-empty string")
    case_id = case_id.strip()

    arm = data["arm"]
    if not isinstance(arm, str) or not arm.strip():
        raise ValueError(f"line {line_no}: arm must be a non-empty string")
    arm = arm.strip()

    passed = data["pass"]
    if not isinstance(passed, bool):
        raise ValueError(f"line {line_no}: pass must be a boolean")

    score = _number(data.get("score"), "score")
    if score is not None and score > 1:
        raise ValueError(f"line {line_no}: score must be between 0 and 1")

    tokens = _number(data.get("tokens"), "tokens")
    duration_ms = _number(data.get("duration_ms"), "duration_ms")
    mechanism = _optional_text(data.get("mechanism"), "mechanism")
    notes = data.get("notes", "")
    if not isinstance(notes, str):
        raise ValueError(f"line {line_no}: notes must be a string")

    return Result(
        case_id=case_id,
        arm=arm,
        passed=passed,
        score=score,
        tokens=tokens,
        duration_ms=duration_ms,
        mechanism=mechanism,
        notes=notes,
        line_no=line_no,
    )


def parse_lines(lines: Iterable[str]) -> list[Result]:
    """Parse and validate JSONL rows, keeping duplicate detection strict."""
    results: list[Result] = []
    seen: set[tuple[str, str]] = set()
    for line_no, raw in enumerate(lines, start=1):
        raw = raw.strip()
        if not raw:
            continue
        item = _parse_line(raw, line_no)
        key = (item.case_id, item.arm)
        if key in seen:
            raise ValueError(f"line {line_no}: duplicate case_id/arm: {item.case_id} / {item.arm}")
        seen.add(key)
        results.append(item)
    return results


def load_jsonl(path: str | Path) -> list[Result]:
    with open(path, encoding="utf-8") as fh:
        return parse_lines(fh)


def wilson_interval(passed: int, n: int, z: float = 1.96) -> tuple[float | None, float | None]:
    """Wilson score interval for a binomial proportion."""
    if n <= 0:
        return None, None
    p = passed / n
    denom = 1.0 + (z * z) / n
    centre = (p + (z * z) / (2.0 * n)) / denom
    half = z * math.sqrt((p * (1.0 - p) / n) + (z * z) / (4.0 * n * n)) / denom
    return max(0.0, centre - half), min(1.0, centre + half)


def _mean(values: list[float]) -> float | None:
    return sum(values) / len(values) if values else None


def _stats(items: list[Result]) -> dict:
    n = len(items)
    passed = sum(1 for item in items if item.passed)
    scores = [item.score for item in items if item.score is not None]
    tokens = [item.tokens for item in items if item.tokens is not None]
    durations = [item.duration_ms for item in items if item.duration_ms is not None]
    low, high = wilson_interval(passed, n)
    return {
        "n": n,
        "passed": passed,
        "failed": n - passed,
        "pass_rate": (passed / n) if n else None,
        "wilson_low": low,
        "wilson_high": high,
        "avg_score": _mean(scores),
        "score_n": len(scores),
        "tokens_per_case": _mean(tokens),
        "tokens_n": len(tokens),
        "duration_ms_per_case": _mean(durations),
        "duration_ms_n": len(durations),
    }


def _cost_metric(pairs: list[tuple[Result, Result]], field: str) -> dict:
    values = []
    for baseline, candidate in pairs:
        base_value = getattr(baseline, field)
        cand_value = getattr(candidate, field)
        if base_value is not None and cand_value is not None:
            values.append((base_value, cand_value))
    if not values:
        return {
            "n": 0,
            "baseline_mean": None,
            "candidate_mean": None,
            "delta": None,
            "delta_pct": None,
        }
    base_mean = sum(v[0] for v in values) / len(values)
    cand_mean = sum(v[1] for v in values) / len(values)
    delta = cand_mean - base_mean
    delta_pct = (delta / base_mean) if base_mean else None
    return {
        "n": len(values),
        "baseline_mean": base_mean,
        "candidate_mean": cand_mean,
        "delta": delta,
        "delta_pct": delta_pct,
    }


def _mechanism_key(item: Result) -> str:
    return item.mechanism or "unattributed"


def _pair_mechanism(baseline: Result, candidate: Result) -> str:
    base_key = _mechanism_key(baseline)
    cand_key = _mechanism_key(candidate)
    return base_key if base_key == cand_key else f"{base_key} -> {cand_key}"


def summarize(results: Iterable[Result], baseline: str = "baseline", candidate: str = "candidate") -> dict:
    """Build a report dictionary for two named arms."""
    if base := baseline.strip():
        baseline = base
    else:
        raise ValueError("baseline arm must be non-empty")
    if cand := candidate.strip():
        candidate = cand
    else:
        raise ValueError("candidate arm must be non-empty")
    if baseline == candidate:
        raise ValueError("baseline and candidate arms must differ")

    items = list(results)
    allowed = {baseline, candidate}
    for item in items:
        if item.arm not in allowed:
            raise ValueError(
                f"line {item.line_no}: unknown arm {item.arm!r}; expected {baseline!r} or {candidate!r}"
            )

    base_items = [item for item in items if item.arm == baseline]
    cand_items = [item for item in items if item.arm == candidate]
    base_by_id = {item.case_id: item for item in base_items}
    cand_by_id = {item.case_id: item for item in cand_items}

    common = sorted(set(base_by_id) & set(cand_by_id))
    base_only = sorted(set(base_by_id) - set(cand_by_id))
    cand_only = sorted(set(cand_by_id) - set(base_by_id))
    pairs = [(base_by_id[case_id], cand_by_id[case_id]) for case_id in common]

    wins: list[str] = []
    losses: list[str] = []
    ties: list[str] = []
    for base_item, cand_item in pairs:
        if cand_item.passed and not base_item.passed:
            wins.append(cand_item.case_id)
        elif base_item.passed and not cand_item.passed:
            losses.append(cand_item.case_id)
        else:
            ties.append(cand_item.case_id)

    unpaired = [{"case_id": case_id, "missing": [candidate]} for case_id in base_only]
    unpaired += [{"case_id": case_id, "missing": [baseline]} for case_id in cand_only]

    mechanism_groups: dict[str, dict] = {}
    for item in base_items:
        group = mechanism_groups.setdefault(_mechanism_key(item), {"baseline": [], "candidate": [], "new_pass": [], "new_fail": []})
        group["baseline"].append(item)
    for item in cand_items:
        group = mechanism_groups.setdefault(_mechanism_key(item), {"baseline": [], "candidate": [], "new_pass": [], "new_fail": []})
        group["candidate"].append(item)
    for base_item, cand_item in pairs:
        key = _pair_mechanism(base_item, cand_item)
        group = mechanism_groups.setdefault(key, {"baseline": [], "candidate": [], "new_pass": [], "new_fail": []})
        if cand_item.passed and not base_item.passed:
            group["new_pass"].append(cand_item.case_id)
        elif base_item.passed and not cand_item.passed:
            group["new_fail"].append(cand_item.case_id)

    mechanisms = {}
    for key in sorted(mechanism_groups):
        group = mechanism_groups[key]
        mechanisms[key] = {
            "baseline": _stats(group["baseline"]),
            "candidate": _stats(group["candidate"]),
            "new_pass": sorted(group["new_pass"]),
            "new_fail": sorted(group["new_fail"]),
        }

    return {
        "arms": {
            baseline: _stats(base_items),
            candidate: _stats(cand_items),
        },
        "paired": {
            "n": len(pairs),
            "win": len(wins),
            "tie": len(ties),
            "loss": len(losses),
            "win_cases": wins,
            "tie_cases": ties,
            "loss_cases": losses,
            "new_pass": wins,
            "new_fail": losses,
            "unpaired": unpaired,
        },
        "cost": {
            "tokens": _cost_metric(pairs, "tokens"),
            "duration_ms": _cost_metric(pairs, "duration_ms"),
        },
        "mechanisms": mechanisms,
    }


def _fmt_number(value: float | None, digits: int = 3) -> str:
    if value is None:
        return "n/a"
    if abs(value - round(value)) < 1e-9:
        return f"{value:.0f}"
    return f"{value:.{digits}f}"


def _fmt_pct(value: float | None) -> str:
    return "n/a" if value is None else f"{value * 100:.1f}%"


def _fmt_ci(low: float | None, high: float | None) -> str:
    if low is None or high is None:
        return "n/a"
    return f"{low * 100:.1f}% - {high * 100:.1f}%"


def _bullet_list(values: list[str]) -> str:
    return "\n".join(f"- `{value}`" for value in values) if values else "_none_"


def render_markdown(summary: dict) -> str:
    arm_names = list(summary["arms"])
    baseline, candidate = arm_names[0], arm_names[1]
    paired = summary["paired"]
    cost = summary["cost"]

    lines = [
        "# EV-PRES summary",
        "",
        "Win/loss use the binary `pass` outcome only; score is reported separately.",
        "",
        "## Arms",
        "",
        "| Arm | Cases | Pass | Pass rate | Wilson 95% CI | Avg score (n) | Tokens/case (n) | ms/case (n) |",
        "|---|---:|---:|---:|---|---:|---:|---:|",
    ]
    for name, stats in summary["arms"].items():
        lines.append(
            "| {name} | {n} | {passed} | {rate} | {ci} | {score} ({score_n}) | {tokens} ({tokens_n}) | {dur} ({dur_n}) |".format(
                name=name,
                n=stats["n"],
                passed=stats["passed"],
                rate=_fmt_pct(stats["pass_rate"]),
                ci=_fmt_ci(stats["wilson_low"], stats["wilson_high"]),
                score=_fmt_number(stats["avg_score"]),
                score_n=stats["score_n"],
                tokens=_fmt_number(stats["tokens_per_case"], 1),
                tokens_n=stats["tokens_n"],
                dur=_fmt_number(stats["duration_ms_per_case"], 1),
                dur_n=stats["duration_ms_n"],
            )
        )

    lines += [
        "",
        "## Paired",
        "",
        f"- Paired cases: {paired['n']}",
        f"- Win / Tie / Loss: {paired['win']} / {paired['tie']} / {paired['loss']}",
        "",
        "### New pass",
        "",
        _bullet_list(paired["new_pass"]),
        "",
        "### New fail",
        "",
        _bullet_list(paired["new_fail"]),
        "",
        "### Unpaired",
        "",
    ]
    if paired["unpaired"]:
        lines += ["| Case | Missing arms |", "|---|---|"]
        for item in paired["unpaired"]:
            lines.append(f"| `{item['case_id']}` | {', '.join(item['missing'])} |")
    else:
        lines.append("_none_")

    lines += [
        "",
        "## Cost (paired only)",
        "",
        "| Metric | Pairs | Baseline | Candidate | Delta | Delta % |",
        "|---|---:|---:|---:|---:|---:|",
    ]
    for label, key in (("tokens", "tokens"), ("duration_ms", "duration_ms")):
        metric = cost[key]
        lines.append(
            "| {label} | {n} | {base} | {cand} | {delta} | {pct} |".format(
                label=label,
                n=metric["n"],
                base=_fmt_number(metric["baseline_mean"], 1),
                cand=_fmt_number(metric["candidate_mean"], 1),
                delta=_fmt_number(metric["delta"], 1),
                pct=_fmt_pct(metric["delta_pct"]),
            )
        )

    lines += [
        "",
        "## By mechanism",
        "",
        "| Mechanism | Baseline | Candidate | New pass | New fail |",
        "|---|---:|---:|---|---|",
    ]
    for key, group in summary["mechanisms"].items():
        lines.append(
            f"| {key} | {group['baseline']['passed']}/{group['baseline']['n']} | "
            f"{group['candidate']['passed']}/{group['candidate']['n']} | "
            f"{', '.join(f'`{c}`' for c in group['new_pass']) or '-'} | "
            f"{', '.join(f'`{c}`' for c in group['new_fail']) or '-'} |"
        )
    return "\n".join(lines) + "\n"


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="ev-pres",
        description="Pairwise preservation-aware evaluation summary.",
    )
    parser.add_argument("input", help="JSONL file with one row per arm per case")
    parser.add_argument("--json", action="store_true", help="emit JSON instead of Markdown")
    parser.add_argument("--baseline", default="baseline", help="baseline arm name (default: baseline)")
    parser.add_argument("--candidate", default="candidate", help="candidate arm name (default: candidate)")
    parser.add_argument(
        "--fail-on-new-fail",
        action="store_true",
        help="exit 1 when a paired case passed in baseline and fails in candidate",
    )
    return parser


def main(argv: list[str] | None = None) -> int:
    args = _parser().parse_args(argv)
    try:
        results = load_jsonl(args.input)
        summary = summarize(results, baseline=args.baseline, candidate=args.candidate)
    except (OSError, ValueError) as exc:
        print(f"ev-pres: {exc}", file=sys.stderr)
        return 2

    if args.json:
        print(json.dumps(summary, ensure_ascii=False, indent=2))
    else:
        print(render_markdown(summary), end="")

    if args.fail_on_new_fail and summary["paired"]["new_fail"]:
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
