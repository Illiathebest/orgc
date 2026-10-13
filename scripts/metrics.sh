#!/usr/bin/env bash
# Числа orgc, що залежать від коду (обсяг, покриття, тести, корпус, вихід
# render, діагностики), — в образі iac-runner, щоб версія Go була та сама,
# що в CI. З кореня репозиторію:
#
#   scripts/metrics.sh [каталог]     # типово out/metrics
set -euo pipefail

image=${IAC_RUNNER_IMAGE:-iac-runner:dev}
repo=$(cd "$(dirname "$0")/.." && pwd)
out=${1:-out/metrics}
cache="$HOME/.cache/orgc-metrics"
mkdir -p "$cache"

docker run --rm -u "$(id -u):$(id -g)" -e HOME=/tmp -e PYTHONDONTWRITEBYTECODE=1 \
  -v "$repo":/w -v "$cache":/cache -e GOMODCACHE=/cache/mod -e GOCACHE=/cache/build -e GOFLAGS=-buildvcs=false \
  -e METRICS_COMMIT="$(git -C "$repo" rev-parse --short HEAD 2>/dev/null || cat "$repo/.source-commit" 2>/dev/null || echo '?')" \
  -w /w "$image" python scripts/metrics.py "/w/$out"
