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
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestTokenAuthGetRequestMetadata(t *testing.T) {
	auth := TokenAuth{Token: "mytoken123"}
	md, err := auth.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := md["authorization"]
	if !ok {
		t.Fatal("missing 'authorization' key")
	}
	if got != "Bearer mytoken123" {
		t.Errorf("expected 'Bearer mytoken123', got: %s", got)
	}
}

func TestTokenAuthGetRequestMetadata_EmptyToken(t *testing.T) {
	auth := TokenAuth{Token: ""}
	md, err := auth.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if md["authorization"] != "Bearer " {
		t.Errorf("expected 'Bearer ', got: %s", md["authorization"])
	}
}

func TestTokenAuthRequireTransportSecurity(t *testing.T) {
	auth := TokenAuth{Token: "tok"}
	if !auth.RequireTransportSecurity() {
		t.Error("RequireTransportSecurity should return true")
	}
}

func TestWithPerRPCToken(t *testing.T) {
	opt := WithPerRPCToken("sometoken")
	if opt == nil {
		t.Error("WithPerRPCToken should return a non-nil DialOption")
	}
}

func TestTokenAuthCheck_Success(t *testing.T) {
	srvToken := "secrettoken"
	md := metadata.Pairs("authorization", "Bearer "+srvToken)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	newCtx, err := TokenAuthCheck(ctx, srvToken)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if newCtx == nil {
		t.Error("returned context should not be nil")
	}
}

func TestTokenAuthCheck_NoMetadata(t *testing.T) {
	ctx := context.Background() // no metadata
	_, err := TokenAuthCheck(ctx, "tok")
	if err == nil {
		t.Fatal("expected error for missing metadata")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got: %s", st.Code())
	}
}

func TestTokenAuthCheck_NoAuthHeader(t *testing.T) {
	md := metadata.Pairs("x-other-header", "value")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err := TokenAuthCheck(ctx, "tok")
	if err == nil {
		t.Fatal("expected error for missing authorization header")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got: %s", st.Code())
	}
}

func TestTokenAuthCheck_MissingBearerPrefix(t *testing.T) {
	md := metadata.Pairs("authorization", "Basic somevalue")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err := TokenAuthCheck(ctx, "somevalue")
	if err == nil {
		t.Fatal("expected error for missing Bearer prefix")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got: %s", st.Code())
	}
	if !strings.Contains(st.Message(), "Bearer") {
		t.Errorf("error message should mention Bearer, got: %s", st.Message())
	}
}

func TestTokenAuthCheck_InvalidToken(t *testing.T) {
	md := metadata.Pairs("authorization", "Bearer wrongtoken")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err := TokenAuthCheck(ctx, "correcttoken")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got: %s", st.Code())
	}
}

func TestTokenAuthCheck_TokenWithSpaces(t *testing.T) {
	// Token that itself contains spaces should still match exactly
	srvToken := "multi word token"
	md := metadata.Pairs("authorization", "Bearer "+srvToken)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err := TokenAuthCheck(ctx, srvToken)
	if err != nil {
		t.Errorf("expected success for token with spaces, got: %v", err)
	}
}

func TestTokenAuthFunc(t *testing.T) {
	srvToken := "testtoken"
	fn := TokenAuthFunc(srvToken)
	if fn == nil {
		t.Fatal("TokenAuthFunc should return a non-nil function")
	}

	// Test that the returned function validates correctly
	md := metadata.Pairs("authorization", "Bearer "+srvToken)
	ctx := metadata.NewIncomingContext(context.Background(), md)
	newCtx, err := fn(ctx)
	if err != nil {
		t.Errorf("AuthFunc returned unexpected error: %v", err)
	}
	if newCtx == nil {
		t.Error("AuthFunc returned nil context")
	}
}

func TestTokenAuthFunc_InvalidToken(t *testing.T) {
	fn := TokenAuthFunc("correct")
	md := metadata.Pairs("authorization", "Bearer wrong")
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
