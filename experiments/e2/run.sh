#!/usr/bin/env bash
# E2: повнота виявлення дефектів мутаційним тестуванням. Запуск з кореня
# репозиторію на хості з Docker:
#
#   experiments/e2/run.sh
#
# Виконується в образі iac-runner (змінна IAC_RUNNER_IMAGE). E2_PER_CLASS —
# мутантів на клас (типово 50). Результат — experiments/e2/results/.
set -euo pipefail

image=${IAC_RUNNER_IMAGE:-iac-runner:dev}
repo=$(cd "$(dirname "$0")/../.." && pwd)
cache="$HOME/.cache/orgc-e2"
mkdir -p "$cache"

docker run --rm -u "$(id -u):$(id -g)" -e HOME=/tmp -e PYTHONDONTWRITEBYTECODE=1 \
  -v "$repo":/w -v "$cache":/cache -e GOMODCACHE=/cache/mod -e GOCACHE=/cache/build -e GOFLAGS=-buildvcs=false \
  -e E2_PER_CLASS="${E2_PER_CLASS:-50}" -w /w "$image" python experiments/e2/run.py
