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
	"errors"
	"log"
	"strings"
	"sync"

	twofactor "github.com/acacio/totp-token/twofactor"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// TOTPAuth is a thread-safe wrapper around a TOTP instance that can be used as
// both a gRPC PerRPCCredentials (client-side) and a server-side token validator.
//
// The underlying Totp state is mutable (failure counter, lockout time, counter sync),
// so all method calls are serialised with a mutex.
type TOTPAuth struct {
	mu   sync.Mutex
	totp *twofactor.Totp
}

// NewTOTPAuth creates a TOTPAuth from an existing *twofactor.Totp.
// Typically the client and server each hold a separate TOTPAuth instance
// constructed from the same shared key via NewTOTPFromKey or TOTPFromBytes.
func NewTOTPAuth(t *twofactor.Totp) *TOTPAuth {
	return &TOTPAuth{totp: t}
}

// OTP generates the current TOTP token. Thread-safe.
func (a *TOTPAuth) OTP() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.totp.OTP()
}

// Validate checks a user-supplied token against the TOTP. Thread-safe.
// Returns twofactor.LockDownError after max_failures (3) bad attempts until
// the 5-minute backoff elapses.
func (a *TOTPAuth) Validate(token string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.totp.Validate(token)
}

// GetRequestMetadata implements credentials.PerRPCCredentials.
// Generates a fresh TOTP code on every RPC and sends it as a Bearer token.
func (a *TOTPAuth) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	token, err := a.OTP()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate TOTP: %v", err)
	}
	return map[string]string{
		"authorization": prefix + token,
	}, nil
}

// RequireTransportSecurity implements credentials.PerRPCCredentials.
// TOTP tokens must not be transmitted over unencrypted connections.
func (a *TOTPAuth) RequireTransportSecurity() bool {
	return true
}

// WithPerRPCTOTP creates a gRPC DialOption that attaches a fresh TOTP Bearer token
// to every outgoing RPC. Requires a TLS transport (RequireTransportSecurity = true).
func WithPerRPCTOTP(auth *TOTPAuth) grpc.DialOption {
	return grpc.WithPerRPCCredentials(auth)
}

// TOTPAuthFunc returns a grpc_auth.AuthFunc suitable for use with
// grpc_auth.UnaryServerInterceptor / grpc_auth.StreamServerInterceptor.
// It validates incoming Bearer tokens using the provided TOTPAuth.
func TOTPAuthFunc(auth *TOTPAuth) grpc_auth.AuthFunc {
	return func(ctx context.Context) (context.Context, error) {
		return TOTPAuthCheck(ctx, auth)
	}
}

// TOTPAuthCheck validates the Bearer token in the incoming gRPC context using
// the provided TOTPAuth.
//
// Error codes:
//   - codes.InvalidArgument — no metadata or missing Authorization header
//   - codes.Unauthenticated — invalid or expired TOTP token
//   - codes.ResourceExhausted — TOTP is locked out due to too many failures
func TOTPAuthCheck(ctx context.Context, auth *TOTPAuth) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		log.Println("TOTP auth: metadata retrieval failed")
		return ctx, status.Errorf(codes.InvalidArgument, "retrieving metadata failed")
	}

	authHeader, ok := md["authorization"]
	if !ok || len(authHeader) == 0 {
		log.Println("TOTP auth: no auth details supplied")
		return ctx, status.Errorf(codes.InvalidArgument, "no auth details supplied")
	}

	if !strings.HasPrefix(authHeader[0], prefix) {
		return ctx, status.Error(codes.Unauthenticated, `missing "Bearer " prefix in "Authorization" header`)
	}

	token := strings.TrimPrefix(authHeader[0], prefix)
	if err := auth.Validate(token); err != nil {
		if errors.Is(err, twofactor.LockDownError) {
			log.Println("TOTP auth: locked out due to too many failures")
			return ctx, status.Error(codes.ResourceExhausted, "TOTP authentication locked: too many failures, retry after 5 minutes")
		}
		log.Printf("TOTP auth: validation failed: %v\n", err)
		return ctx, status.Error(codes.Unauthenticated, "invalid TOTP token")
	}

	log.Println("TOTP auth: authenticated successfully")
	return ctx, nil
}
