# EV-PRES：保全感知的成对评测汇总

给现有评测补一层“基线 vs 候选”的报告形状。它不重新跑模型，也不做 LLM
裁判；只消费已有的逐 case 结果，回答四个问题：

1. 两个 arm 各自 Pass 多少，区间有多宽？
2. 候选有没有**新过**，有没有**新挂**？
3. 成本变了多少（只看成对样本）？
4. 变化落在哪个机制层（Tool / Context / Completion…）？

一句话：**总分涨不是结论；旧任务有没有丢，才是结论的一部分。**

---

## 用法

```bash
python scripts/ev-pres/summarize.py results.jsonl
python scripts/ev-pres/summarize.py results.jsonl --json
python scripts/ev-pres/summarize.py results.jsonl --fail-on-new-fail
```

默认 arm 名是 `baseline` / `candidate`；如果评测里叫 `control` / `variant`：

```bash
python scripts/ev-pres/summarize.py results.jsonl --baseline control --candidate variant
```

- Markdown 输出到 stdout；`--json` 输出同结构报告。
- `--fail-on-new-fail`：存在“基线过、候选挂”的成对 case 时退出 1。
- 校验失败退出 2；正常退出 0。

先用模板写预注册，再跑候选：

- [`prereg.md`](prereg.md)：问题、construct、干预/机制、arms 与 binding、
  语料与 hold-out、指标与阈值、停手规则、决策。
- [`report.md`](report.md)：填汇总输出、解释、局限、决策。

最小示例：

```bash
python scripts/ev-pres/summarize.py scripts/ev-pres/sample.jsonl
# 示例里故意有一个 new fail，所以下面这条会退出 1：
python scripts/ev-pres/summarize.py scripts/ev-pres/sample.jsonl --fail-on-new-fail
```

---

## 输入 schema

JSONL，每行一个 arm 的一次 case 结果：

```json
{"case_id":"ticket-01","arm":"baseline","pass":true,"score":1.0,"tokens":1200,"duration_ms":9000,"mechanism":"Tool","notes":""}
{"case_id":"ticket-01","arm":"candidate","pass":true,"score":1.0,"tokens":1100,"duration_ms":8500,"mechanism":"Tool","notes":""}
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `case_id` | 是 | 同一对照内唯一；`case_id + arm` 不得重复 |
| `arm` | 是 | 默认 `baseline` / `candidate`，可由 CLI 改名 |
| `pass` | 是 | 二元准则结果；必须是 JSON boolean |
| `score` | 否 | 0..1 连续分；不替代 `pass` |
| `tokens` | 否 | 本轮成本代理，非负 |
| `duration_ms` | 否 | 端到端时长，非负 |
| `mechanism` | 否 | 机制标签；空值归为 `unattributed` |
| `notes` | 否 | 失败原因、证据链接 |

未知字段被忽略；坏行、缺必填、重复 `case_id + arm`、非法数值都会报错。

---

## 指标口径

- **每 arm**：`n`、`passed`、Pass rate、Wilson 95% CI、Avg score（只对有
  `score` 的样本）、tokens/case、ms/case（各自只对有的样本求均值）。
- **成对**：以 `pass` 为主。
  - Win = 基线挂、候选过（即 **new pass**）
  - Loss = 基线过、候选挂（即 **new fail**）
  - Tie = 两边同过或同挂；分数差异另看 Avg score。
- **未配对**：只出现在一个 arm 的 case 会逐条列出，不悄悄丢掉。
- **成本差**：`candidate − baseline`，只用两侧都有该指标的成对样本；`n`
  是该指标的成对样本数。
- **机制**：按每行 `mechanism` 分组。成对两侧标签一致时按该标签；不一致
  时显示 `baseline -> candidate`，避免把归因混在一起。

> 样本小的时候 Wilson CI 会很宽；报告不得只拿点估计宣称“显著改善”。

---

## 与现有评测的关系

| Harness | 负责 |
|---------|------|
| [`../evidence-eval`](../evidence-eval) | 写操作前证据是否立住？ |
| [`../tool-routing-eval`](../tool-routing-eval) | 选得到对的工具吗？ |
| **`ev-pres`** | 基线 vs 候选：Pass/Avg、得失、成本、机制归因 |

EV-PRES 不替代具体 harness 的评分器；它把逐 case 结果汇总成可预注册、可
对照、可进 PR 门禁的报告。

---

## 非目标

- 不新建评测平台或 UI。
- 不做 LLM judge。
- 不自动改 Skill、不自动上线。
- 不做自动进化 / ModularRSI / AGE。
- 不扫描生产轨迹。

---

## 测试

```bash
python scripts/ev-pres/test_summarize.py
```

仅用 Python 标准库（`unittest` / `argparse` / `json` / `math`）。
