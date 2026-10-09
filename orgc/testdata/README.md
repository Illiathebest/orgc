# Тестові дані `orgc`

| Каталог | Що лежить |
|---|---|
| `valid/` | Моделі, які мають проходити `verify` без помилок. |
| `broken/` | Моделі з дефектами: по одному порушенню на файл плюс зведений файл. |
| `golden/` | Еталонні виходи бекендів для побайтового порівняння. |

Правило проєкту: кожен інваріант має позитивний і негативний тест, і
негативний перевіряє, що ловиться саме цей код, а не будь-яка помилка.
Звідси «одне порушення на файл»: інакше тест не доводить, який саме інваріант
спрацював.

Назва файла в `broken/` починається з очікуваного коду:

- `sNNN-*.yml` — структурний дефект: модель не проходить схему, інваріанти не
  запускаються;
- `eNNN-*.yml`, `wNNN-*.yml` — порушення інваріанта: модель мусить
  проходити схему, інакше конвеєр зупиниться на структурному шарі й
  негативний тест нічого не доведе. Це стереже
  `TestSemanticFixturesAreStructurallyClean` в `internal/model`. Із
  зареєстрованими перевірками така модель дає рівно одну діагностику свого
  коду в очікуваному рядку; таблиця очікувань — в `internal/verify`.

Зараз у `valid/` лежать три моделі (`minimal`, `s-profile`, `m-profile`), на
яких проганяються тести `internal/model`, `internal/lower`, `internal/verify` і
`cmd/orgc`; кожна з них має проходити всі інваріанти. У `broken/` —
структурні дефекти `sNNN` і порушення кожного інваріанта E001–E008, W001–W004, по одному дефекту на файл. Каталогу `golden/` ще немає: він з'явиться разом із
бекендами `render`. Входом CI-стадії `validate` поки лишається `examples/`.

## Інваріант → тест

Кожна модель з `broken/` разом з усіма перевірками дає рівно одну
діагностику свого коду в указаному рядку (`TestInvariantFixturesMatchTable`).
`invariants-aggregate.yml` — копія `examples/organization.invariants-broken.yml`:
проходить схему й дає дванадцять діагностик, по одній на код
(`TestInvariantsAggregate`). Позитивний бік для всіх кодів разом — `valid/`:
жодної діагностики (`TestValidCorpusIsClean`).

| Код | Негативні приклади в `broken/` (рядок діагностики) | Зведений приклад, рядок | Позитивні й межові тести (`internal/verify`) |
|---|---|---|---|
| E001 | `e001-overlap-between-sites.yml`:25<br>`e001-overlap-in-site.yml`:17<br>`e001-rule-across-overlap.yml`:31 | 37 | `TestE001AllowOverlapBetweenSites`, `TestAddressPlanBoundaries` |
| E002 | `e002-duplicate-network-name.yml`:17<br>`e002-duplicate-site-name.yml`:23<br>`e002-duplicate-uplink.yml`:16<br>`e002-duplicate-vlan.yml`:16<br>`e002-host-name-clash.yml`:24 | 39 | `TestE002ScopesAllowReuse`, `TestE002ModelWideScopes` |
| E003 | `e003-control-network-other-site.yml`:9<br>`e003-no-l3-gateway.yml`:15<br>`e003-two-l3-gateways.yml`:26<br>`e003-unknown-control-network.yml`:9<br>`e003-unknown-control-site.yml`:8<br>`e003-unknown-rule-segment.yml`:26 | 103 | `TestE003ResolvedReferencesPass`, `TestE003ReportsBothRuleEnds` |
| E004 | `e004-mgmt-ip-in-servers-network.yml`:22<br>`e004-mgmt-of-other-site.yml`:27<br>`e004-no-mgmt-network.yml`:22 | 60 | `TestE004` |
| E005 | `e005-any-to-any.yml`:27<br>`e005-from-internet.yml`:27 | 110 | `TestE005AllowsWhatIsNotDefaultDenyBreaking`, `TestOneCodePerRule` |
| E006 | `e006-mgmt-not-routed.yml`:18<br>`e006-no-rule.yml`:18<br>`e006-ssh-only.yml`:26 | 100 | `TestE006`, `TestE006SkipsOtherCodes`, `TestReachable` |
| E007 | `e007-control-node-outside-network.yml`:10<br>`e007-gateway-in-pool.yml`:16<br>`e007-gateway-outside-cidr.yml`:16<br>`e007-pool-inverted.yml`:17<br>`e007-pool-on-broadcast.yml`:17<br>`e007-pool-outside-cidr.yml`:17<br>`e007-server-outside-site.yml`:24<br>`e007-static-collision.yml`:24<br>`e007-static-in-pool.yml`:10<br>`e007-static-network-address.yml`:24 | 48 | `TestE007Exceptions`, `TestE007SkipsOtherCodes`, `TestAddressPlanBoundaries` |
| E008 | `e008-prefix-too-small.yml`:17<br>`e008-slash31-with-gateway.yml`:17 | 44 | `TestAddressPlanBoundaries`, `TestE008CountsDistinctAddresses` |
| W001 | `w001-guest-not-isolated.yml`:17<br>`w001-guest-to-servers.yml`:28 | 32 | `TestW001`, `TestOneCodePerRule` |
| W002 | `w002-capability-missing.yml`:19<br>`w002-version-out-of-range.yml`:20 | 64 | `TestW002Required`, `TestW002` |
| W003 | `w003-no-backup-uplink.yml`:13 | 16 | `TestW003` |
| W004 | `w004-mgmt-open-to-users.yml`:28 | 105 | `TestW004`, `TestOneCodePerRule` |
