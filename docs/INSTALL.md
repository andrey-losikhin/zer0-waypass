# Установка из исходников

Готовый пакет MVP поддерживается только на Arch Linux и совместимых системах.
На других Linux-системах можно собрать helper вручную, если доступны Go,
`gopass`, GnuPG, `wl-copy` и Noctalia Plugin API 24+.

Сборка helper без cgo:

```sh
GOTOOLCHAIN=local CGO_ENABLED=0 GOFLAGS=-buildvcs=false \
  go build -trimpath -o /tmp/zer0-waypass-helper ./cmd/zer0-waypass-helper
install -Dm755 /tmp/zer0-waypass-helper "$HOME/.local/bin/zer0-waypass-helper"
```

Установите plugin вручную в каталог, который использует ваша Noctalia:

```sh
mkdir -p "$HOME/.local/share/noctalia/plugins/zer0-waypass"
cp -R noctalia-plugin/waypass/. \
  "$HOME/.local/share/noctalia/plugins/zer0-waypass/"
```

Проверьте наличие runtime-зависимостей:

```sh
command -v gopass gpg gpg-agent wl-copy noctalia
```

После установки вручную включите plugin и добавьте IPC keybind по примеру
[`examples/hyprland.lua`](../examples/hyprland.lua). Проект не изменяет
Hyprland, GPG, Noctalia или clipboard-manager конфигурации автоматически.

Эта инструкция описывает source build, а не готовый пакет и не расширяет
официальную packaged support matrix.
