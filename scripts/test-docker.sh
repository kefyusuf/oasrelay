#!/bin/sh
set -eu

image="${OASRELAY_IMAGE:-oasrelay:test}"

docker build --tag "$image" .
OASRELAY_IMAGE="$image" go test -tags=container -count=1 -v ./internal/containertest
