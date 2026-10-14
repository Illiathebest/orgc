# Тестові дані `orgc`

| Каталог | Що лежить |
|---|---|
| `valid/` | Моделі, які мають проходити `verify` без помилок. |
| `broken/` | Моделі з дефектами: по одному порушенню на файл плюс зведений файл. |
| `golden/` | Еталонні виходи бекендів для побайтового порівняння. |

У проєкті кожен інваріант має позитивний і негативний тест. Негативний тест
перевіряє, що спрацював очікуваний код помилки. Будь-яка інша помилка його не
задовольняє. Тому в кожному файлі одне порушення. Якщо порушень кілька, тест не
доводить, який інваріант спрацював.

Назва файла в `broken/` починається з очікуваного коду:

- `sNNN-*.yml` позначає структурний дефект. Модель не проходить схему, і
  інваріанти не запускаються;
- `eNNN-*.yml`, `wNNN-*.yml` позначають порушення інваріанта. Така модель має
  проходити схему. Інакше конвеєр зупиниться на структурному шарі, і
  негативний тест нічого не доведе. Цю умову перевіряє
  `TestSemanticFixturesAreStructurallyClean` в `internal/model`. Із
  зареєстрованими перевірками модель дає рівно одну діагностику свого
  коду в очікуваному рядку. Таблиця очікувань лежить в `internal/verify`.

Зараз у `valid/` лежать три моделі (`minimal`, `s-profile`, `m-profile`). На
них проганяються тести `internal/model`, `internal/lower`, `internal/verify` і
`cmd/orgc`. Кожна з цих моделей має проходити всі інваріанти.

У `broken/` лежать структурні дефекти `sNNN` і порушення кожного інваріанта
E001–E010, W001–W006, по одному дефекту на файл. У `golden/` лежать еталони
виходу `orgc render`, по підкаталогу на модель (див. `golden/README.md`).

CI-стадія `validate` проганяє `orgc verify` на прикладах `examples/` і моделях
експериментів `experiments/e4/models/`. Корпус `testdata` перевіряє `go test`.

## Інваріант → тест

Кожна модель з `broken/` разом з усіма перевірками дає рівно одну
діагностику свого коду в указаному рядку (`TestInvariantFixturesMatchTable`).
Файл `invariants-aggregate.yml` – це копія `examples/organization.invariants-broken.yml`.
Він проходить схему й дає шістнадцять діагностик, по одній на код
(`TestInvariantsAggregate`). Позитивний тест для всіх кодів разом виконується на
`valid/`. Там не має бути жодної діагностики (`TestValidCorpusIsClean`).

| Код | Негативні приклади в `broken/` (рядок діагностики) | Зведений приклад, рядок | Позитивні й межові тести (`internal/verify`) |
|---|---|---|---|
| E001 | `e001-overlap-between-sites.yml`:25<br>`e001-overlap-in-site.yml`:17<br>`e001-rule-across-overlap.yml`:31 | 37 | `TestE001AllowOverlapBetweenSites`, `TestAddressPlanBoundaries` |
| E002 | `e002-duplicate-network-name.yml`:17<br>`e002-duplicate-site-name.yml`:23<br>`e002-duplicate-uplink.yml`:16<br>`e002-duplicate-vlan.yml`:16<br>`e002-host-name-clash.yml`:24<br>`e002-port-used-twice.yml`:67 | 39 | `TestE002ScopesAllowReuse`, `TestE002ModelWideScopes`, `TestConnectionChecks` |
| E003 | `e003-access-network-unknown.yml`:64<br>`e003-control-network-other-site.yml`:9<br>`e003-link-unknown-device.yml`:74<br>`e003-link-unknown-vlan.yml`:73<br>`e003-no-l3-gateway.yml`:15<br>`e003-two-l3-gateways.yml`:26<br>`e003-unknown-control-network.yml`:9<br>`e003-unknown-control-site.yml`:8<br>`e003-unknown-rule-segment.yml`:26<br>`e003-uplink-port-not-gateway.yml`:16 | 148 | `TestE003ResolvedReferencesPass`, `TestE003ReportsBothRuleEnds`, `TestConnectionChecks` |
| E004 | `e004-mgmt-ip-in-servers-network.yml`:22<br>`e004-mgmt-of-other-site.yml`:27<br>`e004-no-mgmt-network.yml`:22 | 60 | `TestE004` |
| E005 | `e005-any-to-any.yml`:27<br>`e005-from-internet.yml`:27 | 155 | `TestE005AllowsWhatIsNotDefaultDenyBreaking`, `TestOneCodePerRule` |
| E006 | `e006-mgmt-not-routed.yml`:18<br>`e006-mgmt-pruned.yml`:94<br>`e006-no-rule.yml`:18<br>`e006-ssh-only.yml`:26 | 145 | `TestE006`, `TestE006SkipsOtherCodes`, `TestE006L2`, `TestReachable` |
| E007 | `e007-control-node-outside-network.yml`:10<br>`e007-gateway-in-pool.yml`:16<br>`e007-gateway-outside-cidr.yml`:16<br>`e007-pool-inverted.yml`:17<br>`e007-pool-on-broadcast.yml`:17<br>`e007-pool-outside-cidr.yml`:17<br>`e007-server-outside-site.yml`:24<br>`e007-static-collision.yml`:24<br>`e007-static-in-pool.yml`:10<br>`e007-static-network-address.yml`:24 | 48 | `TestE007Exceptions`, `TestE007SkipsOtherCodes`, `TestAddressPlanBoundaries` |
| E008 | `e008-prefix-too-small.yml`:17<br>`e008-slash31-with-gateway.yml`:17 | 44 | `TestAddressPlanBoundaries`, `TestE008CountsDistinctAddresses` |
| E009 | `e009-device-not-connected.yml`:57<br>`e009-vlan-not-delivered.yml`:64 | 100 | `TestConnectionChecks`, `TestConnectionCascades` |
| E010 | `e010-loop.yml`:75 | 138 | `TestConnectionChecks`, `TestConnectionCascades`, `TestStandbyLinks` |
| W001 | `w001-guest-not-isolated.yml`:17<br>`w001-guest-to-servers.yml`:28 | 32 | `TestW001`, `TestOneCodePerRule` |
| W002 | `w002-capability-missing.yml`:19<br>`w002-port-name.yml`:66<br>`w002-version-out-of-range.yml`:20 | 64 | `TestW002Required`, `TestW002`, `TestConnectionChecks` |
| W003 | `w003-no-backup-uplink.yml`:13 | 16 | `TestW003` |
| W004 | `w004-mgmt-open-to-users.yml`:28 | 150 | `TestW004`, `TestOneCodePerRule` |
| W005 | `w005-access-feeds-access.yml`:74 | 108 | `TestConnectionChecks` |
| W006 | `w006-standby-no-mgmt.yml`:101 | 110 | `TestStandbyLinks`, `TestW006NeedsStandbyLinks` |
