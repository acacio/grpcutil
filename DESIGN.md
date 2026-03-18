# DESIGN.md — grpcutil

## Overview

`grpcutil` is a thin wrapper library that consolidates common gRPC boilerplate for Go services: TLS/auth setup, middleware chaining, Prometheus metrics, and protocol bridging. It is deliberately kept narrow in scope — zero generated code, no proto files.

---

## Architecture

```
┌─────────────────────────────────────────┐
│  Application Layer                      │
│  (service implementations)              │
├─────────────────────────────────────────┤
│  Middleware Layer  (srvopts.go)          │
│  · Prometheus interceptors              │
│  · Panic recovery                       │
│  · gRPC built-in chain (v1.21+)         │
├────────────────┬────────────────────────┤
│  Auth Layer    │  Protocol Layer        │
│  bearer.go     │  grpcweb.go            │
│  basicauth.go  │  listen.go             │
│  serverauth.go │  prometheus.go         │
├────────────────┴────────────────────────┤
│  Transport Layer                        │
│  client.go — grpc.NewClient             │
│  peer.go   — peer info extraction       │
│  TLS via github.com/acacio/tlsutil      │
└─────────────────────────────────────────┘
```

---

## Design Decisions

### gRPC API: `grpc.NewClient` over `grpc.Dial`

**Decision:** Use `grpc.NewClient` (added in gRPC v1.63) as the sole connection factory.

**Rationale:**
- `grpc.Dial` is deprecated as of gRPC v1.64.
- `grpc.NewClient` has cleaner semantics: it constructs a `ClientConn` lazily without initiating a connection immediately.
- `grpc.WithBlock()` and `grpc.FailOnNonTempDialError` are deprecated and incompatible with `grpc.NewClient`.

**Blocking behavior:** When `ClientOpts.Block == true`, we call `conn.Connect()` and use `conn.WaitForStateChange` with a 30-second timeout to implement blocking semantics without the deprecated `WithBlock` option.

---

### Insecure Transport: `insecure.NewCredentials()` over `grpc.WithInsecure()`

**Decision:** Use `google.golang.org/grpc/credentials/insecure.NewCredentials()`.

**Rationale:**
- `grpc.WithInsecure()` is deprecated since gRPC v1.45.
- The `insecure` package makes non-TLS intent explicit and type-safe.

---

### Middleware Chaining: gRPC Built-ins over `go-grpc-middleware` v1 chains

**Decision:** Use `grpc.ChainUnaryInterceptor` / `grpc.ChainStreamInterceptor` (built into gRPC since v1.21) instead of `grpc_middleware.ChainUnaryServer` / `grpc_middleware.ChainStreamServer` from `go-grpc-middleware` v1.

**Rationale:**
- The v1 chain helpers are redundant now that gRPC ships its own.
- Reduces dependency on `go-grpc-middleware` v1 (kept only for Prometheus interceptors).

---

### Middleware: `go-grpc-middleware/v2` for Auth and Recovery

**Decision:** Updated `auth` and `recovery` interceptors to `github.com/grpc-ecosystem/go-grpc-middleware/v2`.

**Rationale:**
- v2 is the maintained branch with gRPC v1.63+ compatibility.
- Package paths changed:
  - `auth`: `v1/auth` → `v2/interceptors/auth`
  - `recovery`: `v1/recovery` → `v2/interceptors/recovery`
  - `retry`: `v1/retry` → `v2/interceptors/retry`
- **Not updated:** `go-grpc-prometheus` stays at v1.2.0 because the prometheus package was removed from `go-grpc-middleware/v2`. No v2 of `go-grpc-prometheus` exists independently.

---

### Removed Dependencies

| Package | Reason |
|---|---|
| `github.com/davecgh/go-spew` | Debug-only `spew.Dump()` call removed from production code |
| `github.com/golang/protobuf` | Superseded by `google.golang.org/protobuf` (same wire format) |
| `github.com/opentracing/opentracing-go` | OpenTracing is deprecated; replaced by OpenTelemetry in modern systems. Removed from default interceptors. |
| `github.com/grpc-ecosystem/go-grpc-middleware` v1 chains | Replaced by gRPC built-in chaining |
| `github.com/grpc-ecosystem/go-grpc-middleware` ctxtags | Not present in v2; removed from defaults |

---

### Protobuf Getter API

**Decision:** All access to Protobuf-generated message fields uses generated getter methods (`GetXxx()`) rather than direct struct field access or pointer dereferences.

**Rationale:** The CLAUDE.md project standard. Getter methods are nil-safe, forward-compatible across protobuf schema evolution, and work correctly with both `proto2` and `proto3` optional fields.

**Changed in `prometheus.go`:**

| Before | After |
|---|---|
| `*data.Histogram.SampleCount` | `data.GetHistogram().GetSampleCount()` |
| `*data.Histogram.SampleSum` | `data.GetHistogram().GetSampleSum()` |
| `data.Label` | `data.GetLabel()` |
| `*v.Name == "grpc_method"` | `v.GetName() == "grpc_method"` |
| `*v.Value` | `v.GetValue()` |

---

### Nil Safety in `peer.go`

**Decision:** Added nil check for `p.AuthInfo` before calling `.AuthType()`.

**Rationale:** Insecure (non-TLS) gRPC connections set `AuthInfo` to nil. The previous code would panic at `p.AuthInfo.AuthType()` on any plaintext connection.

---

### Security Constraint: Token + Insecure Transport

`TokenAuth.RequireTransportSecurity()` returns `true`. gRPC enforces this at `NewClient` time: attempting to use `WithPerRPCCredentials(TokenAuth{...})` with `insecure.NewCredentials()` returns an error. This is **by design** — Bearer tokens must not be sent over unencrypted connections.

In tests that need to exercise auth logic without TLS, inject the token directly into the outgoing context metadata:
```go
ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
```

---

---

## TOTP Authentication Design

### Motivation

Static Bearer tokens (from `bearer.go`) are long-lived secrets — if intercepted they grant indefinite access. TOTP Bearer tokens are valid for only ~30 seconds (±1 window), providing a significant security improvement with no infrastructure dependency beyond a shared key.

### `TOTPAuth` wrapper struct

`twofactor.Totp` is stateful: `Validate()` mutates `totalVerificationFailures`, `lastVerificationTime`, and `clientOffset`. `OTP()` mutates `counter`. Both methods are used concurrently by different gRPC goroutines.

**Decision:** Wrap `*twofactor.Totp` in a `TOTPAuth` struct with a `sync.Mutex`. All method calls are serialised.

**Alternative considered:** Require callers to bring their own synchronisation. Rejected — too easy to misuse and the locking overhead is negligible relative to HMAC computation.

### Separate client and server instances

Both client and server use `TOTPAuth`, but from **separate** `twofactor.Totp` instances sharing the same key. Using a single shared instance would cause state clobbering: `Validate()` updates `clientOffset` (counter sync) while `OTP()` uses it to generate the next token.

Constructor: `NewTOTPFromKey(key, account, issuer, digits)` — deterministic from a shared secret. For provisioning, callers can use `ToBytes`/`TOTPFromBytes` to securely distribute the key.

### Bearer token format

TOTP codes are sent in the same `Authorization: Bearer <code>` format as static tokens. This means:
- `TOTPAuthCheck` and `TokenAuthCheck` are drop-in alternatives for the same interceptor slot.
- Clients cannot be distinguished by header format alone — they are distinguished by which `Auth` type the server is configured with.

### Lockout handling

`twofactor.Validate()` enforces a 3-attempt lockout with a 5-minute backoff (`LockDownError`). grpcutil maps this to `codes.ResourceExhausted`, which correctly signals a rate-limit rather than an auth failure (`Unauthenticated`). Clients that receive `ResourceExhausted` should back off and retry after 5 minutes.

---

## Options Considered but Not Taken

### OpenTelemetry tracing interceptors
- v2 of `go-grpc-middleware` supports OTel tracing via `interceptors/tracing`.
- Not adopted here because it would require users to also set up an OTel SDK/exporter, making grpcutil significantly heavier.
- Recommendation: users who need tracing should add OTel interceptors via `DefaultServerOptions`'s `srvOpts` parameter.

### Custom Prometheus registry
- Considered using a separate `prometheus.NewRegistry()` instead of the default global registry.
- Rejected to maintain backward compatibility with existing Prometheus scrape configurations and to keep `EnablePrometheus` zero-config.
