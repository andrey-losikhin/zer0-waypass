# AI Context: zer0-waypass

## Что строим

Системный password launcher для Wayland с интерфейсом в Noctalia. Пользователь
открывает панель горячей клавишей, ищет запись и копирует username/password
для ручной вставки. Браузерные расширения намеренно не требуются.

## Зачем

Пользователь работает примерно с восемью разными браузерами. Установка и
поддержка отдельной интеграции в каждом браузере неудобна. Системный launcher и
Wayland clipboard дают один сценарий для браузеров, терминалов и desktop-приложений.

## Ключевые решения

1. Название проекта: `zer0-waypass`.
2. Планируемый Noctalia plugin ID: `zer0/waypass`.
3. Единственный backend MVP: `gopass` с GPG; адаптеры `pass` и age не планируются.
4. Криптография: только GPG/GPG-agent, без собственных алгоритмов.
5. Поиск MVP использует только имя/путь записи из `gopass ls --flat`; username и URL в UI не выводятся.
6. Секрет передаётся из backend напрямую в `wl-copy` по pipe.
7. Для clipboard используется `wl-copy --sensitive --foreground` и ограниченное время жизни.
8. Автоматическое заполнение форм и определение URL активной вкладки не входят в MVP.
9. `lock` и очистка общего `gpg-agent` не входят в MVP.
10. Синхронизация не входит в первый релиз. Позже допускается Git over SSH для уже зашифрованных файлов.
11. Публикация планируется через `noctalia-dev/community-plugins` после локальной стабилизации.
12. Production helper пишется на Go, stdlib-first, Linux-only и без cgo.

## Границы безопасности

- Никогда не передавать пароль аргументом процесса или через shell substitution.
- Никогда не писать пароль, master password или GPG material в логи.
- Никогда не хранить расшифрованный секрет в Noctalia settings/state/cache.
- Не возвращать секрет из helper-а в Luau/Noctalia для действия `copy`.
- Валидировать entry ID; запрещать path traversal, произвольные shell fragments и option injection.
- Использовать exec с фиксированными argv там, где API Noctalia это позволяет.
- Не включать autosubmit или синтетический ввод клавиатуры.
- Не считать очистку текущего clipboard очисткой истории clipboard manager.
- Гарантия для clipboard history действует только для проверенной конфигурации; неизвестные clipboard managers остаются риском.
- Не очищать общий кэш `gpg-agent`: это может затронуть unrelated GPG keys пользователя.
- Ограничивать deadline-ом всю copy-operation, включая ожидание gopass/GPG/pinentry, а не только clipboard TTL.

## Проверенные факты окружения на 2026-08-14

- Установлен Noctalia `v5.0.0 (v5.0.0-beta.8-51-gf7c12ef9fb9b)`.
- Установлены обязательные `gpg`, `wl-copy`; локально также используется необязательный `cliphist`.
- `wl-copy --help` содержит `--sensitive`, `--foreground`, `--clear`, `--paste-once`.
- `gopass` в PATH не обнаружен.
- Go в PATH не обнаружен; локально доступен Rust, но production stack выбран в пользу Go.
- В `/home/zer0/.config/hypr/hyprland.lua` текстовый clipboard сейчас безусловно передаётся в `cliphist store`.
- Локально установлен официальный Noctalia Bitwarden plugin; его структура подтверждает entry types `service`, `launcher_provider` и `panel`.
- Локальный community Bookmarks plugin использует Plugin API 13.
- Актуальная официальная type definition требует Plugin API 24 для безопасной argv-table формы `noctalia.runAsync`; строковая форма использует `/bin/sh -c`.

## Предположения, которые нужно проверить перед кодом

- Как безопаснее распространять helper: как файл внутри plugin package или отдельный OS package.
- Как community CI проверяет дополнительные executable-файлы и declared dependencies.
- Какой plugin API будет актуален на момент начала реализации.
- Поддерживает ли установленная к началу разработки Noctalia Plugin API 24+; иначе локальная реализация блокируется до обновления.

## Минимальная модель зависимостей

- Noctalia plugin зависит только от `zer0-waypass-helper`.
- Helper напрямую зависит от `gopass`, GPG/GPG-agent и `wl-copy`.
- `cliphist` не требуется; проверка нужна только если clipboard manager уже установлен.
- Дополнительные password-store и crypto backend-ы не добавлять в MVP.
- Helper использует стандартную библиотеку Go; не добавлять Cobra/Viper, logging/DI frameworks, БД, gopass SDK, Wayland или GPG libraries без отдельного ADR.

## Стек helper-а

- `os/exec` с фиксированными argv, без shell.
- `os.Pipe` для прямого FD-to-FD потока `gopass -> wl-copy`.
- `context`, `time`, `os/signal` для общего operation deadline и завершения.
- `syscall.SysProcAttr`, process group и Linux parent-death mechanism; точная модель подтверждается crash-тестом в Milestone 1.
- `encoding/json`, `encoding/base64`, `path`, `strings` для протокола и ID.
- `testing` и fake executables через временный `PATH`; реальные integration tests только на isolated synthetic store.

## Metadata policy MVP

- Разрешены только opaque ID и display name, полученные из имени/пути записи.
- Username, URL и custom fields не входят в результаты поиска.
- Отдельный plaintext metadata index не создаётся.
- Username расшифровывается только после явного действия `copy username` и сразу передаётся в clipboard.

## Планируемая структура репозитория

```text
zer0-waypass/
├── README.md
├── AGENTS.md
├── LICENSE
├── docs/
│   ├── VISION.md
│   ├── ARCHITECTURE.md
│   └── SECURITY.md
├── cmd/zer0-waypass-helper/
├── internal/
│   ├── backend/
│   ├── clipboard/
│   └── protocol/
├── noctalia-plugin/
│   └── waypass/
│       ├── plugin.toml
│       ├── README.md
│       ├── launcher.luau
│       ├── panel.luau
│       ├── translations/en.json
│       └── thumbnail.webp
├── packaging/
│   └── arch/
└── .docs/execplans/zer0-waypass/
```

Эта структура целевая, а не уже существующий код.

## Правила реализации для AI

1. Выполнять только один milestone из ExecPlan за раз.
2. Перед изменениями обновлять `status.md`.
3. Не добавлять зависимости без фиксации причины в плане/ADR.
4. После каждого milestone выполнять его mandatory gate.
5. При провале security gate не переходить дальше.
6. Не изменять пользовательский Hyprland/cliphist config автоматически.
7. Не публиковать и не коммитить реальные password-store fixtures; использовать только синтетические данные.
