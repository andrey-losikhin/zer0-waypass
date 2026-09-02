# ADR 0002: CLI-контракт backend `gopass`

## Статус

Принято 2026-08-24 для `gopass 1.16.1`. Контракт должен повторно
проверяться при изменении поддерживаемой версии `gopass`.

## Решение

Helper запускает `gopass` напрямую через fixed argv, без shell.
`/usr/bin/gopass` ниже — подтверждённый путь Arch Linux spike. Способ
production resolution и test injection будет спроектирован отдельно, но также
без shell.

Список opaque entry path для metadata-поиска получается только командой:

```text
/usr/bin/gopass ls --flat
```

Полный production copy flow сначала декодирует и валидирует ID, затем выполняет
fresh membership check выбранного entry через `gopass ls --flat` и только после
успешной проверки расшифровывает эту запись. Для password helper использует:

```text
/usr/bin/gopass show --password -- <entry>
```

Для поля `username` helper использует:

```text
/usr/bin/gopass show -- <entry> username
```

`--` обязателен перед entry как defense-in-depth после отдельной валидации ID.
Production validation до вызова backend отклоняет entry path, начинающийся с
`-`; option-like запись ниже использовалась только как test-only compatibility
probe безопасной CLI-формы. Команда username требует field после entry;
обратный порядок означает поиск записи `username` и не подходит.

### ID и fresh membership invariant

Metadata ID — это unpadded URL-safe base64 от байтов
`0x01 || UTF-8 canonical relative entry path`. Байт `0x01` версионирует формат
ID отдельно от версии JSON envelope `protocol=1`. Видимый ID целиком ограничен
алфавитом `A-Z`, `a-z`, `0-9`, `_`, `-`.

ID является только обратимым opaque transport для UI, а не средством secrecy,
integrity, authentication или authorization. Encoder применяет к каждому path
из list pipeline ту же strict canonical relative path validation, что и decoder;
decoder строго валидирует untrusted ID. Реализация этих primitives сама по себе
не добавляет copy или membership execution.

Перед каждым будущим copy helper обязан декодировать и валидировать ID, заново
выполнить `gopass ls --flat` и потребовать точного membership decoded canonical
path в свежем результате. Только после этого разрешён один single-entry decrypt.
Cached UI list и сам ID недостаточны; отсутствие exact match закрывает операцию
без decrypt.

Production-код считает только exit code `0` успехом. Любой ненулевой exit code
является ошибкой; конкретное значение и текст stderr не используются как
стабильный API. Secret принимается только из stdout успешного процесса и не
возвращается в UI. Raw stderr не должен включаться в пользовательские сообщения
или логи.

## Проверенные факты

Проверка выполнена на isolated synthetic fixture с `gopass 1.16.1` и GPG 2.4.9.
Ни пользовательский store, ни пользовательский `GNUPGHOME` не использовались.

- `gopass ls --flat` завершился с code `0`, вернул только entry path с `LF` и
  не вызвал GPG decrypt backend.
- Password-команда завершилась с code `0`; stdout содержал ровно первую строку
  записи без завершающего `LF`, stderr был пуст.
- Username-команда завершилась с code `0`; stdout содержал ровно значение поля
  без префикса `username:` и без завершающего `LF`, stderr был пуст.
- Полученные password и username совпали соответственно с первой строкой и
  значением поля исходной двухстрочной synthetic записи.
- Отсутствующее поле и отсутствующая запись завершились с code `11`, пустым
  stdout и диагностикой в stderr. Ни password, ни username в этих stderr не
  обнаружены.
- Test-only option-like запись успешно получена в форме с `--`. Это проверка
  совместимости CLI, а не разрешение такого entry path в production.
- `/proc/<pid>/cmdline` во время decrypt подтвердил argv password-команды:
  `/usr/bin/gopass show --password -- synthetic/alice`; secret в argv
  отсутствовал.
- В isolated extraction probe test-only GPG proxy зарегистрировал один вызов
  `gpg --decrypt` для одного password action. В argv extraction-команды был
  ровно один выбранный entry; сам probe не запускал `ls` или search. Это не
  отменяет обязательный отдельный fresh membership check полного production
  copy flow перед decrypt.

Размеры stdout зависят от synthetic значений и не являются частью production
контракта. В spike они фиксировались только для доказательства отсутствия
лишнего текста.

## Предположения и ограничения

- Числовой exit code `11` и точные формулировки ошибок считаются наблюдением
  версии 1.16.1, а не переносимым контрактом следующих версий.
- Формат полей и позиция field являются CLI-version contract. Обновление
  `gopass` требует повторить success/error matrix, option-like case, leak scan и
  `/proc` argv inspection до изменения минимальной поддерживаемой версии.
- Имена entry являются разрешённой metadata-утечкой MVP; значения полей и
  password — нет.
- Проверка этого ADR не подтверждает clipboard lifecycle, deadlines, signals
  или parent-death process model; они проверяются отдельными шагами Milestone 1.

## Отклонённые варианты

- `gopass show -- <entry>`: выводит всю запись и потому не подходит для
  password/username actions.
- `gopass show username -- <entry>`: интерпретирует `username` как имя записи.
- Decrypt без fresh membership check отклонён: полный production copy flow
  обязан подтвердить выбранный entry отдельным `gopass ls --flat`. При этом
  extraction остаётся fixed single-entry `show` argv, а не list/search-командой.
- Shell, command substitution и передача secret через argv, environment или
  plaintext intermediate file запрещены.
