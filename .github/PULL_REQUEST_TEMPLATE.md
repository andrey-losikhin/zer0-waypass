## Summary

<!-- What user-visible or maintenance outcome does this change provide? -->

## Scope and security impact

<!-- Account for new processes, argv, network calls, filesystem writes and secret data paths. Write "None" if unchanged. -->

## Verification

<!-- List the exact commands run and their results. Use synthetic data only. -->

## Checklist

- [ ] The change is focused and documented where necessary.
- [ ] Tests use synthetic data and contain no secrets or real password-store fixtures.
- [ ] Passwords, decrypted fields and GPG material never enter argv, logs or Noctalia state/settings/cache.
- [ ] New external commands are declared in `plugin.toml` and documented.
- [ ] `go test ./...`, `go test -race ./...`, `go vet ./...` and the static build pass, or the exception is explained.
- [ ] User-facing changes are recorded in `CHANGELOG.md`.
- [ ] The plugin version is bumped when preparing a Noctalia community submission.
