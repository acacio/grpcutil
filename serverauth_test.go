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
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestCheckRPCAuth_NoMetadata(t *testing.T) {
	ctx := context.Background()
	err := CheckRPCAuth(ctx)
	if err == nil {
		t.Fatal("expected error when no metadata is present")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %T %v", err, err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got: %s", st.Code())
	}
}

func TestCheckRPCAuth_NoAuthorizationHeader(t *testing.T) {
	md := metadata.Pairs("content-type", "application/grpc")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	err := CheckRPCAuth(ctx)
	if err == nil {
		t.Fatal("expected error when authorization header is absent")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got: %s", st.Code())
	}
}

func TestCheckRPCAuth_WithAuthorizationHeader(t *testing.T) {
	md := metadata.Pairs("authorization", "Bearer sometoken")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	err := CheckRPCAuth(ctx)
	if err != nil {
		t.Errorf("expected nil error when authorization header is present, got: %v", err)
	}
}

func TestCheckRPCAuth_MultipleMetadataValues(t *testing.T) {
	// Multiple authorization values — should not error as long as key exists
	md := metadata.MD{
		"authorization": []string{"Bearer token1", "Bearer token2"},
	}
	ctx := metadata.NewIncomingContext(context.Background(), md)

	err := CheckRPCAuth(ctx)
	if err != nil {
		t.Errorf("expected nil error with multiple auth values, got: %v", err)
	}
}

func TestCheckRPCAuth_EmptyAuthorizationValue(t *testing.T) {
	// Key present but empty value — still considered present
	md := metadata.Pairs("authorization", "")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	err := CheckRPCAuth(ctx)
	if err != nil {
		t.Errorf("expected nil error when authorization key exists (even empty), got: %v", err)
	}
}
