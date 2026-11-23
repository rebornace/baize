# memory-dedupe-eval

离线阈值评测：自动抽取的“近义复述”去重。

- `pairs.jsonl`：人工标注的三类对子（`near_dup` / `paraphrase` / `distinct`）。
- `eval.py`：字符 bigram Jaccard + 数字/长度守卫，扫 0.50–0.95 阈值，输出 TP/FP/TN/FN、P/R/F1 与误判明细。
- `scores.py`：逐对打印 unigram/bigram 分数，便于加语料时判断。

运行：

```bash
python scripts/memory-dedupe-eval/eval.py
```

当前结论（2026-11-19）：阈值 **0.72** 在 14 组不同事实上零误判，能挡住本次 dogfood 的近义对；语义改写 0/8，属非目标。详见 [`../../docs/superpowers/specs/2026-11-19-memory-neardup-design.md`](../../docs/superpowers/specs/2026-11-19-memory-neardup-design.md)。
