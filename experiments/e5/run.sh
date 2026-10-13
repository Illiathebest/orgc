#!/usr/bin/env bash
# E5: локальність diff — скільки виходу orgc render зачіпає одна зміна
# моделі. Запуск з кореня репозиторію на хості з Docker:
#
#   experiments/e5/run.sh
#
# Виконується в образі iac-runner (змінна IAC_RUNNER_IMAGE). Результат —
# experiments/e5/results/.
set -euo pipefail

image=${IAC_RUNNER_IMAGE:-iac-runner:dev}
repo=$(cd "$(dirname "$0")/../.." && pwd)
cache="$HOME/.cache/orgc-e5"
mkdir -p "$cache"

docker run --rm -u "$(id -u):$(id -g)" -e HOME=/tmp -e PYTHONDONTWRITEBYTECODE=1 \
  -v "$repo":/w -v "$cache":/cache -e GOMODCACHE=/cache/mod -e GOCACHE=/cache/build -e GOFLAGS=-buildvcs=false \
  -w /w "$image" python experiments/e5/run.py
