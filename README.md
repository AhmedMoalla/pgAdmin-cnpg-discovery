# pgAdmin CNPG Discovery

![Tests](https://github.com/AhmedMoalla/pgadmin-cnpg-discovery/actions/workflows/test.yml/badge.svg)
![Coverage](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/AhmedMoalla/1273b85033b31d41f9db050263558f5f/raw/coverage.json)

A Kubernetes sidecar that automatically discovers [CloudNativePG](https://cloudnative-pg.io/) (CNPG) clusters and writes pgAdmin-compatible `servers.json` and `.pgpass` files.

## How It Works

The sidecar runs alongside pgAdmin in the same pod. On a configurable interval it:

1. Lists all CNPG `Cluster` resources across all namespaces (or a specific one)
2. Reads each cluster's `-superuser` secret (falls back to `-app` secret) to get connection details
3. Writes `servers.json` and `.pgpass` to a shared volume so pgAdmin picks them up on startup

Generated server entries are tagged with a `"Managed by cnpg-discovery"` comment in `servers.json`.

## Prerequisites

- A Kubernetes cluster with the [CloudNativePG operator](https://cloudnative-pg.io/docs/) installed
- At least one CNPG `Cluster` resource deployed
- `kubectl` configured to access your cluster

## Quick Start

### 1. Create the pgAdmin credentials secret

The deployment expects a secret named `pgadmin-credentials` in the target namespace with your pgAdmin login credentials. pgAdmin itself requires this secret to initialize and start. **You must create this before deploying.**

```bash
kubectl create secret generic pgadmin-credentials \
  --from-literal=PGADMIN_DEFAULT_EMAIL=admin@example.com \
  --from-literal=PGADMIN_DEFAULT_PASSWORD=your-secure-password
```

### 2. Deploy with Kustomize

```bash
kubectl apply -k kustomize/
```

This deploys into the **default** namespace and creates:

- A `ServiceAccount` for the sidecar
- A `ClusterRole` and `ClusterRoleBinding` granting read access to CNPG clusters and secrets
- A `Deployment` with pgAdmin and the cnpg-discovery sidecar
- A `Service` exposing pgAdmin on port 80

The sidecar is configured to run as numeric UID/GID `5050` so Kubernetes can validate `runAsNonRoot` and pgAdmin can read the shared `.pgpass` file written with mode `0600`.

### 3. Access pgAdmin

```bash
kubectl port-forward svc/pgadmin 8080:80
```

Open [http://localhost:8080](http://localhost:8080) and log in with the credentials you set in the secret. Your CNPG clusters will be loaded from `servers.json` when pgAdmin starts under the **CNPG Clusters** server group.

## Deploy with FluxCD

### 1. Create the credentials secret

Create the secret in your cluster (or manage it via SOPS / External Secrets):

```bash
kubectl create secret generic pgadmin-credentials \
  --from-literal=PGADMIN_DEFAULT_EMAIL=admin@example.com \
  --from-literal=PGADMIN_DEFAULT_PASSWORD=your-secure-password
```

### 2. Add a GitRepository source

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: pgadmin-cnpg-discovery
  namespace: flux-system
spec:
  interval: 10m
  url: https://github.com/AhmedMoalla/pgadmin-cnpg-discovery
  ref:
    branch: main
```

### 3. Add a Kustomization

```yaml
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: pgadmin
  namespace: flux-system
spec:
  interval: 10m
  path: ./kustomize
  prune: true
  sourceRef:
    kind: GitRepository
    name: pgadmin-cnpg-discovery
  targetNamespace: default
```

Apply both manifests to your cluster and Flux will reconcile the deployment automatically.

## Configuration

The sidecar is configured via environment variables. These can be adjusted in `kustomize/deployment.yaml`:

| Variable | Default | Description |
|---|---|---|
| `POLL_INTERVAL` | `30s` | How often to poll for CNPG clusters |
| `SERVERS_JSON_PATH` | `/shared/servers.json` | Path to write servers.json |
| `PGPASS_PATH` | `/shared/.pgpass` | Path to write .pgpass |
| `SERVER_GROUP_NAME` | `CNPG Clusters` | Server group name in pgAdmin |
| `NAMESPACE` | *(empty = all)* | Restrict discovery to a namespace |
| `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |

## RBAC

The sidecar requires cluster-wide read access. The kustomize manifests include a `ClusterRole` with:

- `get`, `list`, `watch` on `clusters.postgresql.cnpg.io`
- `get`, `list` on `secrets`

If you want to restrict to a single namespace, use a `Role` / `RoleBinding` instead and set the `NAMESPACE` environment variable.

## Building from Source

```bash
# Build the binary
make build

# Build the Docker image
make docker-build

# Push the Docker image
make docker-push

# Run go mod tidy
make tidy
```

Override the image registry or tag:

```bash
make docker-build DOCKER_USER=myuser IMAGE_TAG=v1.0.0
make docker-push DOCKER_USER=myuser IMAGE_TAG=v1.0.0
```
