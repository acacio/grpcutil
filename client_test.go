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
	"testing"

	"google.golang.org/grpc"
)

func TestSetupConnection_InsecureNoToken(t *testing.T) {
	opts := &ClientOpts{
		TLSType: "insecure",
	}
	conn, err := SetupConnection("localhost:50051", opts)
	if err != nil {
		t.Fatalf("unexpected error creating insecure connection: %v", err)
	}
	defer conn.Close()
	if conn == nil {
		t.Error("expected non-nil connection")
	}
}

func TestSetupConnection_WithToken_InsecureRejectsTokenAuth(t *testing.T) {
	// TokenAuth.RequireTransportSecurity() returns true; gRPC correctly rejects
	// pairing PerRPCCredentials that require TLS with an insecure transport.
	opts := &ClientOpts{
		TLSType: "insecure",
		Token:   "mytoken",
	}
	_, err := SetupConnection("localhost:50051", opts)
	if err == nil {
		t.Error("expected error: cannot use PerRPCCredentials requiring TLS with insecure transport")
	}
}

func TestSetupConnection_EmptyTLSType(t *testing.T) {
	// Empty TLSType should default to insecure
	opts := &ClientOpts{}
	conn, err := SetupConnection("localhost:50051", opts)
	if err != nil {
		t.Fatalf("unexpected error with empty TLSType: %v", err)
	}
	defer conn.Close()
	if conn == nil {
		t.Error("expected non-nil connection")
	}
}

func TestSetupDialOpts_Insecure(t *testing.T) {
	opts, err := setupDialOpts("insecure", "", "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(opts) == 0 {
		t.Error("expected at least one dial option")
	}
}

func TestSetupDialOpts_EmptyType(t *testing.T) {
	opts, err := setupDialOpts("", "", "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error for empty TLS type: %v", err)
	}
	if len(opts) == 0 {
		t.Error("expected insecure credentials option")
	}
}

func TestSetupDialOpts_InvalidTLSType(t *testing.T) {
	// Non-empty, non-"insecure" type should attempt TLS setup; invalid CA should fail
	_, err := setupDialOpts("mtls", "/nonexistent/ca.pem", "", "", nil)
	if err == nil {
		t.Error("expected error for invalid TLS config")
	}
}

func TestSetupDialOpts_PreservesExistingOpts(t *testing.T) {
	// Use WithAuthority as a harmless non-deprecated extra option to verify preservation
	existing := []grpc.DialOption{grpc.WithAuthority("example.com")}
	opts, err := setupDialOpts("insecure", "", "", "", existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have the existing option plus the insecure credentials
	if len(opts) <= len(existing) {
		t.Errorf("expected more options than input; got %d", len(opts))
	}
}
