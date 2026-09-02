# ADR 0004: контракт карточки полей `gopass`

## Статус

Принят Waypass Field Contract v1 2026-08-31: encrypted manifest без values и
отдельный encrypted value entry на поле. Нормативный формат находится в
`docs/FIELD-CONTRACT.md`.

## Контекст

Карточка должна вернуть в Noctalia только упорядоченные имена и opaque ID полей,
а выбранное значение направить напрямую `gopass stdout -> wl-copy stdin`.
Расшифрованная запись, значения полей и произвольный body не могут проходить
через Go-парсер, JSON или Luau.

## Проверенные факты `gopass 1.16.1`

Проверка выполнена на isolated synthetic store в `/tmp`; пользовательский store
и пользовательский `GNUPGHOME` не использовались.

- В проверенном `show --help` нет флага или подкоманды, возвращающей только
  имена полей. `ls --flat` возвращает только пути записей. Дополнительный
  bounded survey `find`, `grep`, `cat` и `process` по их help также не нашёл
  list-only field API: `grep` явно расшифровывает все записи, `cat` печатает
  содержимое, `find` может показать exact match, а `process` обрабатывает
  внешний template-файл.
- `gopass show --password -- <entry>` возвращает первую строку без завершающего
  LF. Это пригодно для существующего password pipeline.
- `gopass show -- <entry> <field>` возвращает значение запрошенного поля без
  имени. Отсутствующее и пустое поле завершаются non-zero с пустым stdout.
- Повторяющееся имя поля возвращает несколько значений, разделённых LF. Поэтому
  raw field name не адресует один однозначный экземпляр.
- Indented continuation после `notes:` не входит в результат field query.
  YAML-подобное `private_key: |` возвращает только литерал `|`; следующие строки
  остаются body. Произвольный body без `name: value` не имеет field ID.
- Среди проверенных read-команд `gopass show -- <entry>` и `--noparsing`
  являются единственным наблюдавшимся способом узнать фактически присутствующие
  имена, но одновременно раскрывают все значения вызывающему процессу.
- `gopass templates` управляет отдельными `.pass-template`; CLI не предоставляет
  доказуемое соответствие каждой существующей записи конкретной template и не
  превращает произвольные legacy/custom поля в безопасную live-схему.

Размеры и конкретные synthetic значения не являются частью контракта. Exact
exit codes и diagnostics также не считаются стабильным API.

## Решение

Не реализовывать `fields <entry-id>` через `gopass show` с последующим parsing:
это потребовало бы прочитать и буферизовать всю расшифрованную запись в helper и
нарушило бы secret pipeline. Не выводить предполагаемый фиксированный список:
он позволил бы probing неизвестных полей и не описывал бы duplicates/body.

Protocol v1 остаётся без изменений. Команды `fields` и `copy field` публично не
добавляются; текущие password/username actions сохраняются.

## Проверяемый путь разблокировки

Возможен отдельный encrypted schema sidecar на запись, например зашифрованный
GPG-файл рядом с entry. После fresh entry membership helper расшифровывает
только schema, строго валидирует bounded ordered descriptors и возвращает
Noctalia opaque field ID плюс безопасное display name и неперсистентный opaque
schema revision/content digest. Значение затем извлекается fixed argv по
descriptor напрямую в `wl-copy`, но только после повторного fresh membership,
повторного decrypt+validation schema и exact проверки revision и отображения
field ID в тот же descriptor. Missing/stale/mismatched schema fail-closed, чтобы
reorder или edit после открытия карточки не мог выбрать другой secret.

До реализации нужно отдельным ADR и spike доказать:

1. атомарную связь schema с entry и fail-closed поведение при stale/missing
   schema;
2. schema-bound field ID, revision/content digest, повторную проверку schema при
   copy и строгую валидацию display name, порядка и duplicates;
3. однозначную политику multiline/body (поддержка отдельным extractor либо
   явный отказ);
4. отсутствие plaintext schema index, persistence в Noctalia и secret в
   argv/env/logs/temp files;
5. создание и сопровождение schema внешним TUI без изменений Noctalia.

Encrypted schema раскрывает имена полей helper-у при открытии карточки, но не
значения записи. Это отдельная осознанная metadata boundary и не разрешено этим
ADR автоматически.
