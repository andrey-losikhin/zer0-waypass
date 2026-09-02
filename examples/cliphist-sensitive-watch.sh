#!/bin/sh
# Opt-in cliphist wrapper. It must be wired manually in the user's watcher.
# Sensitive clipboard bytes are discarded without being logged or written.
set -eu

if [ "${CLIPBOARD_STATE:-data}" = "sensitive" ]; then
    cat >/dev/null
    exit 0
fi

exec cliphist store
