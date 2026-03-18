# grpcutil
gRPC Utility functions

A Go library providing helpers for building gRPC servers and clients: authentication (Bearer token, Basic auth), Prometheus metrics, keepalive configuration, gRPC-Web bridging, and middleware chaining.

Requires **Go 1.21+** and **gRPC v1.63+**.

---

## Server Setup

```go
import (
    "github.com/acacio/grpcutil"
    "google.golang.org/grpc"
    "google.golang.org/grpc/reflection"
    pb "path-to-your-proto/proto"
)

func startServer(port, grpcwebport string) {
    // Build server options with default middleware (Prometheus metrics + panic recovery)
    opts := grpcutil.DefaultServerOptions([]grpc.ServerOption{
        grpcutil.KeepAliveDefault(),
    })

    // Optionally add Bearer token auth for all RPCs
    opts = append(opts, grpc.ChainUnaryInterceptor(
        grpc_auth.UnaryServerInterceptor(grpcutil.TokenAuthFunc("your-secret-token")),
    ))

    s := grpc.NewServer(opts...)

    // Register your services
    pb.RegisterYourServer(s, &yourServiceImpl{})
    reflection.Register(s) // optional: for grpcurl / gRPC reflection

    // Enable Prometheus metrics endpoint
    metricsHandler := grpcutil.EnablePrometheus(s, port)
    http.Handle("/metrics", metricsHandler)
    go http.ListenAndServe(":9090", nil)

    // Optional: enable gRPC-Web on a separate port
    go grpcutil.StartgRPCWeb(s, grpcwebport)

    // Start serving (blocks; Fatal on error)
    grpcutil.Serve(s, port)
}
```

---

## Client Setup

```go
import "github.com/acacio/grpcutil"

conn, err := grpcutil.SetupConnection("server:50051", &grpcutil.ClientOpts{
    TLSType: "tls",          // "tls", "mtls", or "insecure"
    CA:      "/path/ca.pem", // for "tls" / "mtls"
    Cert:    "/path/crt.pem",// for "mtls"
    Key:     "/path/key.pem",// for "mtls"
    Token:   "your-bearer-token", // omit for no auth (requires TLS when set)
    Block:   false,          // true to wait for READY state (30s timeout)
})
```

`SetupConnection` uses `grpc.NewClient` (lazy connection) and applies automatic retry with linear backoff for `DeadlineExceeded` and `Unavailable` errors.

> **Note:** `Token` requires a TLS transport (`TLSType != "insecure"`). Using a token with insecure transport returns an error.

---

## Authentication

### Bearer Token (server-side)

```go
import grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"

// Validate incoming Bearer tokens against a server secret
grpc.ChainUnaryInterceptor(
    grpc_auth.UnaryServerInterceptor(grpcutil.TokenAuthFunc("your-server-token")),
)

// Or call directly in your RPC handler
func (s *Server) MyRPC(ctx context.Context, req *pb.Req) (*pb.Resp, error) {
    if _, err := grpcutil.TokenAuthCheck(ctx, "your-server-token"); err != nil {
        return nil, err
    }
    // ...
}
```

### Bearer Token (client-side)

```go
// As a DialOption — requires TLS transport
opt := grpcutil.WithPerRPCToken("your-token")
conn, _ := grpc.NewClient(addr, opt, grpc.WithTransportCredentials(creds))

// Or inject per-call via context (works with insecure transport in tests)
ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer your-token")
```

### Basic Auth (client-side)

```go
creds := &grpcutil.BasicAuthCreds{} // set via struct literal (fields are unexported by design)
conn, _ := grpc.NewClient(addr,
    grpc.WithTransportCredentials(tlsCreds),
    grpc.WithPerRPCCredentials(creds),
)
```

---

## Prometheus Metrics

```go
// Register and enable Prometheus after all services are registered
metricsHandler := grpcutil.EnablePrometheus(s, port)
http.Handle("/metrics", metricsHandler)
go http.ListenAndServe(":9090", nil)

// Query average latency per method (milliseconds)
lats := grpcutil.GetgRPCMetrics() // map[methodName]latencyMs

// Query histogram bucket distribution per method
hists := grpcutil.GetgRPCHistograms() // map[methodName]map[upperBound]count
```

---

## Keepalive

```go
// KeepAliveDefault uses 2h ping interval, 30s timeout, unlimited connection lifetime
opts := grpcutil.DefaultServerOptions([]grpc.ServerOption{
    grpcutil.KeepAliveDefault(),
})
```

---

## gRPC-Web

```go
// StartgRPCWeb wraps the gRPC server and serves gRPC-Web on a separate HTTP port
go grpcutil.StartgRPCWeb(s, ":8080")
```

---

## TOTP Authentication

Two-Factor Time-based One-Time Password authentication for gRPC, backed by [`github.com/acacio/totp-token`](https://github.com/acacio/totp-token).

### Key sharing

Client and server each hold a separate `TOTPAuth` built from the **same shared key**. Use `NewTOTPFromKey` for deterministic key setup, or `NewTOTP` + `ToBytes`/`TOTPFromBytes` for secure serialized provisioning:

```go
import (
    "crypto"
    twofactor "github.com/acacio/totp-token/twofactor"
    "github.com/acacio/grpcutil"
)

sharedKey := []byte("your-32-byte-secret-key-here!!!!") // keep secret

// Server side — validates tokens
serverTotp, _ := twofactor.NewTOTPFromKey(sharedKey, "user@example.com", "MyApp", 6)
serverAuth := grpcutil.NewTOTPAuth(serverTotp)

// Client side — generates tokens
clientTotp, _ := twofactor.NewTOTPFromKey(sharedKey, "user@example.com", "MyApp", 6)
clientAuth := grpcutil.NewTOTPAuth(clientTotp)
```

### Server setup

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

Or call `TOTPAuthCheck` directly inside an RPC handler:

```go
func (s *Server) MyRPC(ctx context.Context, req *pb.Req) (*pb.Resp, error) {
    if _, err := grpcutil.TOTPAuthCheck(ctx, serverAuth); err != nil {
        return nil, err
    }
    // ...
}
```

### Client setup

`WithPerRPCTOTP` attaches a freshly generated TOTP code to every outgoing RPC as a Bearer token. **Requires TLS transport.**

```go
conn, err := grpc.NewClient(addr,
    grpc.WithTransportCredentials(tlsCreds),
    grpcutil.WithPerRPCTOTP(clientAuth),
)
```

For testing without TLS, inject the token directly into the outgoing context:

```go
token, _ := clientAuth.OTP()
ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
```

### Error codes

| Situation | gRPC code |
|---|---|
| Missing or malformed `Authorization` header | `InvalidArgument` |
| Wrong or expired TOTP code | `Unauthenticated` |
| Locked out after 3 failures (5-min backoff) | `ResourceExhausted` |

### Thread safety

`TOTPAuth` is safe for concurrent use. The underlying `twofactor.Totp` state (failure counter, lockout timestamp, counter sync offset) is protected by a mutex.

---

## Peer Information

```go
func (s *Server) MyRPC(ctx context.Context, req *pb.Req) (*pb.Resp, error) {
    addr := grpcutil.PeerAddress(ctx) // returns net.Addr or nil
    // ...
}
```

---

## Configuration Reference

### `ClientOpts`

| Field     | Type   | Description                                      |
|-----------|--------|--------------------------------------------------|
| `TLSType` | string | `"tls"`, `"mtls"`, or `"insecure"` (default)     |
| `CA`      | string | Path to CA certificate (TLS/mTLS)                |
| `Cert`    | string | Path to client certificate (mTLS)                |
| `Key`     | string | Path to client private key (mTLS)                |
| `Token`   | string | Bearer token — requires TLS transport when set   |
| `Block`   | bool   | Wait up to 30s for READY state after connecting  |
