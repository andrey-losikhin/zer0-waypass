# План реализации: zer0-waypass

## Отдельный milestone: карточка произвольных полей

### Цель

Показать public values и имена secret fields, копируя любое значение через
ограниченный clipboard pipeline.

### Gate 0 — backend contract

1. Проверить `gopass 1.16.1` на isolated synthetic store: list-only API,
   password, single/missing/empty/duplicate/multiline/custom/body semantics.
2. Запретить production implementation, если имена можно узнать только parsing
   полного decrypt output.
3. При blocker оформить отдельную encrypted-schema схему и не расширять v1;
   field ID обязан быть связан с revision/content digest, проверяемой заново
   перед каждым copy.

### Результат 2026-08-31

Gate 0 остановил milestone: безопасного list-only API нет, duplicate и
multiline/body selectors неоднозначны. Production helper, clipboard и panel не
изменяются. Следующий допустимый шаг — отдельный ADR/spike encrypted schema,
атомарно сопровождаемой внешним TUI; он не является продолжением текущего
milestone без нового решения.

### Stop rule

Не реализовывать decrypt-and-parse, plaintext metadata index, probing raw field
names или UI с предполагаемым фиксированным набором полей.

### Gate 1 — Field Contract/runtime

- encrypted manifest и отдельные encrypted value entries;
- fixed standard visibility и explicit custom visibility;
- protocol v2 fields/copy-field с fresh membership и revision binding;
- panel card: Enter, `1–9`, arrows, Enter-copy, Escape/Backspace, scroll;
- rollback сохраняет protocol v1 и legacy username/password path.

## Общий подход

Сначала доказать backend, process и clipboard contracts на синтетических данных.
Затем отдельно реализовать metadata/backend core, отдельно clipboard owner и
только после этого подключить Noctalia. MVP не содержит URL, TOTP, `lock`,
синхронизацию или plaintext metadata index.

## Milestone 1: Compatibility и security spike

### Цель

Подтвердить интерфейсы `gopass`, жизненный цикл `wl-copy`, Noctalia API 24+ и
правила community packaging до production-кода.

### Вероятно затронет

- `docs/SECURITY.md`
- `docs/decisions/0001-helper-stack.md`
- `docs/decisions/0002-backend-contract.md`
- `docs/decisions/0003-clipboard-process-model.md`
- `.docs/execplans/zer0-waypass/status.md`

### Шаги

1. После явного разрешения установить `gopass` и создать isolated GNUPGHOME с синтетическим store.
2. Проверить `gopass ls --flat` и доказать, что список имён не расшифровывает все записи.
3. Проверить точные команды получения password/username, stdout/stderr и exit codes.
4. Проверить `wl-copy --sensitive --foreground`: TTL, смену ownership, signal handling и отсутствие удаления более нового clipboard.
5. Проверить test-only watcher с `CLIPBOARD_STATE=sensitive`; пользовательский config не менять.
6. Проверить актуальные community CI, manifest и minimum Plugin API.
7. Зафиксировать факт: argv-table `noctalia.runAsync` требует Plugin API 24; строковая форма запрещена.
8. Проверить, поддерживает ли установленная Noctalia API 24. Если нет — зафиксировать обязательное обновление как блокер UI.
9. Проверить выбранную Go process model: fixed argv, FD-to-FD pipe, process groups, общий acquisition deadline и parent-death.
10. Проверить `Pdeathsig` с `runtime.LockOSThread` crash-тестом и отмену зависшего GPG/pinentry без остановки общего `gpg-agent`.
11. Зафиксировать минимальную поддерживаемую версию Go и результат в ADR.

### Acceptance criteria

- Backend contract использует только `gopass ls --flat`; UI-search получает лишь entry path/name.
- Username/password расшифровываются только для выбранной записи.
- Зафиксирована воспроизводимая clipboard process model без внешнего `timeout`.
- Clipboard ownership budget начинается после backend exit `0` и pipe EOF;
  фактическое доступное окно не превышает настроенный budget и может быть короче.
- Ownership budget включает kill grace: при `B > G` TERM отправляется в
  `max(0, B-G)`, KILL оставшейся group — на границе `B`; глобальный
  `wl-copy --clear` запрещён.
- Более новое clipboard value сохраняется после старого TTL.
- Зависшие gopass/GPG/pinentry ограничены acquisition deadline; общий `gpg-agent` не завершается.
- Для test cliphist sensitive fixture отсутствует в history.
- Подтверждены API 24+, community validation commands и Arch release target.
- Реальные пользовательские пароли не использовались.

### Validation

- `gpg --version`, `gopass --version`, `wl-copy --version`, `noctalia --version`.
- Isolated GNUPGHOME/store, captured stdout/stderr, process-tree inspection,
  проверка границ acquisition deadline и clipboard ownership budget, включая
  TERM в `max(0, B-G)`, KILL в `B` и bounded reap tolerance, isolated cliphist
  DB при наличии cliphist.
- Сверка `noctalia.d.luau`: argv-table `runAsync` требует Plugin API 24.

### Риски

- Установленная локальная Noctalia может не поддерживать API 24.
- `gopass ls` может раскрывать структуру store или требовать неподходящий backend access.
- `wl-copy` lifecycle может потребовать platform-specific supervision.
- Pinentry может принадлежать process tree общего `gpg-agent`; безопасная отмена пока не доказана.

### Stop rule

Не переходить дальше без утверждённых backend/process contracts и API 24-compatible среды.

## Milestone 2: Helper metadata и backend core

### Цель

Реализовать строгий CLI и backend adapter без clipboard/process логики.

### Вероятно затронет

- `cmd/zer0-waypass-helper/`
- `internal/backend/`
- `internal/protocol/`
- `docs/PROTOCOL.md`
- `docs/SECURITY.md`

### Шаги

1. Создать versioned protocol с полями только `id` и `label`.
2. Определить ID как base64url от канонического относительного пути с обязательной повторной проверкой membership через `gopass ls` перед copy.
3. Реализовать `list [query]` и `status`.
4. Получать label только из имени/пути записи; не создавать plaintext index.
5. Реализовать строгую валидацию/декодирование ID без shell и path traversal.
6. Добавить структурированные redacted errors.
7. Покрыть metadata backend и synthetic `gopass ls --flat` store тестами; secret extraction в этом milestone запрещён.

### Acceptance criteria

- `list/status` никогда не содержат username, URL или secret.
- Поиск не расшифровывает все записи.
- Выбор одной записи не позволяет option/path/shell injection.
- Ошибки и debug output не содержат secret.

### Validation

- `gofmt`, `go test ./...`, `go vet ./...`, `CGO_ENABLED=0 go build -trimpath ./cmd/zer0-waypass-helper`.
- Negative matrix: `../`, leading `-`, shell metacharacters, invalid UTF-8/oversize input.
- Leak scan stdout/stderr/logs на synthetic marker.

### Риски

- Имена файлов сами раскрывают домены и структуру store; это документируется как принятая утечка metadata.
- Backend CLI может менять формат вывода между версиями.

### Stop rule

Не реализовывать clipboard, пока metadata contract и negative tests не зелёные.

## Milestone 3: Helper clipboard owner

### Цель

Добавить `copy username/password`, не возвращая secret вызывающему UI и не оставляя неограниченных процессов.

### Вероятно затронет

- `internal/backend/secret.go`
- `internal/clipboard/`
- `internal/clipboard/*_test.go`
- `docs/CLIPBOARD.md`
- `docs/SECURITY.md`

### Шаги

До отдельного шага wiring публичная команда `copy` не поддерживается и обязана
fail-closed возвращать `invalid_invocation`, в том числе для синтаксически
корректного вызова. Шаги 2–7 реализуются и проверяются через внутреннюю границу
copy-operation, не подключённую к публичному dispatcher.

1. Реализовать внутренний parser точной грамматики
   `copy <username|password> <id> --ttl <seconds>` и typed request validation:
   allowlist action, strict ID decode и строгий positive decimal TTL,
   представимый как `time.Duration`; не сохранять raw untrusted ID или secret.
2. Во внутренней copy-operation запускать gopass только здесь и передавать secret через `os.Pipe` напрямую в stdin `wl-copy --sensitive --foreground`, без Go `string` и промежуточного пользовательского буфера.
3. Ограничить безопасными диапазонами два параметра: acquisition deadline и
   clipboard TTL; `--ttl` означает максимальный ownership budget от успешного
   backend completion и pipe EOF, а не полный интервал после фактической
   публикации. Budget `B` включает kill grace `G` и требует `B > G`; допустимые
   ranges и defaults зафиксировать после spike.
4. Helper остаётся supervisor до завершения всей операции и управляет process groups gopass и wl-copy.
5. Реализовать bounded cleanup и parent-death model последовательно, не
   подключая public dispatcher до завершения всех подпунктов:
   1. **5a — cleanup core:** единая идемпотентная state machine для retained
      groups: почти одновременный TERM, один общий grace, KILL оставшихся,
      single-Wait leaders и bounded empty-group verification на normal/error и
      partial-start путях.
   2. **5b — acquisition deadline/signals:** deadline начинается до pipe/workers
      и вместе с handled signals использует тот же cleanup core; race backend
      result/owner exit/deadline разрешается fail-closed.
   3. **5c — ownership budget:** после backend exit `0` + pipe EOF применять
      TERM в `B-G`, KILL в `B`, без дополнительного grace; проверить early
      replacement и сохранение более нового clipboard ownership.
   4. **5d — guardian/parent death:** same-helper per-operation guardian,
      `runtime.LockOSThread`, `Pdeathsig=SIGTERM`, READY/START/REGISTERED и
      controller-crash cleanup без worker `Pdeathsig`.
   5. **5e — lifecycle gate:** repeated/concurrent normal, error, signal,
      deadline и crash tests; отсутствие orphan groups и bounded timing proof.
6. Проверить отмену pinentry без завершения или очистки общего `gpg-agent`.
7. Не использовать внешний `timeout`, shell, command substitution или secret argv.
8. Только после GREEN шагов 2–7 подключить публичный dispatcher `copy` к
   проверенной internal operation. С этого момента синтаксически корректный
   вызов не должен возвращать `invalid_invocation`; invalid grammar по-прежнему
   fail-closed завершается с `invalid_invocation` до backend call.
9. Зафиксировать, что detached dispatch сообщает только принятие команды, а не успех расшифровки.

### Acceptance criteria

- До шага 8 `copy` не является публично поддерживаемой командой: любой публичный
  вызов возвращает `invalid_invocation`, не запускает backend и ничего не пишет
  в stdout. После шага 8 корректная grammar достигает только проверенной
  copy-operation и не может быть отклонена как `invalid_invocation`.
- Secret отсутствует в argv/stdout/stderr/logs/files.
- Acquisition deadline начинается до запуска процессов и ограничивает
  gopass/GPG/pinentry; ownership budget начинается только после backend exit `0`
  и pipe EOF.
- В `max(0, B-G)` helper отправляет TERM retained owner group, на границе `B` —
  KILL любой оставшейся group; после `B` разрешены только bounded
  reap/verification и измеримая scheduler/test tolerance.
- Copy command завершается не позднее acquisition deadline + ownership budget +
  bounded reap/verification tolerance; отдельный grace сверх budget запрещён.
- Фактическое окно, в котором owner предоставляет secret, не превышает
  настроенный budget и может быть короче из-за задержки публикации или смены
  ownership.
- Смена clipboard ownership завершает старый owner раньше TTL.
- Старый TTL не очищает новое значение.
- Нет orphan `wl-copy` после normal exit, handled signals и тестируемого parent failure.
- Нет orphan gopass/GPG/pinentry после acquisition timeout; общий `gpg-agent` продолжает работать.
- Sensitive fixture отсутствует в истории проверенной cliphist-конфигурации.

### Validation

- До wiring: exhaustive parser grammar matrix и публичный fail-closed test для
  valid/invalid `copy`, с пустым stdout и без backend call. После wiring:
  повторить invalid grammar matrix и доказать, что valid syntax не возвращает
  `invalid_invocation`, а достигает internal operation boundary.
- Process-tree/latency tests с отдельными acquisition/budget timestamps: TERM в
  `max(0, B-G)`, KILL stubborn owner в `B`, пустая group и отсутствие paste
  после `B` в пределах явно измеренной tolerance; signal matrix, ownership
  race, helper crash simulation.
- Isolated cliphist DB only for supported configuration.
- Повторные copy-actions и параллельные вызовы на synthetic secrets.

### Риски

- SIGKILL нельзя обработать в helper; child containment зависит от выбранного platform mechanism.
- Неизвестный clipboard manager может проигнорировать sensitive hint.

### Stop rule

Не подключать UI при orphan process, clipboard race или любой утечке synthetic secret.

## Milestone 4: Noctalia launcher provider

### Цель

Добавить минимальный `/wp` provider с поиском и copy-actions через безопасный argv dispatch.

### Вероятно затронет

- `noctalia-plugin/waypass/plugin.toml`
- `noctalia-plugin/waypass/launcher.luau`
- `noctalia-plugin/waypass/translations/en.json`
- `noctalia-plugin/waypass/README.md`

### Шаги

1. Создать manifest `zer0/waypass`, `plugin_api = 24`, dependency `zer0-waypass-helper`.
2. Добавить `/wp` provider и поиск по label.
3. Запускать query commands только argv-table с callback и bounded timeout.
4. Запускать copy commands argv-table без callback как detached TTL-limited helper.
5. Добавить missing-helper, empty-result и backend-error состояния.
6. Не помещать secrets, username или URL в Luau tables/state/notifications.
7. Для принятого dispatch показывать только «доступно не более N секунд»; не
   сообщать об успешной расшифровке или полном интервале после публикации.

### Acceptance criteria

- Provider работает только на API 24+ и не содержит строковых `runAsync` calls.
- Результаты содержат только id/label.
- Copy dispatch содержит только fixed argv + validated opaque ID.
- Disable/reload не создаёт бессрочных процессов; уже запущенный owner живёт не дольше TTL.
- Manifest и translations проходят community validation.

### Validation

- `noctalia config validate`, `noctalia plugins lint .`, актуальные community scripts.
- Path-source enable/query/copy/disable/reload smoke на synthetic store.
- Static grep/review: отсутствие string-form `runAsync` и secret handling в Luau.

### Риски

- Локальная Noctalia beta может потребовать обновления до API 24.
- Detached action не может синхронно сообщить ошибку расшифровки; UX должен честно показывать только dispatch.

### Stop rule

Не переходить к panel, пока launcher и leak checks не зелёные.

## Milestone 5: Noctalia panel и session integration

### Цель

Добавить глобально вызываемую keyboard-first panel и документировать поддерживаемую clipboard-конфигурацию.

### Вероятно затронет

- `noctalia-plugin/waypass/panel.luau`
- `examples/hyprland.lua`
- `examples/cliphist-sensitive-watch.sh`
- `docs/INSTALL.md`
- `docs/CLIPBOARD.md`

### Шаги

1. Добавить panel с поиском и двумя действиями: copy username/password.
2. Добавить IPC toggle и пример `SUPER+P`.
3. Добавить first-run предупреждение: sensitive hint не гарантирует поведение неизвестного clipboard manager.
4. Добавить opt-in wrapper для текущей cliphist-конфигурации.
5. Не добавлять `open URL`, TOTP, `lock` или автоматическое изменение configs.
6. Проверить normal/sensitive/newer-value сценарии в поддерживаемой конфигурации.

### Acceptance criteria

- Panel keyboard navigation работает без launcher.
- UI не получает secret и не обещает успешное копирование до фактической вставки.
- Copy-action формулируется как «доступно не более N секунд»; dispatch не
  сообщает об успешной расшифровке.
- Поддерживаемая cliphist-конфигурация не сохраняет sensitive fixture.
- Для неизвестного manager документация не заявляет абсолютной гарантии.
- Пользовательские Hyprland/GPG configs меняются только вручную.

### Validation

- Panel open/search/copy/close/reload smoke.
- Shell syntax/lint examples; isolated cliphist DB.
- Review всех пользовательских формулировок о clipboard guarantees.

### Риски

- Clipboard managers имеют разные semantics sensitive hints.
- Plugin API beta может изменить panel lifecycle.

### Stop rule

Не переходить к packaging при утечке в поддерживаемой конфигурации или абсолютных недоказуемых гарантиях в документации.

## Milestone 6: Arch packaging и community release candidate

### Цель

Подготовить воспроизводимый Arch package helper-а и community plugin с честной матрицей поддержки.

### Вероятно затронет

- `packaging/arch/PKGBUILD`
- release workflow/checksums
- `noctalia-plugin/waypass/thumbnail.webp`
- public README/changelog/license
- подготовленная копия `waypass/` для community repository

### Шаги

1. Добавить license/versioning и reproducible Arch package.
2. Проверить clean install/uninstall без удаления password store.
3. Указать Arch Linux как единственный packaged target MVP.
4. Для других систем дать проверяемую source-build инструкцию, не обещая native packages.
5. Подтвердить у community maintainers допустимость external Arch-only helper dependency.
6. Создать thumbnail 960x540, прогнать полный community CI.
7. Провести security review и skeptic review.
8. Подготовить PR, но не отправлять без явного запроса владельца.

### Acceptance criteria

- Fresh Arch install воспроизводим и helper появляется в PATH.
- Uninstall не удаляет пользовательский store/GPG material.
- Community README явно описывает dependency installation и support matrix.
- Все mandatory gates зелёные; fixtures/screenshots синтетические.

### Validation

- Clean package build, package content inspection, disposable install/uninstall.
- Fresh-user E2E с synthetic GPG store.
- Community validation scripts и ручной review PR payload.

### Риски

- Community maintainers могут не принять external helper dependency.
- Имя каталога `waypass` может быть занято.
- Пользователи других дистрибутивов не получат готовый package в MVP.

### Stop rule

Не публиковать release/PR при failed clean install, security finding или неподтверждённой community policy.

## Milestone 7: Опциональная синхронизация

### Цель

После стабильного MVP добавить документированный Git over SSH workflow для уже зашифрованных записей.

### Вероятно затронет

- `docs/SYNC.md`
- integration tests с локальным bare Git repository

### Шаги

1. Зафиксировать metadata leakage и conflict policy.
2. Использовать штатный gopass/Git workflow; не писать собственный sync protocol без отдельного плана.
3. Запретить destructive auto-resolution и force operations.
4. Проверить two-clone synthetic scenario и recovery.

### Acceptance criteria

- Remote содержит только зашифрованные файлы и известную Git metadata.
- Конфликты не затираются автоматически.
- Восстановление из history воспроизводимо.

### Validation

- Локальный bare remote, два clones, simultaneous/offline edit, failed push и recovery.

### Риски

- Имена файлов/Git metadata раскрывают структуру store.
- Зашифрованные файлы нельзя содержательно merge-ить без расшифровки.

### Stop rule

Не включать auto-sync по умолчанию и не выполнять force push/reset.
