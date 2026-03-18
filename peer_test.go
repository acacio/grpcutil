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

	"google.golang.org/grpc/peer"
)

func TestPeerAddress_NoContext(t *testing.T) {
	ctx := context.Background()
	addr := PeerAddress(ctx)
	if addr != nil {
		t.Errorf("expected nil addr for context without peer info, got: %v", addr)
	}
}

func TestPeerAddress_WithPeer_NilAuthInfo(t *testing.T) {
	expectedAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
	p := &peer.Peer{
		Addr:     expectedAddr,
		AuthInfo: nil, // no TLS — nil AuthInfo should not panic
	}
	ctx := peer.NewContext(context.Background(), p)

	addr := PeerAddress(ctx)
	if addr == nil {
		t.Fatal("expected non-nil addr")
	}
	if addr.String() != expectedAddr.String() {
		t.Errorf("expected %s, got: %s", expectedAddr, addr)
	}
}

func TestPeerAddress_WithPeer_WithAuthInfo(t *testing.T) {
	expectedAddr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 8080}
	p := &peer.Peer{
		Addr:     expectedAddr,
		AuthInfo: mockAuthInfo{authType: "tls"},
	}
	ctx := peer.NewContext(context.Background(), p)

	addr := PeerAddress(ctx)
	if addr == nil {
		t.Fatal("expected non-nil addr")
	}
	if addr.String() != expectedAddr.String() {
		t.Errorf("expected %s, got: %s", expectedAddr, addr)
	}
}

func TestPeerAddress_WithUnixSocket(t *testing.T) {
	socketAddr := &net.UnixAddr{Name: "/tmp/test.sock", Net: "unix"}
	p := &peer.Peer{
		Addr:     socketAddr,
		AuthInfo: nil,
	}
	ctx := peer.NewContext(context.Background(), p)

	addr := PeerAddress(ctx)
	if addr == nil {
		t.Fatal("expected non-nil addr for unix socket peer")
	}
	if addr.Network() != "unix" {
		t.Errorf("expected network 'unix', got: %s", addr.Network())
	}
}

// mockAuthInfo implements credentials.AuthInfo for testing.
type mockAuthInfo struct {
	authType string
}

func (m mockAuthInfo) AuthType() string { return m.authType }
