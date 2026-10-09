# M0 — Звіт результатів (2026-06-10)

## Підсумок

Мінімальний DoD M0 досягнуто: ctrl-01 працює, lint-пайплайн зелений, runbook записаний.
Фокус етапу — вузол pve-a (HQ); pve-b і мережеву частину перенесено на наступні етапи.

## Що зроблено

- pve-a: Proxmox VE 9.2 bare-metal (`192.168.1.40`), no-subscription, оновлено. Кришка не присипляє. Працює Wake-on-LAN з повного вимкнення (S5): ноут піднімається віддалено скриптом `tools/wake-pve-a.bat`.
- API-токен `terraform@pve!tofu` створено (для OpenTofu у M2).
- Шаблон Ubuntu 24.04 cloud-init (VMID 9000).
- ctrl-01 (VMID 101, 2 vCPU / 6 ГБ / 40 ГБ): Docker + GitLab CE 18.11.4 + Runner 18.11.3 (docker compose, тюнінг puma=2).
- Репозиторій `iac-infra`: каркас за ARCHITECTURE.md §4, `main` захищена, самореєстрація вимкнена.
- Пайплайн lint (yamllint + ansible-lint, профіль production) — зелений на `main` (#1) і в MR (#2).
- Створено: `.gitlab-ci.yml` + `ci/lint.gitlab-ci.yml`, `.yamllint`, `.ansible-lint`, `bootstrap/ctrl-01/` (compose + .env.example), `ansible/` (ansible.cfg, requirements.yml, інвентар staging з group_vars), `scripts/vault-pass.sh`, runbook `docs/runbooks/00-bootstrap-lab.md`.

## Ключові відхилення та рішення

1. RAM хоста 8 ГБ замість запланованих 12 (придбано не ту планку). Рішення: ctrl-01 знижено 8→6 ГБ, mon-01 на pve-a відкладено. Виміри: GitLab ~3.3-4.5 ГБ, на хості без свопу/OOM. Після апгрейду до 12-16 ГБ повернути 8 ГБ і mon-01.
2. GitLab CE 18.11.4 (зріла 18.x) замість 19.0 заради стабільності на 2 ядрах.
3. WoL з S5 запрацював. Отже, «завжди доступний HQ» з ADR-001 можливий без виділеного сервера.

## Відкриті питання

- Cisco-образи (ADR-003): перевірити доступ через робочі ресурси (CML/VIRL) до M3; інакше лише MikroTik в емуляції + borrowed-залізо.
- Апгрейд RAM pve-a до 12-16 ГБ (сумісна планка).

## Готовність до наступних мілстоунів

- M1: образ `iac-runner` у GitLab Registry (Ansible+колекції+NAPALM+OpenTofu+лінтери); схема `organization.yml` + валідація.
- M2: Terraform-модуль `proxmox-vm`, HTTP state backend у GitLab, підключення pve-b (nested Proxmox на ПК). Токен `terraform@pve` уже є.
