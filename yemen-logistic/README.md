# MIT Yemen Logistics Platform — Phase 1

Core architecture, database schema, and offline-first cryptographic QR
verification service.

## Build

This sandbox has no Go toolchain and no network access, so the commands
below could not be executed here — run them yourself before deploying:

```bash
go mod download
go build ./...
go vet ./...
go run ./cmd/api
```

Once running, the diagnostic round-trip route is at:

```
GET http://localhost:8080/api/v1/qr/test
```

It generates a fresh RSA key pair at process startup, signs a sample
invoice into an RS256 QR token, verifies that token completely offline
(no DB, no network call in the verification path), and returns both the
token and the decoded/verified claims as JSON — proving the full
checkpoint workflow end to end.

## What was manually audited (in lieu of a live `go build`)

- Every `{`/`}`, `(`/`)` pair in every file balances.
- Every imported package is referenced at least once (no "imported and not
  used" compile errors); every declared variable is used.
- `internal/checkpoint/qr_service.go` cross-checked line-by-line against the
  real `github.com/golang-jwt/jwt/v5` API surface (`jwt.NewParser`,
  `jwt.WithValidMethods`, `jwt.WithIssuer`, `jwt.RegisteredClaims` field
  names, `jwt.ErrTokenExpired` / `ErrTokenSignatureInvalid` / `ErrTokenInvalidIssuer`
  sentinel errors) as of v5.2.x.
- `internal/domain/models.go` struct field names/types match the DDL in
  `migrations/000001_init_schema.up.sql` column-for-column.
- `cmd/api/main.go` cross-checked against the real `go-chi/chi/v5`,
  `go-chi/cors`, and stdlib `net/http`/`context`/`os/signal` APIs.

Please still run `go build ./...` yourself as the final gate before this
touches production — an LLM-assisted manual review is not a substitute for
the compiler.

## Next phases (not in scope today)

- Postgres connection pool + repository implementations
  (`internal/platform/database`, `internal/merchant`, `internal/shipment`,
  `internal/invoice`) fulfilling the `internal/ports` interfaces.
- Auth/RBAC for Ministry staff and inspectors (referenced but deliberately
  not modeled yet — see the comment on `checkpoints.inspector_id` in the
  migration).
- Checkpoint scan audit log table + background sync protocol for when an
  offline checkpoint device regains connectivity.
- Key rotation strategy for the RS256 signing key.
