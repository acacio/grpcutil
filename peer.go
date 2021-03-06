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

// PeerAddress returns connecion peer address
func PeerAddress(ctx context.Context) net.Addr {
	p, ok := peer.FromContext(ctx)
	if !ok {
		println("INFO: No peer information")
		return nil
	}

	log.Println("CONN: ", p.Addr.String(), p.AuthInfo.AuthType())
	return p.Addr
}
