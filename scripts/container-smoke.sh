#!/usr/bin/env bash
set -euo pipefail

IMAGE="${IMAGE:-personal-connection-check:local}"
PORT="${PCC_SMOKE_PORT:-18082}"
NAME="pcc-smoke-$$"
ORIGIN="http://127.0.0.1:${PORT}"

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
  -p "127.0.0.1:${PORT}:8080" \
  -e "PCC_PUBLIC_ORIGIN=${ORIGIN}" \
  -e "PCC_SHARED_PASSWORD_HASH=${PASSWORD_HASH}" \
  -e "PCC_SESSION_KEYS=${SESSION_KEY}" \
  "$IMAGE" >/dev/null

for _ in $(seq 1 30); do
  if curl -fsS "${ORIGIN}/healthz" >/dev/null; then
    break
  fi
  sleep 1
done
curl -fsS "${ORIGIN}/healthz" >/dev/null

docker exec "$NAME" /pcc healthcheck
[ "$(docker inspect --format '{{.State.Running}}' "$NAME")" = "true" ]

HTML="$(curl -fsS "${ORIGIN}/")"
ASSETS="$(printf '%s' "$HTML" | grep -Eo '/assets/[^" ]+\.(js|css)' | sort -u)"
[ -n "$ASSETS" ]
while IFS= read -r asset; do
  curl -fsS "${ORIGIN}${asset}" >/dev/null
done <<<"$ASSETS"

printf 'container smoke passed: health, root, and hashed assets\n'
