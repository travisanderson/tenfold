#!/bin/sh
# Scores the detectors on a range of holdout seeds and prints the tables in
# the Evaluation section of backend/README.md. Detectors only: no model is
# called, and nothing is uploaded.
#
#   scripts/holdout-sweep.sh            # seeds 101 to 150, the reported run
#   scripts/holdout-sweep.sh 201 260    # any other range
#
# The reported run also rewrites backend/eval/holdout-sweep.txt. If git then
# shows that file as changed, the results have changed.
#
# It needs Go, and about 25 MB of temporary disk per seed.
set -eu

first=${1:-101}
last=${2:-150}
root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

(cd "$root/samples" && go build -o "$work/loggen" ./loggen)
for seed in $(seq "$first" "$last"); do
	"$work/loggen" --profile holdout --seed "$seed" --out "$work/days/$seed" >/dev/null
done

out=
if [ "$first-$last" = "101-150" ]; then
	out="$root/backend/eval/holdout-sweep.txt"
fi
cd "$root/backend"
HOLDOUT_SWEEP_DIR="$work/days" HOLDOUT_SWEEP_OUT="$out" \
	go test ./internal/detect -run 'TestHoldoutSweep$' -count=1 -v
