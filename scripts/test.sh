#!/bin/sh
set -eu
CDPATH= cd -- "$(dirname -- "$0")/.."
go build -o bin/gg ./cmd/gg
if [ "$(go env CGO_ENABLED)" = 1 ]; then
    go test -race ./...
else
    go test ./...
    printf '%s\n' 'Race detector skipped: CGO is disabled (a C compiler is required).' >&2
fi
go vet ./...
python3 -m unittest discover -s tests -v
