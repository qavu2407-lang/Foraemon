# eval

One place to test a change against the baseline without redesigning the evaluation each time.
Nothing here runs in the daily email.

| Part | What it does |
|---|---|
| [score/](score/main.go) | The judge: factual precision, contradiction rate, importance-weighted recall and F1 against the run's own evidence, plus every claim's label, cited evidence, quote and reason for auditing |
| [replay/](replay/main.go) | Re-runs the generator on saved run records' inputs |
| [compare/](compare/main.go) | Pairs baseline and variant scores by date: means, paired wins/losses/ties, sign-test p |
| [run.sh](run.sh) | One variant end to end: worktree → replay → score |
| [cases/](cases/main.go) | Synthetic cases from real days: plant, remove or flip a driver, shuffle or pad headlines, inject an instruction; checks each replayed output in code, no judge |
| [experiments/](experiments/) | The log: one file per experiment, decided before running, filled in after |

## Running an experiment

1. Copy [experiments/TEMPLATE.md](experiments/TEMPLATE.md) to `experiments/NNN-name.md`. Fill in everything above **Results** before spending anything.
2. Make the change on a branch cut from the baseline commit. The branch is the variant.
3. Get the run records: `git fetch origin eval-data && git worktree add ../eval-data eval-data`.
4. Run both arms on the same records, into the eval-data worktree so master stays small:

   ```sh
   set -a; . ./.env; set +a
   export JUDGE_MODEL=gpt-4.1
   R=../eval-data/tests/results/2026-1*.json   # pick the days, then don't change them
   eval/run.sh <baseline ref> ../eval-data/experiments/NNN/baseline $R
   eval/run.sh <variant ref>  ../eval-data/experiments/NNN/variant  $R
   go run ./eval/compare ../eval-data/experiments/NNN/baseline ../eval-data/experiments/NNN/variant
   ```

5. Paste the table into the experiment file, write the decision and why, commit it.

## Rules that keep a comparison fair

- **Same inputs.** Both arms replay the same records. A change to what is *fetched* can't be
  replayed: the saved inputs don't hold the new evidence. That needs live arms (not built yet).
- **Same judge.** `run.sh` scores with the checkout you run it from, never the variant's.
  `compare` refuses scores from different judge models or prompts (`prompts_hash`). Changing the
  judge means re-scoring the baseline: record it as a new baseline, not as a variant.
- **Same facts.** Frozen per day and per judge in the facts folder, shared by both arms.
- **Both arms replayed.** Don't compare a variant's replay against the live run's original
  output: replay the baseline too, so both went through the same path.
- **Decide before you look.** Primary metric, minimum effect and pass rule go in the experiment
  file before running. Picking them after seeing the numbers turns noise into findings.
- **Variants need this folder.** `run.sh` runs the variant's own `eval/replay`, so a variant must
  branch from a commit that has it.

## Synthetic cases

When there aren't enough real days, each real record becomes 9 cases with a known right answer:

```sh
go run ./eval/cases make -out ../eval-data/cases/inputs ../eval-data/tests/results/*.json
N=3 eval/run.sh <ref> ../eval-data/cases/<ref> ../eval-data/cases/inputs/*.json
go run ./eval/cases check ../eval-data/cases/<ref>
```

`run.sh` also scores them with the judge; to skip that cost, run `eval/replay` in the variant's
checkout instead. The planted headlines are hand-written in [cases/stories.go](cases/stories.go).
Cases test whether the system *can* find a driver, not how often it does on real days.
