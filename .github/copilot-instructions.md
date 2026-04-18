# Project Guidelines

## Architecture

Kubernetes sidecar (Go) that runs alongside pgAdmin, discovers CloudNativePG clusters, and registers them as pgAdmin servers.

**Package responsibilities:**
- `cmd/` — Entry point, signal handling, log setup
- `internal/config/` — Env var parsing into `Config` struct
- `internal/discovery/` — K8s dynamic client for CNPG CRD listing + typed client for secret reads
- `internal/pgadmin/` — `servers.json`/`.pgpass` generation
- `internal/reconciler/` — Periodic reconciliation loop: discover → write files
- `kustomize/` — FluxCD-ready Kubernetes manifests

The sidecar and pgAdmin share an `emptyDir` volume at `/shared` for `servers.json` and `.pgpass`.

## Build and Test

```bash
make build          # Static binary (CGO_ENABLED=0) → bin/pgadmin-cnpg-discovery
make docker-build   # Multi-stage Docker build → distroless/static:nonroot
make docker-push    # Build + push to registry
make tidy           # go mod tidy
go vet ./...        # Lint
go build ./...      # Compile check
```

## Conventions

- **Error handling:** Wrap with `fmt.Errorf("context: %w", err)`. Fatal only in `main()`. Per-cluster failures log and skip, never crash the loop.
- **Logging:** `log/slog` with JSON handler. Use structured fields: `slog.Info("message", "key", value)`. No `fmt.Println`.
- **File writes:** Always atomic (temp file → chmod → close → rename). `.pgpass` must be `0600`.
- **Kubernetes API:** Dynamic client for CNPG CRDs (avoids importing CNPG Go module). Typed client for core resources (secrets).
- **Managed server tagging:** Servers the sidecar creates carry comment `"Managed by cnpg-discovery"`. Never modify servers without this tag.
- **Secret fallback:** Try `<cluster>-superuser` first, fall back to `<cluster>-app`.
- **Deterministic ordering:** Sort clusters by `namespace/name` before assigning server IDs.
- **Config:** All configuration via environment variables with sensible defaults. See `internal/config/config.go`.

## Project Details

- **Module:** `github.com/AhmedMoalla/pgadmin-cnpg-discovery`
- **Go version:** 1.26
- **Runtime image:** `gcr.io/distroless/static:nonroot`
- **License:** MIT — see `LICENCE.md`
- **Docs:** See `README.md` for deployment guide (kustomize + FluxCD)
