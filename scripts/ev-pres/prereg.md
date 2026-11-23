# EV-PRES preregistration

> Fill this in **before** running the candidate. Do not edit the metric or
> threshold after seeing results; if you must, record it as a separate run.

## 1. Question and construct

- **Question:**
- **What the primary criterion actually measures (construct):**
- **What it does not measure:**

## 2. Arms and binding

| Field | Baseline | Candidate |
|-------|----------|-----------|
| Arm name | `baseline` | `candidate` |
| Model / provider | | |
| Prompt / skill version | | |
| Runtime settings / flags | | |
| Tool / connector set | | |
| Binding hash or commit | | |

> A pair only counts if both arms use the same case, grader and environment.
> Record any binding difference; changes to more than one factor make
> attribution ambiguous.

## 3. Corpus and hold-out

- **Source of cases:**
- **How cases were selected:**
- **Tuning corpus vs. acceptance corpus:**
- **Hold-out cases (not used while tuning):**
- **Known coverage gaps:**

## 4. Metrics

- **Primary:** binary `pass` per case. Define the exact criterion:
- **Secondary:** `score`, `tokens`, `duration_ms`.
- **Mechanism label:** Agent Loop / Tool / Observation / Context / Completion,
  or a project-specific enum. Write the allowed values here:
- **Grader:** deterministic code / human rubric / narrow judge (explain why).

## 5. Thresholds and decision

- **New pass target:**
- **New fail ceiling:** (default 0)
- **Average score change:**
- **Cost ceiling:** tokens/case or ms/case
- **Mechanism expectation:**
- **Small-sample caveat:** Wilson CI is reported; do not claim significance
  from point estimates alone.

## 6. Stop rule and decision

- **Stop early if:**
- **Decision rule:**
  - Promote only if:
  - Retest if:
  - Reject if:
- **Human approval required for any runtime/Skill change:** yes / no

## 7. Artifacts

- Input JSONL:
- Summary command:
- Report path:
- Commit / tag:
