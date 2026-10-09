# ADR-004: Стек моніторингу та SIEM (модуль Security & Observability)

- **Статус:** Proposed (rev. 2 — оновлено під 32 ГБ і принцип популярності)
- **Дата:** 2026-06-07; переглянуто 2026-06-08
- **Контекст:** Модуль — у MVP. Потрібен стек, який масштабується разом із профілями S/M/L і демонструє «безпеку як вимірюваний стан». Бюджет: 32 ГБ на всю лабораторію.

## Довідка по технологіях

- Prometheus + Grafana — де-факто стандарт моніторингу в DevOps-світі: Prometheus збирає метрики (pull-модель, exporters), Grafana малює дашборди. Loki — «Prometheus для логів» від Grafana Labs, легкий збір логів. Alertmanager — маршрутизація алертів.
- Zabbix — класика enterprise/ISP-моніторингу (особливо популярний в Україні у телекомі), сильний у SNMP, але GUI-центричний: конфігурація живе в його БД, а не в Git, що ламає нашу IaC-тезу.
- VictoriaMetrics — швидша й економніша заміна Prometheus (українська розробка), сумісна з його екосистемою; менш відома за межами моніторинг-спільноти.
- Wazuh — найпопулярніший опенсорсний SIEM/XDR: агенти на хостах (контроль цілісності файлів FIM, виявлення руткітів, CIS-комплаєнс), manager аналізує події за правилами, indexer (OpenSearch) зберігає й шукає, dashboard — веб-інтерфейс.
- CrowdSec — легкий «колективний fail2ban», інший клас рішення (блокування атак, не SIEM).

## Варіанти

### Метрики + логи (observability)

| Критерій | **A. Prometheus + Grafana + Loki** | B. Zabbix | C. VictoriaMetrics + Grafana |
|---|---|---|---|
| RAM на стенді | ~1–1.5 ГБ | ~1.5–2 ГБ (+SQL БД) | ~0.7–1 ГБ |
| Конфігурація як код | так (yaml, provisioning дашбордів) | частково (API/шаблони) | так |
| SNMP (Cisco/MikroTik) | snmp_exporter | нативно, сильна сторона | snmp_exporter |
| Логи | Loki + promtail/alloy — легкі | слабко | потрібен окремий стек |
| Екосистема/типовість для DevOps | еталонна | legacy-enterprise | сумісна з Prometheus |

### SIEM / host security

| Критерій | A. Wazuh all-in-one | B. Wazuh manager + Loki/Grafana як бекенд подій | C. CrowdSec + auditd |
|---|---|---|---|
| RAM | 4–8 ГБ (indexer = OpenSearch) | ~1–1.5 ГБ (без indexer/dashboard) | ~0.3 ГБ |
| FIM, виявлення руткітів, SCA/CIS-комплаєнс | так | так (це функції manager/agent) | ні (інший клас) |
| Повноцінний SIEM-пошук | так | обмежений (LogQL замість OpenSearch) | ні |
| Відповідність поняттю SIEM | так, але не влазить | так: агенти+правила ті самі, зріз у Grafana | підміна поняття SIEM |

## Рішення (рекомендація, rev. 2)

**Observability: варіант A** — Prometheus + Alertmanager + Grafana + Loki (node_exporter на серверах, snmp_exporter для Cisco/MikroTik). Це індустріальний стандарт, який повністю описується кодом: дашборди й алерти генеруються з `organization.yml`. Zabbix відхилено через GUI-центричну конфігурацію (суперечить IaC-тезі), VictoriaMetrics розглядаємо лише як сумісну оптимізацію.

**SIEM: повний Wazuh single-node** (manager + indexer + dashboard, окрема VM ~4 ГБ). На 32 ГБ він уміщається, і це найпопулярніший шлях: стандартні дашборди, повний пошук по подіях, типова конфігурація з документації. Manager-only варіант з подіями в Loki (rev. 1) переноситься в документацію продукту як *лайт-режим S-профілю для слабкого заліза*.

Drift detection (нічний `tofu plan` + `ansible --check`) репортить у Alertmanager. Так замикається теза «несанкціонована зміна конфігурації = інцидент безпеки».

## Наслідки

- (+) Канонічний Wazuh-досвід: стандартні правила, комплаєнс-звіти CIS, повний UI.
- (+) Grafana — для метрик/логів/drift, Wazuh dashboard — для security-подій; інтеграція алертів обох у Alertmanager.
- (−) +4 ГБ RAM (окрема VM `wazuh-01`); це враховано в бюджеті M0.
- Профільна матриця: S-lite = manager-only→Loki; S/M = single-node all-in-one; L = Wazuh cluster + federated Prometheus (агенти й правила незмінні).
