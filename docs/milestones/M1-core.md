# M1 — Core: execution environment + модель даних

> Статус: **виконано — M1 ✅** (підсумок у [M1-report.md](M1-report.md)). Передумова: M0 ✅ (ctrl-01, GitLab, lint-пайплайн).
> Мета: дати платформі (1) відтворюване середовище виконання й (2) серце концепції Organization-as-Data — машиночитану модель компанії з валідацією в CI.

## Чому саме це зараз

M0 дав інфраструктуру CI. M2 (Terraform) і далі будуть спиратися на однакове оточення інструментів і на модель даних, з якої генеруються інвентарі/конфіги. Тому M1 — фундамент: без нього кожен наступний мілстоун винаходив би оточення й формат даних заново.

## Обсяг

### 1. Образ `iac-runner` (execution environment)

Власний Docker-образ у GitLab Container Registry, у якому виконуються всі CI-jobs — щоб версії інструментів були зафіксовані й однакові локально та в пайплайні.

- База: `python:3.12-slim` (популярна, легка; обґрунтування проти `ubuntu`/`alpine` — в ADR-005).
- Вміст: `ansible-core`, колекції з `requirements.yml` (cisco.ios, community.routeros, community.docker, ansible.netcommon), NAPALM, OpenTofu, лінтери (yamllint, ansible-lint, tflint), `jsonschema`/`check-jsonschema`.
- Збірка — окремий пайплайн `ci/runner-image.gitlab-ci.yml` (Kaniko, без docker-in-docker), тег за git SHA + `latest`.
- Lint-стадія M0 переводиться на цей образ замість публічних.

### 2. Модель даних `organization.yml` + JSON Schema

- `schema/organization.schema.json` — формальна схема (JSON Schema draft 2020-12): сайти, мережі (VLAN/CIDR), пристрої, сервери, політики, прапорці модулів, профіль S/M/L.
- `examples/organization.s-profile.yml`, `examples/organization.m-profile.yml` — еталонні приклади (на наш двовузловий стенд: pve-a = HQ, pve-b = філія).
- CI-стадія `validate`: `check-jsonschema` валідує всі `organization*.yml` проти схеми на кожен MR. Невалідна модель — червоний пайплайн.
- Документація формату: `docs/data-model.md` (опис кожного поля простою мовою).

### 3. Нове рішення під ADR

- **ADR-005** — мова/спосіб валідації моделі: JSON Schema vs Pydantic vs Cerberus vs Rego/OPA (порівняння, вибір під принцип популярності).

## Definition of Done

- [x] `iac-runner` збирається пайплайном (Kaniko) і лежить у Registry як `:latest`+`:SHA`; lint-jobs M0 переведено на нього.
- [x] `organization.schema.json` (JSON Schema 2020-12) покриває всі поля з прикладу CONCEPT.md §2; приклади s/m-профілів валідні.
- [x] `validate`-стадія: коректні приклади проходять, навмисно зламаний валить пайплайн (job `validate-negative`) — обидва напрямки.
- [x] `docs/data-model.md` + ADR-005 записані.
- [x] (рекомендоване §4) `smoke_routeros.yml` прочитав факти з живого RB750 (RouterOS 6.48) через CI, read-only (`changed=0`).
- [x] (старт M1) ctrl-01 повернуто на 8 ГБ; `mon-01` (Prometheus + Alertmanager + Grafana + Loki) піднято на pve-a.

### 4. (Рекомендовано) Ранній network-smoke проти живого RB750

Дешева перевірка, що ланцюг «iac-runner → RouterOS API → реальний пристрій» працює, ще до повноцінного мережевого модуля (M3):

- Read-only плейбук `ansible/playbooks/smoke_routeros.yml`: підключення до RB750 (RouterOS 6.48) через `community.routeros`, читання фактів (`/system/resource`, інтерфейси) — **нічого не змінює**.
- Запуск з iac-runner у CI (manual job) або локально; облікові дані — через Ansible Vault / CI variable.
- Підтверджує керованість фізичного MikroTik рано й безпечно. Конфігурація (запис) лишається на M3-M4.

> Обґрунтування порядку: read-only smoke не потребує `organization.yml`, тому не порушує принцип «фундамент перший»; повноцінна мережева автоматизація (генерація конфігів з моделі) — M3+.

## Поза M1 (далі)

Генерація інвентарів/конфігів з моделі — M2+ (коли буде куди генерувати: Terraform-VM, мережа). M1 лише фіксує й валідує формат. Повний мережевий модуль (запис конфігів, Safe Mode/rollback) — M3-M4.
