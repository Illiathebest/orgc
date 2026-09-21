# Runbook 00 — Bootstrap лабораторного стенда (M0, вузол pve-a)

> Мета: ноутбук (2 ядра / 12 ГБ / 128 ГБ) → Proxmox VE → VM ctrl-01 → GitLab CE +
> Runner → репозиторій `iac-infra` → зелений lint-пайплайн.
> Версії на момент написання (2026-06-08): Proxmox VE 9.2, Ubuntu 24.04 LTS,
> GitLab CE 18.11.4, GitLab Runner 18.11.3.

## 0. Передумови та адресний план

| Параметр | Значення |
|---|---|
| LAN | 192.168.1.0/24, шлюз 192.168.1.1 |
| pve-a (ноутбук, Proxmox) | 192.168.1.40 |
| ctrl-01 (VM: GitLab + Runner) | 192.168.1.50 |
| GitLab URL | `http://192.168.1.50` (registry — `:5050`, git ssh — `:2222`) |
| Користувач автоматизації на VM | `ansible-svc` |

Потрібно: USB-флешка ≥4 ГБ; [ISO Proxmox VE](https://www.proxmox.com/en/downloads/proxmox-virtual-environment/iso);
Rufus (режим **DD**) або balenaEtcher; SSH-ключ на робочому ПК
(`ssh-keygen -t ed25519 -C "illia@lab"`, якщо ще немає). Дані на ноутбуці буде знищено.

⚠️ Якщо шлюз не 192.168.1.1 — виправ його тут, у `bootstrap/ctrl-01/.env` та в команді
`qm set 101 --ipconfig0` нижче.

## 1. Proxmox VE на ноутбук (bare-metal)

1. Запиши ISO на флешку (Rufus → режим DD), завантаж ноутбук з USB
   (Boot Menu зазвичай F12/F9/Esc; вимкни Secure Boot, якщо інсталятор не стартує).
2. `Install Proxmox VE (Graphical)` → диск: цілий 128 ГБ, ФС за замовчуванням
   (LVM-thin) → країна/часовий пояс → пароль root + email.
3. Мережа: hostname `pve-a.lab.lan`, IP `192.168.1.40/24`, gateway `192.168.1.1`,
   DNS `192.168.1.1`.
4. Після перезавантаження зайди з ПК: `https://192.168.1.40:8006` (root, попередження
   про самопідписаний сертифікат — нормально).

### 1.1. Пост-інсталяція

У веб-GUI: **pve-a → Updates → Repositories** → Disable для `pve-enterprise` (і `ceph`
enterprise, якщо є) → Add → `No-Subscription`. Потім **Updates → Refresh → Upgrade**.

Ноутбук працює як сервер із закритою кришкою — у консолі pve-a (Shell у GUI або SSH):

```bash
sed -i -e 's/^#\?HandleLidSwitch=.*/HandleLidSwitch=ignore/' \
       -e 's/^#\?HandleLidSwitchExternalPower=.*/HandleLidSwitchExternalPower=ignore/' \
       /etc/systemd/logind.conf
systemctl restart systemd-logind
```

### 1.1.1. Wake-on-LAN — віддалене ввімкнення pve-a

Щоб HQ-вузол можна було піднімати без фізичного дотику (ADR-001 — «завжди доступний
control plane»), вмикаємо WoL на дротовій карті. На pve-a (root), `nic0` — фізичний
порт моста vmbr0 (підстав свою назву з `ip -br link`):

```bash
ethtool nic0 | grep -i wake     # потрібна літера 'g' у "Supports Wake-on"
ethtool -s nic0 wol g           # увімкнути magic packet
```

Закріпити після перезавантажень — у `/etc/network/interfaces`, у блок `iface nic0 inet manual`:

```text
iface nic0 inet manual
    post-up /usr/sbin/ethtool -s nic0 wol g
```

У **BIOS/UEFI** увімкнути «Wake on LAN / Resume by LAN / Power On by PCI-E» (на цьому
залізі опція знайдена — WoL працює навіть із повного вимкнення S5). Запиши MAC:
`ip -br link show nic0`.

Будити з ПК (Windows) — скрипт `tools/wake-pve-a.ps1` (MAC у ньому) запускається через
`tools/wake-pve-a.bat`; або утиліта WakeMeOnLan (NirSoft): MAC + broadcast `192.168.1.255`.

Режими зупинки: `systemctl suspend` (S3, RAM живиться, миттєве пробудження, VM лишаються
в RAM) — основний для пауз; `poweroff` (S5, повне знеструмлення) — VM перезапускаються при
старті (`--onboot 1`), GitLab підіймається 10–15 хв. WoL піднімає з обох станів.

### 1.2. API-токен для OpenTofu (знадобиться у M2, створюємо зараз)

```bash
pveum user add terraform@pve --comment "OpenTofu provisioning"
pveum aclmod / -user terraform@pve -role PVEVMAdmin
pveum aclmod /storage -user terraform@pve -role PVEDatastoreUser
pveum user token add terraform@pve tofu --privsep 0
```

Виведений `value` токена показується **один раз** — збережи у менеджері паролів.
Згодом він стане protected CI variable, у Git не потрапляє ніколи.

**✅ Контрольна точка 1:** GUI доступний, репозиторії оновлені, кришка не присипляє,
токен збережено.

## 2. Шаблон Ubuntu 24.04 cloud-init

У консолі pve-a:

```bash
wget -O /root/noble-server-cloudimg-amd64.img \
  https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img

qm create 9000 --name ubuntu2404-tpl --ostype l26 \
  --cpu host --cores 1 --memory 1024 \
  --net0 virtio,bridge=vmbr0 \
  --scsihw virtio-scsi-single --agent enabled=1 \
  --serial0 socket --vga serial0

qm set 9000 --scsi0 local-lvm:0,import-from=/root/noble-server-cloudimg-amd64.img
qm set 9000 --ide2 local-lvm:cloudinit --boot order=scsi0
qm template 9000
```

Пояснення: cloud image — готовий диск Ubuntu без інсталятора; cloud-init при першому
старті задає IP, hostname, користувача та SSH-ключ; `--agent` вмикає QEMU guest agent
(Terraform через нього читатиме IP VM).

**✅ Контрольна точка 2:** `qm list` показує VM 9000 зі статусом `stopped` і позначкою
template у GUI.

## 3. VM ctrl-01 (2 vCPU / 8 ГБ / 40 ГБ)

Поклади публічний ключ зі свого ПК на pve-a (вміст `~/.ssh/id_ed25519.pub` → файл
`/root/ctrl-01.pub`), потім:

```bash
qm clone 9000 101 --name ctrl-01 --full
qm set 101 --cores 2 --memory 6144 --balloon 0 --onboot 1
qm disk resize 101 scsi0 40G
qm set 101 --ciuser ansible-svc --sshkeys /root/ctrl-01.pub \
  --ipconfig0 ip=192.168.1.50/24,gw=192.168.1.1 --nameserver 192.168.1.1
qm start 101
```

`--memory 6144` (6 ГБ): хост pve-a має 8 ГБ, тож VM лишаємо ~2 ГБ під сам Proxmox;
GitLab CE з тюнінгом (puma=2) у 6 ГБ для M0 (лише lint) працює. Коли докинеш RAM до
12-16 ГБ — підніми до 8192. `--balloon 0` вимикає динамічну пам'ять: GitLab погано
переживає її відбирання.

Через ~1 хв з ПК: `ssh ansible-svc@192.168.1.50`. Усі команди на ctrl-01 виконуються
від root — одразу переходимо в root-шелл (тому далі по тексту sudo вже немає):

```bash
sudo -i
apt-get update && apt-get -y upgrade
apt-get -y install qemu-guest-agent
systemctl enable --now qemu-guest-agent
```

### 3.1. Docker

```bash
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
  -o /etc/apt/keyrings/docker.asc
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] \
https://download.docker.com/linux/ubuntu noble stable" \
  > /etc/apt/sources.list.d/docker.list
apt-get update
apt-get -y install docker-ce docker-ce-cli containerd.io \
  docker-buildx-plugin docker-compose-plugin
usermod -aG docker ansible-svc
```

`usermod` додає `ansible-svc` у групу docker, щоб згодом запускати docker без root.
Перевіримо це окремою сесією: `exit` (вийти з root), `exit` (вийти з ssh), знову
`ssh ansible-svc@192.168.1.50` — нова сесія підхопить групу.

**✅ Контрольна точка 3:** від звичайного `ansible-svc` (без sudo, без root):
`docker run --rm hello-world` друкує «Hello from Docker!».

## 4. GitLab CE + Runner

З ПК скопіюй compose-стек на ctrl-01 (з кореня папки проєкту):

```bash
scp -r iac-infra/bootstrap/ctrl-01 ansible-svc@192.168.1.50:~/ctrl-01
```

На ctrl-01 (знову від root — `sudo -i`; scp вище поклав стек у домівку ansible-svc):

```bash
sudo -i
mkdir -p /srv/gitlab /srv/gitlab-runner
cd /home/ansible-svc/ctrl-01
cp .env.example .env        # значення за замовчуванням уже правильні
docker compose up -d
watch docker compose ps     # чекаємо health: starting → healthy (10-15 хв на 2 ядрах)
```

Початковий пароль root (діє 24 години):

```bash
cat /srv/gitlab/config/initial_root_password
```

### 4.1. Початкове налаштування GitLab (`http://192.168.1.50`)

1. Увійди як `root`, одразу зміни пароль: avatar → Edit profile → Password.
2. Вимкни самореєстрацію: **Admin → Settings → General → Sign-up restrictions** →
   зніми "Sign-up enabled" → Save.
3. Створи Personal Access Token для git: avatar → **Edit profile → Access tokens** →
   ім'я `git-pc`, scope `write_repository`, `api` → збережи токен.

**✅ Контрольна точка 4:** обидва контейнери `healthy`/`running`, вхід у GitLab працює.

## 5. Проєкт iac-infra та реєстрація runner

1. **New project → Create blank project**: назва `iac-infra`, namespace root,
   Private, БЕЗ "Initialize with README".
2. Runner: **Project → Settings → CI/CD → Runners → New project runner** →
   постав "Run untagged jobs" → Create → скопіюй токен `glrt-…`.
3. На ctrl-01:

```bash
docker exec gitlab-runner gitlab-runner register --non-interactive \
  --url "http://192.168.1.50" \
  --token "glrt-ВСТАВ_СВІЙ_ТОКЕН" \
  --executor docker \
  --docker-image "alpine:3.21" \
  --docker-pull-policy "if-not-present" \
  --description "ctrl-01-docker"

# одна задача одночасно — у нас 2 ядра (від root: sudo -i)
sed -i 's/^concurrent = .*/concurrent = 1/' /srv/gitlab-runner/config/config.toml
docker restart gitlab-runner
```

**✅ Контрольна точка 5:** runner зелений у Settings → CI/CD → Runners.

## 6. Пуш каркаса та зелений пайплайн

На ПК, у каталозі `iac-infra` папки проєкту:

```bash
git init -b main
git add .
git update-index --chmod=+x scripts/vault-pass.sh   # Windows не зберігає біт виконання
git commit -m "M0: каркас репозиторію, compose ctrl-01, lint-пайплайн"
git remote add origin http://192.168.1.50/root/iac-infra.git
git push -u origin main      # логін root, пароль = PAT з кроку 4.1
```

Перевір: **Settings → Repository → Branch rules** — `main` захищена (Maintainers only,
це дефолт). Пайплайн на main: **Build → Pipelines** — стадія lint, два джоби
(yamllint, ansible-lint). Перший запуск тягне образ python — кілька хвилин.

Перевірка MR-потоку («хибна» зміна → MR → lint):

```bash
git switch -c test/lint-pipeline
echo "" >> README.md && git commit -am "test: перевірка MR-пайплайна"
git push -u origin test/lint-pipeline
```

У GitLab створи Merge Request → пайплайн `merge_request_event` має пройти → Merge.

### 6.1. Секрети на майбутнє

- **Settings → CI/CD → Variables** → `ANSIBLE_VAULT_PASSWORD`
  (Masked, Protected) — значення згенеруй: `openssl rand -base64 24`. Те саме значення
  локально: `mkdir -p ~/.config/iac-infra && echo 'ЗНАЧЕННЯ' > ~/.config/iac-infra/vault-pass && chmod 600 ~/.config/iac-infra/vault-pass`.
- Перший секрет (наприклад, пароль Cisco) додається так:
  `cd ansible && ansible-vault create inventories/staging/group_vars/cisco_switches/vault.yml` —
  файл комітиться лише зашифрованим.
- Токен `terraform@pve` стане protected variable `PVE_API_TOKEN` у M2.

**✅ Контрольна точка 6 (DoD M0, мінімум):** lint-пайплайн зелений на main і в MR;
ctrl-01 стабільно працює; цей runbook відтворюваний.

## 7. Troubleshooting

| Симптом | Причина/дія |
|---|---|
| GitLab віддає 502 | Ще стартує — на 2 ядрах до 15 хв; `docker compose ps` → чекати `healthy` |
| Контейнер gitlab у restart loop | `docker logs gitlab --tail 50`; найчастіше — друкарська помилка в `GITLAB_OMNIBUS_CONFIG` після ручних правок |
| Повільний веб-інтерфейс | Норма для 2 ядер; перевір `free -h` на ctrl-01 — swap не має бути забитий |
| Job висить у pending | Runner offline або не "run untagged" → Settings → CI/CD → Runners |
| Job не тягне образ | На ctrl-01: `docker pull python:3.12-slim` вручну, далі спрацює `if-not-present` |
| Диск pve-a закінчується | `df -h` на ctrl-01 та pve-a; чистити: `docker system prune`, старі логи `/srv/gitlab/logs` |
| Забув пароль root GitLab | `docker exec -it gitlab gitlab-rake "gitlab:password:reset[root]"` |
