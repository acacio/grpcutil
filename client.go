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
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	grpc_codes "google.golang.org/grpc/codes"
	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/retry"

	"github.com/acacio/tlsutil"
)

func setupDialOpts(tlstype, ca, crt, key string, opts []grpc.DialOption) ([]grpc.DialOption, error) {
	if tlstype != "" && tlstype != "insecure" {
		config, err := tlsutil.SetupClientTLS(tlstype, ca, crt, key)
		if err != nil {
			return nil, err
		}
		creds := credentials.NewTLS(config)
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		// Use insecure credentials instead of deprecated grpc.WithInsecure()
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	return opts, nil
}

// ClientOpts configure gRPC client connection
type ClientOpts struct {
	TLSType string
	CA      string
	Cert    string
	Key     string
	Token   string
	// Block causes SetupConnection to wait until the connection reaches READY state.
	// Uses a 30-second timeout for the wait.
	Block bool
}

// SetupConnection handles base gRPC connection establishment.
// Uses grpc.NewClient which connects lazily; set Block=true in opts to wait for READY state.
func SetupConnection(addr string, opts *ClientOpts) (*grpc.ClientConn, error) {
	retryOpts := []grpc_retry.CallOption{
		grpc_retry.WithBackoff(grpc_retry.BackoffLinear(100 * time.Millisecond)),
		grpc_retry.WithCodes(grpc_codes.DeadlineExceeded, grpc_codes.Unavailable),
	}

	dialopts := []grpc.DialOption{
		grpc.WithChainStreamInterceptor(grpc_retry.StreamClientInterceptor(retryOpts...)),
		grpc.WithChainUnaryInterceptor(grpc_retry.UnaryClientInterceptor(retryOpts...)),
	}

	if opts.Token != "" {
		dialopts = append(dialopts, grpc.WithPerRPCCredentials(TokenAuth{Token: opts.Token}))
	}

	dialopts, err := setupDialOpts(opts.TLSType, opts.CA, opts.Cert, opts.Key, dialopts)
	if err != nil {
		fmt.Printf("ERROR: Failed to setup gRPC connection:\n%v\n", err)
		return nil, err
	}

	conn, err := grpc.NewClient(addr, dialopts...)
	if err != nil {
		return nil, err
	}

	if opts.Block {
		conn.Connect()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for {
			state := conn.GetState()
			if state == connectivity.Ready {
				break
			}
			if !conn.WaitForStateChange(ctx, state) {
				conn.Close()
				return nil, fmt.Errorf("timed out waiting for connection to become ready")
			}
		}
	}

	return conn, nil
}
