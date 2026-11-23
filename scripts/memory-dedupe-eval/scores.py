import json
import os
import re
import unicodedata

HERE = os.path.dirname(os.path.abspath(__file__))


def normalize(s):
    s = unicodedata.normalize("NFKC", s).lower()
    return "".join(ch for ch in s if ch.isalnum() or "\u4e00" <= ch <= "\u9fff")


def grams(s, n):
    s = normalize(s)
    if n == 1:
        return set(s)
    if len(s) < n:
        return {s} if s else set()
    return {s[i : i + n] for i in range(len(s) - n + 1)}


def jac(a, b, n):
    A, B = grams(a, n), grams(b, n)
    return len(A & B) / len(A | B) if A and B else 0


pairs = []
with open(os.path.join(HERE, "pairs.jsonl"), encoding="utf-8") as f:
    for line in f:
        line = line.strip()
        if line:
            pairs.append(json.loads(line))

print("label      uni    bi     pair")
for p in pairs:
    print(f"{p['label']:9} {jac(p['a'],p['b'],1):.2f}  {jac(p['a'],p['b'],2):.2f}  {p['a']} | {p['b']}")
