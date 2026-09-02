# Personal Connection Check

> [!WARNING]
> This entire codebase was written by AI. No warranty or support is provided; review it carefully and use it entirely at your own risk.

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

| Variable                     | Required               | Example                                | Purpose / default                                                                                    |
| ---------------------------- | ---------------------- | -------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| `PCC_PUBLIC_ORIGIN`          | Yes                    | `https://connection.example.com`       | Exact external origin used for Host, CSRF, WebSocket origin, secure-cookie, and OIDC callback checks |
| `PCC_SESSION_KEYS`           | Yes                    | `<32-byte base64url key>`              | Comma-separated AEAD keys; the first encrypts and remaining keys decrypt old sessions                |
| `PCC_SHARED_PASSWORD_HASH`   | Unless OIDC is enabled | `<argon2id PHC hash>`                  | Enables shared-password login                                                                        |
| `PCC_OIDC_ISSUER`            | With OIDC              | `https://idp.example.com/oidc`         | OIDC discovery issuer                                                                                |
| `PCC_OIDC_CLIENT_ID`         | With OIDC              | `pcc`                                  | OIDC client ID                                                                                       |
| `PCC_OIDC_CLIENT_SECRET`     | With OIDC              | `<OIDC client secret>`                 | OIDC client secret                                                                                   |
| `PCC_OIDC_EMAIL_ALLOWLIST`   | With OIDC              | `alice@example.com,bob@example.com`    | Comma-separated allowed email addresses; each ID token must contain `email_verified=true`            |
| `PCC_LISTEN`                 | No                     | `:8080`                                | Listen address; default `:8080`                                                                      |
| `PCC_TRUSTED_PROXY_CIDRS`    | No                     | `192.0.2.10/32`                        | Comma-separated actual ingress proxy networks; default trusts none                                   |
| `PCC_MAX_RUNS`               | No                     | `2`                                    | Maximum concurrent test runs; default `2`                                                            |
| `PCC_MAX_STREAMS`            | No                     | `8`                                    | Maximum streams per run; default `8`, maximum `32`                                                   |
| `PCC_MAX_DIRECTION_DURATION` | No                     | `15s`                                  | Per-direction cap; default and maximum `15s`                                                         |
| `PCC_UPLOAD_LIMIT`           | No                     | `16777216`                             | Maximum bytes accepted by one upload request; default `16777216` (16 MiB)                            |
| `PCC_GEOIP_CITY_DB`          | No                     | `/dbip/dbip-city-lite.mmdb`            | Local GeoIP City MMDB path; disabled when unset                                                      |
| `PCC_GEOIP_ASN_DB`           | No                     | `/dbip/dbip-asn-lite.mmdb`             | Local GeoIP ASN MMDB path; disabled when unset                                                       |
| `PCC_FOOTER_MESSAGE`         | No                     | `IP Geolocation by https://db-ip.com/` | Public plain-text footer; HTTP(S) URLs become links; default empty                                   |
| `PCC_HEALTHCHECK_URL`        | No                     | `http://127.0.0.1:8080/healthz`        | URL used only by the image `healthcheck` command; shown value is the default                         |
| `PCC_SMOKE_URL`              | No                     | `http://127.0.0.1:8080`                | Base URL used only by the release-test `smoke` command; shown value is the default                   |

At least one complete authentication method is required: set `PCC_SHARED_PASSWORD_HASH`, or set all four OIDC variables marked **With OIDC**. OIDC startup fails closed when discovery or required configuration is invalid. Omitting `PCC_SHARED_PASSWORD_HASH` disables shared-password login; there is no default or fallback password.

The service writes structured JSON lifecycle and authentication events to stdout. Fields never include passwords, hashes, session keys, authorization codes, tokens, cookies, state/nonce values, email addresses, or OIDC client secrets. See the Kubernetes deployment guide for event and reason codes.

The OIDC callback path is fixed and requires no environment variable. Register the following exact redirect URI with the OIDC provider:

```text
https://connection.example.com/api/auth/oidc/callback
```

It is always derived as `PCC_PUBLIC_ORIGIN` + `/api/auth/oidc/callback`; replace the example origin with the service's actual public origin.

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

The full clean-runner release gate additionally installs Playwright browsers, runs desktop/mobile Chromium and desktop Firefox E2E tests, builds the OCI image, and executes its read-only smoke test. Safari and WebKit-based browsers are excluded from the supported CI matrix because they do not provide a reproducible non-macOS validation loop for this streaming workload. They remain usable but display a non-blocking unsupported-browser warning. If any browser still yields a bounded partial stream, its received bytes are retained and the UI plus console disclose that the result may be conservative:

```sh
make e2e-install
make release-gate
```

See `docs/kubernetes-deployment.md`, `docs/dbip.md`, `docs/measurement-methodology.md`, `docs/security.md`, and `docs/tdd-evidence.md` for deployment, optional DB-IP data, measurement semantics, security boundaries, and RED/GREEN evidence.

## License

MIT © 2026 ModelGarden
