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
	"log"
	"net"

	"google.golang.org/grpc/peer"
)

// PeerAddress returns the connection peer address from the gRPC context.
// Returns nil if no peer information is available.
func PeerAddress(ctx context.Context) net.Addr {
	p, ok := peer.FromContext(ctx)
	if !ok {
		log.Println("INFO: No peer information available in context")
		return nil
	}

	if p.AuthInfo != nil {
		log.Printf("CONN: %s auth=%s\n", p.Addr.String(), p.AuthInfo.AuthType())
	} else {
		log.Printf("CONN: %s (no auth)\n", p.Addr.String())
	}
	return p.Addr
}
