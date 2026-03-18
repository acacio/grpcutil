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
	"math"
	"time"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

// KeepAliveDefault returns a gRPC ServerOption with sensible keepalive defaults:
// connections are kept alive indefinitely, with a 2-hour ping interval and 30-second timeout.
func KeepAliveDefault() grpc.ServerOption {
	return grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionIdle:     time.Duration(math.MaxInt64),
		MaxConnectionAge:      time.Duration(math.MaxInt64),
		MaxConnectionAgeGrace: time.Duration(math.MaxInt64),
		Time:                  2 * time.Hour,
		Timeout:               30 * time.Second,
	})
}

// DefaultServerOptions returns the provided ServerOptions extended with default middleware:
// Prometheus metrics collection and panic recovery for both unary and streaming RPCs.
// Uses gRPC's built-in chain interceptors (available since gRPC v1.21).
func DefaultServerOptions(srvOpts []grpc.ServerOption) []grpc.ServerOption {
	srvOpts = append(srvOpts,
		grpc.ChainStreamInterceptor(
			grpc_prometheus.StreamServerInterceptor,
			grpc_recovery.StreamServerInterceptor(),
		),
		grpc.ChainUnaryInterceptor(
			grpc_prometheus.UnaryServerInterceptor,
			grpc_recovery.UnaryServerInterceptor(),
		),
	)
	return srvOpts
}
