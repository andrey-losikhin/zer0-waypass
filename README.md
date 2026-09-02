# zer0-waypass

`zer0-waypass` — проект системного менеджера паролей для Wayland и Noctalia.

Главная идея: пользователь вызывает поиск паролей глобальной горячей клавишей,
выбирает учётную запись и вручную вставляет логин или пароль в любое приложение.
Проект не привязан к конкретному браузеру и не требует браузерных расширений.

## Цель

- единый интерфейс для Firefox-, Chromium- и других приложений;
- локальное зашифрованное хранилище на базе `gopass` и GPG;
- настраиваемое кэширование passphrase штатным `gpg-agent`;
- копирование секрета в Wayland clipboard с пометкой `sensitive`;
- ограничение времени выдачи секрета текущим clipboard owner;
- отсутствие паролей в Noctalia state, настройках и логах;
- отсутствие паролей в истории clipboard в документированной и проверенной конфигурации;
- публикация интерфейса как community-плагина Noctalia.

## Предлагаемая схема

```text
Noctalia launcher/panel
        |
        | entry id + action, без секрета
        v
zer0-waypass-helper
        |
        v
gopass -> GPG -> gpg-agent
        |
        v
wl-copy --sensitive --foreground
        |
        v
ручная вставка Ctrl+V в любое приложение
```

## Почему не собственная криптография

Проект создаёт пользовательский интерфейс, безопасный helper и интеграцию с
Wayland. Формат хранилища и криптографию он делегирует существующим инструментам
`gopass`, GPG и `gpg-agent`. Самописное шифрование не входит в scope.

## Минимальные зависимости

- `gopass` — единственный поддерживаемый password-store backend;
- `gpg`/`gpg-agent` — шифрование и разблокировка;
- `wl-copy` — sensitive Wayland clipboard.

`pass`, age и `cliphist` не являются зависимостями проекта. Если у пользователя
уже запущен `cliphist` или другой clipboard manager, его нужно отдельно настроить
на игнорирование sensitive clipboard.

Пометка `sensitive` является подсказкой, а не универсальной гарантией. Проект
гарантирует отсутствие секрета в истории только для явно протестированных
конфигураций, перечисленных в документации.

## Стек реализации

- helper: Go, Linux-only, стандартная библиотека, `CGO_ENABLED=0`;
- UI: Luau и Noctalia Plugin API 24+;
- runtime: `gopass`, GPG/GPG-agent и `wl-copy`;
- первый пакет: Arch Linux `PKGBUILD`.

Для MVP готовый пакет поддерживается только на Arch Linux и совместимых
дистрибутивах (включая CachyOS). Пакеты для Debian, Fedora и других систем не
заявляются; для них будет отдельная проверяемая source-build инструкция.

В MVP не добавляются Cobra/Viper, БД, daemon, Wayland/GPG libraries или
собственная криптография.

## Документация

- [Замысел и продуктовая цель](docs/VISION.md)
- [Архитектура](docs/ARCHITECTURE.md)
- [AI-контекст и правила работы](AGENTS.md)
- [ExecPlan](.docs/execplans/zer0-waypass/plan.md)
- [Статус](.docs/execplans/zer0-waypass/status.md)

## Текущее состояние

Metadata/backend helper Milestone 2 реализован: доступны строгие команды
`list [query]` и `status` для `gopass ls --flat`. Clipboard owner/copy logic и
Noctalia UI ещё не реализованы.
