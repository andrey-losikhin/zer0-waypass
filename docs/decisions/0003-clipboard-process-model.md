# ADR 0003: Process model получения секрета и владения clipboard

## Статус

Принято для MVP 2026-08-24. Parent-death и real GPG/pinentry gates приняты; это
утверждает process model, которую нужно реализовать и повторно проверить
production-тестами в Milestone 3, а не сам production-код.

`wl-copy 2.3.0` не даёт подтверждённого successful-publication signal. Поэтому
для MVP `--ttl` принят как консервативный максимальный clipboard ownership
budget от backend-success/pipe-EOF, а не обещание полного интервала после
фактической публикации selection. Фактическое доступное окно не превышает
настроенный budget и может быть короче. Kill grace входит в этот budget.

## Проверяемая модель

Один краткоживущий guardian создаётся на каждую copy-operation. Это внутренний
subprocess того же helper binary, а не daemon или общий сервис. Guardian создаёт
`os.Pipe` и запускает без shell два процесса с фиксированными argv:

```text
/usr/bin/gopass show --password -- synthetic/alice
/usr/bin/wl-copy --sensitive --foreground --type text/plain;charset=utf-8
```

В production вместо `synthetic/alice` используется отдельно валидированный и
повторно найденный в `gopass ls --flat` entry. Password не входит в argv,
environment, файл или лог. Write-end pipe назначается непосредственно
`gopass.Stdout`, read-end — `wl-copy.Stdin`. Parent закрывает обе свои копии FD
сразу после соответствующего `Start` и никогда не вызывает `Output`,
`CombinedOutput`, `Read` или пользовательскую буферизацию на secret stream.

Оба непосредственных child guardian-а запускаются с Linux `Setpgid: true`.
Поэтому backend/GPG descendants и clipboard peer имеют разные process groups,
и guardian завершает только принадлежащие текущей операции группы, а не общий
`gpg-agent`. В spike `/proc/<pid>/stat` подтвердил отдельные PGID, равные PID
лидеров; test descendants сохранили PGID своего лидера.

## Parent-death model

`syscall.SysProcAttr.Pdeathsig` относится не к абстрактному parent process, а к
Linux thread, который создал child: сигнал доставляется при смерти именно этого
thread. Поэтому controller вызывает `runtime.LockOSThread` до `Start` guardian-а
и удерживает этот OS thread до завершения guardian-а; unlock сразу после
`Start` или handshake запрещён. Guardian получает `Pdeathsig: SIGTERM`, заранее
устанавливает signal handler и только затем сообщает `READY`. Workers нельзя
запускать до ответного `START`. После каждого успешного `Start` guardian сразу
сохраняет PID/PGID; `REGISTERED` возвращается только когда обе группы находятся
под его контролем. Partial-start обязан пройти тот же bounded cleanup.

В принятом crash-test workers и их stubborn descendants намеренно не имели
собственного `Pdeathsig`: их исчезновение не могло быть побочным эффектом смерти
guardian-а. Для normal и parent-crash путей guardian отдельно записал
`CLEANUP_ENTERED` и `CLEANUP_COMPLETED`; только после обоих событий controller
подтвердил отсутствие всех PID. `Pdeathsig` на worker leader остаётся лишь в
отдельном direct-insufficient control, где его недостаточность и проверялась.

Прямой `Pdeathsig: SIGKILL` для backend/clipboard leader отвергнут. Контрольный
crash-test подтвердил: leader исчезает, но его stubborn descendant той же PGID
остаётся жить. В принятой модели SIGKILL controller-а вызывает `SIGTERM`
guardian-у, а живой guardian выполняет общий TERM/grace/KILL state machine для
всех сохранённых operation PGID и reap-ит непосредственных children. Это
устраняет окно, в котором worker мог бы стартовать до готовности владельца его
cleanup.

Нормальное завершение controller-а также сначала требует остановить операцию и
дождаться guardian-а. Необработанный normal exit controller-а нельзя
использовать как detached-success: kernel воспримет смерть создающего thread как
parent death и guardian fail-closed очистит операцию.

## Deadline и cleanup state machine

Единый acquisition deadline создаётся guardian-ом до pipe и обоих процессов. Он
ограничивает весь путь до успешного завершения backend, включая ожидание
GPG/pinentry. Внешняя команда `timeout` не используется.

Порядок запуска и cleanup:

1. Создать pipe; настроить stdin/stdout до запуска.
2. Запустить `wl-copy` в собственной group и закрыть parent read-end. При его
   start error закрыть write-end; backend не запускать.
3. Запустить `gopass` в собственной group и закрыть parent write-end. При его
   start error завершить clipboard group.
4. Одновременно ждать backend exit, ранний clipboard exit или общий deadline.
5. Backend non-zero: немедленно начать единый cleanup обеих groups, включая
   backend group, даже если её лидер уже reaped.
6. Любой ранний clipboard exit до backend completion, включая exit code `0`,
   является operation error с отдельным sentinel; cleanup охватывает обе
   groups и всех descendants.
7. Cleanup, в том числе после parent death, сначала посылает `SIGTERM` всем
   начатым groups почти одновременно,
   затем ждёт один общий короткий grace period. После него каждая всё ещё
   существующая group получает `SIGKILL`, независимо от состояния её лидера.
   Затем все ещё не reaped лидеры проходят `Wait`, и для каждого сохранённого
   PGID проверяется отсутствие процессов. Последовательный grace на каждую
   group запрещён. После начала ownership budget remaining grace clipboard
   group всегда ограничивается границей `B`; этот общий cleanup не разрешает
   продлевать owner за правило пункта 8.
8. Только backend exit `0` вместе с pipe EOF завершает acquisition и запускает
   clipboard ownership budget `B`. Clipboard owner остаётся под наблюдением.
   Его exit до конца budget после успешного acquisition завершает ownership
   раньше. Kill grace `G` входит в `B`: при обязательном `B > G` guardian
   отправляет TERM retained owner group в `max(0, B-G)`, а на границе `B` —
   KILL любой оставшейся group. После `B` выполняются только bounded reap и
   проверка отсутствия group. Глобальный `wl-copy --clear` не вызывается,
   поэтому не удаляется более новое clipboard ownership.

Race между почти одновременным backend result, clipboard exit и deadline должен
разрешаться fail-closed: после выбранного backend-success события production
реализация обязана ещё проверить канал exit владельца и продолжать наблюдать его
до конца ownership budget. Cleanup должен быть идемпотентным относительно
`ESRCH`, уже закрытых FD и уже reaped лидеров. PGID сохраняется отдельно от
`Process`, так как потомок
может удерживать группу после exit её лидера. Числовой PGID равен PID только что
запущенного лидера; cleanup начинается сразу и bounded. Это сужает окно reuse,
но не является абсолютной защитой от повторного использования numeric PGID;
запрещено откладывать такой cleanup или повторно сигналить сохранённый PGID
после завершения bounded state machine.

## TTL boundary и ограничение `wl-copy`

В проверенном CLI/`--help` `wl-copy 2.3.0` нет документированного readiness API
или подтверждения фактической публикации selection. Поэтому нельзя считать
доказанной несуществующую границу «clipboard готов». Принятая для MVP
консервативная граница ownership budget — успешный exit backend после
закрытия его stdout, то есть момент, когда весь secret уже передан в pipe и
получен EOF. Это ограничивает срок жизни owner, но фактическая публикация может
произойти немного позже и доступное пользователю окно будет короче настроенного
budget. Guardian начинает штатное завершение owner в `max(0, B-G)`; owner обязан
прекратить предоставление secret не позднее границы budget, кроме явно
измеримой scheduler/test tolerance. Для budget `B` и включённого в него kill
grace `G`, где `B > G`, KILL оставшейся group отправляется в `B`. После этого
разрешены только bounded reap/verification. От старта
операции общий hard bound равен acquisition deadline плюс ownership budget плюс
bounded reap/verification tolerance; отдельного grace сверх `B` нет.
Milestone 3 step 3 фиксирует production policy после spike: acquisition
deadline и `B` имеют inclusive range 5–120 s и default 30 s; `G` имеет range
150 ms–2 s и default 500 ms. Любая конфигурация требует `B > G`. Это
консервативный product-policy выбор, а не экспериментальное доказательство
универсально безопасных числовых границ. Минимальный `G` соответствует
проверенному spike, default добавляет cleanup margin; 30 s соответствует
зафиксированному UX proposal. Верхние границы ограничивают pinentry wait и
clipboard exposure. Само runtime-применение timers остаётся Milestone 3 steps
4–5.

На трёх последних реальных isolated запусках backend-success/EOF наблюдался через
19.6–20.2 ms после старта операции. Независимый test-only observer подтвердил
clipboard value сравнением SHA-256 с отдельным чтением той же synthetic entry
через 22.4–24.2 ms после этой границы. Это наблюдение, а не readiness contract.
Production supervisor secret не читал; observer был отдельным test process и не
выводил ни secret, ни digest. Это не является readiness contract и не позволяет
обещать полный интервал после фактической публикации.

Guardian не меняет эту границу и применяет зафиксированную в шаге 9 fail-closed
race/cleanup model с grace внутри ownership budget. Parent crash означает
немедленный cleanup, а не попытку сохранить clipboard до TTL. Глобальный
`wl-copy --clear` по-прежнему запрещён: старый TTL или crash-cleanup не должны
очищать более новое clipboard ownership.

## Отклонённая и отложенная альтернатива

Полный TTL, отсчитываемый после подтверждённой публикации, отклонён для MVP:
у проверенного `wl-copy 2.3.0` нет readiness FD или иного документированного
сигнала такой границы. Patched `wl-copy`, readiness FD и отдельное packaging
решение отложены; их принятие потребует нового ADR и повторной проверки process
model. Придумывать readiness signal на основании задержки observer-а запрещено.

## GPG/pinentry boundary

`gpg-agent` не является потомком operation group. В real spike получено дерево:

```text
gopass (PGID = gopass PID) -> /usr/bin/gpg (тот же PGID)
isolated shared gpg-agent (отдельный существующий PGID) -> test pinentry
```

Поэтому guardian сигналит только отрицательные PGID gopass и `wl-copy`. PID или
PGID `gpg-agent` и pinentry напрямую не сигналятся; запрещены `gpgconf --kill`,
reload и очистка agent cache во время операции. Завершение зависшей operation
group закрывает GPG Assuan request, после чего проверенный GnuPG agent послал
pinentry interrupt и сохранил работоспособность.

Test-only pinentry реализовал достаточный Assuan subset: greeting, ответы на
`OPTION`, `GETINFO`, `SETKEYINFO`, `SETDESC`, `SETPROMPT` и намеренное зависание
без ответа на `GETPIN`. В лог попадали только PID, классы команд, `READY_GETPIN`
и класс сигнала; description, prompt, passphrase и secret не записывались.

Три последовательные decrypt-operation достигли настоящего `/usr/bin/gpg` и
test pinentry, затем deadline очистил gopass/GPG group и pinentry. Во всех трёх
запусках `/proc` показал pinentry с PPID/PGID одного и того же isolated
`gpg-agent` PID `547579`; этот же PID отвечал на `GETINFO pid` после каждой
отмены. После assertions остановлен только этот isolated agent. Пользовательские
`~/.gnupg`, agent, cache и конфиги не использовались и не изменялись.

## Выполненный spike

Test-only stdlib Go исходники и артефакты находятся только в
`/tmp/zer0-waypass-m1-CVzpfV/step9-*` и `step10-*`; это не production helper.
Использовались только canonical isolated `HOME`/XDG/GNUPGHOME/store и synthetic
entries.

Трижды подряд подтверждено:

- `/proc` actual argv и отдельные PGID для `gopass` и `wl-copy`;
- direct FD-to-FD success с настоящими `gopass`, GPG и `wl-copy`;
- owner жил до test TTL 350 ms и завершался на TTL;
- общий 2 s acquisition deadline завершал две группы, в каждой из которых
  лидер и descendant намеренно игнорировали `SIGTERM`; после 150 ms применялся
  `SIGKILL`, все четыре PID исчезали;
- mixed cleanup посылал `SIGTERM` обеим groups до единого grace: оба лидера
  фиксировали exit по `SIGTERM`, оба заранее переведённых в ignore-state
  descendant переживали TERM и удалялись последующим `SIGKILL`; оба PGID после
  cleanup были пусты;
- test clipboard с ранним exit code `0` всегда давал operation-error sentinel;
  зависший backend и переживший clipboard-лидера descendant не оставались;
- backend non-zero с пережившим лидера descendant очищал backend и clipboard
  groups;
- start error каждой стороны закрывал pipe; backend не стартовал после
  clipboard start error, clipboard group не оставалась после backend start
  error;
- formatter, `go vet` и static `CGO_ENABLED=0` build прошли;
- отдельный scan не нашёл synthetic secret в source/stdout/stderr и не нашёл
  записанных или иных `step9-spike` процессов.

Шаг 10 дополнительно подтвердил:

- Linux semantics `Pdeathsig` по creating thread сверена с документацией
  `syscall.SysProcAttr`; spawning path использовал `runtime.LockOSThread`;
- direct-Pdeathsig control действительно оставил stubborn descendant и поэтому
  не был принят как решение;
- guardian normal cleanup три раза и controller `SIGKILL` три раза сохранили
  ожидаемые PID/PPID/PGID и удалили guardian, оба leader и обоих stubborn
  descendants без worker/descendant `Pdeathsig` за bounded 2 s; все шесть
  запусков записали `CLEANUP_ENTERED` и `CLEANUP_COMPLETED`;
- real argv из `/proc`: `/usr/bin/gopass show --password -- synthetic/locked`,
  `/usr/bin/gpg ... --decrypt`, `step10-pinentry --display :0`; passphrase и
  secret отсутствовали;
- real GPG/pinentry acquisition deadline 750 ms прошёл три раза с одним
  responsive isolated agent PID; полный bounded exit занял 872–874 ms, после
  каждого запуска gopass, GPG и pinentry отсутствовали;
- `gofmt`, `GO111MODULE=off go vet` и `GO111MODULE=off CGO_ENABLED=0 go build
  -trimpath` прошли для четырёх test-only программ;
- интерактивный leak scan получил markers только через hidden stdin и сообщил
  `LEAK_SCAN matches=0`; финальный process scan после явного cleanup isolated
  agent не нашёл `step10-*`, synthetic gopass/GPG или pinentry processes.

Дополнительный bounded proof `/tmp/ttl-budget-proof` подтвердил строгую budget
semantics с `B=500 ms`, `G=150 ms` и tolerance `50 ms` в трёх повторах:

- реальный `wl-copy 2.3.0` получил TERM через 350.222–350.596 ms и исчез через
  350.557–351.160 ms;
- stubborn fake owner игнорировал TERM через 350.094–350.169 ms, получил KILL
  через 500.217–500.352 ms и исчез через 500.601–500.927 ms;
- retained старая group игнорировала TERM и получила KILL, при этом более новое
  synthetic clipboard value сохранялось; проверка завершалась через
  4.235–4.885 ms;
- все process groups были пусты в пределах `B + 50 ms`; использовались fixed
  argv без shell, случайные synthetic values, три повтора, `gofmt`, `go vet` и
  static `CGO_ENABLED=0` build. Глобальный clear и пользовательская history не
  использовались.

## Ограничения подтверждения

- Не заявляется защита от одновременного `SIGKILL` controller-а и guardian-а,
  kernel crash, power loss или adversarial PID/PGID reuse.
- Guardian — per-operation subprocess, не daemon; его собственный `SIGKILL`
  обходит cleanup. Это остаётся явно документированной Linux boundary.
- Проверен установленный GnuPG `2.4.9` и test pinentry, который зависает на
  `GETPIN`, но корректно завершается по agent interrupt. Произвольный сломанный
  pinentry, игнорирующий agent protocol и все сигналы, не покрыт.
- Проверена невмешивающаяся отмена на одном isolated agent, shared между тремя
  test operations. Это доказывает отсутствие необходимости убивать/очищать
  agent в выбранной модели, но не разрешает тесты на пользовательском agent.
- `wl-copy` не имеет подтверждённого readiness API; принятая консервативная
  ownership-budget boundary шага 9 остаётся в силе.
- Окончательная минимальная версия Go относится к шагу 11.
