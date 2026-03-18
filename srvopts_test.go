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

func TestKeepAliveDefault_ReturnsNonNilOption(t *testing.T) {
	opt := KeepAliveDefault()
	if opt == nil {
		t.Error("KeepAliveDefault() should return a non-nil ServerOption")
	}
}

func TestKeepAliveDefault_ApplicableToServer(t *testing.T) {
	// Verify the option can be used to create a gRPC server without panicking
	opt := KeepAliveDefault()
	s := grpc.NewServer(opt)
	defer s.GracefulStop()
	if s == nil {
		t.Error("expected non-nil gRPC server with keepalive option")
	}
}

func TestDefaultServerOptions_AppendsToExisting(t *testing.T) {
	existing := []grpc.ServerOption{KeepAliveDefault()}
	result := DefaultServerOptions(existing)

	// Should have the original option plus the two chain interceptors (stream + unary)
	if len(result) <= len(existing) {
		t.Errorf("DefaultServerOptions should append options; got %d, had %d", len(result), len(existing))
	}
}

func TestDefaultServerOptions_EmptyInput(t *testing.T) {
	result := DefaultServerOptions(nil)
	if len(result) == 0 {
		t.Error("DefaultServerOptions(nil) should return at least the middleware options")
	}
}

func TestDefaultServerOptions_CreatesValidServer(t *testing.T) {
	opts := DefaultServerOptions(nil)
	s := grpc.NewServer(opts...)
	defer s.GracefulStop()
	if s == nil {
		t.Error("expected non-nil gRPC server created from DefaultServerOptions")
	}
}

func TestDefaultServerOptions_WithKeepAlive(t *testing.T) {
	opts := DefaultServerOptions([]grpc.ServerOption{KeepAliveDefault()})
	s := grpc.NewServer(opts...)
	defer s.GracefulStop()
	if s == nil {
		t.Error("expected non-nil server with KeepAlive + default options")
	}
}
