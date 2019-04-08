#!/usr/bin/env bash
set -ex

rm -rf bin
mkdir bin
CROSSPLATFORMS='linux/amd64 darwin/amd64'

for platform in ${CROSSPLATFORMS}; do
  GOOS=${platform%/*} \
  GOARCH=${platform##*/} \
    go build -v -o bin/gslock-${platform%/*}-${platform##*/}
done
