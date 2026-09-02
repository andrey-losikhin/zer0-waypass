# ADR 0001: Go для `zer0-waypass-helper`

## Статус

Принято 2026-08-14; version policy уточнена 2026-08-24. Этот ADR принимает
только выбор toolchain, не clipboard/process model целиком.

## Решение

Helper реализуется на Go для Linux/Wayland, stdlib-first и без cgo. Runtime
пакета зависит только от `gopass`, `gnupg` и `wl-clipboard`; Go является только
build dependency.

Минимальная **поддерживаемая версия build environment для MVP — Go 1.26**.
Проверенная версия — upstream Go 1.26.6 в пакете CachyOS
`go 2:1.26.6-2`; `go version` возвращает нестандартную сборку
`go1.26.6-X:nodwarf5 linux/amd64`. Префикс `2:` — epoch пакета, `-2` — pkgrel,
а `-X:nodwarf5` — суффикс дистрибутивной toolchain; они не меняют upstream
версию Go 1.26.6. Локальная metadata Arch Extra (`pacman -Si extra/go`) на
момент проверки предлагала `2:1.26.6-1`, где `-1` также является pkgrel.
Онлайн-каталог Arch Extra на ту же дату уже показывал `2:1.27.0-1`, поэтому
вывод `pacman -Si` здесь является точным локальным sync snapshot, а не заявкой
на актуальность mirror metadata.

Это policy floor, а не доказанный теоретический compiler floor. Go-модуль с
директивой `go 1.26` и metadata/backend helper Milestone 2 уже реализованы, но
сборки более старыми toolchain не выполнялись. Поэтому версии ниже 1.26
считаются неподдерживаемыми и непроверенными; утверждение, что они технически не
смогут собрать helper, не делается. Go 1.26 выбран как нижняя
поддерживаемая upstream ветка на дату решения: после выхода Go 1.27 официальный
release policy продолжает поддерживать две последние major-ветки.

Основные API: `os/exec`, `os.Pipe`, `context`, `time`, `os/signal`,
`encoding/json`, `encoding/base64`, `path`, `strings`, `testing` и Linux process
control через `syscall.SysProcAttr`.

Проверенный stdlib/Linux API inventory process spike для Go 1.26.6:

- `exec.Command`, `Cmd.Start`/`Wait`, stdin/stdout wiring и `os.Pipe` — fixed
  argv и прямой FD-to-FD pipe без shell;
- `context.WithTimeout`, timers и `os/signal` — общий deadline и сигналы;
- `syscall.SysProcAttr{Setpgid, Pdeathsig}` и `syscall.Kill` с отрицательным
  PGID — Linux process groups, parent-death signal и TERM/KILL cleanup;
- `runtime.LockOSThread` — выполнение Linux `Pdeathsig` setup с учётом привязки
  parent-death signal к создавшему thread;
- `encoding/json`, `encoding/base64`, `path`, `strings` и `testing` —
  запланированные protocol, ID и test primitives.

Все перечисленные символы доступны в установленной Go 1.26.6 toolchain;
минимальный stdlib probe собирается и проходит `go vet` при `CGO_ENABLED=0`.
Это проверка доступности API, а не production implementation и не основание
понижать policy floor.

Все release/package builds обязательны с `CGO_ENABLED=0`. В `go.mod` при его
появлении фиксируется `go 1.26`; CI и Arch package должны собирать актуальным
security patch поддерживаемой ветки и записывать точные результаты `go version`
и package version. Patch-обновления применяются без изменения ADR после gates;
переход policy floor на новую major-ветку требует повторных build/vet/tests и
обновления этого ADR, `go.mod` и packaging metadata.

Первичные источники: [Go release history и policy](https://go.dev/doc/devel/release),
[семантика версий toolchain](https://go.dev/doc/toolchain),
[официальный пакет Arch Extra](https://archlinux.org/packages/extra/x86_64/go/).

## Почему

- `os/exec` запускает fixed argv без shell.
- Стандартной библиотеки достаточно для CLI, JSON, pipe, deadlines и тестов.
- Один binary без cgo упрощает Arch packaging.
- По сравнению с Rust для этого узкого helper-а требуется меньше сторонних
  библиотек и меньше supply-chain surface.

## Ограничения и обязательная проверка

Linux parent-death signal связан с создавшим thread. Реализация с
`runtime.LockOSThread`, process groups и аварийным завершением должна пройти
отдельный crash-тест. Также нужно доказать, что acquisition deadline отменяет
gopass/GPG/pinentry, не завершая и не очищая общий `gpg-agent`.

Compatibility spike шагов 9–10 выполнен на указанной toolchain; его выводы и
ограничения принадлежат ADR 0003 и должны быть повторены на production-коде.
Этот ADR принимает только выбор toolchain и его version policy. Он не принимает
clipboard/process model целиком. Строгий post-publication readiness/full-TTL
contract отложен, а не является blocker Milestone 1: для MVP ADR 0003 принимает
консервативный ownership budget от backend-success/pipe-EOF.

## Не добавлять без нового ADR

Cobra/Viper, logging/DI frameworks, `x/sys`, gopass SDK, Wayland/GPG libraries,
daemon, БД и собственную криптографию.
