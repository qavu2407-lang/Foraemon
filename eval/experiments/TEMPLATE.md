# NNN: <name>

Status: planned | running | decided
Date:

## Before running

**Hypothesis:** what you expect to change, and why.

**Module changed:** prompt / model / output format / evidence fetched / other. Exactly one.

**Mode:** replay (prompt, model, format) or live (what is fetched).

**Baseline:** experiment and git ref. **Variant:** git ref, and one line on the diff.

**Inputs:** which run-record dates, and samples per day (N).

**Judge:** model and `prompts_hash`. Must match the baseline's.

**Primary metric:** one. **Minimum effect that matters:**

**Pass rule:** e.g. primary metric wins on ≥ 6 of 7 paired days.

**Guardrails:** what must not get worse (failures, cost, tokens, contradiction rate, badq).

**Budget:** expected calls/tokens.

## Results

Paste `eval/compare` output. Note the score folder on eval-data.

## Decision

Adopt / reject / rerun, and why. Anything you read in the judge's reasons that changed your mind.
