# Security model

## Field names and entry cards

Имена полей считаются чувствительными metadata и не сохраняются постоянно.
Bounded spike `gopass 1.16.1` подтвердил, что live-список имён нельзя получить
отдельно от расшифрованной записи. Helper не имеет права читать полный `show`
output для parsing имён: это поместило бы password, notes, keys, tokens и body в
Go memory и разрушило прямую FD-to-FD границу.

Parsing legacy entry и probing raw names запрещены. Field Contract v1 хранит
schema в encrypted manifest, а каждое value — отдельным encrypted entry.
`public` values осознанно возвращаются в ephemeral protocol-v2 JSON и Noctalia;
`secret` values остаются только в FD-to-FD clipboard pipeline. Copy повторно
проверяет manifest, revision, field mapping и value membership.

## Metadata in Milestone 2

Entry names and relative paths returned live by `gopass ls --flat` are accepted
plaintext disclosure to the launcher. They can themselves contain sensitive
information, so users must choose entry names with that limitation in mind.
The protocol exposes the current path unchanged as `label` and an opaque,
reversible ID derived from that path.

The metadata surface is deliberately closed. It must not decrypt, parse, index,
cache, persist, or log passwords, usernames, URLs, custom fields, GPG material,
or raw backend errors. It also must not write entry metadata to a separate disk
index, settings, state, or cache. Each `list` and `status` operation obtains a
fresh live listing. Filtering applies only to the listed path/label. `status`
probes the listing capability but returns no entry paths.

Entry IDs are reversible transport encodings, not secrets, capabilities, or
authorization. Both encoding of backend paths and decoding of untrusted IDs
enforce a non-empty valid-UTF-8 canonical relative slash path of at most 4096
bytes. Absolute paths, dot/traversal segments, empty or repeated segments,
leading `-`, backslash, Unicode control code points, and ASCII shell metacharacters are
rejected. Spaces, Unicode names, nested paths, and dotfiles remain valid. No
Unicode normalization is applied because future authorization must compare the
exact decoded Linux filename bytes with a fresh backend listing.

The raw-base64url decoder rejects padding, non-canonical encodings, invalid
trailing bits, wrong/missing format versions, empty payloads, invalid UTF-8,
and oversized input. Its maximum encoded length is checked before decode-buffer
allocation. All validation failures use one redacted error without the ID,
path, or decoded bytes. An invalid path anywhere in a backend listing fails the
whole `list` response before stdout is written, even when that path would not
match the query.

Tests use synthetic names and fake executables or an isolated synthetic store;
the user's real password store is never a fixture.

Metadata failures expose only a closed, stable protocol-v1 code. Backend stderr
is discarded, and raw backend errors are replaced by safe sentinels without
wrapping or retaining their text. Error JSON contains no free-form fields,
rejected path, query, command, backend output, or cause. A failed stdout write
may leave a partial metadata prefix; callers must discard stdout whenever the
process exits non-zero. This metadata transport contains no secret in Milestone
2, and these error codes do not yet define copy-error semantics.

The optional list query is bounded to 4096 bytes and must be valid UTF-8 before
any backend call. It remains literal data even when it contains whitespace,
Unicode, control characters, or shell metacharacters, and is never forwarded
in the `gopass` argv.

Milestone 3 steps 1 and 3 added an internal parser for the exact copy grammar. It
allowlists the action, strictly decodes the ID, and accepts only a positive
ASCII-decimal TTL in the inclusive 5–120 second policy range. Its typed result
does not retain the raw transport ID or any secret. Out-of-range, overflow and
invalid policy values produce only a redacted sentinel. Milestone 3 step 8
connected the public dispatcher for the exact `username|password` allowlist;
malformed and unsupported actions are rejected before any backend call.

The operational copy path confirms exact membership in a
fresh listing and stream the selected field directly from the backend to
`wl-copy` without returning it to the UI, argv, logs, settings, state, or cache.
The caller receives no decrypt result or secret, only redacted
acceptance/error semantics. Arbitrary field actions remain unsupported under
the field-card blocker above.

Milestone 3 step 2 implements the internal FD-to-FD data boundary. It performs
a fresh exact membership check before every extraction, allowlists only
`username` and `password`, and connects `gopass.Stdout` directly to
`wl-copy.Stdin` with `os.Pipe`. The production supervisor never reads or
buffers those bytes. Backend and clipboard diagnostics are disconnected and
only typed redacted sentinels cross the operation boundary. Tests use compiled
synthetic fake executables in a temporary `PATH`; only the fake clipboard
consumer compares the synthetic secret bytes.

Steps 4–5d add separate operation groups, bounded descendant cleanup,
acquisition cancellation, conservative ownership-budget enforcement and a
same-helper per-operation guardian. The controller starts that guardian with
`Pdeathsig=SIGTERM` while pinned to one OS thread through guardian exit. A
strict inherited-pipe READY/START/REGISTERED handshake prevents workers before
the guardian signal handler is ready; REGISTERED contains both validated worker
PID/PGID pairs. Worker processes have no `Pdeathsig`. Neither the request nor
the control protocol carries secret bytes. Global `wl-copy --clear` and public
`copy` dispatch remain unavailable.

Milestone 3 step 3 fixes the lifecycle policy without yet applying its timers:
acquisition deadline 5–120 seconds (default 30 seconds), ownership budget `B`
5–120 seconds (default 30 seconds), and included kill grace `G` 150 ms–2 seconds
(default 500 ms). Defaults and all accepted values are positive and require
`B > G`. Acquisition will start before process/pipe setup and therefore bounds
pinentry wait. After backend exit 0 plus pipe EOF, `B` is only an upper bound:
TERM is scheduled for `B-G`, KILL for `B`, and publication delay or ownership
loss can shorten the usable interval. Steps 5a–5d implement these runtime and
tested controller-crash containment guarantees. Step 5e now covers repeated
normal/error/deadline/concurrent/crash operations with nonce-scoped orphan
checks. Step 6 verifies bounded pinentry cancellation while the isolated/common
`gpg-agent` remains alive; no global agent signal or cleanup is performed.
Public `copy` dispatch uses validated opaque IDs and bounded policy values; it
never returns secret material. Global `wl-copy --clear` remains unavailable.
