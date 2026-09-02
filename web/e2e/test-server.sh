#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
make build
export PCC_LISTEN=127.0.0.1:18080 PCC_PUBLIC_ORIGIN=http://127.0.0.1:18080
export PCC_SESSION_KEYS="$(openssl rand -base64 32 | tr '+/' '-_' | tr -d '=')"
# This non-production credential exists only in this process environment.
export PCC_SHARED_PASSWORD_HASH="$(printf '%s\n' pcc-e2e-only-password | ./pcc hash-password)"
exec ./pcc
