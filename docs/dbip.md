# Optional DB-IP data

Personal Connection Check can enrich the observed public IP address from operator-mounted MMDB files. Lookups remain local: the application does not send an IP address to DB-IP or another lookup service.

DB-IP Lite is useful when an operator prefers a manually reviewed, fixed snapshot. Its free City Lite and ASN Lite databases are distributed under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/). That license requires attribution but does not require automatic updates or deletion of an older snapshot. Review the current terms on the official download pages before selecting a release:

- [IP to City Lite](https://db-ip.com/db/download/ip-to-city-lite)
- [IP to ASN Lite](https://db-ip.com/db/download/ip-to-asn-lite)

## 1. Select and retain a snapshot

Download the MMDB variants manually. City Lite supplies approximate country and city names; ASN Lite supplies the autonomous-system number and organization displayed by the application as the ISP. Either file may be used alone.

Record the release month and the checksum shown on the DB-IP download page before installing the files. Keep the original compressed downloads, license information, and recorded checksums together so that the selected data can be audited later. Do not commit the databases to this repository.

An example immutable layout on a single k3s node is:

```text
/var/lib/pcc/dbip/2026-09/dbip-city-lite.mmdb
/var/lib/pcc/dbip/2026-09/dbip-asn-lite.mmdb
```

Replace `2026-09` with the release you reviewed. Decompress the downloaded `.mmdb.gz` files, copy them into that directory, and make them readable but not writable by the application:

```bash
set -euo pipefail
release=2026-09
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

gzip -dc "dbip-city-lite-$release.mmdb.gz" > "$work/dbip-city-lite.mmdb"
gzip -dc "dbip-asn-lite-$release.mmdb.gz" > "$work/dbip-asn-lite.mmdb"
sha1sum "$work/dbip-city-lite.mmdb" "$work/dbip-asn-lite.mmdb"
# Compare both outputs with the SHA-1 values on the official download pages.

sudo install -d -m 0755 "/var/lib/pcc/dbip/$release"
sudo install -m 0444 "$work/dbip-city-lite.mmdb" \
  "/var/lib/pcc/dbip/$release/dbip-city-lite.mmdb"
sudo install -m 0444 "$work/dbip-asn-lite.mmdb" \
  "/var/lib/pcc/dbip/$release/dbip-asn-lite.mmdb"
```

No updater or network access from the application pod is needed. Replace the snapshot only after choosing and reviewing another release yourself.

## 2. Mount it in k3s

Merge the following fragment into the Deployment in `docs/k3s-deployment.md`:

```yaml
spec:
  template:
    spec:
      containers:
        - name: pcc
          env:
            # Keep the existing PCC_PUBLIC_ORIGIN entry.
            - name: PCC_GEOIP_CITY_DB
              value: /dbip/dbip-city-lite.mmdb
            - name: PCC_GEOIP_ASN_DB
              value: /dbip/dbip-asn-lite.mmdb
            - name: PCC_FOOTER_MESSAGE
              value: "IP Geolocation by DB-IP: https://db-ip.com/"
          volumeMounts:
            # Keep the existing tmp mount.
            - name: dbip
              mountPath: /dbip
              readOnly: true
      volumes:
        # Keep the existing tmp volume.
        - name: dbip
          hostPath:
            path: /var/lib/pcc/dbip/2026-09
            type: Directory
```

Merge these entries with the existing `env`, `volumeMounts`, and `volumes` lists; do not create duplicate YAML keys. A `hostPath` binds the pod to the node containing the snapshot, which is appropriate for the guide's single-node, single-replica deployment. Use a read-only PVC instead on a multi-node cluster.

Apply the manifest and recreate the pod so the application opens the selected files:

```sh
kubectl apply -f pcc.yaml
kubectl -n pcc rollout restart deployment/pcc
kubectl -n pcc rollout status deployment/pcc --timeout=120s
```

Sign in and confirm that a public client IP shows the expected city/country or ASN organization. Private, loopback, and some reserved addresses have no public DB-IP record.

## 3. Footer and attribution behavior

`PCC_FOOTER_MESSAGE` is optional and public. It appears before and after login. The application renders it as plain text and converts only `http://` and `https://` URL substrings into links; HTML is never interpreted. Do not put credentials, internal notes, or other secrets in this value.

When DB-IP Lite results are used in the web application, retain the attribution link required by DB-IP. The example value above provides that visible link. The same variable can instead carry a short operator notice when no attribution is needed.

Changing only an environment value still requires the Deployment to roll out a new pod. The message and opened MMDB snapshot remain fixed for that pod's lifetime.
