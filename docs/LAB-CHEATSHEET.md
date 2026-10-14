# Стенд — шпаргалка (стан на 2026-06-15, після M1; емуляція — 2026-10-13)

> Тут зібрано, що є на стенді і як з ним працювати. Деталі наведено в `docs/milestones/*-report.md`.

## Що де живе

| Об'єкт | Адреса / доступ | Що це |
|---|---|---|
| pve-a (ноут, HQ) | Proxmox web: `https://192.168.1.40:8006` | Гіпервізор HQ. Open-bench, тільки дротовий Ethernet. |
| ├ ctrl-01 (VM, 8 ГБ) | GitLab: `http://192.168.1.50` · SSH: `ssh -p 2222` · Registry: `192.168.1.50:5050` | Docker + GitLab CE + Runner. |
| └ mon-01 (VM, 3 ГБ) | Grafana: `http://192.168.1.51:3000` · Prometheus: `:9090` | Prometheus + Alertmanager + Grafana + Loki + node-exporter. |
| clab-01 (VM у Hyper-V на ноутбуці, 20 ГБ) | SSH: `ssh clab-01` (`10.99.0.10`, лише з ноутбука; далі — через Tailscale) | Docker + containerlab, образи `ceos:4.36.2F` і `vrnetlab/mikrotik_routeros:6.48`. Мережа керування емуляції `10.99.1.0/24` (ADR-010). |
| RB750 (MikroTik) | `192.168.1.220` · RouterOS 6.49.23 · WinBox/SSH | Реальна мережева ціль. Користувачі `ansible-ro` (читання) і `ansible-rw` (запис, для застосування конфігурації). |
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

> Реальні паролі зберігаються в менеджері секретів. У репозиторії їх немає. Якщо пароль GitLab-root втрачено, його скидають на ctrl-01 командою `docker exec -it gitlab gitlab-rake "gitlab:password:reset[root]"`.

## Увімкнути / вимкнути

- pve-a з вимкненого стану вмикаємо коротким замиканням power-on падів на материнській платі. Далі працює WoL (`tools/wake-pve-a.bat`). Вузол тримаємо на AC, батарея служить як UPS.
- VM на pve-a запускаємо і зупиняємо через Proxmox web або командами `qm start <VMID>` / `qm stop <VMID>` через SSH.
- clab-01 запускаємо у Hyper-V на ноутбуці на час експериментів. Топологію розгортає команда `containerlab deploy -t containerlab/lab.clab.yml` у каталозі виходу `orgc render`. Прибирає її команда `containerlab destroy -t … --cleanup`.

## Робочий цикл (як вносити зміни)

1. Створюємо гілку `git checkout -b <feature>` і вносимо правки в `iac-infra`.
2. Виконуємо `git push` і відкриваємо Merge Request у GitLab.
3. CI сам запускає lint (yamllint, ansible-lint) і validate (`organization.yml` проти JSON Schema). Обидві стадії мають пройти.
4. Після review виконуємо merge у `main`.
5. Ручні дії (наприклад, мережевий smoke) запускаються як manual job у пайплайні на `main`.

## Часті команди (з ctrl-01 / iac-runner)

```bash
# перевірити модель даних
check-jsonschema --schemafile orgc/schema/organization.schema.json examples/organization.s-profile.yml

# read-only факти з живого RB750 (нічого не змінює)
ansible-playbook -i ansible/inventories/staging/hosts.yml ansible/playbooks/smoke_routeros.yml

# ВАЖЛИВО: інвентар передавати явно через -i (CI-каталог world-writable → ansible.cfg ігнорується)
```

## Пастки (з M1, щоб не наступити знову)

- GitLab Registry працює без TLS. Тому Docker і Kaniko запускаються з `--insecure` / `insecure-registries`.
- Диск pve-a має лише 128 ГБ, а thin-pool працює з over-commit. Заповнення періодично перевіряємо командами `lvs` / `df -h`.
- Секрети зберігаються в GitLab CI variables (masked+protected), наприклад `ROUTEROS_PASSWORD`. У репозиторій вони не потрапляють.
- Інструменти для MikroTik написано під RouterOS v6. RB750 має 32 МБ RAM, і v7 на ньому не працює.

## Поточний фокус

План етапу M2 (Terraform-модуль `proxmox-vm`, state у GitLab, підключення pve-b) знято з роботи разом із Terraform.
