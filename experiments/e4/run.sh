#!/usr/bin/env bash
# Наскрізна перевірка в емуляції: модель → orgc render → containerlab deploy
# → звірка стану з моделлю → destroy. Запускається на хості containerlab
# (clab-01) з кореня репозиторію:
#
#   experiments/e4/run.sh experiments/e4/models/minimal.yml [--keep]
#
# --keep лишає топологію розгорнутою після звірки.
# Образ з Go, Python і NAPALM — IAC_RUNNER_IMAGE (типово iac-runner:dev).
# Вихід orgc, звіт і час кроків — experiments/e4/out/<модель>/.
set -euo pipefail

model=${1:?використання: run.sh <organization.yml> [--keep]}
keep=${2:-}
image=${IAC_RUNNER_IMAGE:-iac-runner:dev}
repo=$(cd "$(dirname "$0")/../.." && pwd)
name=$(basename "$model" .yml)
out="experiments/e4/out/$name"
lab=lab # name: у топології, яку генерує orgc

cd "$repo"
rm -rf "$out"
mkdir -p "$out"

# Контейнер працює від імені користувача, щоб файли у репозиторії не
# належали root; кеш Go — у домашньому каталозі хоста.
cache="$HOME/.cache/orgc-e4"
mkdir -p "$cache"
runner() {
  docker run --rm -u "$(id -u):$(id -g)" -e HOME=/tmp -e PYTHONDONTWRITEBYTECODE=1 \
    -v "$repo":/w -v "$cache":/cache -e GOMODCACHE=/cache/mod -e GOCACHE=/cache/build \
    -e GOFLAGS=-buildvcs=false "$@"
}

now() { date +%s.%N; }
span() { awk -v a="$1" -v b="$2" 'BEGIN { printf "%.1f", b - a }'; }
t0=$(now)
runner -w /w/orgc "$image" go run ./cmd/orgc render -o "/w/$out/render" "/w/$model"
t1=$(now)

containerlab deploy -t "$out/render/containerlab/lab.clab.yml" --reconfigure >"$out/deploy.log" 2>&1
t2=$(now)

# Готовність: cEOS відповідає в CLI, CHR — здоровий за healthcheck vrnetlab.
ceos=$(docker ps --filter "label=containerlab=$lab" --filter "label=clab-node-kind=arista_ceos" --format '{{.Names}}')
ros=$(docker ps --filter "label=containerlab=$lab" --filter "label=clab-node-kind=mikrotik_ros" --format '{{.Names}}')
for c in $ceos; do
  until docker exec "$c" Cli -p 15 -c "show version" >/dev/null 2>&1; do sleep 3; done
done
for c in $ros; do
  until [ "$(docker inspect -f '{{.State.Health.Status}}' "$c")" = healthy ]; do sleep 3; done
done
t3=$(now)

# Обліковий запис лабораторії на cEOS. orgc облікових записів не генерує —
# на справжньому обладнанні їх так само заводять поза orgc.
for c in $ceos; do
  docker exec "$c" Cli -p 15 -c $'configure\nusername admin privilege 15 role network-admin secret admin\nend' >/dev/null
done

# Вузол керування — сервер моделі з адресою control_node; звірка йде з
# його мережевого простору.
control=$(runner "$image" python /w/experiments/e4/expected.py "/w/$model" |
  python3 -c 'import json,sys; print(json.load(sys.stdin)["control_node"]["server"] or "")')
if [ -z "$control" ]; then
  echo "у моделі немає сервера з адресою вузла керування — звіряти нема звідки" >&2
  exit 2
fi

code=0
runner --network "container:clab-$lab-$control" -w /w/experiments/e4 "$image" \
  python compare.py "/w/$model" --out "/w/$out/report.json" --wait 120 || code=$?
t4=$(now)

printf 'render %s s\ndeploy %s s\nready %s s\ncheck %s s\n' \
  "$(span "$t0" "$t1")" "$(span "$t1" "$t2")" "$(span "$t2" "$t3")" "$(span "$t3" "$t4")" | tee "$out/timing.txt"

if [ "$keep" != --keep ]; then
  containerlab destroy -t "$out/render/containerlab/lab.clab.yml" --cleanup >>"$out/deploy.log" 2>&1
fi
exit "$code"
