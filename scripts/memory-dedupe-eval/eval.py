"""Offline threshold sweep for near-duplicate memory text.

Target: near-verbatim restatements, not semantic paraphrases.
Metric: character-bigram Jaccard over NFKC-normalized text (punctuation and
whitespace removed).
Guards: differing numeric tokens or an extreme length ratio are never duplicates.
"""

import json
import os
import re
import unicodedata

HERE = os.path.dirname(os.path.abspath(__file__))


def normalize(s: str) -> str:
    s = unicodedata.normalize("NFKC", s).lower()
    return "".join(ch for ch in s if ch.isalnum() or "\u4e00" <= ch <= "\u9fff")


def bigrams(s: str) -> set:
    s = normalize(s)
    if len(s) < 2:
        return {s} if s else set()
    return {s[i : i + 2] for i in range(len(s) - 1)}


def numbers(s: str) -> set:
    return set(re.findall(r"\d+(?:\.\d+)?", unicodedata.normalize("NFKC", s)))


def similar(a: str, b: str, threshold: float) -> bool:
    na, nb = numbers(a), numbers(b)
    if na and nb and na != nb:
        return False
    la, lb = len(normalize(a)), len(normalize(b))
    if la and lb and max(la, lb) / min(la, lb) > 2.5:
        return False
    ba, bb = bigrams(a), bigrams(b)
    if not ba or not bb:
        return False
    return len(ba & bb) / len(ba | bb) >= threshold


def main():
    pairs = []
    with open(os.path.join(HERE, "pairs.jsonl"), encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if line:
                pairs.append(json.loads(line))

    rows = []
    for i in range(0, 46):
        t = round(0.50 + i * 0.01, 2)
        tp = fp = tn = fn = 0
        para_hit = para_total = 0
        misses = []
        for p in pairs:
            got = similar(p["a"], p["b"], t)
            if p["label"] == "paraphrase":
                para_total += 1
                if got:
                    para_hit += 1
                continue
            dup = p["label"] == "near_dup"
            if dup and got:
                tp += 1
            elif dup and not got:
                fn += 1
                misses.append(("FN", p))
            elif not dup and got:
                fp += 1
                misses.append(("FP", p))
            else:
                tn += 1
        precision = tp / (tp + fp) if tp + fp else 1.0
        recall = tp / (tp + fn) if tp + fn else 0.0
        f1 = 2 * precision * recall / (precision + recall) if precision + recall else 0.0
        rows.append((t, tp, fp, tn, fn, precision, recall, f1, para_hit, para_total, misses))

    print("threshold  TP FP TN FN   P      R      F1    para")
    for r in rows:
        if r[2] == 0 or r[0] in (0.60, 0.65, 0.70, 0.75, 0.80):
            print(f"{r[0]:9.2f}  {r[1]:2d} {r[2]:2d} {r[3]:2d} {r[4]:2d}  {r[5]:.3f}  {r[6]:.3f}  {r[7]:.3f}  {r[8]}/{r[9]}")

    zero_fp = [r for r in rows if r[2] == 0 and r[1] > 0]
    chosen = max(zero_fp, key=lambda r: (r[7], -r[0])) if zero_fp else None
    if chosen:
        print(f"\nchosen threshold={chosen[0]:.2f} TP={chosen[1]} FP={chosen[2]} FN={chosen[4]} "
              f"P={chosen[5]:.3f} R={chosen[6]:.3f} paraphrase={chosen[8]}/{chosen[9]}")
        for kind, p in chosen[10]:
            print(f"  {kind}: {p['a']} | {p['b']} | {p['note']}")
    else:
        print("no zero-FP threshold with TP>0")


if __name__ == "__main__":
    main()
