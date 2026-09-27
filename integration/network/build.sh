#!/bin/sh
set -eu
mkdir -p /tmp/common/integration/network
cp -R /common/. /tmp/common/
cp /suite/*_test.go /tmp/common/integration/network/
unformatted=$(gofmt -l /tmp/common/integration/network)
test -z "$unformatted" || { printf 'needs gofmt: %s\n' "$unformatted" >&2; exit 1; }
go -C /tmp/common vet ./integration/network
if test "$1" = test; then
    go -C /tmp/common test -c -o /output/network.test ./integration/network
fi
