# Security Policy

## Supported versions

Until the first stable release, only the latest commit on `main` and the latest
published `0.x` release receive security fixes. Older snapshots are unsupported.

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability. Email the maintainer at
[andrey.losikhin@gmail.com](mailto:andrey.losikhin@gmail.com) with the subject
`zer0-waypass security report`. The maintainer will move confirmed reports to a
private GitHub Security Advisory. Private vulnerability reporting is not enabled
on this repository yet.

Do not attach real password-store contents, decrypted fields, passwords,
private keys or GPG home directories. A minimal synthetic reproducer is preferred.

Include affected version or commit, impact, prerequisites and reproduction steps.
The maintainer will acknowledge a report when it is seen, coordinate validation
and remediation privately, and publish a security advisory when a fix is ready.
No response-time SLA is promised for this personal project.

## Security boundaries

The detailed threat model, accepted risks and secret-flow invariants are in
[docs/SECURITY.md](docs/SECURITY.md). In particular, the project does not claim
that the `sensitive` clipboard hint is honored by every clipboard manager.
