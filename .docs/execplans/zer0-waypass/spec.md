# Спецификация: zer0-waypass

## Цель

Создать безопасный системный password launcher для Wayland с интерфейсом
Noctalia, использующий существующее GPG-хранилище и sensitive clipboard вместо
браузерных расширений.

## Контекст

- Затронутые модули: новый helper, новый community plugin Noctalia, документация интеграции с clipboard manager, packaging.
- Пользовательские сценарии: поиск учётной записи по имени/пути, копирование username/password.
- Данные: GPG-зашифрованные записи `gopass`; UI получает только opaque ID и имя/путь записи; секрет только в краткоживущем clipboard.
- Обязательные интеграции: Noctalia Plugin API, `gopass`, GPG-agent, `wl-copy`, Hyprland keybind.
- Необязательная совместимость: уже установленный clipboard manager, включая `cliphist`.

## Проверенные факты

- Установленная Noctalia `v5.0.0-beta.8-142-gc874d65e6824` поддерживает Plugin
  API `3..28`, включая требуемый API 24.
- Community plugin оформляется каталогом с `plugin.toml`, Luau entry points, README, английским translation-файлом и thumbnail.
- Noctalia community plugins являются trusted/unsandboxed и обязаны декларировать внешние команды.
- В окружении уже доступны обязательные GPG и wl-copy с sensitive flag; также установлен необязательный cliphist.
- На 2026-08-24 установлен и проверен `gopass 1.16.1`.
- Текущая локальная конфигурация cliphist без фильтра сохраняет весь текст.

## Scope

### Входит

- документированный helper API;
- поиск только по именам/путям из `gopass ls --flat`, без plaintext metadata index;
- безопасные copy-actions;
- sensitive clipboard TTL;
- Noctalia launcher provider и panel;
- обработка missing/backend-error состояний;
- тесты на отсутствие секретов в argv/log/stdout/UI state;
- инструкция безопасной настройки стороннего clipboard manager, если он используется;
- подготовка структуры для community publication;
- базовый Arch packaging после стабилизации core.

### Не входит

- собственная криптография;
- browser extensions и определение origin;
- auto-type/автоматическая отправка форм;
- облачный сервис;
- sync в MVP;
- `open URL`, TOTP и управление общим `gpg-agent`;
- мобильный клиент;
- автоматическое редактирование пользовательских Hyprland/GPG конфигов.

## Ограничения

### Технические

- Wayland clipboard, не X11 Auto-Type.
- Секрет не должен возвращаться из helper-а в Noctalia.
- Функции должны работать без конкретного браузера.

### Архитектурные

- Helper и UI разделены.
- Backend и crypto не реализуются проектом.
- Protocol metadata версионируется.
- Plugin запускает helper только argv-table формой `noctalia.runAsync`.

### Совместимость

- Требуется Noctalia Plugin API 24+: только он поддерживает argv-table без `/bin/sh -c`.
- Для установленной сборки обновление до API 24+ не требуется; перед будущим UI
  milestone нужно повторно проверить актуальные Plugin API и community rules.
- В MVP обязательна одна поддерживаемая комбинация: Arch Linux + Noctalia v5 API 24+ + gopass + GPG + wl-clipboard 2.3+.
- Поддерживается только GPG backend `gopass`; дополнительные backend-ы не входят в MVP.

### Безопасность

- Только синтетические fixtures.
- Никаких secrets в command line, logs, settings, state, crash output.
- Clipboard history filtering является release gate только для документированной поддерживаемой конфигурации.
- Для неизвестных clipboard managers абсолютная гарантия не заявляется; UI обязан показать предупреждение.
- GPG cache policy остаётся выбором пользователя.
- Acquisition deadline начинается до запуска процессов и ограничивает всю фазу
  gopass/GPG/pinentry. После успешного завершения backend и pipe EOF начинается
  clipboard ownership budget: `--ttl` задаёт верхнюю границу, а не обещание
  полного интервала после фактической публикации. Секрет доступен не более
  указанного времени; фактическое окно может быть короче. Budget включает
  kill grace: при `B > G` helper посылает `SIGTERM` в `max(0, B-G)`, при `B` —
  `SIGKILL` оставшейся process group, после чего выполняет только bounded
  reap/verification. Production policy после spike: acquisition deadline и `B`
  имеют inclusive range 5–120 секунд и default 30 секунд; `G` имеет range
  150 ms–2 секунды и default 500 ms; всегда требуется `B > G`. Это policy
  choice на основании measured evidence и UX proposal, а не доказательство
  универсально безопасных числовых границ.

## Предположения

- Предположение: имя `waypass` свободно в community repository на момент публикации.
- Предположение: author/ID `zer0/waypass` соответствует будущему GitHub handle владельца.
- Предположение: helper будет отдельным executable и package dependency плагина.
- Предположение: первый релиз ориентирован на Linux/Wayland, прежде всего Arch-подобные системы.
- Предположение: helper собирается на Go без cgo и без сторонних Go modules.

## Нужно уточнение

- Нужно уточнение: публичный GitHub/GitLab namespace и точное имя автора для community ID.
- Нужно уточнение: поведение при отсутствии sensitive-aware clipboard manager.
- Нужно уточнение: подтверждение community maintainers, что Arch-only helper dependency допустима для community store.
