# k3s deployment

This guide deploys one Personal Connection Check instance behind the Traefik ingress controller included with a default k3s installation.

> [!IMPORTANT]
> The test measures the complete phone → DNS/proxy/VPN → ingress → application path. To measure the NAS path itself, avoid a CDN or externally proxied DNS record; use direct DNS, split DNS, or the intended VPN path.

## 1. Prerequisites

- A working k3s cluster with Traefik or another WebSocket-capable ingress controller
- `kubectl` access
- A DNS name pointing to the ingress, such as `connection.example.com`
- A TLS Secret named `pcc-tls`, created manually or by cert-manager
- Bash, OpenSSL, and Docker on the administration machine for secret generation

Use an immutable `sha-<12>` image tag for a stable deployment. The examples use `sha-cbed2b5d4c26`; replace it when selecting another verified revision.

## 2. Create the authentication Secret

The password is read from standard input and is never placed in a process argument. Temporary files are mode `0600` and removed automatically.

```bash
set -euo pipefail
kubectl create namespace pcc --dry-run=client -o yaml | kubectl apply -f -

IMAGE='git.kyu.sh/modelgarden/personal-connection-check:sha-cbed2b5d4c26'
secret_dir="$(mktemp -d)"
chmod 700 "$secret_dir"
trap 'rm -rf "$secret_dir"; unset PASSWORD' EXIT

read -rsp 'Shared password: ' PASSWORD
printf '\n'
printf '%s\n' "$PASSWORD" | docker run --rm -i "$IMAGE" hash-password \
  | tr -d '\r\n' > "$secret_dir/PCC_SHARED_PASSWORD_HASH"
unset PASSWORD
openssl rand -base64 32 | tr '+/' '-_' | tr -d '=\n' \
  > "$secret_dir/PCC_SESSION_KEYS"
chmod 600 "$secret_dir"/*

kubectl -n pcc create secret generic pcc-secrets \
  --from-file="$secret_dir/PCC_SHARED_PASSWORD_HASH" \
  --from-file="$secret_dir/PCC_SESSION_KEYS" \
  --dry-run=client -o yaml | kubectl apply -f -
```

Keep the session key stable across restarts. Rotating it invalidates existing login sessions; comma-separated old keys may be retained after the new first key.

## 3. Apply the workload

Save the following as `pcc.yaml`. Replace all three occurrences of `connection.example.com` and confirm the image tag.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: pcc
  namespace: pcc
spec:
  replicas: 1
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: pcc
  template:
    metadata:
      labels:
        app: pcc
    spec:
      automountServiceAccountToken: false
      securityContext:
        runAsNonRoot: true
        seccompProfile:
          type: RuntimeDefault
      containers:
        - name: pcc
          image: git.kyu.sh/modelgarden/personal-connection-check:sha-cbed2b5d4c26
          imagePullPolicy: IfNotPresent
          ports:
            - name: http
              containerPort: 8080
          env:
            - name: PCC_PUBLIC_ORIGIN
              value: https://connection.example.com
          envFrom:
            - secretRef:
                name: pcc-secrets
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop: ["ALL"]
            readOnlyRootFilesystem: true
          readinessProbe:
            httpGet:
              path: /healthz
              port: http
            periodSeconds: 5
            timeoutSeconds: 2
          livenessProbe:
            httpGet:
              path: /healthz
              port: http
            periodSeconds: 30
            timeoutSeconds: 2
          resources:
            requests:
              cpu: 25m
              memory: 64Mi
            limits:
              memory: 256Mi
          volumeMounts:
            - name: tmp
              mountPath: /tmp
      volumes:
        - name: tmp
          emptyDir:
            medium: Memory
            sizeLimit: 16Mi
---
apiVersion: v1
kind: Service
metadata:
  name: pcc
  namespace: pcc
spec:
  selector:
    app: pcc
  ports:
    - name: http
      port: 80
      targetPort: http
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: pcc
  namespace: pcc
  annotations:
    traefik.ingress.kubernetes.io/router.entrypoints: websecure
    traefik.ingress.kubernetes.io/router.tls: "true"
spec:
  ingressClassName: traefik
  tls:
    - hosts:
        - connection.example.com
      secretName: pcc-tls
  rules:
    - host: connection.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: pcc
                port:
                  name: http
```

```sh
kubectl apply -f pcc.yaml
kubectl -n pcc rollout status deployment/pcc --timeout=120s
```

Keep `replicas: 1`: measurement run state is in memory, and parallel streams must reach the same pod. Traefik handles WebSocket upgrades without a special middleware. Do not attach compression, buffering, or caching middleware to this route.

The example intentionally omits a CPU limit because cgroup throttling can become the measured bottleneck on fast links. Add a CPU limit only after confirming that it does not cap the expected throughput.

`PCC_TRUSTED_PROXY_CIDRS` is intentionally omitted. The application remains safe but may display the Traefik peer address. Set it only to stable, verified ingress source CIDRs; never trust arbitrary client networks merely to recover `X-Forwarded-For`.

## 4. Verify and use

```sh
kubectl -n pcc get pods,service,ingress
kubectl -n pcc logs deployment/pcc
curl -fsS https://connection.example.com/healthz
```

Open the HTTPS URL on the phone, sign in, and start a measurement. If the health check works but measurement fails, verify that no upstream proxy buffers or compresses responses and that its request timeout exceeds 25 seconds.

To update, change the immutable image tag and run `kubectl apply -f pcc.yaml`; to remove everything, run `kubectl delete namespace pcc`.
