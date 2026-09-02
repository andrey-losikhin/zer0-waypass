# Waypass Field Contract v1

## Назначение

TUI создаёт логическую запись-bundle. Зашифрованный manifest хранит только
схему, а каждое значение — отдельный зашифрованный `gopass` entry. Поэтому
helper может показать только объявленные поля и передать secret value напрямую
в `wl-copy`, не разбирая общую запись.

## Пути

- пользовательский entry path, например `work/database`, остаётся legacy/
  compatibility entry и не читается целиком для карточки;
- manifest: `.zer0-waypass/v1/manifests/<entry-id>`, где entry ID — действующий
  canonical opaque ID пользовательского пути;
- value: `.zer0-waypass/v1/<bundle-id>/<revision>/<field-id>`.

`.zer0-waypass/` — зарезервированный namespace: helper исключает его из `list`.
Все три ID — canonical unpadded base64url от 16 random bytes (22 символа).

## Manifest

Manifest получается fixed argv `gopass show --noparsing -- <manifest-path>` и содержит
только bounded JSON, без field values:

```json
{
  "format": "zer0-waypass/fields-v1",
  "bundle_id": "AAAAAAAAAAAAAAAAAAAAAA",
  "revision": "BBBBBBBBBBBBBBBBBBBBBB",
  "fields": [
    {"id":"CCCCCCCCCCCCCCCCCCCCCC","kind":"password","name":"Password","visibility":"secret"},
    {"id":"DDDDDDDDDDDDDDDDDDDDDD","kind":"notes","name":"Notes","visibility":"public"}
  ]
}
```

Порядок массива — порядок карточки. Manifest не более 64 KiB, содержит 1–64
поля и является closed schema: unknown/duplicate JSON keys, duplicate IDs/names,
unknown kinds, invalid UTF-8/control/bidi characters и non-canonical IDs
fail-closed отклоняют bundle.

## Поля и видимость

| Kind | Default name | Visibility | Multiline |
| --- | --- | --- | ---: |
| `password` | Password | secret | нет |
| `username` | Username | public | нет |
| `url` | URL | public | нет |
| `email` | Email | public | нет |
| `notes` | Notes | public | да |
| `host` | Host | public | нет |
| `port` | Port | public | нет |
| `database` | Database | public | нет |
| `engine` | Engine | public | нет |
| `dsn` | DSN | public | нет |
| `client_id` | Client ID | public | нет |
| `jump_host` | Jump host | public | нет |
| `api_key` | API key | secret | нет |
| `token` | Token | secret | нет |
| `client_secret` | Client secret | secret | нет |
| `private_key` | Private key | secret | да |
| `passphrase` | Passphrase | secret | нет |
| `sudo_password` | Sudo password | secret | нет |
| `totp_secret` | TOTP secret | secret | нет |
| `recovery_codes` | Recovery codes | secret | да |
| `custom` | задаёт TUI | задаёт TUI | задаёт TUI |

Standard kind обязан использовать visibility и multiline policy из таблицы.
`custom` явно задаёт `visibility` и `multiline`. Standard kind уникален; custom
fields различаются opaque ID и уникальным display name. Пустое значение не
создаётся: отсутствующий descriptor означает «не показывать поле».

`public` означает осознанное раскрытие значения helper-у и Noctalia: оно
возвращается в fields JSON, показывается на экране и очищается из in-memory
card state при возврате к списку, закрытии панели или plugin reload. Пока
карточка открыта, значение остаётся на экране. Оно не пишется в
settings/cache/logs.
`secret` означает: JSON содержит только ID/name/kind, UI не получает value.

Все значения остаются чувствительными для логирования и persistence, включая
public notes/host/database. UI не предоставляет переключатель показа secret.

## Значения

Value entry содержит только значение одного поля. TUI при записи обеспечивает:
valid UTF-8, 1 byte–1 MiB,
LF line endings, без NUL/control (кроме LF/TAB для multiline). Single-line поля
запрещают CR/LF. Binary values не поддерживаются.

Для `public` helper читает только отдельный value entry после fresh membership,
останавливает чтение на 256 KiB, проверяет text policy и включает value в
ephemeral fields JSON; aggregate public payload ограничен 1 MiB. Для
`secret` helper никогда не читает value в Go; copy соединяет fixed-argv
`gopass show --noparsing -- <value-path>` stdout напрямую с `wl-copy` stdin.
Из-за прямого pipe helper не проверяет bytes secret value: их text/size policy
является обязательным write-time invariant TUI. Точный byte roundtrip
`show --noparsing` остаётся mandatory synthetic gate.

## Protocol v2

```text
zer0-waypass-helper fields <entry-id>
zer0-waypass-helper copy field <entry-id> <revision> <field-id> --ttl <seconds>
```

`fields` возвращает `{protocol:2, revision, fields:[...]}`. Wire `revision` —
canonical base64url SHA-256 exact manifest bytes, а не только mutable generation
ID из manifest. Каждый item содержит
только `id`, `name`, `kind`, `visibility`, `multiline`; `value` присутствует
только для `public`.

Copy повторно выполняет fresh membership manifest, заново валидирует manifest,
требует exact revision/field ID, выводит value path только из проверенных IDs,
повторно проверяет membership value и лишь затем запускает guarded pipeline.
Stale revision, missing field/value и manifest mismatch fail-closed.

## Запись TUI

Обновление copy-on-write:

1. новый random revision и новые field IDs;
2. записать/проверить все value entries;
3. заменить manifest последним;
4. старые encrypted values удалить только после recovery window.

Авария до шага 3 оставляет старый bundle рабочим. Manifest со ссылкой на
missing value отклоняется целиком. In-place изменение с прежним revision
запрещено. Legacy entry без `format` не является field bundle; миграция только
явным действием TUI.

## Не входит

Secret preview, TOTP generation, URL launch, autofill/autosubmit, browser
integration, plaintext index и автоматическая миграция/удаление данных.
