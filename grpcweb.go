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
	"log"
	"net/http"

	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
)

// StartgRPCWeb encapsulates a gRPC server and serves gRPC on a new port
func StartgRPCWeb(s *grpc.Server, grpcwebport string) {
	println("Starting gRPCWeb at " + grpcwebport)
	wrappedServer := grpcweb.WrapServer(s)

	handler := func(res http.ResponseWriter, req *http.Request) {
		wrappedServer.ServeHTTP(res, req)
	}

	httpServer := &http.Server{
		Addr:    grpcwebport,
		Handler: http.HandlerFunc(handler),
	}

	grpclog.Println("Starting server...")
	log.Printf("Could not start gRPC-WEB endpoint: %v", httpServer.ListenAndServe())
}
