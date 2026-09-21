# M1 — Звіт результатів (2026-06-15)

> Короткий підсумок для головного чату планування. План і DoD — у `M1-core.md`.

## Підсумок

**DoD M1 досягнуто повністю.** Платформа отримала два фундаменти: (1) відтворюване середовище виконання `iac-runner` і (2) серце концепції Organization-as-Data — машиночитану модель `organization.yml` з валідацією в CI. Додатково: рання read-only перевірка живого RB750 і повернення відкладеного моніторингу `mon-01`.

Усі зміни пройшли через MR у захищену `main` (lint → validate зелені). Вузол фокусу — pve-a (HQ); pve-b і повна мережа лишаються на M2-M3.

## Що зроблено

- **Образ `iac-runner`** (execution environment) у GitLab Container Registry. База `python:3.12-slim-bookworm`; усередині: `ansible-core` 2.18.1, колекції (`cisco.ios` 11.4.1, `ansible.netcommon` 8.5.2, `community.routeros` 3.20.0, `community.docker` 5.2.1, `ansible.posix` 2.2.0), NAPALM з `napalm-ros` 1.2.6, `librouteros`, OpenTofu 1.12.1, tflint 0.63.1, лінтери (yamllint 1.37.1, ansible-lint 25.5.0), `check-jsonschema` 0.37.2. Збірка — окремим пайплайном `ci/runner-image.gitlab-ci.yml` через **Kaniko (без docker-in-docker)**, теги `:SHA` + `:latest`. Lint-стадію M0 переведено на цей образ.
- **Модель даних:** `schema/organization.schema.json` (JSON Schema 2020-12) покриває всі поля CONCEPT §2; приклади `examples/organization.{s,m}-profile.yml` під двовузловий стенд; навмисно зламаний `organization.broken.yml`. CI-стадія **`validate`** (коректні проходять) + **`validate-negative`** (зламаний валить пайплайн). Опис кожного поля простою мовою — `docs/data-model.md`.
- **ADR-005** — спосіб валідації моделі: обрано **JSON Schema** (vs Pydantic/Cerberus/OPA) за принципом популярності й декларативності; Додаток А обґрунтовує базу `python:3.12-slim`.
- **Network-smoke:** `ansible/playbooks/smoke_routeros.yml` через CI (manual job на `main`) прочитав факти живого **RB750 (RouterOS 6.48)** — версію, памʼять, аптайм, інтерфейси; **read-only підтверджено** (`changed=0`). Read-only користувач `ansible-ro` (група `read`), пароль — masked+protected CI variable `ROUTEROS_PASSWORD`.
- **Залізо:** ctrl-01 повернуто на **8 ГБ**; піднято VM **`mon-01`** (`192.168.1.51`, 2 vCPU / 3 ГБ / 20 ГБ) зі стеком **Prometheus + Alertmanager + Grafana + Loki + node-exporter** (Docker Compose, `bootstrap/mon-01/`). Grafana з автопідключеними джерелами Prometheus і Loki; Prometheus-таргети UP.

## Ключові рішення та відхилення (для диплома)

1. **`setuptools` у образі.** `napalm` на старті імпортує `pkg_resources`, якого немає у свіжому `python:3.12-slim` → довелося явно додати `setuptools`. Типова пастка міграції на Python 3.12.
2. **Insecure HTTP-registry.** Registry стенда — без TLS, тож Kaniko збирає з `--insecure`, а Docker-демон ctrl-01 отримав `insecure-registries`. Перехід на TLS — кандидат на майбутнє (не блокує M1).
3. **Секрети smoke — через CI variable, не Vault.** Для read-only smoke обрано простіший шлях (env-змінна `ROUTEROS_PASSWORD`); міграцію на Ansible Vault разом із write-кредами та Cisco заплановано на M3-M4 (ARCHITECTURE §2.4).
4. **World-writable CI-каталог.** GitLab клонує репо у world-writable теку, тож Ansible ігнорує `ansible.cfg`; рішення — передавати інвентар явно (`-i`). Урок про середовище виконання.
5. **`community.routeros` 3.x і RouterOS v6.** Декларативні модулі колекції тяжіють до v7, але `api`-модуль читає шляхи незалежно від версії — для read-only smoke v6.48 працює. Це підтверджує життєздатність тулінгу під наш RB750.
6. **`mon-01` — окрема VM (варіант A).** Моніторинг відокремлено від control plane, щоб переживав падіння вузла, який моніторить. Повний скрейп ctrl-01/GitLab/RB750 і збір логів — M4.
7. **Thin-pool over-commit на pve-a.** Resize диска mon-01 дав попередження про перепідписку thin-пулу (128 ГБ диск ноута). Безпечно, поки фактичне використання менше за пул; потребує періодного контролю `lvs`/`df` — жива ілюстрація обмеженого заліза.

## Відкриті питання

- **Cisco-образи (ADR-003):** доступ через робочі ресурси (CML/VIRL) до M3 — лишається відкритим; MikroTik вже на реальному залізі.
- **Семантична валідація моделі:** неперетин CIDR, унікальність VLAN, default-deny — поза JSON Schema; шар політик (OPA/`conftest` або власний валідатор) — кандидат на M3+ (зафіксовано в ADR-005).
- **Секрети:** міграція кредів RouterOS + додавання write-кредів у Ansible Vault — M3-M4.
- **Повна спостережуваність:** node-exporter/promtail на хостах, snmp_exporter, реальні приймачі Alertmanager, дашборди — M4.
- **Диск pve-a:** контроль заповнення thin-пулу.

## Готовність до наступних мілстоунів

- **M2 (Provisioning):** `iac-runner` уже містить OpenTofu 1.12.1 + tflint; токен `terraform@pve!tofu` готовий з M0. Далі — модуль `proxmox-vm`, HTTP state backend у GitLab, підключення pve-b (nested Proxmox). Модель даних існує — з неї згодом генеруватимуться інвентарі.
- **M3 (Network Fabric):** валідовано ланцюг до живого RB750; інвентар і групові змінні MikroTik готові; наступне — write-конфігурація (Safe Mode/rollback) і генерація з `organization.yml`.
- **M4 (Security & Observability):** стек `mon-01` піднято — лишається наповнити (скрейп хостів/мережі, логи, Wazuh, дашборди, drift detection).
