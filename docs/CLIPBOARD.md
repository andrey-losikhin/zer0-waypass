# Clipboard pipeline

## Milestone 3 internal clipboard boundary and policy

The internal copy operation performs a fresh `gopass ls --flat`, validates
every returned path, and requires an exact match for the already validated
entry path. Only then may it start one of these fixed-argv extractions:

```text
gopass show --password -- <entry>
gopass show -- <entry> username
```

The operation creates an `os.Pipe`. Its write end is assigned directly to
`gopass.Stdout` and its read end directly to `wl-copy.Stdin`. The clipboard
owner argv is exactly:

```text
wl-copy --sensitive --foreground --trim-newline --type text/plain;charset=utf-8
```

Production Go code does not read, convert, buffer, log, persist, or return the
secret stream. Child stdout/stderr other than the direct pipe endpoints are
disconnected, and process failures are replaced with closed safe sentinels.

This remains an internal stage and is not connected to the public CLI
dispatcher. Steps 4–5d add separate process groups, descendant cleanup,
acquisition cancellation, the ownership budget and one short-lived same-helper
guardian per operation. The controller starts it with a fixed hidden argv,
passes the non-secret request through inherited pipes and holds
`runtime.LockOSThread` through guardian exit. Guardian installs its SIGTERM
handler before READY; workers start only after START; REGISTERED is emitted
only with both validated PID/PGID pairs retained. Controller SIGKILL therefore
causes guardian cleanup of both worker groups and stubborn descendants. Step 5e
now covers repeated normal/error/deadline/concurrent/crash operations; pinentry
cancellation and public `copy` dispatch remain later Milestone 3 steps.

Step 3 defines a fail-closed lifecycle policy in `internal/clipboard`:

| Phase/value | Inclusive minimum | Default | Inclusive maximum |
| --- | ---: | ---: | ---: |
| Acquisition deadline | 5 s | 30 s | 120 s |
| Ownership budget `B` | 5 s | 30 s | 120 s |
| Included kill grace `G` | 150 ms | 500 ms | 2 s |

Every accepted policy requires positive durations and `B > G`. The public
`--ttl` is an exact ASCII-decimal seconds value in the ownership-budget range;
it remains mandatory even though a default is recorded for later UI/policy use.
Out-of-range, signed, suffixed, non-ASCII or overflowing values are rejected
before any backend call with a redacted sentinel.

The acquisition deadline starts before future pipe/process setup and bounds the
entire gopass/GPG/pinentry phase. `B` starts only after backend exit 0 and pipe
EOF. It includes `G`: TERM is due at `B-G` and KILL at `B`, with only bounded
reap/verification afterward. Because `wl-copy` has no verified publication
signal, this is a maximum owner lifetime, not a promise of `B` usable seconds.

These numbers are conservative product policy choices, not experimentally
proven universal safe limits. Milestone 1 measured normal synthetic acquisition
at about 17–20 ms, a real stuck pinentry cleanup with a 750 ms deadline at
872–874 ms including bounded cleanup, and the ownership state machine at
`B=500 ms`, `G=150 ms` with 50 ms tolerance. The 30-second defaults follow the
recorded UX proposal; the larger bounds allow interactive pinentry and manual
paste while keeping both phases finite. The 500 ms default grace adds margin
over the measured 150 ms test grace. Steps 5a–5d now apply the deadline,
ownership bounds and parent-death containment with one shared cleanup grace and
no global clipboard clear; the broader lifecycle gate is covered by step 5e.
Step 6 additionally verifies bounded pinentry cancellation with an isolated
agent remaining alive; the common `gpg-agent` is never signaled or cleared.
The public `copy` dispatcher now passes only validated action, decoded entry
path and bounded ownership budget into this lifecycle; it does not return the
secret to the caller.

For the supported `cliphist` setup, `examples/cliphist-sensitive-watch.sh` is
an opt-in wrapper: it discards stdin for `CLIPBOARD_STATE=sensitive` and
delegates normal data to `cliphist store`. It is never wired automatically.
