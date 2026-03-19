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
	"strings"
	"sync"
	"testing"

	twofactor "github.com/acacio/totp-token/twofactor"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// newTestTOTPPair creates two TOTPAuth instances sharing the same key:
// one for OTP generation (client-side) and one for validation (server-side).
// Using separate instances with the same key avoids state clobbering between
// OTP() and Validate() calls during the test.
func newTestTOTPPair(t *testing.T) (client, server *TOTPAuth) {
	t.Helper()
	// 20-byte key matches SHA1 hash size (default for NewTOTPFromKey)
	key := []byte("grpcutil-test-key!!")
	clientTotp, err := twofactor.NewTOTPFromKey(key, "test@example.com", "grpcutil-test", 6)
	if err != nil {
		t.Fatalf("failed to create client TOTP: %v", err)
	}
	serverTotp, err := twofactor.NewTOTPFromKey(key, "test@example.com", "grpcutil-test", 6)
	if err != nil {
		t.Fatalf("failed to create server TOTP: %v", err)
	}
	return NewTOTPAuth(clientTotp), NewTOTPAuth(serverTotp)
}

// --- Unit tests ---

func TestNewTOTPAuth_NonNil(t *testing.T) {
	totp, err := twofactor.NewTOTPFromKey([]byte("01234567890123456789"), "u", "i", 6)
	if err != nil {
		t.Fatalf("NewTOTPFromKey: %v", err)
	}
	auth := NewTOTPAuth(totp)
	if auth == nil {
		t.Error("NewTOTPAuth should return non-nil")
	}
}

func TestTOTPAuth_OTP_ReturnsSixDigits(t *testing.T) {
	clientAuth, _ := newTestTOTPPair(t)
	token, err := clientAuth.OTP()
	if err != nil {
		t.Fatalf("OTP() returned error: %v", err)
	}
	if len(token) != 6 {
		t.Errorf("expected 6-digit OTP, got %q (len %d)", token, len(token))
	}
	for _, ch := range token {
		if ch < '0' || ch > '9' {
			t.Errorf("OTP contains non-digit character %q", ch)
		}
	}
}

func TestTOTPAuth_Validate_ValidToken(t *testing.T) {
	clientAuth, serverAuth := newTestTOTPPair(t)
	token, err := clientAuth.OTP()
	if err != nil {
		t.Fatalf("OTP(): %v", err)
	}
	if err := serverAuth.Validate(token); err != nil {
		t.Errorf("Validate() rejected valid token: %v", err)
	}
}

func TestTOTPAuth_Validate_InvalidToken(t *testing.T) {
	_, serverAuth := newTestTOTPPair(t)
	if err := serverAuth.Validate("000000"); err == nil {
		t.Error("Validate() should reject obviously invalid token '000000'")
	}
}

func TestTOTPAuth_Validate_EmptyToken(t *testing.T) {
	_, serverAuth := newTestTOTPPair(t)
	if err := serverAuth.Validate(""); err == nil {
		t.Error("Validate() should reject empty token")
	}
}

func TestTOTPAuth_RequireTransportSecurity(t *testing.T) {
	clientAuth, _ := newTestTOTPPair(t)
	if !clientAuth.RequireTransportSecurity() {
		t.Error("RequireTransportSecurity should return true")
	}
}

func TestTOTPAuth_GetRequestMetadata_BearerFormat(t *testing.T) {
	clientAuth, _ := newTestTOTPPair(t)
	md, err := clientAuth.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("GetRequestMetadata(): %v", err)
	}
	authVal, ok := md["authorization"]
	if !ok {
		t.Fatal("missing 'authorization' key in metadata")
	}
	if !strings.HasPrefix(authVal, "Bearer ") {
		t.Errorf("expected 'Bearer ' prefix, got: %q", authVal)
	}
	token := strings.TrimPrefix(authVal, "Bearer ")
	if len(token) != 6 {
		t.Errorf("expected 6-digit token, got %q", token)
	}
}

func TestTOTPAuth_GetRequestMetadata_ValidatesOnServer(t *testing.T) {
	clientAuth, serverAuth := newTestTOTPPair(t)
	md, err := clientAuth.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("GetRequestMetadata(): %v", err)
	}
	token := strings.TrimPrefix(md["authorization"], "Bearer ")
	if err := serverAuth.Validate(token); err != nil {
		t.Errorf("token from GetRequestMetadata should validate on server: %v", err)
	}
}

func TestWithPerRPCTOTP_ReturnsNonNilOption(t *testing.T) {
	clientAuth, _ := newTestTOTPPair(t)
	opt := WithPerRPCTOTP(clientAuth)
	if opt == nil {
		t.Error("WithPerRPCTOTP should return a non-nil DialOption")
	}
}

func TestWithPerRPCTOTP_InsecureRejectsTokenAuth(t *testing.T) {
	// RequireTransportSecurity = true: gRPC rejects PerRPCCredentials + insecure transport
	clientAuth, _ := newTestTOTPPair(t)
	_, err := grpc.NewClient("localhost:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		WithPerRPCTOTP(clientAuth),
	)
	if err == nil {
		t.Error("expected error: TOTP credentials require TLS but insecure transport was used")
	}
}

// --- TOTPAuthCheck unit tests ---

func TestTOTPAuthCheck_Success(t *testing.T) {
	clientAuth, serverAuth := newTestTOTPPair(t)
	token, _ := clientAuth.OTP()
	md := metadata.Pairs("authorization", "Bearer "+token)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	newCtx, err := TOTPAuthCheck(ctx, serverAuth)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if newCtx == nil {
		t.Error("returned context should not be nil")
	}
}

func TestTOTPAuthCheck_NoMetadata(t *testing.T) {
	_, serverAuth := newTestTOTPPair(t)
	ctx := context.Background()
	_, err := TOTPAuthCheck(ctx, serverAuth)
	if err == nil {
		t.Fatal("expected error for missing metadata")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got: %s", st.Code())
	}
}

func TestTOTPAuthCheck_NoAuthHeader(t *testing.T) {
	_, serverAuth := newTestTOTPPair(t)
	md := metadata.Pairs("x-other", "value")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	_, err := TOTPAuthCheck(ctx, serverAuth)
	if err == nil {
		t.Fatal("expected error for missing authorization header")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got: %s", st.Code())
	}
}

func TestTOTPAuthCheck_MissingBearerPrefix(t *testing.T) {
	clientAuth, serverAuth := newTestTOTPPair(t)
	token, _ := clientAuth.OTP()
	md := metadata.Pairs("authorization", token) // no "Bearer " prefix
	ctx := metadata.NewIncomingContext(context.Background(), md)
	_, err := TOTPAuthCheck(ctx, serverAuth)
	if err == nil {
		t.Fatal("expected error for missing Bearer prefix")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got: %s", st.Code())
	}
}

func TestTOTPAuthCheck_InvalidToken(t *testing.T) {
	_, serverAuth := newTestTOTPPair(t)
	md := metadata.Pairs("authorization", "Bearer 999999")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	_, err := TOTPAuthCheck(ctx, serverAuth)
	if err == nil {
		t.Fatal("expected error for invalid TOTP token")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got: %s", st.Code())
	}
}

func TestTOTPAuthCheck_LockdownAfterThreeFailures(t *testing.T) {
	_, serverAuth := newTestTOTPPair(t)
	md := metadata.Pairs("authorization", "Bearer 000000")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	// Exhaust the 3-attempt budget
	for i := 0; i < 3; i++ {
		TOTPAuthCheck(ctx, serverAuth) //nolint:errcheck
	}

	// Fourth attempt should be locked out
	_, err := TOTPAuthCheck(ctx, serverAuth)
	if err == nil {
		t.Fatal("expected lockout error after max failures")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.ResourceExhausted {
		t.Errorf("expected ResourceExhausted for lockout, got: %s", st.Code())
	}
	if !strings.Contains(st.Message(), "locked") {
		t.Errorf("lockout message should mention 'locked', got: %s", st.Message())
	}
}

// --- TOTPAuthFunc tests ---

func TestTOTPAuthFunc_ReturnsNonNil(t *testing.T) {
	_, serverAuth := newTestTOTPPair(t)
	fn := TOTPAuthFunc(serverAuth)
	if fn == nil {
		t.Error("TOTPAuthFunc should return a non-nil AuthFunc")
	}
}

func TestTOTPAuthFunc_ValidToken(t *testing.T) {
	clientAuth, serverAuth := newTestTOTPPair(t)
	fn := TOTPAuthFunc(serverAuth)

	token, _ := clientAuth.OTP()
	md := metadata.Pairs("authorization", "Bearer "+token)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	newCtx, err := fn(ctx)
	if err != nil {
		t.Errorf("AuthFunc rejected valid token: %v", err)
	}
	if newCtx == nil {
		t.Error("AuthFunc returned nil context")
	}
}

func TestTOTPAuthFunc_InvalidToken(t *testing.T) {
	_, serverAuth := newTestTOTPPair(t)
	fn := TOTPAuthFunc(serverAuth)
	md := metadata.Pairs("authorization", "Bearer 000001")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	_, err := fn(ctx)
	if err == nil {
		t.Error("AuthFunc should reject invalid token")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got: %s", st.Code())
	}
}

// --- Concurrency test ---

func TestTOTPAuth_ConcurrentOTP(t *testing.T) {
	clientAuth, _ := newTestTOTPPair(t)
	const goroutines = 20
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := clientAuth.OTP()
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent OTP() error: %v", err)
	}
}

func TestTOTPAuth_ConcurrentValidate_NoDataRace(t *testing.T) {
	clientAuth, serverAuth := newTestTOTPPair(t)
	token, _ := clientAuth.OTP()

	const goroutines = 20
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			serverAuth.Validate(token) //nolint:errcheck // concurrent reads of same token
		}()
	}
	wg.Wait()
}

// --- Integration test ---

func TestTOTPAuth_EndToEnd_ValidToken(t *testing.T) {
	clientAuth, serverAuth := newTestTOTPPair(t)

	s, lis := newBufConnServer(t,
		grpc.ChainUnaryInterceptor(grpc_auth.UnaryServerInterceptor(TOTPAuthFunc(serverAuth))),
	)
	defer s.GracefulStop()

	conn := newBufConnClient(t, lis)
	hc := grpc_health_v1.NewHealthClient(conn)

	// Inject TOTP token into outgoing context (avoids TLS constraint in unit tests)
	token, err := clientAuth.OTP()
	if err != nil {
		t.Fatalf("OTP(): %v", err)
	}
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)

	resp, err := hc.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("unexpected error with valid TOTP token: %v", err)
	}
	if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Errorf("unexpected health status: %v", resp.GetStatus())
	}
}

func TestTOTPAuth_EndToEnd_InvalidToken(t *testing.T) {
	_, serverAuth := newTestTOTPPair(t)

	s, lis := newBufConnServer(t,
		grpc.ChainUnaryInterceptor(grpc_auth.UnaryServerInterceptor(TOTPAuthFunc(serverAuth))),
	)
	defer s.GracefulStop()

	conn := newBufConnClient(t, lis)
	hc := grpc_health_v1.NewHealthClient(conn)

	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer 999998")
	_, err := hc.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err == nil {
		t.Fatal("expected error with invalid TOTP token")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got: %s", st.Code())
	}
}

func TestTOTPAuth_EndToEnd_MissingToken(t *testing.T) {
	_, serverAuth := newTestTOTPPair(t)

	s, lis := newBufConnServer(t,
		grpc.ChainUnaryInterceptor(grpc_auth.UnaryServerInterceptor(TOTPAuthFunc(serverAuth))),
	)
	defer s.GracefulStop()

	conn := newBufConnClient(t, lis)
	hc := grpc_health_v1.NewHealthClient(conn)

	// No authorization header at all
	_, err := hc.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{})
	if err == nil {
		t.Fatal("expected error with missing TOTP token")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for missing auth, got: %s", st.Code())
	}
}

func TestTOTPAuth_EndToEnd_Lockout(t *testing.T) {
	_, serverAuth := newTestTOTPPair(t)

	s, lis := newBufConnServer(t,
		grpc.ChainUnaryInterceptor(grpc_auth.UnaryServerInterceptor(TOTPAuthFunc(serverAuth))),
	)
	defer s.GracefulStop()

	conn := newBufConnClient(t, lis)
	hc := grpc_health_v1.NewHealthClient(conn)

	// Exhaust failure budget via direct validation (bypasses server to avoid extra state)
	for i := 0; i < 3; i++ {
		serverAuth.Validate("111111") //nolint:errcheck
	}

	// Fourth call should hit lockout
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer 111111")
	_, err := hc.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err == nil {
		t.Fatal("expected lockout error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.ResourceExhausted {
		t.Errorf("expected ResourceExhausted for lockout, got: %s", st.Code())
	}
}

func TestTOTPAuth_GetRequestMetadata_OTPError(t *testing.T) {
	// Creating an empty Totp struct which fails on OTP() call since it's uninitialized
	auth := NewTOTPAuth(&twofactor.Totp{})
	_, err := auth.GetRequestMetadata(context.Background())
	if err == nil {
		t.Error("expected error when OTP generation fails")
	}
}
