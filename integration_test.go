/*
Copyright 2021 Acacio Cruz acacio@acacio.coom

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package grpcutil

import (
	"context"
	"net"
	"testing"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

// newBufConnServer creates a gRPC server backed by an in-memory bufconn listener.
// The caller must call server.GracefulStop() when done.
func newBufConnServer(t *testing.T, opts ...grpc.ServerOption) (*grpc.Server, *bufconn.Listener) {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	s := grpc.NewServer(opts...)
	grpc_health_v1.RegisterHealthServer(s, health.NewServer())
	go func() {
		if err := s.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			t.Logf("bufconn server error: %v", err)
		}
	}()
	return s, lis
}

// bufDialer returns a ContextDialer for connecting to a bufconn listener.
func bufDialer(lis *bufconn.Listener) func(context.Context, string) (net.Conn, error) {
	return func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
}

// newBufConnClient creates a gRPC client connected to the given bufconn listener.
func newBufConnClient(t *testing.T, lis *bufconn.Listener, opts ...grpc.DialOption) *grpc.ClientConn {
	t.Helper()
	defaults := []grpc.DialOption{
		grpc.WithContextDialer(bufDialer(lis)),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	conn, err := grpc.NewClient("passthrough:///bufnet", append(defaults, opts...)...)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// --- Integration tests ---

// injectBearerToken adds a Bearer token to the outgoing context metadata.
// Used in tests instead of WithPerRPCToken because TokenAuth.RequireTransportSecurity()
// returns true, which gRPC enforces at connection creation with insecure transport.
func injectBearerToken(ctx context.Context, token string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
}

func TestBearerAuth_EndToEnd_ValidToken(t *testing.T) {
	const token = "valid-token"
	s, lis := newBufConnServer(t,
		grpc.ChainUnaryInterceptor(grpc_auth.UnaryServerInterceptor(TokenAuthFunc(token))),
	)
	defer s.GracefulStop()

	conn := newBufConnClient(t, lis) // no WithPerRPCToken — inject via context instead
	hc := grpc_health_v1.NewHealthClient(conn)

	ctx := injectBearerToken(context.Background(), token)
	resp, err := hc.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("unexpected error with valid token: %v", err)
	}
	if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Errorf("unexpected health status: %v", resp.GetStatus())
	}
}

func TestBearerAuth_EndToEnd_InvalidToken(t *testing.T) {
	const serverToken = "correct-token"
	s, lis := newBufConnServer(t,
		grpc.ChainUnaryInterceptor(grpc_auth.UnaryServerInterceptor(TokenAuthFunc(serverToken))),
	)
	defer s.GracefulStop()

	conn := newBufConnClient(t, lis)
	hc := grpc_health_v1.NewHealthClient(conn)

	ctx := injectBearerToken(context.Background(), "wrong-token")
	_, err := hc.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err == nil {
		t.Fatal("expected error with invalid token")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got: %s", st.Code())
	}
}

func TestBearerAuth_EndToEnd_MissingToken(t *testing.T) {
	const serverToken = "correct-token"
	s, lis := newBufConnServer(t,
		grpc.ChainUnaryInterceptor(grpc_auth.UnaryServerInterceptor(TokenAuthFunc(serverToken))),
	)
	defer s.GracefulStop()

	// No token on the client side
	conn := newBufConnClient(t, lis)
	hc := grpc_health_v1.NewHealthClient(conn)

	_, err := hc.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{})
	if err == nil {
		t.Fatal("expected error with missing token")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for missing auth, got: %s", st.Code())
	}
}

func TestDefaultServerOptions_EndToEnd(t *testing.T) {
	opts := DefaultServerOptions(nil)
	s, lis := newBufConnServer(t, opts...)
	defer s.GracefulStop()

	conn := newBufConnClient(t, lis)
	hc := grpc_health_v1.NewHealthClient(conn)

	resp, err := hc.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("unexpected error with default server options: %v", err)
	}
	if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Errorf("unexpected health status: %v", resp.GetStatus())
	}
}

func TestKeepAlive_EndToEnd(t *testing.T) {
	s, lis := newBufConnServer(t, KeepAliveDefault())
	defer s.GracefulStop()

	conn := newBufConnClient(t, lis)
	hc := grpc_health_v1.NewHealthClient(conn)

	resp, err := hc.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("unexpected error with keepalive option: %v", err)
	}
	if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Errorf("unexpected health status: %v", resp.GetStatus())
	}
}

func TestPeerAddress_EndToEnd(t *testing.T) {
	var capturedAddr net.Addr

	// Register a health server that also captures the peer address
	hs := &peerCapturingHealthServer{capturedAddr: &capturedAddr}
	s := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(s, hs)

	lis := bufconn.Listen(bufSize)
	go s.Serve(lis)
	defer s.GracefulStop()

	conn := newBufConnClient(t, lis)
	hc := grpc_health_v1.NewHealthClient(conn)
	hc.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{}) //nolint:errcheck

	if capturedAddr == nil {
		t.Error("expected peer address to be captured during RPC")
	}
}

// peerCapturingHealthServer wraps health.Server and captures the peer address on Check.
type peerCapturingHealthServer struct {
	grpc_health_v1.UnimplementedHealthServer
	capturedAddr *net.Addr
}

func (s *peerCapturingHealthServer) Check(ctx context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	*s.capturedAddr = PeerAddress(ctx)
	return &grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_SERVING,
	}, nil
}
