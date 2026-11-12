#!/usr/bin/env bash
set -euo pipefail
test -f /.dockerenv
test "$PWD" = /workspace
test -n "${OSHELL_WORKSPACE_OWNER:?Missing original workspace owner}"
# The next checkout runs as the host Runner user, not container root.
trap 'chown -R --no-dereference "$OSHELL_WORKSPACE_OWNER" /workspace' EXIT
git config --global --add safe.directory /workspace
go version
go env GOCACHE GOMODCACHE

if [[ "${1:-package}" == linux-core ]]; then
  python3 scripts/ci-test-bundle.py linux-core --shard "${OSHELL_CI_SHARD:?Missing core shard index}"
  exit
elif [[ "${1:-package}" == linux ]]; then
  python3 scripts/ci-test-bundle.py linux
  exit
fi

# One container reuses the build tool, downloaded modules and native libraries.
# Keep targets sequential: build.py temporarily generates shared embedded resources.
python3 scripts/build.py --target windows/amd64 --single-file --archive
python3 scripts/build.py --target linux/amd64 --archive
if [[ "${GITHUB_REF_TYPE:-}" == tag ]]; then
  go build -trimpath -o validation/oshell-release ./cmd/oshell-release
fi
