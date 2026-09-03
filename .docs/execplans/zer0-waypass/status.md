# Статус: zer0-waypass

## Текущее состояние

- Статус: in progress
- Сессия 2026-09-03: начат отдельный repository-hygiene проход без изменения
  runtime: GitHub community profile, contribution/security/release policy, CI,
  единые license/repository metadata и документация подготовки будущего
  Noctalia community PR. Commit/push и публикация не входят в этот проход.
- Сессия 2026-09-03: repository-hygiene проход завершён — GREEN. Добавлены
  GitHub templates/CI/Dependabot/CODEOWNERS, contribution/security/support/
  release policies, changelog и text-file conventions; module/repository/license
  metadata согласованы с публичным GitHub repository и Apache-2.0 `LICENSE`.
  `git diff --check`, gofmt, shell/YAML parse, test, race, vet и static build —
  PASS. Skeptic review исправил shallow-checkout CI, недоступный private report
  channel и преждевременные release links; повторная проверка — GREEN.
  Independent correctness review — GREEN, подтверждённых проблем не найдено.
- Сессия 2026-08-31: начато проектирование Waypass Field Contract v1 по решению
  владельца. Контракт должен поддержать только manifest-declared поля, отдельные
  encrypted value entries и целостное копирование multiline values без parsing
  общей расшифрованной записи. Production-код и внешний TUI в этом шаге не
  изменяются.
- Сессия 2026-08-31: владелец явно изменил visibility boundary и запросил
  runtime: public notes/connection metadata видимы, password/tokens/keys/TOTP/
  recovery codes скрыты. Gate 1 реализуется отдельным protocol v2.
- Сессия 2026-08-31: Gate 1 реализован: strict encrypted manifest/value bundle,
  public/secret policy, digest-bound `fields`/`copy field`, повторный resolve в
  guardian и keyboard-first panel card. Independent review findings по stale
  value, bounded reads, aggregate payload и документации исправлены.
- Follow-up: legacy username теперь `public` и отображается в карточке. Manifest
  перенесён в deterministic encrypted sidecar, поэтому helper не читает legacy
  запись целиком при определении формата; password остаётся hidden.
- Follow-up: panel хранит 3 минуты только query и opaque entry/field IDs. После
  copy и повторного открытия выполняются свежие `list`/`fields`, затем
  восстанавливаются карточка и выбранное поле; field values не запоминаются.
- В заголовок карточки добавлена кнопка `Back to list`; `Backspace` и `Escape`
  вызывают тот же переход к общему списку.
- Сессия 2026-08-31: начат bounded spike карточки произвольных полей. Фактически
  установлены Noctalia `v5.0.0-beta.8-142-gc874d65e6824`, `gopass 1.16.1` и Go
  `1.26.6`; активных helper/gopass/wl-copy/pinentry процессов перед spike не
  обнаружено. Production-код не изменяется до решения security gate.
- Сессия 2026-08-31: Gate 0 завершён с blocker. `gopass 1.16.1` не предоставляет
  список live field names без full decrypt; duplicates и multiline/body не имеют
  однозначного selector. По stop rule helper/panel не изменены, protocol v1 не
  расширен. Следующий возможный шаг — отдельный ADR/spike encrypted schema.
- Independent correctness/skeptic review: production blocker подтверждён.
  Исправлены stale описание public copy в SECURITY, overclaim охвата CLI survey
  и medium TOCTOU в future schema proposal: field ID теперь обязан быть связан
  с повторно проверяемой revision/content digest.
- Backlog после текущего Milestone 6: карточка записи в Noctalia с нумерованным
  keyboard-first выбором поля и безопасным копированием произвольных полей через
  расширенный helper. Реализация не входит в текущий milestone; секретные поля
  не должны возвращаться в Luau/Noctalia.
- Сессия 2026-08-27: завершён UX-проход Milestone 5 для трёхминутного
  in-memory восстановления panel query и выбранного opaque entry ID; persistent
  Noctalia settings/state/cache и helper contract не изменены. После TTL,
  plugin reload или исчезновения записи используется чистое/первое состояние.
- Сессия 2026-08-27: завершён ограниченный UX-проход Milestone 5 для panel:
  полноширинный focused search, keyboard selection/actions, scroll и защита от
  stale async list results; callback-запросы сериализованы и коалесцируются до
  последнего query. Helper/clipboard lifecycle остались без изменений.
- Текущий milestone: Milestone 5 завершён — GREEN
- Текущий шаг: 5 — community dependency policy
- Следующий шаг: выполнить только M6 step 5 отдельным проходом
- Сессия 2026-08-26: preflight подтвердил отсутствие production guardian-кода
  и активных helper/gopass/wl-copy test-процессов; Git status/diff недоступен,
  потому что каталог не является рабочим Git tree.
- Сессия 2026-08-26: step 5e gate расширен repeated normal/error/deadline,
  concurrent и synthetic `/proc` orphan checks; финальная отметка ожидает
  полного gate и review.
- Сессия 2026-08-26: M4 step 1 — создан manifest `zer0/waypass`, Plugin API 24,
  dependency `zer0-waypass-helper`; repository-local community validator отсутствует.

## Checklist Milestone 1

- [x] 1. Установить `gopass` и создать isolated GNUPGHOME с синтетическим store.
- [x] 2. Проверить `gopass ls --flat` и отсутствие массовой расшифровки записей.
- [x] 3. Проверить команды получения password/username, stdout/stderr и exit codes.
- [x] 4. Проверить `wl-copy --sensitive --foreground`: TTL, ownership, signals и сохранение более нового clipboard.
- [x] 5. Проверить test-only watcher с `CLIPBOARD_STATE=sensitive`, не меняя пользовательский config.
- [x] 6. Проверить актуальные community CI, manifest и minimum Plugin API.
- [x] 7. Зафиксировать требование Plugin API 24 для argv-table `noctalia.runAsync` и запрет строковой формы.
- [x] 8. Проверить, поддерживает ли установленная Noctalia API 24; установленный host поддерживает API 3–28.
- [x] 9. Проверить Go process model: fixed argv, FD-to-FD pipe, process groups и общий acquisition deadline.
- [x] 10. Проверить `Pdeathsig`/`runtime.LockOSThread`, parent crash и отмену pinentry без остановки общего `gpg-agent`.
- [x] 11. Зафиксировать минимальную поддерживаемую версию Go и результат в ADR.

## Mandatory gate Milestone 1

- [x] Backend list/single-entry extraction contracts подтверждены на isolated synthetic store.
- [x] Foreground clipboard ownership, replacement race и no-global-clear cleanup подтверждены.
- [x] Acquisition deadline, process groups, parent-death guardian и real pinentry cancellation подтверждены.
- [x] Isolated sensitive-aware cliphist watcher не сохраняет sensitive fixture.
- [x] Noctalia API 24+, community validation и Arch target проверены.
- [x] Conservative TTL budget: `B` начинается после backend exit `0` + pipe EOF, включает kill grace `G`; usable clipboard interval может быть короче `B`.

## Checklist Milestone 2

- [x] 1. Создать versioned protocol с полями только `id` и `label`.
- [x] 2. Определить opaque ID как base64url канонического относительного пути и fresh membership check contract.
- [x] 3. Реализовать `list [query]` и `status`.
- [x] 4. Получать label только из имени/пути записи, без plaintext metadata index.
- [x] 5. Реализовать строгую валидацию/декодирование ID без shell/path traversal.
- [x] 6. Добавить структурированные redacted errors.
- [x] 7. Покрыть metadata backend и synthetic `gopass ls --flat` store тестами; secret extraction запрещён.

## Mandatory gate Milestone 2

- [x] Formatter, tests, race, repeated tests, vet и `CGO_ENABLED=0` static build.
- [x] Negative input matrix для ID/path/query, fixed argv и no-shell scan.
- [x] Closed `id`/`label` metadata allowlist и redacted error/leak checks.
- [x] Isolated synthetic `gopass ls --flat` proof без GPG/decrypt вызовов.
- [x] Независимое финальное security review — GREEN; все findings закрыты.

## Checklist Milestone 3

- [x] 1. Реализовать внутренний strict parser/typed request для `copy <username|password> <id> --ttl <seconds>`; публичный dispatch пока fail-closed.
- [x] 2. Передавать secret из `gopass` в `wl-copy --sensitive --foreground` через прямой `os.Pipe`, без Go `string`/буфера.
- [x] 3. Зафиксировать и валидировать безопасные ranges/defaults acquisition deadline, clipboard budget `B` и включённый grace `G`.
- [x] 4. Реализовать supervision process groups для `gopass` и `wl-copy` на всю операцию.
- [~] 5. Реализовать bounded cleanup по deadline/signal/error и parent-death guardian для abnormal exit.
  - [x] 5a. Единый TERM/shared-grace/KILL/reap/empty-group cleanup core для normal/error/partial-start.
  - [x] 5b. Acquisition deadline, handled signals и fail-closed event races.
  - [x] 5c. Ownership budget: TERM `B-G`, KILL `B`, replacement/newer-owner race.
  - [x] 5d. Same-helper guardian, `LockOSThread`, `Pdeathsig`, READY/START/REGISTERED и parent-crash cleanup.
  - [x] 5e. Repeated/concurrent lifecycle mandatory gate без orphan groups.
- [x] 6. Проверить отмену pinentry без остановки/очистки общего `gpg-agent`.
- [x] 7. Подтвердить отсутствие external `timeout`, shell, command substitution и secret argv.
- [x] 8. Подключить публичный `copy` dispatch только после GREEN шагов 2–7; корректный запрос больше не должен возвращать `invalid_invocation`.
- [x] 9. Зафиксировать detached-dispatch contract: только принятие команды, не результат decrypt.

## Checklist Milestone 4

- [x] 1. Создать manifest `zer0/waypass`, `plugin_api = 24`, dependency `zer0-waypass-helper`.
- [x] 2. Добавить `/wp` provider и поиск по label.
- [x] 3. Запускать query commands только argv-table с callback и bounded timeout.
- [x] 4. Запускать copy commands argv-table без callback как detached TTL-limited helper.
- [x] 5. Добавить missing-helper, empty-result и backend-error состояния.
- [x] 6. Не помещать secrets, username или URL в Luau tables/state/notifications.
- [x] 7. Для принятого dispatch показывать только bounded availability.

## Checklist Milestone 5

- [x] 1. Добавить panel с поиском и двумя действиями: copy username/password.
- [x] 2. Добавить IPC toggle и пример `SUPER+P`.
- [x] 3. Добавить first-run предупреждение о неизвестных clipboard managers.
- [x] 4. Добавить opt-in wrapper для текущей cliphist-конфигурации.
- [x] 5. Не добавлять URL/TOTP/lock и не менять пользовательские configs автоматически.
- [x] 6. Проверить normal/sensitive/newer-value сценарии в поддерживаемой конфигурации.

## Checklist Milestone 6

- [x] 1. Добавить license/versioning и reproducible Arch package.
- [x] 2. Проверить clean install/uninstall без удаления password store.
- [x] 3. Указать Arch Linux как единственный packaged target MVP.
- [x] 4. Добавить source-build инструкцию для других систем.
- [~] 5. Подтвердить community policy для external Arch-only helper dependency.
- [ ] 6. Создать thumbnail 960x540 и прогнать community CI.
- [ ] 7. Провести security и skeptic review.
- [ ] 8. Подготовить PR без отправки.

## Выполненные пункты

- 2026-08-24: Milestone 2, шаг 1 завершён — protocol v1 с closed metadata allowlist `id`/`label`.
- 2026-08-24: Milestone 2, шаг 2 завершён — versioned raw-base64url EntryID и fresh exact membership invariant.
- 2026-08-24: Milestone 2, шаг 3 завершён — metadata-only `list [query]`/`status` с fixed `gopass ls --flat` argv.
- 2026-08-24: Milestone 2, шаг 4 завершён — live path-only labels и отсутствие metadata index/cache закреплены тестами и SECURITY.md.
- 2026-08-24: Milestone 2, шаг 5 завершён — strict symmetric EntryID/path validation и full negative matrix.
- 2026-08-24: Milestone 2, шаг 6 завершён — closed versioned redacted error schema и safe backend error mapping.
- 2026-08-24: Milestone 2, шаг 7 завершён — durable fake-backend tests и isolated synthetic metadata gate подтверждают exact `gopass ls --flat`, отсутствие decrypt и metadata leaks.
- 2026-08-24: финальный mandatory/security gate Milestone 2 — GREEN; high/medium проблем не найдено, два low documentation findings исправлены и повторно проверены.
- 2026-08-24: Milestone 3, шаг 1 завершён — internal exact copy grammar/typed request, strict action/ID/TTL validation и pre-wiring fail-closed behavior.
- 2026-08-24: Milestone 3, шаг 2 завершён — fresh-membership single-entry decrypt передаётся напрямую в sensitive foreground `wl-copy` через `os.Pipe` без secret buffering.
- 2026-08-24: Milestone 3, шаг 3 завершён — единая validated lifecycle policy: acquisition `5..120s` default `30s`, ownership `5..120s` default `30s`, included grace `150ms..2s` default `500ms`, всегда `B > G`.
- 2026-08-25: Milestone 3, шаг 4 завершён — extraction `gopass` и foreground `wl-copy` запускаются в отдельных проверенных process groups; supervisor удерживает PGID и ожидает каждого leader ровно один раз.
- 2026-08-26: Milestone 3, шаг 5d завершён — controller запускает один
  same-helper guardian через inherited pipes, удерживает locked OS thread до
  reap guardian-а, а `Pdeathsig=SIGTERM` переводит controller crash в bounded
  cleanup обеих worker groups без worker `Pdeathsig`.
- 2026-08-26: Milestone 3, шаг 5e завершён — repeated normal/error/deadline и
  concurrent guardian operations прошли synthetic gate; post-start error
  fixture запускал обе stubborn groups, bounded `/proc` scan отфильтрован
  уникальным nonce, orphan processes не обнаружены.
- 2026-08-24: выполнен preflight окружения перед Milestone 1.
- 2026-08-24: шаг 8 завершён — exact build source установленной Noctalia подтверждает поддержку Plugin API 24.
- 2026-08-24: шаг 1 завершён — установлены `gopass` и Go; canonical synthetic fixture создан в `/tmp/zer0-waypass-m1-CVzpfV`.
- 2026-08-24: шаг 2 завершён — `gopass ls --flat` возвращает только path/name и не вызывает GPG backend.
- 2026-08-24: шаг 3 завершён — зафиксированы exact single-entry password/username команды и error matrix в ADR 0002.
- 2026-08-24: шаг 4 завершён — подтверждены foreground ownership, TTL supervision, ownership race и SIGTERM cleanup `wl-copy`.
- 2026-08-24: шаг 5 завершён — isolated sensitive-aware watcher сохранил normal value и отбросил sensitive value.
- 2026-08-24: шаг 6 завершён — воспроизведён актуальный community CI и проверены manifest/API/dependency rules.
- 2026-08-24: шаг 7 завершён — архитектура однозначно фиксирует API 24 argv-table boundary и запрет string-form.
- 2026-08-24: шаг 9 завершён — Go spike подтвердил FD pipe, PGID cleanup и общий acquisition deadline; результат вошёл в ADR 0003.
- 2026-08-24: шаг 10 завершён — guardian parent-death model и isolated real GPG/pinentry cancellation прошли; результат вошёл в ADR 0003.
- 2026-08-24: шаг 11 завершён — policy minimum зафиксирован как Go 1.26, tested version Go 1.26.6.
- 2026-08-24: TTL remediation завершена — TERM в `B-G`, KILL в `B`, bounded reap tolerance; newer clipboard value сохраняется.
- 2026-08-24: финальный независимый mandatory/security gate — GREEN; Milestone 1 завершён.

## Изменённые файлы

- `go.mod` — локальный module path `zer0-waypass`, Go 1.26, без зависимостей.
- `internal/protocol/protocol.go` — versioned list envelope и metadata item `id`/`label`.
- `internal/protocol/protocol_test.go` — exact JSON и structural allowlist tests.
- `docs/PROTOCOL.md` — protocol v1 metadata contract.
- `internal/protocol/id.go`, `id_test.go` — opaque ID format `[0x01] || canonical path` и deterministic tests.
- `docs/decisions/0002-backend-contract.md` — fresh list membership обязателен перед будущим decrypt.
- `cmd/zer0-waypass-helper/` — metadata CLI `list [query]` и `status`, bounded context/output.
- `internal/backend/` — fixed-argv metadata adapter `gopass ls --flat`.
- `docs/SECURITY.md` — metadata threat model, accepted path disclosure и no-index boundary.
- `internal/protocol/id.go`, `id_test.go` — strict encode/decode validation, size/version/UTF-8/path/injection matrix.
- `internal/protocol/error.go`, `error_test.go` — closed error codes/envelope и exact JSON tests.
- `cmd/zer0-waypass-helper/main.go`, `main_test.go` — bounded UTF-8 query validation и durable fake-`gopass` metadata integration tests.
- `README.md`, `docs/decisions/0001-helper-stack.md` — фактическое состояние после Milestone 2 и честная Go toolchain policy.
- `cmd/zer0-waypass-helper/main.go`, `copy_request_test.go` — internal copy request parser и exhaustive grammar/validation matrix.
- `.docs/execplans/zer0-waypass/plan.md`, `docs/PROTOCOL.md`, `docs/SECURITY.md` — безопасная staged decomposition с late public wiring в шаге 8.
- `internal/backend/secret.go`, `secret_test.go` — typed single-entry secret request и exact fixed `gopass show` argv после fresh membership.
- `internal/clipboard/clipboard.go`, `clipboard_test.go`, `docs/CLIPBOARD.md` — internal direct FD pipeline, safe errors и direct-child rollback tests.
- `internal/clipboard/policy.go`, `policy_test.go` — единый source of truth для lifecycle ranges/defaults и strict TTL parser.
- `cmd/zer0-waypass-helper/main.go`, `copy_request_test.go` — делегирование `--ttl` validation общей clipboard policy.
- `internal/backend/secret.go`, `internal/clipboard/clipboard.go`, `clipboard_test.go` — Linux `Setpgid`, проверка `PGID == PID`, distinct group tracking и repeated/concurrent isolation tests.
- `internal/backend/secret.go`, `internal/clipboard/clipboard.go`, `clipboard_test.go` — step 5a single-Wait, TERM/shared-grace/KILL, bounded reap/empty-group verification, PGID invalidation и stubborn-descendant regression.
- `internal/clipboard/guardian.go`, `guardian_test.go` — strict internal
  READY/START/REGISTERED/DONE protocol, inherited FD request, locked-thread
  controller, parent-death guardian и synthetic crash/cancellation tests.
- `cmd/zer0-waypass-helper/main.go` — минимальный hidden same-binary guardian entrypoint; публичный `copy` dispatcher не изменён.
- `.docs/execplans/zer0-waypass/status.md` — добавлены текущее состояние, checklist и результаты preflight.
- `.docs/execplans/zer0-waypass/verification.md` — добавлен фактически выполненный static compatibility check шага 8.
- `docs/decisions/0002-backend-contract.md` — добавлен проверенный backend CLI-контракт `gopass 1.16.1`.
- `.docs/execplans/zer0-waypass/spec.md` — обновлены compatibility facts и conservative TTL contract.
- `.docs/execplans/zer0-waypass/plan.md` — синхронизированы acceptance/validation и TTL budget semantics.
- `docs/ARCHITECTURE.md` — зафиксированы Noctalia argv boundary и принятая clipboard state machine.
- `docs/decisions/0003-clipboard-process-model.md` — принята проверенная MVP process model.
- `docs/decisions/0001-helper-stack.md` — зафиксированы Go 1.26 policy floor, tested version, API inventory и upgrade policy.

## Выполненные проверки

- Milestone 2 step 1: `gofmt`, `go test ./...`, `go vet ./...`, `CGO_ENABLED=0 go test ./internal/protocol`, `go build -trimpath ./...` — PASS; external modules и project ELF отсутствуют.
- Milestone 2 step 2: `gofmt`, `go test ./...`, `go vet ./...`, `CGO_ENABLED=0 go test ./...`, `go build -trimpath ./...` — PASS; raw base64url/version/JSON tests зелёные.
- Milestone 2 step 3: full tests ×10, race tests ×5, `go vet`, static build и runtime fake argv/leak smoke — PASS; metadata execution deadline 10s, stdout cap 8MiB, bounded direct-child reap.
- Milestone 2 step 4: live-label/no-cache/status isolation tests, M2 AST tripwire, full/race tests, vet и static build — PASS.
- Milestone 2 step 5: protocol negative matrix, fail-closed invalid backend path, full/race tests, vet и static build — PASS.
- Milestone 2 step 6: error mapping/leak tests, full/count/race/vet/static gates и runtime missing/failing fake backend smoke — PASS.
- Milestone 2 step 7: query UTF-8/4096-byte boundary, durable temp-PATH fake `gopass`, exact argv/fresh/no-cache/error tests, isolated real `gopass ls --flat` fixture и независимый повторный runtime smoke — PASS; GPG audit пуст, secret extraction не выполнялся.
- Milestone 2 final gate: `gofmt`, full/race/count=10 tests, `go vet`, negative matrix ×25, `CGO_ENABLED=0` static build, independent isolated runtime smoke, no-decrypt/leak/dependency/ELF/orphan scans — PASS.
- Milestone 3 step 1: gofmt, full/targeted-count=10/race tests, vet, `CGO_ENABLED=0` static build и no-shell/show/wl-copy/Pipe/dependency/artifact scans — PASS; повторное independent review — GREEN.
- Milestone 3 step 2: full/race/repeated tests, targeted rollback ×20/race ×10, vet, `CGO_ENABLED=0` static build, exact argv/FD/no-buffer/leak/dependency/artifact/orphan scans — PASS; independent re-review — GREEN.
- Milestone 3 step 3: gofmt, targeted policy/parser ×25, full tests ×10, race, vet, `CGO_ENABLED=0` static build и source/dependency/artifact scans — PASS; independent review — GREEN.
- Milestone 3 step 4: focused process-group tests ×5, full/race tests, vet с `-buildvcs=false`, `CGO_ENABLED=0` static build и root sanity tests ×3 — PASS; independent quick review — GREEN.
- Milestone 3 step 5a implementation gate: stubborn-descendant/shared-grace regression ×5, full tests, backend/clipboard race, vet и ранее static build — PASS; independent review ещё не получен, поэтому пункт остаётся `[~]`.
- Milestone 3 step 5d: focused guardian suite ×3 и focused race — PASS;
  controller SIGKILL crash-case прошёл 3 повтора на запуск, обе stubborn worker
  groups/descendants и guardian исчезли bounded, unrelated process остался жив.
  После review fixes полный `go test ./...`, `go test -race ./...`, `go vet
  ./...` и `CGO_ENABLED=0 go build -trimpath` — PASS.
- Milestone 3 step 5e: normal ×5, post-start backend error ×5 с двумя
  stubborn worker groups, concurrent ×8, acquisition deadline ×3 и repeated
  crash gate; bounded orphan scan с per-run nonce — PASS. После исправления
  fixture race полный `go test ./...`, `go test -race ./...`, `go vet ./...`
  и static build — PASS; final correctness/skeptic/test review — GREEN.
- Pre-install `command -v`: найдены `/usr/bin/gpg`, `/usr/bin/gpg-agent`, `/usr/bin/wl-copy`, `/usr/bin/wl-paste`, `/usr/bin/cliphist`, `/usr/bin/noctalia`, `/usr/bin/qs`; `go` и `gopass` тогда отсутствовали.
- `gpg --version`: `gpg (GnuPG) 2.4.9`.
- `gpg-agent --version`: `gpg-agent (GnuPG) 2.4.9`.
- `wl-copy --version`: `wl-clipboard 2.3.0`.
- `wl-copy --help`: подтверждены `--sensitive`, `--foreground`, `--clear`, `--paste-once`.
- `cliphist version`: `0.7.0`; команда только прочитала версию и текущие пути, history не использовалась.
- `noctalia --version`: `v5.0.0 (v5.0.0-beta.8-142-gc874d65e6824)`.
- `qs --version`: `noctalia-qs 0.0.12`.
- `pacman -Qo /usr/bin/noctalia`: бинарник принадлежит `noctalia-git 5.0.0.r5286.gc874d65e6-1`.
- Pre-install `pacman -Si gopass go`: в локальных sync DB были доступны `gopass 1.16.1-2` (`extra`) и Go `1.26.6` (`extra`/CachyOS).
- `pacman -Q gopass go`: установлены `gopass 1.16.1-2` и Go `2:1.26.6-2`.
- `gopass --version`: `gopass 1.16.1`.
- `go version`: `go1.26.6-X:nodwarf5 linux/amd64`; `CGO_ENABLED=0 go env CGO_ENABLED` вернул `0`.
- Canonical fixture `/tmp/zer0-waypass-m1-CVzpfV`: root и `GNUPGHOME` mode `0700`, одна synthetic GPG identity и одна encrypted entry `synthetic/alice.gpg` mode `0600`.
- `/proc` argv во время `gopass insert`: только `gopass insert --force synthetic/alice`; synthetic marker передан через interactive stdin/write, не через argv/env/plaintext file.
- Isolated `gopass ls --flat`: `synthetic/alice`; это setup smoke и не считается доказательством шага 2.
- Step 2 final probe: `gopass ls --flat` exit `0`, stdout ровно `synthetic/alice\n`, stderr пуст; fail-closed fake GPG invocation log пуст.
- Step 2 positive control: decrypt path вызвал тот же fake GPG и завершился ошибкой без plaintext, что подтверждает работоспособность перехвата.
- Step 3 password argv: `/usr/bin/gopass show --password -- synthetic/alice`; exit `0`, stdout только выбранное synthetic значение без LF, stderr пуст, `/proc` argv не содержит secret.
- Step 3 username argv: `/usr/bin/gopass show -- synthetic/alice username`; exit `0`, stdout только значение поля без LF, stderr пуст и password отсутствует.
- Missing field и missing entry: exit `11`, stdout пуст, stderr без synthetic password/username; production трактует любой non-zero как ошибку.
- Test-only option-like entry с `--` прочитана успешно; production validation всё равно обязана отклонять leading `-`.
- Step 4 final harness: foreground owner остался жив после stdin EOF; TTL `700 ms` сработал за `701 ms`, SIGTERM завершил owner, старое значение стало недоступно.
- Ownership race: A заменён B и завершился раньше TTL; после исходного deadline A значение B сохранилось; `wl-copy --clear` не использовался.
- Supervisor SIGTERM завершил foreground owner; зафиксированные PID отсутствуют. Process groups/Pdeathsig этим тестом не подтверждались.
- Step 5 final harness: exit `0`, isolated cliphist DB mode `0600`, normal count `1`, sensitive count `0`; actual states `data,sensitive`.
- Test watcher audit содержал только `state=data decision=store` и `state=sensitive decision=discard`; clipboard bytes не логировались.
- Community checkout `4f45989f9f470f022627fbfeb7391b7feb5fbf6a`: validator обработал `109` manifests; unit suite — `61` tests, `OK`.
- Актуальные CI-команды: `python3 .github/workflows/scripts/validate-plugins.py` и `python3 -m unittest discover -s .github/workflows/scripts -p 'test_*.py'`.
- Noctalia source `0f61b0ae07607739189d07a0a7617ef0d8f3796c`: supported API `3..28`, direct argv начиная с API 24.
- На проверенных community/official HEAD нет `waypass/` или ID `zer0/waypass`; имя не резервируется этим фактом.
- Pinned official source: string-form `runAsync` формирует `/bin/sh -c`, table-form доступна с Plugin API 24 и идёт в direct argv parser.
- Step 9: direct `os.Pipe` соединил stdout выбранного `gopass` с stdin foreground `wl-copy`; supervisor secret не читал и не буферизовал.
- Timeout/error/start/early-owner regressions выполнены по 3 цикла; обе PGID очищались общим TERM grace и KILL оставшихся descendants.
- Actual acquisition занял около `17–20 ms`, независимый observer видел publication через `20–24 ms`, owner завершён после test TTL `350 ms`.
- `gofmt`, `go vet`, `CGO_ENABLED=0 go build -trimpath` и final leak/orphan scan step 9 прошли.
- Step 10 direct control доказал: один Pdeathsig убивает leader, но оставляет stubborn descendant; direct-only model отклонена.
- Per-operation guardian: `runtime.LockOSThread`, `Pdeathsig=SIGTERM`, READY/START/REGISTERED; normal ×3 и parent SIGKILL ×3 завершили guardian, обе groups и descendants bounded.
- Real isolated gopass/GPG/fake-pinentry GETPIN ×3: acquisition deadline завершил gopass/GPG/pinentry за `872–874 ms`; один и тот же isolated gpg-agent PID остался responsive после каждого run.
- Passphrase передавалась только hidden interactive stdin; persisted leak scan `matches=0`; isolated agent остановлен только после assertions, final orphan scan пуст.
- `go version`: installed `go1.26.6-X:nodwarf5`; package `go 2:1.26.6-2`; Go 1.26 объявлен supported policy floor, а не доказанным theoretical compiler minimum.
- `GOTOOLCHAIN=local CGO_ENABLED=0 go vet` и `go build -trimpath` stdlib/Linux API probe повторно прошли; binary без dynamic interpreter/DT_NEEDED.
- TTL budget proof (`B=500 ms`, `G=150 ms`, tolerance `50 ms`) прошёл 3/3: actual owner исчезал около `351 ms`, stubborn owner принудительно завершался около `501 ms`, newer value сохранялось.
- Финальный scan проекта: executable/ELF/test artifacts отсутствуют; случайный `step5-watcher` перемещён в canonical `/tmp` fixture без overwrite.
- Финальный process scan: активных test/gopass/GPG/pinentry/wl-copy процессов нет.
- `git -C /home/zer0/.cache/paru/clone/noctalia-git/noctalia rev-parse HEAD`: `c874d65e68242d585fbe499e87a96158a41432dd`, совпадает с revision установленного бинарника.
- Static exact-source check `src/scripting/plugin_api.h`: supported Plugin API `3..28`, direct argv введён в API 24.
- Static exact-source check `src/scripting/luau_host.cpp` и `src/core/process/process.{h,cpp}`: string-form использует `/bin/sh -c`, table-form валидируется и запускается direct argv через `execvp`.
- `git status --short`: не выполнен — каталог не является Git working tree.

## Найденные и устранённые проблемы

- Preflight обнаружил отсутствие `gopass` и Go; после разрешения установлены и проверены `gopass 1.16.1-2` и Go `2:1.26.6-2`.
- `cliphist version` сообщил `dconf-CRITICAL` из-за read-only `/run/user/1000/dconf/user`; это не считается результатом clipboard gate.
- Репозиторий не содержит `.git`; Git diff/status недоступны.
- Первый проход исполнителя ошибочно счёл API 24 неподтверждённым; независимое ревью нашло exact build-source cache. Исполнитель перепроверил и исправил вывод.
- Первый fixture `/tmp/zer0-waypass-m1-ofLyEh` отклонён: synthetic marker попал в argv тестового `printf`. Создан новый canonical fixture без этого нарушения; старый помечен `REJECTED` и не используется.
- В предварительной попытке step 2 system-GPG control был запущен до корректной настройки `GOPASS_GPG_BINARY`; исходные артефакты были перезаписаны, поэтому попытка не учитывается как доказательство и не закрывает шаг 3. Использовалась только synthetic запись.
- Первое ревью ADR 0002 нашло два противоречия: option-like probe выглядел разрешённым production input, а fresh membership check был ошибочно исключён. ADR исправлен и повторно принят.
- Первое ревью step 9 нашло early-success `wl-copy` false success и cleanup, оставлявший stubborn descendant после reap лидера. Оба дефекта исправлены и покрыты repeated regression tests.
- Первое ревью step 10 нашло false-positive crash cleanup, противоречие TTL readiness и недостаточный persisted audit trail. Crash test/evidence исправлены; TTL contract затем явно пересмотрен отдельным решением.
- TTL remediation review нашло grace за пределами заявленного budget; `B` изменён так, чтобы включать `G`, и повторный bounded proof прошёл 3/3.
- Финальный gate нашёл случайный test ELF `step5-watcher` в корне; артефакт перемещён в `/tmp`, повторный scan и review зелёные.
- Ревью шага 7 не нашло high/medium проблем; low audit limitation: способ первоначального ввода synthetic secret подтверждён orchestration transcript, но не отдельным persisted redacted setup transcript fixture-а.
- Финальное ревью нашло устаревшие state-описания в `README.md` и ADR 0001; исполнитель исправил оба low finding, reviewer подтвердил закрытие без новых противоречий.
- Первое ревью M3 step 1 нашло MEDIUM: parser-only slice не соответствовал исходной формулировке публичного command. ExecPlan явно декомпозирован: шаги 1–7 остаются internal/fail-closed, public wiring перенесён в шаг 8; повторное ревью закрыло finding.
- Первое ревью M3 step 2 нашло MEDIUM test gap partial-start rollback: не проверялся backend start failure после запуска owner. Добавлен durable regression с PID/reap/no-decrypt assertions; повторное ревью закрыло finding.
- Ревью M3 step 3 не нашло high/medium проблем; числовые ranges признаны conservative product policy, а не универсально доказанными экспериментальными пределами.
- Первые попытки step 4 executor/reviewer долго не показывали активности; один агент завершился infrastructure HTTP 403. Неактивные агенты остановлены, повтор выполнен с `fork_turns=none`; файлов/процессов от неудачных попыток не осталось.
- 2026-08-25: две последовательные попытки executor-а M3 step 5a с `fork_turns=none` не дошли до правок или команд; агенты были остановлены после bounded ожидания. Это infrastructure blocker subagent backend, не установленный design/code blocker; проверки 5a не запускались.
- 2026-08-25: после повторной задержки step 5a реализован локально bounded diff; quick reviewer дважды не вернул verdict в установленный timebox. Код не отмечен завершённым до независимого review.
- 2026-08-25: владелец явно разрешил временно заменить недоступное independent subagent review усиленным self-review; step 5a закрыт после повторной проверки cleanup error propagation и PGID invalidation.
- 2026-08-25: step 5b завершён — acquisition context создаётся до fresh membership/workers, backend success завершает только acquisition phase, cancellation/deadline/owner/backend races разрешаются fail-closed через единый cleanup core.
- 2026-08-25: step 5c завершён — ownership timestamp фиксируется сразу после backend `Wait`, TERM планируется по абсолютной границе `B-G`, KILL входит в `B`, post-KILL verification ограничена 250 ms; новая отдельная owner group не очищается.
- 2026-08-26: step 5e review выявил и исправил test-only race: post-start error
  fixture ждёт оба descendant marker-а, а orphan scan фильтрует текущий
  synthetic nonce. Попытка сигналить непроверенный candidate PGID удалена:
  до успешного `Getpgid` сохраняется только direct-child rollback, чтобы не
  затронуть unrelated process group.
- 2026-08-26: review 5d подтвердил GREEN production lifecycle. Test review
  нашёл assertion gaps: REGISTERED frame дополнен обеими PID/PGID,
  partial-start сделан детерминированным, добавлены substituted regular-FD и
  cancel-at-READY cases, crash argv-log scan; повторные focused/full checks GREEN.
- 2026-08-26: skeptic повторно отметил уже документированный ADR 0003 риск
  numeric PGID reuse после reap leader-а. Это не изменение 5d: retained PGID
  необходим для cleanup stubborn descendants, cleanup выполняется немедленно и
  bounded. Замена на cgroup/другую identity model выходит за scope 5d.

## Решения

- 2026-08-14: проект назван `zer0-waypass`, планируемый plugin ID — `zer0/waypass`.
- 2026-08-14: helper отделён от Noctalia; production helper — Go stdlib-first, Linux-only, `CGO_ENABLED=0`.
- 2026-08-14: поиск использует только имя/путь; username/URL index, `open URL`, `lock`, собственная криптография и sync в MVP исключены.
- 2026-08-14: отсутствие sensitive secret в history гарантируется только для документированной проверенной конфигурации.
- 2026-08-14: безопасный запуск требует argv-table `runAsync`; строковая shell-форма запрещена.
- 2026-08-24: пока установка заблокирована ожиданием разрешения, первым независимым безопасным пунктом выбран шаг 8 — локальная проверка API 24.
- 2026-08-24: установленная Noctalia поддерживает Plugin API 3–28; API 24 доступен, обновление Noctalia для UI не требуется.
- 2026-08-24: string-form `runAsync` остаётся запрещённой; API 24 argv-table проходит validated direct-exec path.
- 2026-08-24: даже synthetic secret запрещено передавать через argv любого процесса; тестовые insert выполняются через interactive stdin/write.
- 2026-08-24: backend list contract — `gopass ls --flat`; результат содержит только entry path/name, GPG decrypt backend при listing не вызывается.
- 2026-08-24: полный copy flow обязан выполнить fresh membership check через `gopass ls --flat`, затем ровно один single-entry decrypt; `--` остаётся defense-in-depth, не заменяя validation.
- 2026-08-24: TTL cleanup завершает только старого foreground owner и не вызывает global `wl-copy --clear`, поэтому новое clipboard value не удаляется.
- 2026-08-24: history guarantee пока подтверждена только для isolated test wrapper, который discard-ит `CLIPBOARD_STATE=sensitive`; это не гарантия для текущего пользовательского watcher или неизвестных managers.
- 2026-08-24: acquisition deadline cleanup — TERM всем retained PGID, единый grace, KILL оставшихся groups, reap и проверка пустых PGID; early clipboard exit всегда ошибка.
- 2026-08-24: configured TTL budget `B` включает kill grace `G`: TERM в `max(0, B-G)`, KILL оставшейся старой group в `B`; после `B` допускается только bounded scheduling/reap verification tolerance.
- 2026-08-24: Milestone 2 закрыт как GREEN; clipboard/copy/process guardian остаются вне его scope и начинаются только в Milestone 3.
- 2026-08-24: публичный `copy` не подключается до GREEN шагов 2–7; pre-wiring любой top-level `copy` безопасно возвращает redacted `invalid_invocation` без backend вызова.
- 2026-08-24: policy defaults/ranges централизованы в `internal/clipboard`; runtime enforcement ownership budget и guardian lifecycle остаются шагами 4–5.
- 2026-08-25: step 4 сохраняет group identity, но не заявляет descendant cleanup; simultaneous TERM/grace/KILL, PGID lifecycle defenses и parent-death guardian остаются строго step 5.

## Будущие риски, не блокирующие Milestone 1

- Community `dependencies` поддерживает metadata внешнего helper, но принятие конкретного Arch-only package не подтверждено maintainers.
- Допустимость bundled Go ELF не подтверждена: validator явно не запрещает ELF, но repository policy требует readable/non-generated code и текущих ELF-примеров нет.
- Не выбран публичный hosting namespace/author identity для будущего PR.
- Runtime synthetic plugin с `plugin_api = 24` и source/unit tests Noctalia остаются проверками будущего UI milestone.
- Строгий полный интервал TTL после publication не обещается; readiness-FD/patched `wl-copy` отложен и потребует нового ADR.

## Исследование блокера TTL

- Official `wl-clipboard v2.3.0` tag commit `67a7b937895bceec1ae5ccebb10216f63f70ca1b` подтверждает: foreground callback не выдаёт readiness наружу; backend EOF предшествует `set_selection`/Wayland roundtrip и не равен успешной publication.
- `wl-paste --list-types` с custom non-secret MIME marker пригоден только для synthetic feasibility spike: он наблюдает offer, но не аутентифицирует конкретного owner и подвержен replacement/clipboard-manager races.
- Non-foreground `wl-copy` имеет observable parent exit после internal roundtrip/fork, но меняет принятую foreground/process model и требует нового полного crash/orphan spike.
- Минимальный вариант MVP: отдельным ADR определить TTL как conservative upper bound от backend success/pipe EOF. Это не увеличивает lifetime секрета, но usable clipboard interval может быть короче настроенного TTL.
- Строгий полный TTL после publication требует readiness FD в pinned/upstream-patched `wl-copy` либо собственного Wayland client; оба варианта меняют dependency/packaging model и требуют отдельного ADR.

## Решение владельца по gate

- 2026-08-24: для завершения Milestone 1 выбран минимальный безопасный MVP-вариант — TTL является верхней границей lifetime старого clipboard owner от backend success/pipe EOF. Полный интервал после publication не обещается.

## Изменения плана

- 2026-08-26: M3 step 6 закрыт — synthetic isolated gpg-agent/pinentry cancellation ×5; pinentry завершён, isolated agent сохранён, общий agent не сигналится.
- 2026-08-26: review gap закрыт post-cancel health roundtrip; correctness/skeptic/test review — GREEN.
- 2026-08-26: M3 step 7 security/source gate GREEN: production tripwire, secret-pipe scan, dependency/ELF/process scans прошли; shell/timeout/`wl-copy --clear` не обнаружены.
- 2026-08-26: M3 step 8 GREEN: public `copy username|password <opaque-id> --ttl <5..120>` dispatch подключён; metadata timeout ограничен list/status, error mapping redacted, dispatcher/action/path/TTL и sentinel matrix покрыты тестами; reviews GREEN.
- 2026-08-26: M3 step 9 GREEN: detached-dispatch contract зафиксирован в `docs/PROTOCOL.md`; caller получает только acceptance/validation semantics, decrypt/clipboard result и secret не возвращаются.
- 2026-08-26: M4 step 2: добавлен `/wp` launcher provider; он отображает только `id`/`label` из versioned helper metadata и не содержит copy/secret actions.
- 2026-08-26: M4 step 3: query запускается только argv-table с callback; manifest debounce и host bounded `runAsync` callback ограничивают быстрые/зависшие запросы, stale query results заменяются только для текущего query.
- 2026-08-26: M4 step 4: metadata entries дают password/username actions; copy запускается detached argv-table без callback с фиксированным TTL 30s и opaque ID.
- 2026-08-26: M4 step 5: launcher показывает только фиксированные состояния helper unavailable/backend error/no entries; raw stdout/stderr и backend details не выводятся.
- 2026-08-26: M4 step 6: source review подтверждает отсутствие state/notify/persistent secret handling; launcher принимает только opaque id/label и статические action/state strings.
- 2026-08-26: M4 step 7: copy actions показывают только «up to 30 seconds»; detached activation не имеет callback/notify и не сообщает успех decrypt или publication.
- 2026-08-26: M5 step 2: IPC toggle `noctalia msg panel-toggle zer0/waypass:waypass` и ручной Hyprland пример `SUPER+P` добавлены; пользовательские configs не изменялись.
- 2026-08-26: M5 step 3: panel при каждом открытии показывает предупреждение о том, что sensitive hint не гарантирует поведение неизвестного clipboard manager.
- 2026-08-26: M5 step 4: добавлен opt-in `examples/cliphist-sensitive-watch.sh`; `CLIPBOARD_STATE=sensitive` отбрасывается, normal data передаётся в `cliphist store`, автоматического подключения нет.
- 2026-08-26: M5 step 5: scope/security scan panel/examples — PASS; URL/TOTP/lock, persistent state writes и automatic config mutation отсутствуют.
- 2026-08-26: M5 step 6: supported normal/sensitive/newer-owner tests и isolated cliphist wrapper smoke — PASS; sensitive fixture не сохраняется, newer owner не удаляется.
- 2026-08-26: M6 step 2: disposable package extraction/removal simulation — PASS; helper/plugin удаляются, synthetic store sentinel остаётся. Real pacman root install не выполнялся: окружение требует root.
- 2026-08-26: M6 step 3: README явно ограничивает packaged target Arch Linux и Arch-compatible CachyOS; native packages для других систем не обещаются.
- 2026-08-26: M6 step 4: добавлена `docs/INSTALL.md` с CGO-free source build и ручной plugin install; support matrix не расширяется.
- 2026-08-26: M6 step 5: локальные community manifests подтверждают формат metadata `dependencies` для внешних команд, но принятие конкретной Arch-only зависимости maintainer-ами не подтверждено; публикация/запрос не выполнялись по инструкции владельца.

- 2026-08-24: TTL уточнён как conservative maximum ownership budget `B` от backend success/pipe EOF, включающий kill grace `G`; это явное изменение первоначальной post-publication semantics.
- Milestone 1 закрыт только после повторного synthetic proof и независимого GREEN gate review.
