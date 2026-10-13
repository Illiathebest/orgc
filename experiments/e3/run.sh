#!/usr/bin/env bash
# E3: коефіцієнт абстракції — рядки моделі проти рядків виходу. Запуск з кореня
# репозиторію на хості з Docker:
#
#   experiments/e3/run.sh
#
# Виконується в образі iac-runner (змінна IAC_RUNNER_IMAGE).
set -euo pipefail

image=${IAC_RUNNER_IMAGE:-iac-runner:dev}
repo=$(cd "$(dirname "$0")/../.." && pwd)
cache="$HOME/.cache/orgc-e3"
mkdir -p "$cache"

docker run --rm -u "$(id -u):$(id -g)" -e HOME=/tmp -e PYTHONDONTWRITEBYTECODE=1 \
  -v "$repo":/w -v "$cache":/cache -e GOMODCACHE=/cache/mod -e GOCACHE=/cache/build -e GOFLAGS=-buildvcs=false \
  -w /w "$image" python experiments/e3/run.py
