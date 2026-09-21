# ADR-002: Git + CI/CD платформа

- **Статус:** Accepted (rev. 2, 2026-06-08) — **GitLab CE**
- **Дата:** 2026-06-07; переглянуто 2026-06-08 після уточнення обмежень (32 ГБ RAM) і принципу «популярність за замовчуванням»
- **Контекст:** Потрібна self-hosted платформа Git + CI (runner у mgmt VLAN, дані в компанії). GitLab CE вимагає 4–8 ГБ RAM — на 16 ГБ це було блокером і rev. 1 рекомендував Forgejo; на 32 ГБ обмеження зникає.

## Довідка по технологіях

- **GitLab CE** — найпопулярніша self-hosted DevOps-платформа «все в одному» (Git, CI/CD, container registry, Terraform state backend, environments з manual gates). Індустріальний стандарт, де-факто синонім GitOps у enterprise.
- **Gitea / Forgejo** — легкі (~0.3–0.5 ГБ) опенсорсні Git-сервери; Forgejo — спільнотний форк Gitea (на ньому працює Codeberg). CI з'явився нещодавно (Actions-сумісний), екосистема значно менша.
- **Woodpecker CI** — мінімалістичний окремий CI-сервер, ніша self-hosted ентузіастів.
- **GitHub** — найпопулярніший узагалі, але SaaS: код і пайплайни поза компанією, що суперечить вимогам продукту.

## Варіанти

| Критерій | A. GitLab CE | **B. Forgejo + Forgejo Actions** | C. Gitea + Woodpecker CI | D. GitHub + self-hosted runner |
|---|---|---|---|---|
| RAM (сервер + idle runner) | 4–8 ГБ | **~0.5–1 ГБ** | ~1 ГБ (два сервіси) | 0 (SaaS) + runner |
| CI-функціональність | еталонна (environments, manual gates, registry) | GitHub Actions-сумісний синтаксис; gates простіші | повноцінна, окремий продукт | еталонна |
| Container registry | вбудований | вбудований | вбудований (Gitea) | ghcr.io |
| Terraform state backend | вбудований (HTTP) | **немає → потрібен MinIO (S3)** | немає → MinIO | немає → MinIO/TF Cloud |
| Self-hosted (вимога: runner у mgmt VLAN, дані в компанії) | так | так | так | частково (код у хмарі) |
| Ліцензія/відкритість | open-core | повністю FOSS (форк Gitea під Codeberg) | FOSS | пропрієтарна SaaS |
| Відповідність S-профілю продукту | ні (заважкий) | так | так | ні (дані поза компанією) |

## Рішення

**Варіант A: GitLab CE** (повертаємо рішення ARCHITECTURE.md v0.1).

Обґрунтування за принципом відбору №1: Forgejo вигравав *лише* за RAM — на 32 ГБ ця перевага не кратна, а GitLab дає вбудовані Terraform state backend (не потрібен MinIO), environments з manual gates (не потрібні самописні обхідні рішення), registry і найбільшу екосистему. Для магістерської і кар'єрно GitLab-компетенція цінніша.

Продуктова нотатка: модулі платформи прив'язуємо до інтерфейсів (Git remote, CI з docker-executor і секретами, OCI registry, state backend), а не до GitLab API — у документації продукту Forgejo+MinIO лишається задокументованим *лайт-варіантом control plane* для клієнтів S-профілю з дуже слабким залізом. Це демонструє переносимість архітектури.

## Наслідки

- (+) ARCHITECTURE.md v0.1 лишається чинним без переробки (пайплайни — GitLab CI).
- (+) Вбудований TF state backend з locking — мінус один сервіс (MinIO не потрібен).
- (+) Manual gates, protected variables, environments — з коробки.
- (−) ctrl-01 потребує ~5–6 ГБ RAM — враховано в бюджеті M0 (32 ГБ).
- (−) Forgejo-варіант не реалізуємо практично, лише описуємо (скорочення обсягу робіт).
