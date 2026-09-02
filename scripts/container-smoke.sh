#!/usr/bin/env bash
set -euo pipefail

IMAGE="${IMAGE:-personal-connection-check:local}"
NAME="pcc-smoke-$$"

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

PASSWORD_HASH="$(printf '%s\n' 'container-smoke-password' | docker run --rm -i "$IMAGE" hash-password)"
SESSION_KEY="$(head -c 32 /dev/urandom | base64 | tr -d '\n' | tr '+/' '-_' | tr -d '=')"

docker run -d \
  --name "$NAME" \
  --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=16m \
  -e "PCC_PUBLIC_ORIGIN=http://127.0.0.1:8080" \
  -e "PCC_SHARED_PASSWORD_HASH=${PASSWORD_HASH}" \
  -e "PCC_SESSION_KEYS=${SESSION_KEY}" \
  "$IMAGE" >/dev/null

for _ in $(seq 1 30); do
  if docker exec "$NAME" /pcc healthcheck >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

docker exec "$NAME" /pcc healthcheck
docker exec "$NAME" /pcc smoke
[ "$(docker inspect --format '{{.State.Running}}' "$NAME")" = "true" ]

printf 'container smoke passed: /healthz, /, and /assets/ from delivered image\n'
