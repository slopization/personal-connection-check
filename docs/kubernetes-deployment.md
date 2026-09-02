# Kubernetes deployment

This guide deploys one Personal Connection Check instance behind a WebSocket-capable Kubernetes Ingress controller.

> [!IMPORTANT]
> The test measures the complete client → DNS/proxy/VPN → ingress → application path. To measure the origin path itself, avoid a CDN or externally proxied DNS record; use direct DNS, split DNS, or the intended VPN path.

## 1. Prerequisites

- A working Kubernetes cluster with a WebSocket-capable Ingress controller
- `kubectl` access
- A DNS name pointing to the ingress, such as `connection.example.com`
- A TLS Secret named `pcc-tls`, created manually or by cert-manager
- Bash, OpenSSL, and Docker on the administration machine for credential generation

Use a fixed version tag for a stable deployment. The examples use `v1.0.2`; replace it when selecting another verified release.

## 2. Create the authentication Secret

Generate the password hash with the published image, without installing Argon2id tooling or checking out the source. The password is read without terminal echo and is never placed in a process argument:

```bash
read -rsp 'Shared password: ' PASSWORD; printf '\n'
printf '%s\n' "$PASSWORD" | docker run --rm -i git.kyu.sh/modelgarden/personal-connection-check:v1.0.2 hash-password; unset PASSWORD
```

Generate the session key separately:

```sh
openssl rand -base64 32 | tr '+/' '-_' | tr -d '=\n'; printf '\n'
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

Save the following as `pcc.yaml`. Replace all three occurrences of `connection.example.com`, replace `your-ingress-class` with the cluster's IngressClass name, and confirm the image tag.

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
        runAsUser: 65532
        runAsGroup: 65532
        seccompProfile:
          type: RuntimeDefault
      containers:
        - name: pcc
          image: git.kyu.sh/modelgarden/personal-connection-check:v1.0.2
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
spec:
  ingressClassName: your-ingress-class
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

Keep `replicas: 1`: measurement run state is in memory, and parallel streams must reach the same pod. Configure the selected Ingress controller to preserve WebSocket upgrades. Do not attach compression, buffering, or caching middleware to this route.

The example intentionally omits a CPU limit because cgroup throttling can become the measured bottleneck on fast links. Add a CPU limit only after confirming that it does not cap the expected throughput.

`PCC_TRUSTED_PROXY_CIDRS` is intentionally omitted. The application remains safe but may display the Ingress peer address. Set it only to stable, verified ingress source CIDRs; never trust arbitrary client networks merely to recover `X-Forwarded-For`.

## 4. Verify and use

```sh
kubectl -n pcc get pods,service,ingress
kubectl -n pcc logs deployment/pcc
curl -fsS https://connection.example.com/healthz
```

Open the HTTPS URL on the client device, sign in, and start a measurement. If the health check works but measurement fails, verify that no upstream proxy buffers or compresses responses and that its request timeout exceeds 25 seconds.

For optional offline city and ASN data, including a read-only Kubernetes mount and the required attribution, see [`dbip.md`](dbip.md).

To update, change the immutable image tag and run `kubectl apply -f pcc.yaml`; to remove everything, run `kubectl delete namespace pcc`.
