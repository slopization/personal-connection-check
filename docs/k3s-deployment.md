# k3s deployment

This guide deploys one Personal Connection Check instance behind the Traefik ingress controller included with a default k3s installation.

> [!IMPORTANT]
> The test measures the complete phone → DNS/proxy/VPN → ingress → application path. To measure the NAS path itself, avoid a CDN or externally proxied DNS record; use direct DNS, split DNS, or the intended VPN path.

## 1. Prerequisites

- A working k3s cluster with Traefik or another WebSocket-capable ingress controller
- `kubectl` access
- A DNS name pointing to the ingress, such as `connection.example.com`
- A TLS Secret named `pcc-tls`, created manually or by cert-manager
- Bash, OpenSSL, and Docker on the administration machine for credential generation

Use a fixed version tag for a stable deployment. The examples use `v1.0.0`; replace it when selecting another verified release.

## 2. Create the authentication Secret

Generate the password hash and session key locally. The password is read from standard input and is never placed in a process argument. Copy each one-line result for use in the Secret file below.

```bash
set -euo pipefail
IMAGE='git.kyu.sh/modelgarden/personal-connection-check:v1.0.0'

read -rsp 'Shared password: ' PASSWORD
trap 'unset PASSWORD' EXIT HUP INT TERM
printf '\nPCC_SHARED_PASSWORD_HASH='
printf '%s\n' "$PASSWORD" | docker run --rm -i "$IMAGE" hash-password \
  | tr -d '\r\n'
printf '\n'
unset PASSWORD
trap - EXIT HUP INT TERM
printf 'PCC_SESSION_KEYS='
openssl rand -base64 32 | tr '+/' '-_' | tr -d '=\n'
printf '\n'
```

Save the following as `pcc-secrets.yaml`, replace both placeholders with the generated one-line values, and keep the file outside version control:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: pcc-secrets
  namespace: pcc
type: Opaque
stringData:
  PCC_SHARED_PASSWORD_HASH: "<change_me>"
  PCC_SESSION_KEYS: "<change_me>"
```

Protect the file before creating the Secret. `kubectl create -f` reads the values from the file rather than placing them in command arguments.

```sh
chmod 600 pcc-secrets.yaml
kubectl create namespace pcc
kubectl create -f pcc-secrets.yaml
```

Delete the local Secret file after a separately protected backup exists. Do not commit it, paste it into issue comments, or include it in support logs.

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
          image: git.kyu.sh/modelgarden/personal-connection-check:v1.0.0
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

For optional offline city and ASN data, including a read-only k3s mount and the required attribution, see [`dbip.md`](dbip.md).

To update, change the immutable image tag and run `kubectl apply -f pcc.yaml`; to remove everything, run `kubectl delete namespace pcc`.
