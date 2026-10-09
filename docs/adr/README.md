# Architecture Decision Records

Кожне нетривіальне технологічне рішення фіксується як ADR: контекст → варіанти з порівнянням → рішення → наслідки. Статуси: `Proposed` → `Accepted` / `Rejected` / `Superseded by ADR-XXX`.

## Принципи відбору технологій (затверджено)

1. Популярність за замовчуванням. Обираємо індустріальний мейнстрім (більша спільнота, документація, кількість фахівців), окрім випадків, коли альтернатива дає *кратну* перевагу під наші обмеження — тоді це явно доводиться в ADR.
2. Обмеження стенда: одна машина, 32 ГБ RAM. Рішення мусить вміщатися в бюджет M0.
3. Кожен ADR пояснює технології так, щоб вибір був зрозумілий без попереднього знайомства з ними.

| № | Рішення | Статус |
|---|---|---|
| [ADR-001](ADR-001-lab-virtualization.md) | Платформа віртуалізації лабораторного стенда | Proposed |
| [ADR-002](ADR-002-git-ci-platform.md) | Git + CI/CD платформа | Accepted — GitLab CE |
| [ADR-003](ADR-003-network-emulation.md) | Емуляція мережевого обладнання | Proposed |
| [ADR-004](ADR-004-observability-siem.md) | Стек моніторингу та SIEM | Proposed |
| [ADR-005](ADR-005-data-model-validation.md) | Спосіб валідації моделі даних `organization.yml` | Accepted — JSON Schema 2020-12 |
| [ADR-006](ADR-006-go-dependencies-schema-embedding.md) | Залежності компілятора `orgc` і розміщення схеми для `go:embed` | Accepted — `goccy/go-yaml` + `santhosh-tekuri/jsonschema/v6`, схема в `orgc/schema/` |
| [ADR-007](ADR-007-module-path-and-schema-id.md) | Шлях модуля Go та ідентифікатор схеми | Accepted — модуль `orgc`, `$id` `urn:orgc:organization-schema` |
| [ADR-008](ADR-008-ir-and-verify-contract.md) | Проміжне представлення і контракт перевірок `verify` | Accepted — IR з індексом позицій, інваріант — функція над IR, лише W → 0, можливості через `backend.Platform`, E006 на графі мереж і шлюзів |
