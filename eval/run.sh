#!/usr/bin/env bash
# Replays saved run records through one variant's generator, then scores the outputs with this
# checkout's judge, so every variant is judged by the same code.
#
#   eval/run.sh <git ref> <out dir> <record.json>...
#
# Needs model (the generator), JUDGE_MODEL and OPENAI_API_KEY. N=3 takes three samples per
# record. FACTS is the frozen-facts folder, shared by every arm; it defaults to <out dir>/../facts.
set -euo pipefail

if [ $# -lt 3 ]; then
	echo "usage: eval/run.sh <git ref> <out dir> <record.json>..." >&2
	exit 2
fi
: "${model:?set model, the generator model}" "${JUDGE_MODEL:?set JUDGE_MODEL}" "${OPENAI_API_KEY:?set OPENAI_API_KEY}"
ref=$1
mkdir -p "$2"
out=$(cd "$2" && pwd)
shift 2
records=()
for r in "$@"; do
	records+=("$(cd "$(dirname "$r")" && pwd)/$(basename "$r")")
done

wt=$(mktemp -d)
trap 'git worktree remove --force "$wt"' EXIT
git worktree add --detach "$wt" "$ref" >/dev/null
echo "$ref $(git -C "$wt" rev-parse HEAD)" >"$out/VARIANT"

(cd "$wt" && go run ./eval/replay -out "$out" -n "${N:-1}" "${records[@]}")
go run ./eval/score -facts "${FACTS:-$out/../facts}" "$out"/*.json
