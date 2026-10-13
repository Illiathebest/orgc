#!/usr/bin/env bash
# Перемикання STP у розгорнутій топології: вимкнути активне зʼєднання на
# комутаторі cEOS, поки failover.py з мережевого простору вузла керування
# опитує порти керування пристроїв за ним, потім увімкнути назад.
#
#   experiments/e4/failover.sh <назва> <вузол керування> <cEOS> <інтерфейс> <ip:порт>...
#
# Приклад: WATCH=acc-01 failover.sh acc-01 ctrl-01 dist-01 Ethernet2 192.168.10.5:22
# Хронологія: 5 с до відмови, 40 с відмови, 25 с після відновлення.
# Результат — experiments/e4/out/failover/<назва>.json.
set -euo pipefail

name=$1 control=$2 switch=$3 ifc=$4
shift 4
image=${IAC_RUNNER_IMAGE:-iac-runner:dev}
lab=lab
repo=$(cd "$(dirname "$0")/../.." && pwd)
out="$repo/experiments/e4/out/failover"
mkdir -p "$out"

cli() { docker exec "clab-$lab-$switch" Cli -p 15 -c "$(printf 'configure\ninterface %s\n%s\nend' "$ifc" "$1")" >/dev/null; }

docker run --rm -u "$(id -u):$(id -g)" -e PYTHONDONTWRITEBYTECODE=1 --network "container:clab-$lab-$control" \
  -v "$repo":/w -w /w/experiments/e4 "$image" python failover.py 70 "$@" >"$out/$name.json" &
probe=$!
sleep 5
cli shutdown
echo "$(date +%T) $switch $ifc: shutdown"
sleep 5
# WATCH — комутатор, чиї порти STP показати посеред відмови: резервний порт
# має стати root/forwarding.
for w in ${WATCH:-}; do
  echo "  $w посеред відмови:"
  docker exec "clab-$lab-$w" Cli -p 15 -c "show spanning-tree" | grep -E '^Et' | sed 's/^/    /'
done
sleep 35
cli "no shutdown"
echo "$(date +%T) $switch $ifc: no shutdown"
wait "$probe"
python3 - "$out/$name.json" <<'EOF'
import json, sys
for t, r in json.load(open(sys.argv[1])).items():
    print(f"  {t}: недосяжний {r['down_total_s']} с, найдовше {r['longest_outage_s']} с, наприкінці {'доступний' if r['up_at_end'] else 'НЕДОСЯЖНИЙ'}")
EOF
