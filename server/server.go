/*
	Copyright (c) 2020 Docker Inc.

	Permission is hereby granted, free of charge, to any person
	obtaining a copy of this software and associated documentation
	files (the "Software"), to deal in the Software without
	restriction, including without limitation the rights to use, copy,
	modify, merge, publish, distribute, sublicense, and/or sell copies
	of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be
	included in all copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
	EXPRESS OR IMPLIED,
	INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
	IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
	HOLDERS BE LIABLE FOR ANY CLAIM,
	DAMAGES OR OTHER LIABILITY,
	WHETHER IN AN ACTION OF CONTRACT,
	TORT OR OTHERWISE,
	ARISING FROM, OUT OF OR IN CONNECTION WITH
	THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package server

import (
	"context"
	"net"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// New returns a new GRPC server.
func New(ctx context.Context) *grpc.Server {
	s := grpc.NewServer(
		grpc.UnaryInterceptor(unaryServerInterceptor(ctx)),
		grpc.StreamInterceptor(streamServerInterceptor(ctx)),
	)
	hs := health.NewServer()
	grpc_health_v1.RegisterHealthServer(s, hs)
	return s
}

// CreateListener creates a listener either on tcp://, or local listener,
// supporting unix:// for unix socket or npipe:// for named pipes on windows
func CreateListener(address string) (net.Listener, error) {
	if strings.HasPrefix(address, "tcp://") {
		return net.Listen("tcp", strings.TrimPrefix(address, "tcp://"))
	}
	return createLocalListener(address)
}
