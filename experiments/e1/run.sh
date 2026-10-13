#!/usr/bin/env bash
# E1: масштабування orgc verify і orgc render. Запуск з кореня репозиторію
# на хості з Docker:
#
#   experiments/e1/run.sh
#
# Усе виконується в образі iac-runner (Go і Python тієї самої версії, що в
# CI), тож результат не залежить від того, що встановлено на хості.
# Змінні: IAC_RUNNER_IMAGE — образ; E1_RUNS — прогонів CLI на модель
# (типово 11, перший — розігрів); E1_STAGE_RUNS і E1_BENCHTIME — повтори й
# тривалість бенчмарку етапів. Результат — experiments/e1/results/.
set -euo pipefail

image=${IAC_RUNNER_IMAGE:-iac-runner:dev}
repo=$(cd "$(dirname "$0")/../.." && pwd)
cache="$HOME/.cache/orgc-e1"
mkdir -p "$cache"

docker run --rm -u "$(id -u):$(id -g)" -e HOME=/tmp -e PYTHONDONTWRITEBYTECODE=1 \
  -v "$repo":/w -v "$cache":/cache -e GOMODCACHE=/cache/mod -e GOCACHE=/cache/build -e GOFLAGS=-buildvcs=false \
  -e IAC_RUNNER_IMAGE="$image" -e E1_HOST="$(hostname)" -e E1_COMMIT="$(git -C "$repo" rev-parse --short HEAD 2>/dev/null || cat "$repo/.source-commit" 2>/dev/null || echo '?')" \
  -e E1_RUNS="${E1_RUNS:-11}" -e E1_STAGE_RUNS="${E1_STAGE_RUNS:-7}" -e E1_BENCHTIME="${E1_BENCHTIME:-300ms}" \
  -w /w "$image" python experiments/e1/run.py
