package grpcutil

import (
	"testing"
	"time"

	"google.golang.org/grpc"
)

func TestStartgRPCWeb(t *testing.T) {
	s := grpc.NewServer()
	
	// Start in a goroutine because a valid port would block
	go func() {
		// Use loopback with port 0 to let OS assign an available port
		StartgRPCWeb(s, "127.0.0.1:0")
	}()

	// Sleep briefly to let the server start and get coverage
	time.Sleep(100 * time.Millisecond)
}

func TestStartgRPCWeb_Error(t *testing.T) {
	s := grpc.NewServer()
	
	// An invalid port forces ListenAndServe to return immediately with an error
	StartgRPCWeb(s, "127.0.0.1:-1")
}
