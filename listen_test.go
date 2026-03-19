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
	"fmt"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// findFreePort returns an available TCP port on localhost.
func findFreePort(t *testing.T) int {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := lis.Addr().(*net.TCPAddr).Port
	lis.Close()
	return port
}

func TestServe_StartsAndServesRequests(t *testing.T) {
	port := findFreePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	s := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(s, health.NewServer())

	go Serve(s, addr)

	// Wait for the server to be ready
	deadline := time.Now().Add(3 * time.Second)
	var conn *grpc.ClientConn
	var err error
	for time.Now().Before(deadline) {
		conn, err = grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("could not connect to server: %v", err)
	}
	defer conn.Close()
	defer s.GracefulStop()

	hc := grpc_health_v1.NewHealthClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := hc.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Errorf("expected SERVING, got: %v", resp.GetStatus())
	}
}

func TestServe_ListenError(t *testing.T) {
	original := logFatalf
	defer func() { logFatalf = original }()
	logFatalf = func(format string, v ...interface{}) {
		panic(fmt.Sprintf(format, v...))
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic from mock logFatalf on Listen error")
		}
	}()

	// use an invalid port format
	Serve(grpc.NewServer(), "invalid-port")
}

func TestServe_ServeError(t *testing.T) {
	original := logFatalf
	defer func() { logFatalf = original }()
	logFatalf = func(format string, v ...interface{}) {
		panic(fmt.Sprintf(format, v...))
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic from mock logFatalf on Serve error")
		}
	}()

	s := grpc.NewServer()
	port := findFreePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	
	// Stop the server immediately so Serve(lis) will return an error
	s.Stop()
	Serve(s, addr)
}
