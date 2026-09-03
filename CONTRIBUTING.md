# Contributing

Thank you for helping improve zer0-waypass. Keep changes small, explain their
security impact and use synthetic password-store data exclusively.

## Before opening a change

1. Search existing issues and read [the architecture](docs/ARCHITECTURE.md),
   [security model](docs/SECURITY.md) and [field contract](docs/FIELD-CONTRACT.md).
2. Open an issue before changing protocol, crypto/backend scope, process
   supervision, clipboard semantics or external dependencies.
3. Do not include passwords, usernames from a real store, private paths, GPG
   material or production fixtures in issues, commits, logs or tests.

## Development

Requirements are Go 1.26+, Linux and no third-party Go modules. Format Go code
with `gofmt`. The focused local gate is:

```sh
go test ./...
go test -race ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -o /tmp/zer0-waypass-helper ./cmd/zer0-waypass-helper
sh -n examples/cliphist-sensitive-watch.sh packaging/arch/build-local-package.sh
git diff --check
```

Tests that exercise `gopass`, GPG or clipboard tools must use isolated temporary
state and synthetic values. Never change the user's Hyprland, GPG, Noctalia or
clipboard-manager configuration from a test.

## Commits and pull requests

- Base work on `main` and submit one coherent change per pull request.
- Use imperative, descriptive commit subjects; Conventional Commit prefixes
  such as `feat:`, `fix:`, `docs:`, `test:`, `build:` and `chore:` are preferred.
- Explain behavior, security impact, exact verification and known limitations.
- Add user-visible changes to `CHANGELOG.md` under `Unreleased`.
- Do not update generated community `catalog.toml` or submit built binaries.

By contributing, you agree that your contribution is licensed under
[Apache License 2.0](LICENSE).

## Noctalia community submission

The standalone repository is the development source. A future community PR
must copy only `noctalia-plugin/waypass` to the top-level `waypass/` directory
of `noctalia-dev/community-plugins`. Before that PR:

- follow the current community README and PR templates;
- add `README.md`, `translations/en.json` and a real 960x540 `thumbnail.webp`;
- verify that every external command is declared and documented;
- bump the plugin's semantic version;
- run the community repository's validator and test on a supported compositor.
