# Стенд — шпаргалка (стан на 2026-06-15, після M1)

> Що є і як з ним взаємодіяти після паузи. Деталі — у `docs/milestones/*-report.md`.

## Що де живе

| Об'єкт | Адреса / доступ | Що це |
|---|---|---|
| **pve-a** (ноут, HQ) | Proxmox web: `https://192.168.1.40:8006` | Гіпервізор HQ. Open-bench, тільки дротовий Ethernet. |
| ├ **ctrl-01** (VM, 8 ГБ) | GitLab: `http://192.168.1.50` · SSH: `ssh -p 2222` · Registry: `192.168.1.50:5050` | Docker + GitLab CE + Runner. |
| └ **mon-01** (VM, 3 ГБ) | Grafana: `http://192.168.1.51:3000` · Prometheus: `:9090` | Prometheus + Alertmanager + Grafana + Loki + node-exporter. |
| **pve-b** (ПК) | — | VMware встановлено, nested Proxmox ще НЕ піднято (буде в M2). |
| **RB750** (MikroTik) | `192.168.1.220` · RouterOS 6.48 · WinBox/SSH | Реальна мережева ціль. Read-only user `ansible-ro`. |
| Домашній роутер | `192.168.1.1`, LAN `192.168.1.0/24` | Gateway + DHCP. |
| Репозиторій | `iac-infra` у GitLab | Каркас, `main` захищена. |

## Акаунти (паролі — НЕ тут; де лежать — у колонці)

| Де | Логін | Права | Де пароль/секрет |
|---|---|---|---|
| pve-a Proxmox (web/SSH) | `root@pam` | повний | задано при інсталяції (твій запис) |
| Proxmox API (OpenTofu) | токен `terraform@pve!tofu` | PVEVMAdmin на `/`, PVEDatastoreUser на `/storage` | GitLab CI variable (masked+protected) |
| ctrl-01 / mon-01 (OS) | `ansible-svc` | SSH-ключ, у групі `docker`; адмін через `sudo -i` → root | приватний ключ на робочому ПК (`~/.ssh/id_ed25519`); root-пароль VM — твій запис |
| GitLab (web) | `root` | адмін | початковий був у `/srv/gitlab/config/initial_root_password`, змінено при першому вході |
| Git push / API | Personal Access Token | за scope токена | твій запис / GitLab CI |
| Grafana (`:3000`) | `admin` | адмін | дефолт `admin` змінено при першому вході |
| RB750 (WinBox/SSH) | `admin` | повний (RouterOS дефолт) | твій запис |
| RB750 автоматизація | `ansible-ro` | лише читання (група `read`) | env `ROUTEROS_PASSWORD` = GitLab CI masked variable |

> Реальні паролі тримай у менеджері секретів, не в репозиторії. Забув пароль GitLab-root: `docker exec -it gitlab gitlab-rake "gitlab:password:reset[root]"` на ctrl-01.

## Увімкнути / вимкнути

- **pve-a з вимкненого:** коротко замкнути power-on пади на матплаті → далі WoL (`tools/wake-pve-a.bat`). Тримати на AC (батарея = UPS).
- **VM на pve-a:** через Proxmox web або `qm start <VMID>` / `qm stop <VMID>` по SSH.
- **pve-b:** вмикати ПК → VMware → запустити VM з Proxmox лише на час експериментів.

## Робочий цикл (як вносити зміни)

1. `git checkout -b <feature>` → правки в `iac-infra`.
2. `git push` → відкрити Merge Request у GitLab.
3. CI сам ганяє **lint** (yamllint, ansible-lint) + **validate** (`organization.yml` проти JSON Schema). Мають бути зелені.
4. Review → merge у `main`.
5. Ручні дії (напр. мережевий smoke) — **manual job** у пайплайні на `main`.

## Часті команди (з ctrl-01 / iac-runner)

```bash
# перевірити модель даних
check-jsonschema --schemafile schema/organization.schema.json examples/organization.s-profile.yml

# read-only факти з живого RB750 (нічого не змінює)
ansible-playbook -i ansible/inventories/staging/hosts.yml ansible/playbooks/smoke_routeros.yml

# ВАЖЛИВО: інвентар передавати явно через -i (CI-каталог world-writable → ansible.cfg ігнорується)
```

## Пастки (з M1, щоб не наступити знову)

- GitLab Registry **без TLS** → Docker/Kaniko з `--insecure` / `insecure-registries`.
- Диск pve-a лише 128 ГБ, **thin-pool over-commit** → періодично `lvs` / `df -h`.
- Секрети — у GitLab CI variables (masked+protected), напр. `ROUTEROS_PASSWORD`. НЕ в репозиторії.
- Тулінг MikroTik — під **RouterOS v6** (RB750 має 32 МБ RAM, v7 не тягне).

## Поточний фокус

Наступний етап — **M2 (Provisioning):** Terraform-модуль `proxmox-vm`, state у GitLab, підключення pve-b. План: `docs/milestones/M2-provisioning.md`.
