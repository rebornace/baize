#!/usr/bin/env python3
"""Tests for the EV-PRES pairwise summarizer (stdlib only)."""

from __future__ import annotations

import contextlib
import io
import json
import pathlib
import sys
import tempfile
import unittest

HERE = pathlib.Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

import summarize  # noqa: E402


def row(case_id: str, arm: str, passed: bool, **extra):
    data = {"case_id": case_id, "arm": arm, "pass": passed}
    data.update(extra)
    return data


def parse(rows):
    return summarize.parse_lines([json.dumps(r, ensure_ascii=False) for r in rows])


class ValidationTests(unittest.TestCase):
    def test_parses_valid_rows_and_preserves_optional_fields(self):
        results = parse([row("c1", "baseline", True, score=0.5, tokens=10, duration_ms=20, mechanism="tool", notes="ok")])
        self.assertEqual(len(results), 1)
        item = results[0]
        self.assertEqual(item.case_id, "c1")
        self.assertEqual(item.arm, "baseline")
        self.assertTrue(item.passed)
        self.assertEqual(item.score, 0.5)
        self.assertEqual(item.tokens, 10)
        self.assertEqual(item.duration_ms, 20)
        self.assertEqual(item.mechanism, "tool")
        self.assertEqual(item.notes, "ok")

    def test_missing_required_field_is_rejected(self):
        with self.assertRaisesRegex(ValueError, "missing required field: pass"):
            summarize.parse_lines(['{"case_id":"c1","arm":"baseline"}'])
        with self.assertRaisesRegex(ValueError, "missing required field: case_id"):
            summarize.parse_lines(['{"arm":"baseline","pass":true}'])
        with self.assertRaisesRegex(ValueError, "missing required field: arm"):
            summarize.parse_lines(['{"case_id":"c1","pass":true}'])

    def test_duplicate_case_and_arm_is_rejected(self):
        lines = [
            json.dumps(row("c1", "baseline", True)),
            json.dumps(row("c1", "baseline", False)),
        ]
        with self.assertRaisesRegex(ValueError, "duplicate case_id/arm"):
            summarize.parse_lines(lines)

    def test_bad_types_and_ranges_are_rejected(self):
        cases = [
            ({"case_id": "c1", "arm": "baseline", "pass": "yes"}, "pass must be a boolean"),
            (row("c1", "baseline", True, score=1.5), "score must be between 0 and 1"),
            (row("c1", "baseline", True, score="high"), "score must be a number"),
            (row("c1", "baseline", True, tokens=-1), "tokens must be >= 0"),
            (row("c1", "baseline", True, duration_ms=-5), "duration_ms must be >= 0"),
            (row("", "baseline", True), "case_id must be a non-empty string"),
            (row("c1", "", True), "arm must be a non-empty string"),
            (row("c1", "baseline", True, mechanism=3), "mechanism must be a string"),
            (row("c1", "baseline", True, notes=3), "notes must be a string"),
        ]
        for data, message in cases:
            with self.subTest(message=message):
                with self.assertRaisesRegex(ValueError, message):
                    summarize.parse_lines([json.dumps(data)])

    def test_non_object_line_is_rejected(self):
        with self.assertRaisesRegex(ValueError, "line 1: expected a JSON object"):
            summarize.parse_lines(['["not", "an", "object"]'])

    def test_invalid_json_reports_line_number(self):
        with self.assertRaisesRegex(ValueError, "line 2: invalid JSON"):
            summarize.parse_lines(['{"case_id":"c1","arm":"baseline","pass":true}', "{oops}"])


class StatisticsTests(unittest.TestCase):
    def test_wilson_interval_known_values(self):
        low, high = summarize.wilson_interval(10, 10)
        self.assertAlmostEqual(low, 0.7225, places=3)
        self.assertAlmostEqual(high, 1.0, places=6)

        low, high = summarize.wilson_interval(5, 10)
        self.assertAlmostEqual(low, 0.2366, places=3)
        self.assertAlmostEqual(high, 0.7634, places=3)

        self.assertEqual(summarize.wilson_interval(0, 0), (None, None))


class SummarizeTests(unittest.TestCase):
    def setUp(self):
        self.rows = [
            row("c1", "baseline", True, score=1.0, tokens=100, duration_ms=1000, mechanism="tool"),
            row("c1", "candidate", True, score=0.8, tokens=80, duration_ms=900, mechanism="tool"),
            row("c2", "baseline", True, score=1.0, tokens=120, duration_ms=1200, mechanism="tool"),
            row("c2", "candidate", False, score=0.2, tokens=110, duration_ms=1100, mechanism="tool"),
            row("c3", "baseline", False, score=0.3, tokens=90, duration_ms=900, mechanism="context"),
            row("c3", "candidate", True, score=0.9, tokens=130, duration_ms=1300, mechanism="context"),
            row("c4", "baseline", True, score=1.0, tokens=200, duration_ms=2000, mechanism="context"),
            row("c5", "candidate", True, score=1.0, tokens=150, duration_ms=1500, mechanism="completion"),
        ]
        self.results = parse(self.rows)
        self.summary = summarize.summarize(self.results)

    def test_arm_statistics(self):
        baseline = self.summary["arms"]["baseline"]
        candidate = self.summary["arms"]["candidate"]
        self.assertEqual(baseline["n"], 4)
        self.assertEqual(baseline["passed"], 3)
        self.assertAlmostEqual(baseline["pass_rate"], 0.75)
        self.assertAlmostEqual(baseline["avg_score"], 0.825)
        self.assertAlmostEqual(baseline["tokens_per_case"], 127.5)
        self.assertAlmostEqual(baseline["duration_ms_per_case"], 1275.0)
        self.assertAlmostEqual(baseline["wilson_low"], 0.3006, places=3)
        self.assertAlmostEqual(baseline["wilson_high"], 0.9544, places=3)

        self.assertEqual(candidate["n"], 4)
        self.assertEqual(candidate["passed"], 3)
        self.assertAlmostEqual(candidate["pass_rate"], 0.75)
        self.assertAlmostEqual(candidate["avg_score"], 0.725)
        self.assertAlmostEqual(candidate["tokens_per_case"], 117.5)
        self.assertAlmostEqual(candidate["duration_ms_per_case"], 1200.0)

    def test_paired_changes_and_unpaired_cases(self):
        paired = self.summary["paired"]
        self.assertEqual(paired["n"], 3)
        self.assertEqual(paired["win"], 1)
        self.assertEqual(paired["tie"], 1)
        self.assertEqual(paired["loss"], 1)
        self.assertEqual(paired["new_pass"], ["c3"])
        self.assertEqual(paired["new_fail"], ["c2"])
        self.assertEqual(paired["win_cases"], ["c3"])
        self.assertEqual(paired["loss_cases"], ["c2"])
        self.assertEqual(paired["tie_cases"], ["c1"])
        self.assertEqual(
            paired["unpaired"],
            [
                {"case_id": "c4", "missing": ["candidate"]},
                {"case_id": "c5", "missing": ["baseline"]},
            ],
        )

    def test_cost_delta_uses_paired_samples_only(self):
        cost = self.summary["cost"]
        self.assertEqual(cost["tokens"]["n"], 3)
        self.assertAlmostEqual(cost["tokens"]["baseline_mean"], 103.3333, places=3)
        self.assertAlmostEqual(cost["tokens"]["candidate_mean"], 106.6667, places=3)
        self.assertAlmostEqual(cost["tokens"]["delta"], 3.3333, places=3)
        self.assertEqual(cost["duration_ms"]["n"], 3)
        self.assertAlmostEqual(cost["duration_ms"]["delta"], 66.6667, places=3)

    def test_cost_without_paired_metric_samples(self):
        results = parse([row("c1", "baseline", True), row("c1", "candidate", True)])
        summary = summarize.summarize(results)
        self.assertEqual(summary["cost"]["tokens"]["n"], 0)
        self.assertIsNone(summary["cost"]["tokens"]["delta"])

    def test_mechanism_groups(self):
        mechanisms = self.summary["mechanisms"]
        self.assertEqual(mechanisms["tool"]["baseline"]["n"], 2)
        self.assertEqual(mechanisms["tool"]["baseline"]["passed"], 2)
        self.assertEqual(mechanisms["tool"]["candidate"]["passed"], 1)
        self.assertEqual(mechanisms["tool"]["new_fail"], ["c2"])
        self.assertEqual(mechanisms["context"]["new_pass"], ["c3"])
        self.assertEqual(mechanisms["completion"]["candidate"]["passed"], 1)

    def test_markdown_contains_required_sections(self):
        text = summarize.render_markdown(self.summary)
        for needle in [
            "# EV-PRES summary",
            "Wilson 95% CI",
            "New pass",
            "New fail",
            "Unpaired",
            "Cost (paired only)",
            "By mechanism",
            "c3",
            "c2",
        ]:
            with self.subTest(needle=needle):
                self.assertIn(needle, text)

    def test_unknown_arm_is_rejected(self):
        results = parse([row("c1", "baseline", True), row("c1", "other", True)])
        with self.assertRaisesRegex(ValueError, "unknown arm 'other'"):
            summarize.summarize(results)

    def test_custom_arm_names(self):
        results = parse([row("c1", "control", True), row("c1", "variant", False)])
        summary = summarize.summarize(results, baseline="control", candidate="variant")
        self.assertEqual(summary["paired"]["new_fail"], ["c1"])
        self.assertEqual(summary["paired"]["unpaired"], [])
        self.assertIn("control", summary["arms"])
        self.assertIn("variant", summary["arms"])

    def test_same_arm_names_are_rejected(self):
        with self.assertRaisesRegex(ValueError, "must differ"):
            summarize.summarize([], baseline="baseline", candidate="baseline")


class CommandLineTests(unittest.TestCase):
    def write_jsonl(self, directory: pathlib.Path, rows) -> pathlib.Path:
        path = directory / "results.jsonl"
        path.write_text("\n".join(json.dumps(r, ensure_ascii=False) for r in rows) + "\n", encoding="utf-8")
        return path

    def test_json_output_and_exit_zero(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = self.write_jsonl(pathlib.Path(tmp), [row("c1", "baseline", True), row("c1", "candidate", True)])
            out = io.StringIO()
            with contextlib.redirect_stdout(out):
                code = summarize.main([str(path), "--json"])
            self.assertEqual(code, 0)
            payload = json.loads(out.getvalue())
            self.assertEqual(payload["paired"]["n"], 1)
            self.assertEqual(payload["paired"]["tie"], 1)

    def test_fail_on_new_fail_exits_one(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = self.write_jsonl(
                pathlib.Path(tmp),
                [row("c1", "baseline", True), row("c1", "candidate", False)],
            )
            with contextlib.redirect_stdout(io.StringIO()):
                self.assertEqual(summarize.main([str(path), "--fail-on-new-fail"]), 1)
            with contextlib.redirect_stdout(io.StringIO()):
                self.assertEqual(summarize.main([str(path)]), 0)

    def test_fail_on_new_fail_passes_without_regression(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = self.write_jsonl(
                pathlib.Path(tmp),
                [row("c1", "baseline", False), row("c1", "candidate", True)],
            )
            with contextlib.redirect_stdout(io.StringIO()):
                self.assertEqual(summarize.main([str(path), "--fail-on-new-fail"]), 0)

    def test_validation_error_exits_two(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp) / "bad.jsonl"
            path.write_text('{"case_id":"c1","arm":"baseline"}\n', encoding="utf-8")
            err = io.StringIO()
            with contextlib.redirect_stderr(err):
                code = summarize.main([str(path)])
            self.assertEqual(code, 2)
            self.assertIn("missing required field", err.getvalue())


if __name__ == "__main__":
    unittest.main(verbosity=2)
