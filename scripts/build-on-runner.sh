#!/usr/bin/env bash
# Host orchestration only: all application compilation happens inside Docker.
set -euo pipefail
repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
cd "$repo_root"
image=oshell-desktop-ci:ubuntu24.04-v2
mode=${1:-package}
container_name="oshell-build-${GITHUB_RUN_ID:-local-$$}-${GITHUB_RUN_ATTEMPT:-1}-$mode"
toolchain=$(go env GOROOT)
owner=$(stat -c '%u:%g' "$repo_root")

if [[ "${OSHELL_CI_IMAGE_READY:-0}" != 1 ]]; then
  docker build --pull --build-arg "CACHE_EPOCH=$(date -u +%Y-%m)" \
    --tag "$image" --file .github/docker/build.Dockerfile .github/docker
fi

# Scope caches to this trusted repository; do not mount the Docker socket or credentials.
cache_root=${OSHELL_CI_CACHE_DIR:-${RUNNER_TEMP:-/tmp}/oshell-ci-cache}
mkdir -p "$cache_root"
cleanup() {
  docker rm --force "$container_name" >/dev/null 2>&1 || true
  # Also recover ownership when a cancelled job forcibly stops the build container.
  docker run --rm --mount "type=bind,source=$repo_root,target=/workspace" \
    --mount "type=bind,source=$cache_root,target=/cache" \
    "$image" chown -R --no-dereference "$owner" /workspace /cache
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
docker run --rm --init --name "$container_name" \
  --mount "type=bind,source=$repo_root,target=/workspace" \
  --mount "type=bind,source=$toolchain,target=/opt/go,readonly" \
  --mount "type=bind,source=$cache_root,target=/cache" \
  --env "OSHELL_WORKSPACE_OWNER=$owner" \
  --env GITHUB_SHA --env GITHUB_RUN_ID --env GITHUB_REF_TYPE --env OSHELL_CI_SHARD \
  "$image" bash scripts/build-container.sh "$mode"
