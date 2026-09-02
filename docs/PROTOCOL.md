# Helper metadata protocol

## Field-card extension status

Protocol v1 не изменён. Карточка использует отдельный protocol v2 и Waypass
Field Contract v1 из `docs/FIELD-CONTRACT.md`: `fields <entry-id>` и
`copy field <entry-id> <revision> <field-id> --ttl <seconds>`. Raw field name
не является authority или готовым argv. Public values могут присутствовать
только в v2 fields envelope; secret values в JSON отсутствуют.

## Version 1

The helper's metadata list is a JSON envelope with an explicit protocol
version:

```json
{
  "protocol": 1,
  "items": [
    { "id": "AWV4YW1wbGUudGVzdA", "label": "example.test" }
  ]
}
```

An empty list is encoded as `"items": []`, never as `null`.

The CLI command is `zer0-waypass-helper list [query]`. It accepts at most one
query argument. The helper always obtains metadata with the fixed command
`gopass ls --flat`, then applies the query in Go as a case-insensitive literal
substring of the path/label. A query is never passed to `gopass` or a shell.
The optional query must be valid UTF-8 and at most 4096 bytes. Invalid or
oversized queries fail as `invalid_invocation` before the backend is called.
Spaces, Unicode, control characters, and shell metacharacters in an otherwise
valid bounded query are treated as literal search text, not syntax.
Backend ordering is preserved and the response is one JSON object followed by
a line feed. Metadata backend execution has a helper-side deadline of 10
seconds; a UI caller may impose a shorter timeout, but never extends this
helper-side deadline. Cancellation is followed by at most one second of bounded
direct-child cleanup/reap tolerance. Captured metadata stdout is limited to 8
MiB.

Each item has an exact metadata allowlist:

- `id`: an opaque entry identifier;
- `label`: a display name derived only from the entry name or path.

For version 1, `label` is exactly the current Go string parsed from the
corresponding non-empty `gopass ls --flat` path line (apart from accepting the
line ending as LF or CRLF). It is not enriched from decrypted username, URL, or
custom fields. Query matching reads only this label/path. Every `list` and
`status` invocation performs a fresh backend listing; the helper has no
cross-invocation metadata cache or plaintext index on disk.

### Entry ID format

An `id` is unpadded URL-safe base64 (`base64.RawURLEncoding`) of these bytes:

```text
0x01 || UTF-8 canonical relative entry path
```

`0x01` is the ID format version. It is independent of the envelope
`"protocol": 1` version; either can evolve without implying a change to the
other. The entire visible ID therefore uses only `A-Z`, `a-z`, `0-9`, `_`, and
`-`, with no `=` padding.

The encoder and decoder apply the same validation. A valid path is non-empty,
valid UTF-8 of at most 4096 bytes, is relative, uses `/` separators, and is
already equal to `path.Clean(path)`. Nested paths, Unicode names, spaces, and
dotfiles are allowed. Unicode normalization is deliberately not performed:
the decoded Linux filename bytes must remain exact for a later fresh membership
comparison.

The following are rejected:

- a leading or trailing `/`, `//`, or any `.` or `..` path segment;
- a path whose first byte is `-`;
- backslash or a Unicode control code point (including NUL and DEL `0x7f`);
- the ASCII shell metacharacters semicolon, ampersand, pipe, dollar, backtick,
  single or double quote, `< >`, `( )`, `{ }`, `[ ]`, `*`, `?`, and `!`;
- invalid UTF-8 or a path longer than 4096 bytes.

Decoding accepts only canonical unpadded raw URL-safe base64. Padding, the
standard base64 alphabet, non-zero trailing padding bits, a non-canonical
re-encoding, a missing or wrong `0x01` version byte, an empty payload, and an
oversized visible ID are rejected. The maximum visible ID length is derived
from the version byte plus the 4096-byte path and is checked before allocating
a decode buffer. Every rejection is represented by one generic error that does
not include the visible ID, decoded bytes, or path.

If `gopass ls --flat` emits even one invalid path, `list` fails closed with no
partial JSON on stdout and the redacted `backend_invalid_data` error on stderr.
Validation is performed before query filtering, so a query cannot hide an
invalid backend path.

The ID is an opaque transport value for the UI, but its encoding is reversible.
It provides no secrecy, integrity, authentication, or authorization. A future
copy operation must decode and validate the ID, obtain a fresh
`gopass ls --flat` result, require an exact match of the decoded canonical path
in that fresh result, and only then run one single-entry decrypt. The cached UI
list and the ID alone are never sufficient authorization.

The metadata protocol forbids passwords, usernames, URLs, custom fields, GPG
material, backend output, and error details in an item or as additional envelope
fields. Entry names and paths remain an accepted metadata disclosure in the MVP;
no separate plaintext metadata index is created.

### Status

`zer0-waypass-helper status` performs the same metadata-only
`gopass ls --flat` probe without decrypting entries. A successful probe returns
exactly one versioned JSON line:

```json
{"protocol":1,"backend":"ready"}
```

The closed version 1 status response contains no executable version or path,
store path, entry metadata, username, URL, or secret. Backend failures produce
no success JSON and raw backend output is not forwarded.

### Errors

Every failed invocation writes exactly one versioned JSON object followed by LF
to stderr and normally writes nothing to stdout:

```json
{"protocol":1,"error":{"code":"backend_unavailable"}}
```

The error object is closed and contains only `code`; it never contains a
free-form message, details, path, command, backend output, or cause. Protocol v1
allows exactly these stable codes:

- `invalid_invocation` (exit 2);
- `backend_unavailable`, `backend_timeout`, `operation_canceled`;
- `backend_invalid_data`, `backend_output_too_large`, `backend_error`;
- `output_error` (all operation and output errors exit 1).

Codes remain stable within protocol v1. Consumers must handle an unknown code
as a generic failure, without trying to display or infer backend details. If a
stdout transport fails after accepting a prefix, partial metadata JSON may be
present on stdout together with `output_error` on stderr; consumers must discard
all stdout for a non-zero exit. Metadata contains no secret in Milestone 2.
Failure to write the error itself is not retried or reported recursively.

The operational bounds above are not wire fields. On timeout, cancellation, or
output overflow, the helper cancels and reaps the direct `gopass` child. The
Milestone 1 isolated-store spike established that real `gopass ls --flat` does
not decrypt entries or invoke GPG. Metadata listing does not claim containment
of descendants from a substituted malicious `gopass` executable. Full process
group and parent-death guardian containment is deferred to the clipboard/copy
process model.

Version 1 consumers must reject unsupported `protocol` values rather than guess
their meaning. Any incompatible envelope or item-shape change requires a new
protocol version. Adding fields to version 1 is incompatible because its field
allowlists are closed.

### Staged copy command grammar

The reserved copy grammar is exactly:

```text
zer0-waypass-helper copy <username|password> <id> --ttl <seconds>
```

The action is one of the two lowercase literals shown above. `<id>` must pass
the strict version-1 decoder. `<seconds>` is a non-empty sequence of ASCII
decimal digits, is greater than zero, and must be representable as a Go
`time.Duration` after conversion to seconds. Signs, whitespace, decimal points,
and unit suffixes are rejected. The inclusive production range is `5..120`
seconds. The policy default is 30 seconds, although version 1 continues to
require an explicit `--ttl`; no omitted-value grammar is introduced. The CLI
parser delegates range enforcement to `internal/clipboard`, which is the single
source of truth for the lifecycle policy.

Milestone 3 step 1 implements this grammar as an internal typed parser only.
The public command dispatcher deliberately continues to reject `copy`,
including grammatically valid forms, with `invalid_invocation`; it must not
report success before fresh membership, secret streaming, clipboard ownership,
and process cleanup are implemented together. The parser retains only the
validated action, decoded canonical path, and duration, not the raw untrusted
ID or any secret. Steps 2–7 build and verify the secret-streaming and lifecycle
pipeline behind an internal operation boundary. Production dispatch is deferred
to the separate late wiring step 8 and is allowed only after steps 2–7 are
GREEN. After that wiring, a grammatically valid invocation must not produce
`invalid_invocation`; malformed grammar remains fail-closed before any backend
call.

The public copy command has a detached-dispatch contract: its caller may only
interpret acceptance/validation and the bounded ownership budget. It must not
expect a decrypt result, clipboard-publication confirmation, or secret on
stdout; copy failures remain redacted protocol errors.

Milestone 3 step 3 defines but does not yet enforce lifecycle timers. The
acquisition deadline range is 5–120 seconds with a 30-second default. The
ownership budget `B` is the explicit `--ttl` above. Included kill grace `G` has
an internal 150 ms–2 s range and a 500 ms default. Every accepted policy must
have `B > G`. Acquisition begins before future process/pipe setup and bounds
gopass/GPG/pinentry. `B` begins only at backend exit 0 plus pipe EOF; `G` is
inside `B`, so the usable clipboard interval may be shorter than `B`. Timer and
process-group enforcement remain steps 4–5, not functionality claimed here.

This document otherwise defines metadata listing, ID encoding/decoding and
validation, and metadata-command errors only. Error codes do not yet define
copy-error semantics, a copy response, or membership execution. The fresh
membership rule above is a mandatory invariant for the future copy
implementation, not functionality provided by metadata listing.
