# zer0-waypass

[![CI](https://github.com/andrey-losikhin/zer0-waypass/actions/workflows/ci.yml/badge.svg)](https://github.com/andrey-losikhin/zer0-waypass/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/andrey-losikhin/zer0-waypass)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](go.mod)

System-wide password launcher for Wayland and Noctalia, backed by `gopass` and
GPG. Search once, copy a selected field to the sensitive Wayland clipboard and
paste it manually into any browser, terminal or desktop application.

> [!IMPORTANT]
> This project is pre-release. Its helper, plugin protocol and installation
> layout may change before the first stable release.

## Why

Browser-specific password extensions become cumbersome when several browsers
and desktop applications are used. zer0-waypass provides one system launcher
without browser integration, URL inspection, automatic typing or form submit.

```text
Noctalia launcher / panel
          |
          | opaque entry and field IDs only
          v
zer0-waypass-helper
          |
          v
gopass -> GPG -> gpg-agent
          |
          | direct pipe; no secret in argv or Noctalia state
          v
wl-copy --sensitive --foreground
```

## Features

- `/wp` Noctalia launcher search and a keyboard-first panel.
- `gopass` with GPG/GPG-agent as the only password-store backend.
- Direct helper-to-`wl-copy` secret stream with a bounded operation deadline.
- Password, token and key values stay out of Noctalia state, settings and logs.
- Fixed-argv process execution without shell interpolation.
- Static, cgo-free Linux helper written with the Go standard library.

## Requirements

- Linux with Wayland;
- Noctalia with Plugin API 24 or newer;
- `zer0-waypass-helper` on `PATH`;
- `gopass`, GnuPG/GPG-agent and `wl-copy` (`wl-clipboard`);
- Go 1.26+ to build from source.

The packaged MVP target is Arch Linux and compatible distributions. Other
Linux systems can use the documented source build, but do not have a maintained
native package yet.

## Install from source

```sh
git clone https://github.com/andrey-losikhin/zer0-waypass.git
cd zer0-waypass
GOTOOLCHAIN=local CGO_ENABLED=0 go build -trimpath \
  -o /tmp/zer0-waypass-helper ./cmd/zer0-waypass-helper
install -Dm755 /tmp/zer0-waypass-helper "$HOME/.local/bin/zer0-waypass-helper"
mkdir -p "$HOME/.local/share/noctalia/plugins/zer0-waypass"
cp -R noctalia-plugin/waypass/. \
  "$HOME/.local/share/noctalia/plugins/zer0-waypass/"
```

Ensure `$HOME/.local/bin` is on `PATH`, then enable the plugin in Noctalia.
Detailed requirements, Arch packaging notes and removal steps are in
[the installation guide](docs/INSTALL.md).

## Usage

Open the panel:

```sh
noctalia msg panel-toggle zer0/waypass:waypass
```

Alternatively, type `/wp` followed by an entry-name query in the Noctalia
launcher. The repository includes an opt-in
[Hyprland keybind example](examples/hyprland.lua); it never edits the user's
configuration automatically.

The panel shows public metadata declared by the encrypted field manifest.
Secret fields are copied only after an explicit action and remain available for
at most the configured clipboard budget.

> [!WARNING]
> `--sensitive` is a hint, not a universal clipboard-history guarantee. Configure
> and verify your clipboard manager separately. See
> [clipboard behavior and the tested cliphist wrapper](docs/CLIPBOARD.md).

## Security

zer0-waypass delegates storage and cryptography to `gopass`, GPG and
`gpg-agent`; it implements no cryptographic algorithms. Secrets must never be
passed in process arguments, logs, Noctalia state/settings/cache or committed
fixtures. Read the [threat model](docs/SECURITY.md) and use the
[private reporting policy](SECURITY.md) for suspected vulnerabilities.

## Project status

The helper, launcher, panel, guarded clipboard lifecycle and Arch packaging are
implemented. The repository is not yet a Noctalia community release candidate:
a real 960x540 WebP thumbnail, plugin-local public README/translations,
community validation and maintainer confirmation for the external Arch-only
helper dependency are still required.

See [CHANGELOG.md](CHANGELOG.md) for release-facing changes and the internal
[implementation status](.docs/execplans/zer0-waypass/status.md) for verified
milestone evidence.

## Documentation

- [Installation](docs/INSTALL.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Security model](docs/SECURITY.md)
- [Clipboard behavior](docs/CLIPBOARD.md)
- [Protocol](docs/PROTOCOL.md)
- [Field contract](docs/FIELD-CONTRACT.md)
- [Release process](docs/RELEASING.md)
- [Architecture decisions](docs/decisions)

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md), [SUPPORT.md](SUPPORT.md) and the
[Code of Conduct](CODE_OF_CONDUCT.md). Contributions are licensed under
[Apache License 2.0](LICENSE).
