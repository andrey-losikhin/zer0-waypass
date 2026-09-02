# Verification Plan: zer0-waypass

## 2026-08-31 — field-card Gate 0

Gate 1 принят после явного изменения visibility boundary владельцем: public
values разрешены в ephemeral helper JSON/Noctalia; secret values запрещены.
Нормативный формат — `docs/FIELD-CONTRACT.md`, wire surface — protocol v2.

Gate 1 verification:

- `go test ./...` — PASS;
- `go test -race ./...` — PASS;
- `go vet ./...` — PASS;
- `CGO_ENABLED=0 go build -trimpath -o /tmp/zer0-waypass-helper ...` — PASS;
- `noctalia plugins lint noctalia-plugin/waypass` — 0 errors, 0 warnings;
- source/dependency/artifact/process scans — без новых production нарушений;
- correctness/skeptic/test review: stale guardian resolve, manifest digest,
  bounded/aggregate public reads, visibility docs и CLI tests исправлены.
- Legacy follow-up: unit test подтвердил visible username через exact field
  query и отсутствие full `show --noparsing` пользовательской legacy entry;
  focused backend/helper/clipboard tests, vet и Noctalia lint — PASS.
- Session follow-up: Noctalia source lint проверяет 3-minute in-memory restore;
  stale entry/field восстанавливаются только после свежих helper responses,
  public/secret values в remembered state отсутствуют.

- Окружение: Noctalia `v5.0.0-beta.8-142-gc874d65e6824`, `gopass 1.16.1`, Go
  `1.26.6`; Git worktree отсутствует.
- Fixture: isolated synthetic `GOPASS_HOMEDIR` и plain crypto store в `/tmp`;
  пользовательский store/GNUPGHOME не использовались.
- `show --help`, `ls --help`, `templates --help` и bounded survey help для
  `find`, `grep`, `cat`, `process`: list-only field-name API не найден;
  `ls --flat` вернул только entry path. Остальные команды либо показывают/
  ищут decrypted content, либо обрабатывают внешний template.
- Password и username/url/notes/custom field extraction выполняются отдельными
  fixed argv. Missing/empty field: non-zero и пустой stdout.
- Duplicate custom field: success, два значения и две stdout lines — один raw
  name не выбирает экземпляр.
- Indented continuation не вошёл в notes; `private_key: |` вернул только `|`;
  свободный body не адресуется.
- Full `show` и `--noparsing`: 270 bytes/12 lines всей synthetic записи; это
  единственный проверенный источник фактических field names и потому запрещён
  для helper parsing.
- Перед и после spike orphan helper/gopass/wl-copy/pinentry не обнаружены.
- Итог: security gate FAIL для production card; документация/ADR обновлены,
  protocol v1 и production code не изменены.
- Independent correctness/skeptic review: high findings нет; medium TOCTOU
  future-schema mapping исправлен schema-bound field ID и повторной exact
  revision/mapping проверкой; два low documentation overclaim/staleness также
  исправлены. Новые reviewers не создавались.

## Обязательные gate-проверки

- Unit: parser, validation, metadata allowlist (`id`, `label`), TTL bounds, error mapping.
- Integration: synthetic GPG store, backend adapter, clipboard ownership/TTL, isolated cliphist DB.
- E2E: Noctalia search -> helper copy -> manual paste -> expiry, только на synthetic secret.
- Lint/format/build: `gofmt`, `go test`, `go vet`, Go build без cgo и Luau/community validation.
- Security: secret отсутствует в argv, stdout, stderr, logs, Noctalia state/settings и notifications; в history — для явно поддерживаемой clipboard-конфигурации.
- Review: отдельный code review helper boundary и plugin boundary.
- Skeptic review: перед release candidate оспорить threat model, race conditions и packaging assumptions.

## Запрещённые тестовые данные

- Реальный password store пользователя.
- Реальные usernames, домены компании, API keys и recovery codes.
- Копии пользовательской clipboard history.

Использовать fixtures вида `example.test`, `alice@example.test`, `SYNTHETIC_SECRET_DO_NOT_USE`.

## Проверка по milestone

### Milestone 1

- Обязательно: версии инструментов; isolated GNUPGHOME; synthetic store; process argv inspection; captured stdout/stderr; isolated cliphist DB; TTL/newer-clipboard race smoke; Noctalia/community compatibility notes.
- Обязательно: Go process spike — FD pipe, process groups, acquisition deadline, `Pdeathsig`/`LockOSThread`, parent crash и отмена pinentry без остановки общего `gpg-agent`.

#### Выполнено

- 2026-08-24 — preflight версий: Noctalia `v5.0.0-beta.8-142-gc874d65e6824`, GnuPG/GPG-agent `2.4.9`, wl-clipboard `2.3.0`, cliphist `0.7.0`; `go` и `gopass` отсутствуют.
- 2026-08-24 — `pacman -Si gopass go`: локальные sync DB предлагают `gopass 1.16.1-2` и Go `1.26.6`; установка не выполнялась.
- 2026-08-24 — static compatibility check exact build source Noctalia commit `c874d65e68242d585fbe499e87a96158a41432dd`: supported Plugin API `3..28`, direct argv требует API 24; string-form `runAsync` использует `/bin/sh -c`, table-form идёт через validated argv и direct `execvp`.
- 2026-08-24 — установлены и проверены `gopass 1.16.1-2` и Go `2:1.26.6-2`; `CGO_ENABLED=0 go env CGO_ENABLED` вернул `0`.
- 2026-08-24 — canonical isolated fixture `/tmp/zer0-waypass-m1-CVzpfV`: отдельные HOME/XDG/GNUPGHOME, permissions `0700`, synthetic key и одна encrypted entry `0600`; secret marker передан только через interactive stdin, `/proc` argv проверен.
- 2026-08-24 — шаг 2: `gopass ls --flat` exit `0`, stdout exact `synthetic/alice\n`, stderr empty; fail-closed `GOPASS_GPG_BINARY` positive control сработал на decrypt command, а listing завершился без единого вызова GPG wrapper.
- 2026-08-24 — шаг 3: fixed-argv password/username success matrix, missing-field/missing-entry errors, option-separator probe, `/proc` argv inspection и single-entry GPG decrypt trace выполнены на isolated synthetic entry; raw outputs остались только в `/tmp` mode `0600`.
- 2026-08-24 — шаг 4: Go compatibility harness без shell/external timeout проверил `wl-copy --sensitive --foreground`, owner-after-EOF, TTL SIGTERM, A→B ownership replacement/newer-value preservation и handled supervisor SIGTERM; final `wl-paste` подтвердил отсутствие selection, все recorded PID завершены.
- 2026-08-24 — шаг 5: test-only Go watcher получил `CLIPBOARD_STATE=data,sensitive`; normal control записан в isolated cliphist DB, sensitive fixture отсутствует в `cliphist list` и raw DB; DB/config/audit mode `0600`, final harness exit `0`.
- 2026-08-24 — шаг 6: на official community commit `4f45989f9f470f022627fbfeb7391b7feb5fbf6a` validator прошёл для 109 manifests; 61 unit test — `OK`; HEAD перепроверен через `git ls-remote`. Manifest/dependency rules, отсутствие ID conflict и official API 3..28/direct argv 24 сверены с pinned primary sources.
- 2026-08-24 — шаг 7: documentation/source review подтвердил `plugin_api >= 24`, string → `/bin/sh -c`, table → validated direct argv; `docs/ARCHITECTURE.md` дополнена явным требованием fixed argv-table и сохранением ID/membership validation в helper-е.
- 2026-08-24 — шаг 9: stdlib Go spike ×3 проверил fixed argv, direct FD pipe, separate PGID, deadline before start, group-aware TERM/grace/KILL, mixed stubborn descendants, backend/start/early-owner errors и actual synthetic `gopass -> wl-copy`; gofmt/vet/static build/leak scan прошли.
- 2026-08-24 — шаг 10: direct-Pdeathsig negative control, per-operation guardian normal/crash ×3, persisted cleanup events, real isolated gopass/GPG/fake-pinentry GETPIN deadline ×3, same-agent liveness, hidden-stdin leak scan и final orphan scan выполнены. Parent-death/pinentry gates зелёные.
- 2026-08-24 — шаг 11: Go 1.26 policy floor / Go 1.26.6 tested toolchain зафиксированы; `GOTOOLCHAIN=local CGO_ENABLED=0 go vet` и `go build -trimpath` API probe прошли, binary статический.
- 2026-08-24 — TTL remediation: при `B=500 ms`, `G=150 ms`, tolerance `50 ms` actual `wl-copy`, stubborn owner и newer-value race прошли 3/3; TERM выполнялся в `B-G`, KILL в `B`, retained groups исчезли в пределах tolerance.
- 2026-08-26 — M3 step 6: synthetic isolated agent/pinentry cancellation ×5; pinentry reaped, agent прошёл post-cancel health marker и остался жив до явного cleanup; общий `gpg-agent` не затрагивался.
- 2026-08-26 — M3 step 7: production persistence/exec tripwire и secret-pipe scan — PASS; external shell/timeout, command substitution, `wl-copy --clear`, новые dependencies, ELF artifacts и orphan processes не обнаружены.
- 2026-08-26 — M3 step 8: public copy dispatcher подключён после валидации opaque ID/action/TTL; username/password, decoded path, TTL и redacted error matrix покрыты focused tests; metadata timeout не ограничивает copy policy.
- 2026-08-26 — M3 step 9: detached-dispatch contract зафиксирован; caller видит только acceptance/validation и bounded budget, не decrypt/clipboard result и не secret.

#### Gate result

- **GREEN:** все acceptance criteria и mandatory validations Milestone 1 подтверждены; high/medium findings закрыты.
- 2026-08-24 — source review `wl-clipboard v2.3.0` подтвердил отсутствие foreground readiness; поэтому принят conservative budget contract, а full post-publication TTL отложен.
- 2026-08-24 — финальный independent review: project executable/ELF scan пуст, активных test/backend/clipboard/GPG/pinentry процессов нет, synthetic artifacts находятся только в `/tmp`.
- Deferred до Milestone 4: source/unit tests Noctalia и runtime synthetic plugin load/call с `plugin_api = 24`.

### Milestone 2 — metadata/backend

- Обязательно: formatter/linter/tests; negative input matrix; no-shell check; leak scan; `id`/`label` allowlist; доказательство отсутствия массовой расшифровки.
- Опционально: sanitizer/fuzz/property tests parser-а и protocol decoder-а.

#### Выполнено

- 2026-08-24 — шаг 1: protocol v1 list envelope; item closed allowlist `id`/`label`; exact one/empty JSON tests (`[]`, не `null`) и reflection guard. `gofmt`, `go test ./...`, `go vet ./...`, `CGO_ENABLED=0 go test ./internal/protocol`, build — PASS.
- 2026-08-24 — шаг 2: ID format `base64.RawURLEncoding([0x01] || canonical UTF-8 path)` с hardcoded vector, alphabet/no-padding/version/JSON tests; fresh exact `gopass ls --flat` membership invariant зафиксирован, decoder/copy не реализованы.
- 2026-08-24 — шаг 3: `list [query]`/`status`, exact `gopass ls --flat` argv, literal in-Go query, closed JSON, generic leak-safe failures, 10s deadline/8MiB cap/direct-child cancellation. Full/race/lifecycle tests и static build — PASS.
- 2026-08-24 — шаг 4: sequential changing-backend tests подтверждают fresh live labels/no cache; query не использует username/URL/custom fields, status не раскрывает paths; production persistence/exec tripwire и SECURITY review зелёные.
- 2026-08-24 — шаг 5: strict RawURL/canonical EntryID validation; invalid base64/version/UTF-8/size/absolute/traversal/leading-dash/shell/control/backslash matrix; encoder/decoder symmetry и invalid-backend fail-closed behavior. Full/race/vet/static gates — PASS.
- 2026-08-24 — шаг 6: closed error envelope/codes, safe sentinel mapping, exact exit/stdout/stderr behavior, raw marker/path/query leak tests и runtime missing/failing backend smokes. Full/race/vet/static gates — PASS.
- 2026-08-24 — шаг 7: query valid UTF-8/maximum 4096 bytes проверяется до backend; durable temp-PATH fake executable принимает только exact `gopass ls --flat` и покрывает list/query/status/fresh/no-cache/redacted failures. Full tests, race, `-count=10`, vet, gofmt и `CGO_ENABLED=0` static build — PASS.
- 2026-08-24 — isolated metadata gate `/tmp/zer0-waypass-m2-fAx4Sy`: два synthetic encrypted entries, list/query/status exact closed JSON, ID decode/roundtrip, stderr empty, argv audit содержит только `ls --flat`, fail-closed GPG audit пуст; `gopass show`/decrypt не выполнялись. Независимый reviewer повторил gate и отдельный isolated runtime smoke — PASS.

#### Gate result

- **GREEN:** все acceptance criteria и mandatory validations Milestone 2 подтверждены; high/medium findings отсутствуют.
- 2026-08-24 — финальный independent review повторил `gofmt`, full/race/count=10 tests, `go vet`, negative matrix ×25, `CGO_ENABLED=0` static build, fixed-argv/no-shell/no-decrypt scans, isolated runtime smoke и leak/dependency/ELF/orphan checks — PASS.
- Два low documentation findings в `README.md` и ADR 0001 исправлены и точечно повторно приняты reviewer-ом; Go-код после финального gate не менялся.
- Optional fuzz не запускался. Git diff/unrelated check недоступен: рабочее Git metadata отсутствует. Persisted fixture не содержит отдельного redacted transcript первоначального insert; безопасный stdin setup подтверждён orchestration transcript.

### Milestone 3 — clipboard owner

- Обязательно: acquisition timeout + TTL/process-tree/signal/ownership race; no orphan gopass/GPG/pinentry/wl-copy; parent-death crash; newer clipboard preserved; leak scan; supported cliphist check.

#### Выполнено

- 2026-08-24 — шаг 1: internal typed parser exact grammar `copy <username|password> <id> --ttl <seconds>`; strict action allowlist, EntryID decode и positive ASCII decimal TTL representable as `time.Duration`. До late wiring шага 8 публичный `copy` fail-closed возвращает `invalid_invocation`, stdout пуст, backend не вызывается. Full/targeted-count=10/race tests, gofmt, vet, `CGO_ENABLED=0` static build и static scans — PASS; independent re-review — GREEN.
- 2026-08-24 — шаг 2: internal copy-operation делает fresh exact membership/all-path validation, затем ровно один fixed-argv `gopass show` и прямой `os.Pipe` в `wl-copy --sensitive --foreground --type text/plain;charset=utf-8`; production supervisor не читает и не буферизует secret. Full/race/repeated gates, partial-start rollback ×20/race ×10, exact argv/FD/leak/dependency/artifact/orphan scans — PASS; independent re-review — GREEN.
- 2026-08-24 — шаг 3: centralized lifecycle policy — acquisition `5..120s`/default `30s`, ownership budget `5..120s`/default `30s`, included grace `150ms..2s`/default `500ms`, invariant `B > G`; strict ASCII-seconds parser используется CLI без duplicate magic ranges. Targeted ×25, full ×10, race, gofmt, vet, `CGO_ENABLED=0` static build и scans — PASS; independent review — GREEN. Runtime timers/supervision этим шагом не заявлены.
- 2026-08-25 — шаг 4: `gopass show` и `wl-copy` получают `Setpgid=true`; после Start проверяется `PGID == leader PID`, группы distinct и сохраняются до единственного `Wait` каждого direct child. Runtime tests подтверждают repeated/concurrent isolation. Focused ×5, full/race, vet `-buildvcs=false`, `CGO_ENABLED=0` static build и root focused sanity ×3 — PASS; independent quick review — GREEN. TERM/KILL/guardian cleanup этим шагом не заявлены.
- 2026-08-25 — шаг 5a: retained groups получают TERM back-to-back, один shared grace, KILL оставшихся, single-Wait и bounded empty-group verification; numeric PGID инвалидируется после pass. Fake leaders/descendants игнорируют TERM и исчезают после KILL; measured cleanup подтверждает один, не последовательный grace. Focused ×5, `go test ./...`, backend/clipboard race и `go vet ./...` с `-buildvcs=false` — PASS. По явному разрешению владельца independent review временно заменён усиленным self-review — GREEN.
- 2026-08-25 — шаг 5b: default acquisition deadline создаётся до `PrepareSecret`/pipe/workers; external handled cancellation и internal deadline используют тот же group cleanup. Backend success проверяет уже observable owner exit до перехода к ownership phase; deadline/backend/owner races fail-closed. Deadline probe, timeout/cancel/stubborn-descendant/error matrix ×3, full/race tests, vet и `CGO_ENABLED=0` static build — PASS; усиленный self-review по разрешению владельца — GREEN.
- 2026-08-25 — шаг 5c: minimum production policy `B=5s`, `G=150ms` проверена на stubborn old owner: TERM наблюдался в `B-G ±150ms`, bounded return — в `B..B+250ms`; отдельная newer-owner PGID осталась живой до явного test cleanup. Full tests, backend/clipboard race, vet, static `CGO_ENABLED=0` build, no-shell/no-global-clear/artifact/process scans — PASS; усиленный self-review — GREEN.
- 2026-08-26 — шаг 5d: same-helper guardian запускается только с hidden fixed
  argv и inherited anonymous pipes; controller удерживает `LockOSThread` от
  `Start` до guardian `Wait`, guardian устанавливает SIGTERM handler до READY,
  принимает один strict START и сообщает REGISTERED с обеими проверенными
  PID/PGID. Workers не имеют `Pdeathsig`. Focused suite ×3, focused race,
  cancel-at-READY ×10, invalid/missing FD и deterministic partial-start — PASS.
  Controller SIGKILL ×3 удалил guardian, обе stubborn groups и descendants за
  <2s в каждом повторе; отдельный unrelated process сохранился. Guardian argv и
  controller/guardian stdout/stderr не содержали entry path/secret; fake argv
  audit не содержал secret. После review fixes: full tests, full race, vet и
  static build — PASS; correctness review GREEN, test findings исправлены.
  Step 5e, real pinentry gate и public dispatcher остаются незавершёнными.
- 2026-08-26 — шаг 5e: repeated normal ×5, post-start backend-error ×5 с
  stubborn descendants обеих worker groups, concurrent ×8 и acquisition
  deadline ×3 прошли synthetic lifecycle gate. `/proc` orphan scan учитывал
  уникальный per-run nonce и ожидал bounded PID markers; repeated crash-test
  оставался GREEN. Full tests, full race, vet и static build — PASS; final
  review не оставил подтверждённых high/medium проблем в scope 5e. Следующий
  незавершённый шаг — M3/6 pinentry.

### Milestone 4 — launcher

- Обязательно: `plugin_api = 24`; argv-table only; manifest/translations/community lint; path-source query/copy/disable/reload.
- Опционально: visual snapshot.

### Milestone 5 — panel/session

- Обязательно: panel keyboard smoke; first-run warning; normal/sensitive/newer-value tests; opt-in config examples.
- Опционально: smoke с другим sensitive-aware manager.

### Milestone 6 — packaging

- Обязательно: clean Arch build; checksums; package inspection; disposable install/uninstall; fresh-user E2E; community CI; security/skeptic review.
- Опционально: source-build smoke на второй системе без обещания package support.

### Milestone 7 — optional sync

- Обязательно: two-clone local remote tests; conflict/no-force behavior; backup/recovery; offline/failure scenarios.
- Опционально: metadata leakage assessment для opaque filenames.

## Ручная security matrix

| Сценарий | Ожидание |
| --- | --- |
| Copy password | Значение доступно не более budget `B`; фактическое окно может быть короче |
| Budget `B` истёк | Старый owner получает KILL; после bounded tolerance secret больше не вставляется |
| После secret скопирован обычный текст | Обычный текст остаётся после старого TTL |
| Открыта history в поддерживаемой cliphist-конфигурации | Secret отсутствует |
| Неизвестный clipboard manager | UI/документация предупреждают, абсолютной гарантии нет |
| Helper получает `../`, `--help`, shell metacharacters | Запрос отклонён как ID |
| Backend требует passphrase | Pinentry управляется gpg-agent, UI не получает passphrase |
| Pinentry не закрыт до acquisition deadline | Copy-operation завершается; общий `gpg-agent` остаётся жив |
| Helper/Noctalia restart | На диске нет decrypted cache проекта |
| Helper получает SIGTERM | Дочерний wl-copy завершается |
| Plugin вызывает helper | Используется argv-table API 24+, не `/bin/sh -c` |
| Debug/logging включены | Secrets всё равно редактируются или не логируются |

## Команды, подтверждённые текущим окружением

```sh
noctalia --version
noctalia config validate
gpg --version
wl-copy --version
gopass --version
go version
```

`cliphist version` и isolated cliphist checks выполняются только если `cliphist`
установлен. Сам `cliphist` не является зависимостью Waypass.

После появления helper обязательны:

```sh
test -z "$(gofmt -l cmd internal)"
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath ./cmd/zer0-waypass-helper
```

Минимальная поддерживаемая policy-версия — Go 1.26; проверено на Go 1.26.6.

### 2026-08-27 — Milestone 5 panel UX refinement

- `noctalia --version`: `v5.0.0-beta.8-142-gc874d65e6824`; helper `status` — backend ready.
- Локальные official/community plugins и exact Noctalia source подтвердили
  `ui.glyph`, `ui.input(focus/flexGrow)`, `ui.scroll`, `keyboard_focus`,
  `capture_keys`, `onKey` и boolean result callback-form `runAsync`.
- `noctalia plugins lint noctalia-plugin/waypass`: `0 errors, 0 warnings`.
- `noctalia config validate`: valid; 7 unrelated warnings пользовательской конфигурации.
- Static scan: copy/query используют только argv-table; string-form `runAsync`,
  shell, notify/state/settings secret handling, raw backend errors и panel
  clipboard warning отсутствуют.
- Materialized source синхронизирован; plugin disable/enable/panel-toggle — `ok`.
- Independent correctness/skeptic review: stale visible results и saturation
  callback race исправлены; high findings закрыты.
- Ограничение host API: `ui.scroll` не экспортирует scroll-to-selected. Wheel
  scroll работает, но автоматическое удержание keyboard selection в viewport
  на длинном списке требует будущего Noctalia API или windowed-list UX.

### 2026-08-27 — Milestone 5 three-minute panel session

- Exact installed-source check подтвердил `noctalia.nowMs()`; TTL установлен в
  `180000 ms` и проверяется при каждом `onOpen`.
- В памяти plugin host сохраняются только query, opaque entry ID и timestamp;
  username/password/URL не читаются, settings/state/cache не используются.
- Восстановленный ID выбирается только после свежего `helper list`; если ID
  отсутствует, выбирается первая актуальная запись.
- Кнопка `Back to list`, `Backspace` и `Escape` вызывают общий обработчик
  возврата из карточки к списку.
- `noctalia plugins lint noctalia-plugin/waypass`: `0 errors, 0 warnings`.
- Static scan: persistent Noctalia state/settings и string-form `runAsync`
  отсутствуют; helper и clipboard lifecycle не изменены.
Актуальные community validation commands берутся из checkout community repo, а
не предполагаются по памяти.

## Definition of Done

- Все milestones MVP 1-6 закрыты; Milestone 7 явно остаётся optional/follow-up.
- Все обязательные проверки зелёные.
- Нет открытых high/critical security findings.
- Secret не проходит через Noctalia runtime для copy-action.
- Sensitive clipboard не сохраняется в истории документированной поддерживаемой конфигурации.
- Пакет не изменяет и не удаляет пользовательский password store.
- Документация объясняет trust model, TTL, cliphist и GPG-agent assumptions.
- Community artifact соответствует актуальному Plugin API и CI.
- Community README честно ограничивает packaged support Arch Linux.
