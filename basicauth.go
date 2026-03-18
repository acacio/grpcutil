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
)

// BasicAuthCreds is an implementation of credentials.PerRPCCredentials
// that encodes username and password as an HTTP Basic Authorization header.
// Use NewBasicAuthCreds to construct from outside this package.
type BasicAuthCreds struct {
	username, password string
}

// NewBasicAuthCreds creates a BasicAuthCreds for use as gRPC PerRPCCredentials.
func NewBasicAuthCreds(username, password string) *BasicAuthCreds {
	return &BasicAuthCreds{username: username, password: password}
}

// GetRequestMetadata sets the value for "authorization" key
func (b *BasicAuthCreds) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Basic " + b.Digest(),
	}, nil
}

// RequireTransportSecurity should be true as even though the credentials are base64, we want to have it encrypted over the wire.
func (b *BasicAuthCreds) RequireTransportSecurity() bool {
	return true
}

//helper function
func (b *BasicAuthCreds) Digest() string {
	auth := b.username + ":" + b.password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

