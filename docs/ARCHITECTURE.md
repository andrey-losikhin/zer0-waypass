# Архитектура zer0-waypass

## Компоненты

### 1. Password store

`gopass` является единственным backend-ом MVP. Записи остаются отдельными
GPG-зашифрованными файлами. GPG-agent отвечает за запрос passphrase и его
временное кэширование.

### 2. `zer0-waypass-helper`

Небольшой Linux executable на Go с узким интерфейсом:

```text
list [query]
copy username <entry-id> [ttl]
copy password <entry-id> [ttl]
status
```

Точный CLI-контракт фиксируется после spike с `gopass`. `list` строится по
`gopass ls --flat` и возвращает только
opaque ID и display name из пути записи, не username/URL. Команда `copy` не
возвращает секрет вызывающему процессу: helper создаёт pipe backend ->
`wl-copy --sensitive --foreground` самостоятельно.

Helper использует стандартную библиотеку Go и собирается с `CGO_ENABLED=0`.
Внешние команды запускаются через `os/exec` с фиксированными argv. Поток секрета
соединяется через `os.Pipe` напрямую между stdout `gopass` и stdin `wl-copy`, без
shell и без хранения секрета в Go `string`.

### 3. Noctalia plugin `zer0/waypass`

Планируемые entry points:

- `launcher_provider`: поиск `/wp`;
- `panel`: keyboard-first окно для глобального бинда.

Плагин получает только JSON/структурированные метаданные. Для copy-action он
передаёт helper-у opaque entry ID.

### 4. Clipboard boundary

Секрет публикуется с sensitive hint и ограниченным временем жизни. Copy helper
запускается из Noctalia через argv-table как detached action, сам владеет
foreground `wl-copy` и контролирует процессы `gopass` и `wl-copy`. Отдельный
acquisition deadline ограничивает ожидание gopass/GPG/pinentry; clipboard TTL
(`--ttl`) является максимальным ownership budget и начинается после успешного
завершения backend и pipe EOF. Он не обещает полный интервал после фактической
публикации: доступное окно не превышает TTL и может быть короче. От начала
операции hard bound равен `acquisition deadline + TTL + bounded reap/verification
tolerance`: kill grace уже входит в TTL. Для budget `B` и grace `G`, где
`B > G`, helper посылает TERM retained foreground `wl-copy` group в
`max(0, B-G)`, а на границе `B` — KILL любой оставшейся group. После `B`
разрешены только bounded reap/verification и явно измеримая scheduler/test
tolerance; owner не должен предоставлять secret после `B`. Допустимые ranges
`B`/`G` фиксируются после spike. Если clipboard ownership перешло к другому
приложению, `wl-copy` завершается
раньше, а helper не очищает более новое значение.

Helper обязан завершить обе дочерние process groups при deadline, штатной
ошибке/сигнале и использовать parent-death/process-group механизм, подтверждённый
в compatibility spike. Отмена pinentry проверяется отдельно и не должна завершать
или очищать общий `gpg-agent`. После
dispatch UI сообщает только принятие команды, а не успешную расшифровку.
Пользовательская формулировка для принятого dispatch: «доступно не более N
секунд».

`sensitive` является hint. Гарантия отсутствия записи в истории даётся только для
проверенной конфигурации clipboard manager. Для неизвестного manager UI показывает
first-run предупреждение; универсальная автоматическая проверка не заявляется.

## Поток данных

### Поиск

```text
Noctalia -> helper list/query -> metadata only -> Noctalia
```

### Копирование

```text
Noctalia -> helper copy password <opaque-id>
helper -> gopass/GPG -> pipe -> wl-copy sensitive
Noctalia <- только подтверждение dispatch, без результата расшифровки и без secret
```

## Trust boundaries

1. Noctalia и community plugins — пользовательский доверенный код, но секреты
   всё равно не должны проходить через UI runtime.
2. Helper — единственный компонент, имеющий право инициировать расшифровку.
3. GPG-agent — владелец unlocked key material.
4. Wayland clipboard — кратковременный канал, не постоянное хранилище.
5. Clipboard managers — отдельная зона риска и обязательная часть проверки.

## Формат обмена

Метаданные helper-а должны быть машинно-читаемыми, версионированными и не
содержать секретов. Предварительный envelope:

```json
{
  "protocol": 1,
  "items": [
    { "id": "opaque-stable-id", "label": "github.com/work" }
  ]
}
```

`id` — versioned base64url encoding канонического относительного пути записи.
Перед copy helper декодирует ID, проверяет формат/границы пути и подтверждает,
что запись присутствует в свежем `gopass ls --flat`. `id` нельзя интерполировать в shell.

## Noctalia process boundary

Плагин требует `plugin_api >= 24`: все вызовы helper-а используют только
argv-table форму `noctalia.runAsync` с фиксированной структурой argv. Строковая
форма запрещена, поскольку выполняется через `/bin/sh -c`. API 24 лишь даёт
прямой argv dispatch и не валидирует entry ID; строгая проверка ID и membership
в актуальном `gopass ls --flat` остаётся обязанностью helper-а. Локальная
разработка блокируется, если установленная Noctalia не поддерживает API 24.

## Распространение

### Community plugin

Целевая структура для отправки в `noctalia-dev/community-plugins`:

```text
waypass/
├── plugin.toml
├── README.md
├── launcher.luau
├── panel.luau
├── translations/en.json
└── thumbnail.webp
```

Планируемый ID — `zer0/waypass`, минимальный Plugin API — 24. Plugin manifest декларирует только
`zer0-waypass-helper`; `gopass`, GPG и `wl-copy` являются зависимостями пакета
helper-а. На момент исследования plugin API находится в beta и может измениться,
поэтому версия и CI community repository перепроверяются перед реализацией.

### Helper package

Предпочтительно распространять helper отдельно, чтобы:

- использовать его вне Noctalia;
- тестировать security boundary независимо;
- пакетировать для Arch/Nix;
- не помещать platform binary непосредственно в community plugin.

Первый release target — Arch Linux. Для других дистрибутивов community README
даёт source-build инструкцию; расширение матрицы пакетов выполняется отдельно.
Правила community repository перепроверяются в compatibility spike.

## Выбранный стек helper-а

- Go standard library: `os/exec`, `os.Pipe`, `context`, `time`, `os/signal`,
  `encoding/json`, `encoding/base64`, `path`, `strings`, `testing`.
- Linux process control: `syscall.SysProcAttr`, отдельные process groups и
  parent-death signal. Из-за thread semantics точная реализация с
  `runtime.LockOSThread` является обязательным предметом spike и crash-теста.
- Сборка: один `CGO_ENABLED=0` binary; Go нужен только при сборке пакета.
- Runtime dependencies Arch package: `gopass`, `gnupg`, `wl-clipboard`.

Не добавляются Cobra/Viper, logging/DI frameworks, `x/sys` без доказанной
необходимости, gopass SDK, clipboard/Wayland/GPG libraries, daemon, БД или
собственная криптография.

## Альтернативы

- Только Luau + прямой вывод `gopass`: проще, но секрет попадёт в Noctalia runtime.
- Shell helper: быстро для прототипа, но сложнее гарантировать отсутствие shell
  injection и корректную обработку процессов.
- Rust helper: сильный альтернативный вариант, но потребует больше crates для
  JSON/process control/zeroization и усложнит минимальный supply chain. Для MVP
  выбран Go stdlib-first.
- Browser extension: точнее знает origin, но противоречит основному UX проекта и
  остаётся только возможным будущим модулем.
