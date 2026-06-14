# mon-01 — стек спостережуваності

Окрема VM на pve-a зі стеком **Prometheus + Alertmanager + Grafana + Loki** (+ node-exporter). Відокремлено від `ctrl-01`, щоб моніторинг пережив падіння вузла, який моніторить (ADR-004). Розгортається через Docker Compose, як і ctrl-01.

## Параметри стенда (M1)

| | Значення |
|---|---|
| VM | `mon-01`, 2 vCPU / 3 GB RAM / 20 GB |
| IP | `192.168.1.51` |
| Веб | Grafana `:3000`, Prometheus `:9090`, Alertmanager `:9093`, Loki `:3100` |

## Розгортання

1. **Створити VM** на pve-a клоном шаблону Ubuntu 24.04 cloud-init (VMID 9000, з M0):
   - 2 vCPU, 3072 MB RAM, 20 GB диск, статична IP `192.168.1.51`;
   - встановити Docker + Docker Compose (як на ctrl-01, runbook `docs/runbooks/00-bootstrap-lab.md`).

2. **Скопіювати каталог** `bootstrap/mon-01/` на VM (напр. у `/srv/mon-01`).

3. **Підготувати `.env`:**

   ```bash
   cd /srv/mon-01
   cp .env.example .env
   # відредагувати GF_ADMIN_PASSWORD
   ```

4. **Підняти стек:**

   ```bash
   docker compose up -d
   docker compose ps        # усі сервіси Up
   ```

5. **Перевірити:**
   - Grafana: `http://192.168.1.51:3000` (логін з `.env`) → Connections → Data sources: **Prometheus** і **Loki** вже підключені.
   - Prometheus: `http://192.168.1.51:9090/targets` → таргети `prometheus` і `node-exporter` у стані **UP**.
   - Alertmanager: `http://192.168.1.51:9093`.

## Межі M1

Стек **піднято й живий**: Prometheus збирає себе та node-exporter mon-01, Grafana з готовими джерелами, Alertmanager приймає алерти (приймач `null`).

Поза M1 (заплановано на **M4**, модуль Security & Observability):
- node-exporter та promtail/alloy на ctrl-01 та інших хостах (повний збір метрик і логів);
- snmp_exporter для Cisco/MikroTik (RB750);
- реальні приймачі Alertmanager (email/telegram/webhook) і маршрути за severity;
- готові дашборди Grafana та інтеграція drift-detection.
