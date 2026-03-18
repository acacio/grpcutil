# grpcutil

gRPC utility library for Go — authentication (Bearer token, TOTP, Basic auth), Prometheus metrics, keepalive, gRPC-Web bridging, and middleware chaining.

```
import "github.com/acacio/grpcutil"
```

Requires **Go 1.21+** and **gRPC v1.63+**.

---

## Table of Contents

- [Server Setup](#server-setup)
- [Client Setup](#client-setup)
- [Authentication](#authentication)
  - [Static Bearer Token](#static-bearer-token)
  - [TOTP Bearer Token](#totp-bearer-token)
  - [Basic Auth](#basic-auth)
  - [Presence Check](#presence-check)
- [Prometheus Metrics](#prometheus-metrics)
- [Keepalive](#keepalive)
- [gRPC-Web](#grpc-web)
- [Peer Information](#peer-information)
- [API Reference](#api-reference)

---

## Server Setup

```go
import (
    "net/http"
    "github.com/acacio/grpcutil"
    grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
    "google.golang.org/grpc"
    "google.golang.org/grpc/reflection"
    pb "path/to/your/proto"
)

func startServer(port, metricsPort, grpcwebport string) {
    // DefaultServerOptions adds Prometheus metrics + panic recovery interceptors.
    // Pass any additional options (keepalive, auth interceptors, etc.) in the slice.
    opts := grpcutil.DefaultServerOptions([]grpc.ServerOption{
        grpcutil.KeepAliveDefault(),
        grpc.ChainUnaryInterceptor(
            grpc_auth.UnaryServerInterceptor(grpcutil.TokenAuthFunc("your-secret-token")),
        ),
        grpc.ChainStreamInterceptor(
            grpc_auth.StreamServerInterceptor(grpcutil.TokenAuthFunc("your-secret-token")),
        ),
    })

    s := grpc.NewServer(opts...)
    pb.RegisterYourServer(s, &yourServiceImpl{})
    reflection.Register(s) // optional: enables grpcurl / gRPC reflection

    // Enable Prometheus metrics — call after all services are registered
    http.Handle("/metrics", grpcutil.EnablePrometheus(s, port))
    go http.ListenAndServe(metricsPort, nil)

    // Optional: bridge gRPC-Web clients on a separate port
    go grpcutil.StartgRPCWeb(s, grpcwebport)

    grpcutil.Serve(s, port) // blocks; calls log.Fatalf on error
}
```

`DefaultServerOptions` uses `grpc.ChainUnaryInterceptor` / `grpc.ChainStreamInterceptor` (gRPC v1.21+), so additional chain calls in the input slice are merged correctly — no panics from duplicate interceptor options.

---

## Client Setup

```go
conn, err := grpcutil.SetupConnection("server:50051", &grpcutil.ClientOpts{
    TLSType: "tls",               // "tls", "mtls", or "insecure"
    CA:      "/path/to/ca.pem",   // required for "tls" and "mtls"
    Cert:    "/path/to/cert.pem", // required for "mtls"
    Key:     "/path/to/key.pem",  // required for "mtls"
    Token:   "your-bearer-token", // static Bearer token; requires TLS
    Block:   false,               // true → wait up to 30s for READY state
})
```

`SetupConnection` uses `grpc.NewClient` (lazy connection, no immediate dial) and automatically retries with linear 100 ms backoff on `DeadlineExceeded` and `Unavailable` errors.

> **Note:** Setting `Token` with `TLSType: "insecure"` returns an error at connection creation time. `TokenAuth.RequireTransportSecurity()` returns `true` and gRPC enforces this.

### `ClientOpts` fields

| Field     | Type   | Description |
|-----------|--------|-------------|
| `TLSType` | string | `"tls"`, `"mtls"`, or `"insecure"` (default when empty) |
| `CA`      | string | Path to PEM CA certificate (`"tls"` / `"mtls"`) |
| `Cert`    | string | Path to PEM client certificate (`"mtls"` only) |
| `Key`     | string | Path to PEM client private key (`"mtls"` only) |
| `Token`   | string | Static Bearer token — requires a TLS transport |
| `Block`   | bool   | Wait up to 30 s for the connection to reach READY state |

---

## Authentication

### Static Bearer Token

A long-lived secret sent as `Authorization: Bearer <token>` on every RPC.

**Server — interceptor (all RPCs):**

```go
import grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"

grpc.ChainUnaryInterceptor(
    grpc_auth.UnaryServerInterceptor(grpcutil.TokenAuthFunc("your-server-token")),
)
grpc.ChainStreamInterceptor(
    grpc_auth.StreamServerInterceptor(grpcutil.TokenAuthFunc("your-server-token")),
)
```

**Server — per-handler check:**

```go
func (s *Server) MyRPC(ctx context.Context, req *pb.Req) (*pb.Resp, error) {
    if _, err := grpcutil.TokenAuthCheck(ctx, "your-server-token"); err != nil {
        return nil, err // codes.InvalidArgument or codes.Unauthenticated
    }
    // ...
}
```

**Client — attach to every RPC (requires TLS):**

```go
conn, _ := grpc.NewClient(addr,
    grpc.WithTransportCredentials(tlsCreds),
    grpcutil.WithPerRPCToken("your-token"),
)
```

**Client — inject per-call (safe with insecure transport, useful in tests):**

```go
ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer your-token")
```

**Error codes:**

| Situation | gRPC code |
|---|---|
| No metadata | `InvalidArgument` |
| Missing `Authorization` header | `InvalidArgument` |
| Missing `Bearer ` prefix | `Unauthenticated` |
| Wrong token | `Unauthenticated` |

---

### TOTP Bearer Token

Time-based One-Time Password (RFC 6238) tokens are valid for ~30 seconds (±1 window) and refresh automatically. Backed by [`github.com/acacio/totp-token`](https://github.com/acacio/totp-token).

**1. Create a shared key and build client/server auth objects:**

```go
import (
    twofactor "github.com/acacio/totp-token/twofactor"
    "github.com/acacio/grpcutil"
)

// Both sides need separate Totp instances built from the same key.
// The underlying Totp state is mutable; grpcutil.TOTPAuth adds a mutex.
sharedKey := []byte("your-secret-key-min-20-bytes!!!!")

serverTotp, _ := twofactor.NewTOTPFromKey(sharedKey, "user@example.com", "MyApp", 6)
serverAuth := grpcutil.NewTOTPAuth(serverTotp)

clientTotp, _ := twofactor.NewTOTPFromKey(sharedKey, "user@example.com", "MyApp", 6)
clientAuth := grpcutil.NewTOTPAuth(clientTotp)
```

Alternatively, create once with `twofactor.NewTOTP` and distribute via `ToBytes` / `TOTPFromBytes`:

```go
totp, _ := twofactor.NewTOTP("user@example.com", "MyApp", crypto.SHA1, 6)
data, _ := totp.ToBytes()                              // encrypted bytes, share securely
clientTotp, _ := twofactor.TOTPFromBytes(data, "MyApp") // on the client
```

**2. Server setup:**

```go
import grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"

opts := grpcutil.DefaultServerOptions([]grpc.ServerOption{
    grpc.ChainUnaryInterceptor(
        grpc_auth.UnaryServerInterceptor(grpcutil.TOTPAuthFunc(serverAuth)),
    ),
    grpc.ChainStreamInterceptor(
        grpc_auth.StreamServerInterceptor(grpcutil.TOTPAuthFunc(serverAuth)),
    ),
})
s := grpc.NewServer(opts...)
```

Per-handler:

```go
func (s *Server) MyRPC(ctx context.Context, req *pb.Req) (*pb.Resp, error) {
    if _, err := grpcutil.TOTPAuthCheck(ctx, serverAuth); err != nil {
        return nil, err
    }
    // ...
}
```

**3. Client setup (requires TLS):**

```go
conn, _ := grpc.NewClient(addr,
    grpc.WithTransportCredentials(tlsCreds),
    grpcutil.WithPerRPCTOTP(clientAuth), // generates a fresh code per RPC
)
```

Per-call context injection (works without TLS, useful in tests):

```go
token, _ := clientAuth.OTP()
ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
```

**Error codes:**

| Situation | gRPC code |
|---|---|
| No metadata | `InvalidArgument` |
| Missing `Authorization` header | `InvalidArgument` |
| Missing `Bearer ` prefix | `Unauthenticated` |
| Wrong or expired code | `Unauthenticated` |
| Locked out after 3 failures | `ResourceExhausted` (retry after 5 min) |

`TOTPAuth` is safe for concurrent use — a `sync.Mutex` protects the mutable `twofactor.Totp` state (failure counter, lockout time, counter sync offset).

---

### Basic Auth

HTTP Basic authentication (`Authorization: Basic <base64(user:pass)>`).

```go
creds := grpcutil.NewBasicAuthCreds("alice", "s3cr3t")

conn, _ := grpc.NewClient(addr,
    grpc.WithTransportCredentials(tlsCreds), // Basic auth requires TLS
    grpc.WithPerRPCCredentials(creds),
)
```

To validate on the server side, decode the `authorization` header value with `base64.StdEncoding.DecodeString` after stripping the `Basic ` prefix, then split on `:`.

---

### Presence Check

`CheckRPCAuth` is a lightweight server-side helper that only verifies the `authorization` header is present — it does **not** validate the value. Useful as a quick guard before delegating to a full auth layer.

```go
func (s *Server) MyRPC(ctx context.Context, req *pb.Req) (*pb.Resp, error) {
    if err := grpcutil.CheckRPCAuth(ctx); err != nil {
        return nil, err // codes.InvalidArgument if header is absent
    }
    // ...
}
```

---

## Prometheus Metrics

```go
// Call after all services are registered with the server.
http.Handle("/metrics", grpcutil.EnablePrometheus(s, port))
go http.ListenAndServe(":9090", nil)
```

Query collected data programmatically:

```go
// Average latency per method in milliseconds
lats := grpcutil.GetgRPCMetrics() // map[string]float64

// Full histogram bucket distribution per method
hists := grpcutil.GetgRPCHistograms() // map[string]map[float64]uint64
```

---

## Keepalive

`KeepAliveDefault` configures the server to keep connections alive indefinitely with a 2-hour ping interval and 30-second timeout.

```go
opts := grpcutil.DefaultServerOptions([]grpc.ServerOption{
    grpcutil.KeepAliveDefault(),
})
```

Default values:

| Parameter | Value |
|---|---|
| `MaxConnectionIdle` | unlimited |
| `MaxConnectionAge` | unlimited |
| `MaxConnectionAgeGrace` | unlimited |
| `Time` (ping interval) | 2 hours |
| `Timeout` | 30 seconds |

---

## gRPC-Web

Wrap the gRPC server and serve gRPC-Web protocol on a second HTTP port (for browser clients):

```go
go grpcutil.StartgRPCWeb(s, ":8080")
```

---

## Peer Information

Extract the client's network address from the gRPC context. Returns `nil` if no peer information is available (e.g. in unit tests without a real transport).

```go
func (s *Server) MyRPC(ctx context.Context, req *pb.Req) (*pb.Resp, error) {
    addr := grpcutil.PeerAddress(ctx) // net.Addr or nil
    if addr != nil {
        log.Printf("request from %s", addr)
    }
    // ...
}
```

---

## API Reference

### Server

| Function | Description |
|---|---|
| `Serve(s *grpc.Server, port string)` | Start gRPC server on `port` (e.g. `":50051"`); blocks |
| `DefaultServerOptions(opts []grpc.ServerOption) []grpc.ServerOption` | Append Prometheus metrics + panic recovery interceptors |
| `KeepAliveDefault() grpc.ServerOption` | Keepalive with 2h ping, 30s timeout, unlimited lifetime |
| `EnablePrometheus(s *grpc.Server, port string) http.Handler` | Register server with Prometheus; returns `/metrics` handler |
| `GetgRPCMetrics() map[string]float64` | Average latency per method (milliseconds) |
| `GetgRPCHistograms() map[string]map[float64]uint64` | Histogram bucket distribution per method |
| `StartgRPCWeb(s *grpc.Server, port string)` | Serve gRPC-Web on a separate HTTP port |
| `PeerAddress(ctx context.Context) net.Addr` | Client network address from gRPC context |

### Client

| Function | Description |
|---|---|
| `SetupConnection(addr string, opts *ClientOpts) (*grpc.ClientConn, error)` | Create a gRPC client connection with TLS and retry |

### Static Bearer Token

| Function | Description |
|---|---|
| `WithPerRPCToken(token string) grpc.DialOption` | DialOption: attach Bearer token to every RPC (requires TLS) |
| `TokenAuthFunc(srvToken string) grpc_auth.AuthFunc` | Server auth function for `grpc_auth` interceptors |
| `TokenAuthCheck(ctx, srvToken string) (context.Context, error)` | Validate Bearer token in incoming context |

### TOTP Bearer Token

| Function | Description |
|---|---|
| `NewTOTPAuth(t *twofactor.Totp) *TOTPAuth` | Create thread-safe TOTP auth wrapper |
| `(*TOTPAuth).OTP() (string, error)` | Generate current TOTP code |
| `(*TOTPAuth).Validate(token string) error` | Validate a TOTP code |
| `WithPerRPCTOTP(auth *TOTPAuth) grpc.DialOption` | DialOption: attach fresh TOTP token to every RPC (requires TLS) |
| `TOTPAuthFunc(auth *TOTPAuth) grpc_auth.AuthFunc` | Server auth function for `grpc_auth` interceptors |
| `TOTPAuthCheck(ctx, auth) (context.Context, error)` | Validate TOTP Bearer token in incoming context |

### Basic Auth

| Function | Description |
|---|---|
| `NewBasicAuthCreds(username, password string) *BasicAuthCreds` | Create Basic auth credentials for use as `grpc.WithPerRPCCredentials` |

### Presence Check

| Function | Description |
|---|---|
| `CheckRPCAuth(ctx context.Context) error` | Verify `authorization` header is present (does not validate its value) |
