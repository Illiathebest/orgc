# ADR-002: Git + CI/CD платформа

- **Статус:** Accepted (rev. 2, 2026-06-08). Рішення: **GitLab CE**
- **Дата:** 2026-06-07; переглянуто 2026-06-08 після уточнення обмежень (32 ГБ RAM) і принципу «популярність за замовчуванням»
- **Контекст:** Потрібна self-hosted платформа Git + CI (runner у mgmt VLAN, дані в компанії). GitLab CE вимагає 4–8 ГБ RAM. На 16 ГБ це було блокером, і rev. 1 рекомендував Forgejo. На 32 ГБ обмеження зникає.

## Довідка по технологіях

- GitLab CE – це найпопулярніша self-hosted DevOps-платформа «все в одному»: Git, CI/CD, container registry, Terraform state backend, environments з manual gates. Індустріальний стандарт.
- Gitea і Forgejo – це легкі (~0.3–0.5 ГБ) опенсорсні Git-сервери. Forgejo є спільнотним форком Gitea, на ньому працює Codeberg. CI з'явився в них нещодавно (Actions-сумісний), і екосистема значно менша.
- Woodpecker CI – це мінімалістичний окремий CI-сервер, ніша self-hosted ентузіастів.
- GitHub найпопулярніший узагалі, але це SaaS. Код і пайплайни опиняються поза компанією, що суперечить вимогам продукту.

## Варіанти

| Критерій | A. GitLab CE | B. Forgejo + Forgejo Actions | C. Gitea + Woodpecker CI | D. GitHub + self-hosted runner |
|---|---|---|---|---|
| RAM (сервер + idle runner) | 4–8 ГБ | ~0.5–1 ГБ | ~1 ГБ (два сервіси) | 0 (SaaS) + runner |
| CI-функціональність | еталонна (environments, manual gates, registry) | GitHub Actions-сумісний синтаксис; gates простіші | повноцінна, окремий продукт | еталонна |
| Container registry | вбудований | вбудований | вбудований (Gitea) | ghcr.io |
| Terraform state backend | вбудований (HTTP) | немає → потрібен MinIO (S3) | немає → MinIO | немає → MinIO/TF Cloud |
| Self-hosted (вимога: runner у mgmt VLAN, дані в компанії) | так | так | так | частково (код у хмарі) |
| Ліцензія/відкритість | open-core | повністю FOSS (форк Gitea під Codeberg) | FOSS | пропрієтарна SaaS |
| Відповідність S-профілю продукту | ні (заважкий) | так | так | ні (дані поза компанією) |

## Рішення

**Варіант A: GitLab CE** (повертаємо рішення ARCHITECTURE.md v0.1).

Рішення ухвалено за [принципом відбору №1](README.md). Forgejo вигравав лише за RAM, а на 32 ГБ ця перевага не кратна. GitLab має вбудований Terraform state backend, тому MinIO не потрібен. Environments з manual gates теж є з коробки, без самописних обхідних рішень. До цього додаються registry і найбільша екосистема.

Нотатка для продукту. Модулі платформи прив'язуємо до інтерфейсів (Git remote, CI з docker-executor і секретами, OCI registry, state backend), щоб вони не залежали від API GitLab. У документації продукту Forgejo+MinIO лишається задокументованим *лайт-варіантом control plane* для клієнтів S-профілю з дуже слабким залізом.

## Наслідки

- (+) ARCHITECTURE.md v0.1 лишається чинним без переробки (пайплайни на GitLab CI).
- (+) Вбудований TF state backend з locking, тому сервісом менше (MinIO не потрібен).
- (+) Manual gates, protected variables, environments є з коробки.
- (−) ctrl-01 потребує ~5–6 ГБ RAM. Це враховано в бюджеті M0 (32 ГБ).
- (−) Forgejo-варіант на практиці не реалізуємо, лише описуємо (скорочення обсягу робіт).
