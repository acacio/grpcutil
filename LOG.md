# LOG.md — grpcutil change log

---

## 2026-03-18T13:47:00

PROMPT: using the module https://github.com/acacio/totp-token to generate and verify TOTP tokens, add an authorization option to have clients authenticate themselves with a TOTP bearer token

### Summary of Changes

**New file: `totpauth.go`**
- `TOTPAuth` struct: thread-safe wrapper around `*twofactor.Totp` with `sync.Mutex`; implements `credentials.PerRPCCredentials` for client-side use
- `NewTOTPAuth(t *twofactor.Totp) *TOTPAuth` — constructor
- `(*TOTPAuth).OTP() (string, error)` — mutex-guarded token generation
- `(*TOTPAuth).Validate(token string) error` — mutex-guarded token validation
- `(*TOTPAuth).GetRequestMetadata(...)` — generates fresh TOTP code as Bearer token per RPC
- `(*TOTPAuth).RequireTransportSecurity() bool` — returns true (TLS required)
- `WithPerRPCTOTP(auth *TOTPAuth) grpc.DialOption` — DialOption helper
- `TOTPAuthFunc(auth *TOTPAuth) grpc_auth.AuthFunc` — server interceptor factory
- `TOTPAuthCheck(ctx, auth) (context.Context, error)` — validates incoming Bearer token; maps `LockDownError` to `codes.ResourceExhausted`, mismatch to `codes.Unauthenticated`

**New file: `totpauth_test.go`** — 20 tests covering:
- Unit: `NewTOTPAuth`, `OTP` format, `Validate` (valid/invalid/empty), `RequireTransportSecurity`, `GetRequestMetadata`
- Unit: `WithPerRPCTOTP`, TLS constraint enforcement
- Unit: `TOTPAuthCheck` (success, no-metadata, no-header, missing-Bearer-prefix, invalid, lockdown after 3 failures)
- Unit: `TOTPAuthFunc` (non-nil, valid, invalid)
- Concurrency: `ConcurrentOTP` (20 goroutines), `ConcurrentValidate_NoDataRace` (20 goroutines); passes `-race` detector
- Integration (bufconn): valid token, invalid token, missing token, lockout

**Dependency added:** `github.com/acacio/totp-token v1.2.2` (transitively: `sec51/convert`, `sec51/cryptoengine`, `sec51/qrcode`, `sec51/gf256`)

**Updated:** `DESIGN.md`, `README.md`

Total tests: 64 (all passing, race-clean).

---

## 2026-03-18T13:34:00

PROMPT: review code and update to latest gRPC and Protobuf APIs. Build extensive coverage tests.

### Summary of Changes

**Updated to latest gRPC and Protobuf APIs:**

- `client.go`: replaced deprecated `grpc.Dial` with `grpc.NewClient`; replaced `grpc.WithInsecure()` with `grpc.WithTransportCredentials(insecure.NewCredentials())`; removed deprecated `grpc.FailOnNonTempDialError` and `grpc.WithBlock()`; implemented blocking semantics via `conn.Connect()` + `WaitForStateChange`; fixed hardcoded token to use `opts.Token`; updated retry interceptor import to `go-grpc-middleware/v2`
- `bearer.go`: updated auth interceptor import from `go-grpc-middleware/auth` to `go-grpc-middleware/v2/interceptors/auth`; removed debug `spew.Dump()` call and `github.com/davecgh/go-spew` dependency
- `srvopts.go`: replaced `grpc_middleware.ChainStreamServer`/`ChainUnaryServer` with gRPC built-in `grpc.ChainStreamInterceptor`/`grpc.ChainUnaryInterceptor`; removed opentracing and ctxtags interceptors (deprecated/removed in v2); updated recovery import to `go-grpc-middleware/v2/interceptors/recovery`
- `prometheus.go`: replaced all direct Protobuf field access with getter methods (`GetHistogram()`, `GetSampleCount()`, `GetSampleSum()`, `GetLabel()`, `GetName()`, `GetValue()`, `GetBucket()`, `GetUpperBound()`, `GetCumulativeCount()`); fixed potential divide-by-zero in `GetgRPCMetrics`
- `peer.go`: added nil check for `p.AuthInfo` to prevent panic on insecure (non-TLS) connections
- `listen.go`: improved log messages with port context

**Dependency updates (`go.mod`):**
- `google.golang.org/grpc`: v1.40.0 → v1.79.3
- `google.golang.org/protobuf`: v1.26.0-rc.1 → v1.36.10
- `github.com/grpc-ecosystem/go-grpc-middleware/v2`: added v2.3.3 (replaces v1 chain/auth/recovery/retry)
- `github.com/prometheus/client_golang`: v1.11.0 → v1.23.2
- `github.com/prometheus/client_model`: v0.2.0 → v0.6.2
- Removed: `github.com/davecgh/go-spew`, `github.com/golang/protobuf`, `github.com/opentracing/opentracing-go`, `go-grpc-middleware` v1 chain package

**New test files (44 tests, all passing):**
- `basicauth_test.go`: 5 tests — `GetRequestMetadata`, `RequireTransportSecurity`, `Digest` variants
- `bearer_test.go`: 12 tests — `TokenAuth` metadata, `TokenAuthCheck` (success, no-metadata, no-header, wrong-prefix, invalid-token, spaces-in-token), `TokenAuthFunc`
- `serverauth_test.go`: 5 tests — `CheckRPCAuth` (no-metadata, no-header, valid, multiple-values, empty-value)
- `peer_test.go`: 4 tests — no-context, nil AuthInfo, with AuthInfo, Unix socket
- `srvopts_test.go`: 6 tests — `KeepAliveDefault`, `DefaultServerOptions` variants
- `client_test.go`: 6 tests — `SetupConnection` and `setupDialOpts` variants including TLS constraint verification
- `listen_test.go`: 1 test — `Serve` start/serve/stop lifecycle
- `integration_test.go`: 6 tests — end-to-end bearer auth (valid/invalid/missing), `DefaultServerOptions`, keepalive, peer address capture via `bufconn`

**New documentation:**
- `DESIGN.md`: documents all API choices, rationale, removed dependencies, security constraints
- `README.md`: updated with current API examples, configuration table, auth patterns, client setup
- `LOG.md`: this file
