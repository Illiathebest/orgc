# iac-infra

Система автоматизованого управління ІТ-інфраструктурою на принципах Infrastructure as Code.
Магістерський проєкт «Infrastructure-in-a-Box». Технічне ядро та обґрунтування рішень — у
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) головної документації проєкту та ADR-001…004.

## Стан: M0 — лабораторний стенд

Реалізовано: каркас репозиторію, compose-стек ctrl-01 (GitLab CE 18.11 + Runner,
тюнінг під 2 vCPU / 8 GB), лінтинг-пайплайн (yamllint + ansible-lint, профіль production),
каркас Ansible-інвентаря staging. Розгортання стенда — за runbook
`docs/runbooks/00-bootstrap-lab.md` (у документації проєкту).

## Структура

```text
iac-infra/
├── .gitlab-ci.yml          # точка входу CI; include: ci/*.yml
├── .yamllint                # стиль YAML
├── .ansible-lint            # профіль production
├── ci/
│   └── lint.gitlab-ci.yml   # стадія lint (M1 додасть validate/plan/apply)
├── bootstrap/
│   └── ctrl-01/             # docker-compose GitLab CE + Runner (.env з .env.example)
├── ansible/
│   ├── ansible.cfg          # запускати ansible-* з каталогу ansible/
│   ├── requirements.yml     # колекції, версії запінено
│   ├── inventories/staging/ # hosts.yml + group_vars (секрети — лише vault.yml)
│   └── playbooks/           # site.yml → linux.yml (мережеві — у M3-M4)
├── terraform/               # Day-0 provisioning — з'являється у M2
└── scripts/
    └── vault-pass.sh        # пароль vault: CI variable або ~/.config/iac-infra/vault-pass
```

## Швидкий старт (локально)

```bash
# Лінтери — ті самі, що в CI
pip install yamllint==1.37.1 ansible-lint==25.5.0
yamllint --strict .
ansible-lint --strict

# Ansible (з каталогу ansible/)
cd ansible
ansible-galaxy collection install -r requirements.yml
ansible-playbook playbooks/site.yml --check --diff
```

## Правила

- Зміни — лише через Merge Request у `main` (захищена гілка); кожен MR проходить lint → dry-run.
- Секрети — тільки `ansible-vault` (`group_vars/*/vault.yml`) або protected CI variables.
- Нетривіальні технологічні рішення — ADR у документації проєкту.
