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
	"encoding/base64"
	"strings"
	"testing"
)

func TestBasicAuthCredsGetRequestMetadata(t *testing.T) {
	creds := &BasicAuthCreds{username: "alice", password: "s3cr3t"}
	md, err := creds.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	auth, ok := md["authorization"]
	if !ok {
		t.Fatal("missing 'authorization' key in metadata")
	}
	if !strings.HasPrefix(auth, "Basic ") {
		t.Errorf("expected 'Basic ' prefix, got: %s", auth)
	}
	encoded := strings.TrimPrefix(auth, "Basic ")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("failed to base64-decode credentials: %v", err)
	}
	if string(decoded) != "alice:s3cr3t" {
		t.Errorf("expected 'alice:s3cr3t', got: %s", decoded)
	}
}

func TestBasicAuthCredsGetRequestMetadata_EmptyCredentials(t *testing.T) {
	creds := &BasicAuthCreds{}
	md, err := creds.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	auth := md["authorization"]
	encoded := strings.TrimPrefix(auth, "Basic ")
	decoded, _ := base64.StdEncoding.DecodeString(encoded)
	if string(decoded) != ":" {
		t.Errorf("expected ':', got: %s", decoded)
	}
}

func TestBasicAuthCredsRequireTransportSecurity(t *testing.T) {
	creds := &BasicAuthCreds{username: "user", password: "pass"}
	if !creds.RequireTransportSecurity() {
		t.Error("RequireTransportSecurity should return true")
	}
}

func TestBasicAuthCredsDigest(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		want     string
	}{
		{
			name:     "standard credentials",
			username: "alice",
			password: "wonderland",
			want:     base64.StdEncoding.EncodeToString([]byte("alice:wonderland")),
		},
		{
			name:     "empty credentials",
			username: "",
			password: "",
			want:     base64.StdEncoding.EncodeToString([]byte(":")),
		},
		{
			name:     "special characters in password",
			username: "bob",
			password: "p@$$w0rd!",
			want:     base64.StdEncoding.EncodeToString([]byte("bob:p@$$w0rd!")),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds := &BasicAuthCreds{username: tc.username, password: tc.password}
			got := creds.Digest()
			if got != tc.want {
				t.Errorf("Digest() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBasicAuthCredsDigest_IsBase64(t *testing.T) {
	creds := &BasicAuthCreds{username: "test", password: "value"}
	digest := creds.Digest()
	if _, err := base64.StdEncoding.DecodeString(digest); err != nil {
		t.Errorf("Digest() returned invalid base64: %v", err)
	}
}
