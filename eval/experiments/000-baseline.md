# 000: Baseline

Status: planned
Date:

## Before running

**Hypothesis:** none; this is the reference every later experiment compares against.

**Module changed:** none.

**Mode:** replay.

**Baseline:** this. Git ref: the first commit that has `eval/` (not committed yet).

**Inputs:** not enough yet. Only 2026-10-07 exists; the daily run-record pipeline is unmerged.

**Judge:** gpt-4.1, `prompts_hash` from the first score file.

**Primary metric:** open. Precision was chosen, but it measured 0.97 on 2026-10-07 (5 judge
runs of one generator output), close to the ceiling.

**Pass rule:** none; record the means and their spread.

**Guardrails:** none.

**Budget:** per day, one replay plus ~5 judge calls for precision and ~34 more for recall.

## Results

Pre-harness measurement, gpt-4.1 judge, `prompts_hash` 2c1db5b458d9, on the original live output
(not replayed), so not directly comparable to replayed runs: precision 1.00 / 1.00 / 1.00 / 0.91 / 0.96, 26–33 claims, badq 17–21 of ~30.

## Decision
