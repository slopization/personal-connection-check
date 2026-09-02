# Personal Connection Check

An access-controlled, single-server web application for measuring the browser-to-server network path.

- Adaptive parallel download and upload measurement
- WebSocket application RTT and jitter monitoring
- 10-minute, 1-hour, and whole-session charts
- Shared-password or standards-based OIDC authentication
- Browser-local IndexedDB history and PNG result export
- Korean UI for `ko` browsers; English fallback
- Optional local GeoIP City/ASN databases

> This measures the browser → ingress → application path. RTT is WebSocket round-trip time, not ICMP ping. A speed test intentionally consumes substantial bandwidth and starts only after explicit user action.

## Quick start

Requirements for source builds: Go 1.27.1 and Node.js 26.0.0.

Generate a password hash without placing the password in process arguments:

```sh
printf '%s\n' 'choose-a-password' | go run ./cmd/pcc hash-password
```

Generate a 32-byte base64url session key and start the service:

```sh
export PCC_SHARED_PASSWORD_HASH='<argon2id hash>'
export PCC_SESSION_KEYS='<base64url key>'
export PCC_PUBLIC_ORIGIN='https://connection.example.com'
go run ./cmd/pcc
```

The OCI image is published as:

```text
git.kyu.sh/modelgarden/personal-connection-check
```

Run the image as a non-root, read-only container and supply authentication values through secrets:

```sh
docker run --rm --read-only --tmpfs /tmp:rw,noexec,nosuid,size=16m \
  -p 8080:8080 \
  -e PCC_PUBLIC_ORIGIN='https://connection.example.com' \
  -e PCC_SHARED_PASSWORD_HASH='<argon2id hash>' \
  -e PCC_SESSION_KEYS='<base64url key>' \
  git.kyu.sh/modelgarden/personal-connection-check:latest
```

## Configuration

| Variable | Purpose |
| --- | --- |
| `PCC_PUBLIC_ORIGIN` | Exact external origin used for Host, CSRF, and WebSocket origin checks |
| `PCC_LISTEN` | Listen address; default `:8080` |
| `PCC_SHARED_PASSWORD_HASH` | Argon2id PHC hash for shared-password login |
| `PCC_SESSION_KEYS` | Comma-separated base64url AEAD keys; first key encrypts, remaining keys decrypt old sessions |
| `PCC_OIDC_ISSUER` | OIDC discovery issuer |
| `PCC_OIDC_CLIENT_ID` | OIDC client ID |
| `PCC_OIDC_CLIENT_SECRET` | OIDC client secret |
| `PCC_OIDC_REDIRECT_URI` | Exact OIDC callback URI |
| `PCC_OIDC_EMAIL_ALLOWLIST` | Comma-separated allowed verified email addresses |
| `PCC_TRUSTED_PROXY_CIDRS` | Comma-separated trusted ingress proxy networks |
| `PCC_MAX_RUNS` | Maximum concurrent test runs; default `2` |
| `PCC_MAX_STREAMS` | Maximum streams per run; default `8` |
| `PCC_MAX_DIRECTION_DURATION` | Per-direction cap, at most `15s` |
| `PCC_UPLOAD_LIMIT` | Maximum bytes accepted by one upload request |
| `PCC_GEOIP_CITY_DB` | Optional local GeoIP City MMDB path |
| `PCC_GEOIP_ASN_DB` | Optional local GeoIP ASN MMDB path |
| `PCC_HEALTHCHECK_URL` | Optional URL used by the image healthcheck; default `http://127.0.0.1:8080/healthz` |

At least one complete authentication method is required. OIDC startup fails closed when discovery or required configuration is invalid. Allowed OIDC emails must have `email_verified=true`.

## Ingress requirements

- Preserve WebSocket upgrades.
- Allow requests to run beyond the 15-second measurement cap.
- Keep upload limits above configured chunks.
- Disable response compression, buffering, and caching for speed endpoints.
- Overwrite forwarding headers and configure only actual ingress peers in `PCC_TRUSTED_PROXY_CIDRS`.
- Use TLS in production; session cookies are `Secure`, `HttpOnly`, and `SameSite=Lax`.

## Data and privacy

Measurement history is stored only in the browser's IndexedDB and can be deleted from the UI. The service does not send observed IP addresses to external lookup APIs. Optional location and ISP data come only from operator-mounted MMDB files. PNG exports may include the displayed IP, ISP, and approximate location; review the image before sharing.

## Development

```sh
cd web && npm ci
make check
```

The full clean-runner release gate additionally installs Playwright browsers, runs desktop/mobile Chromium, desktop Firefox, and mobile WebKit E2E tests, builds the OCI image, and executes its read-only smoke test. Desktop Safari remains usable but unsupported because its streaming reader can remain pending after server EOF; the UI presents a non-blocking warning instead of disabling the test:

```sh
make e2e-install
make release-gate
```

See `docs/measurement-methodology.md`, `docs/security.md`, and `docs/tdd-evidence.md` for measurement semantics, security boundaries, and RED/GREEN evidence.

## License

MIT © 2026 ModelGarden
