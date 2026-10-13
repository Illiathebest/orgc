#!/usr/bin/env bash
# Усі експерименти однією командою, з кореня репозиторію:
#
#   experiments/run.sh            # E1, E2, E3, E5; E4 в емуляції, якщо є containerlab
#   experiments/run.sh --no-e4    # без E4
#
# Кожен експеримент виконується в образі iac-runner (IAC_RUNNER_IMAGE) і пише
# свої результати в experiments/eN/results/. E4 на живому RB750 потребує
# ручних кроків і сюди не входить (experiments/e4/README.md).
set -euo pipefail

cd "$(dirname "$0")/.."
e4=yes
[ "${1:-}" = --no-e4 ] && e4=no

for e in e1 e2 e3 e5; do
  echo "=== $e"
  "experiments/$e/run.sh"
done

if [ "$e4" = yes ]; then
  if command -v containerlab >/dev/null; then
    for m in experiments/e4/models/minimal.yml examples/organization.s-profile.yml \
      experiments/e4/models/eos-gateway.yml examples/organization.m-profile.yml \
      experiments/e4/models/capacity.yml; do
      echo "=== e4 $m"
      experiments/e4/run.sh "$m"
      name=$(basename "$m" .yml)
      mkdir -p "experiments/e4/results/$name"
      cp "experiments/e4/out/$name/report.json" "experiments/e4/out/$name/timing.txt" "experiments/e4/results/$name/"
    done
  else
    echo "=== e4: containerlab не знайдено, E4 пропущено (запуск — на хості емуляції, ADR-010)"
  fi
fi
