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

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"

	"github.com/docker/api/client"
	apicontext "github.com/docker/api/context"
	"github.com/docker/api/context/store"
	"github.com/docker/api/server/proxy"
)

// New returns a new GRPC server.
func New(ctx context.Context) *grpc.Server {
	s := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			unaryMeta(ctx),
			unary,
		),
		grpc.ChainStreamInterceptor(
			grpc.StreamServerInterceptor(stream),
			grpc.StreamServerInterceptor(streamMeta(ctx)),
		),
	)
	hs := health.NewServer()
	grpc_health_v1.RegisterHealthServer(s, hs)
	return s
}

//CreateListener creates a listener either on tcp://, or local listener, supporting unix:// for unix socket or npipe:// for named pipes on windows
func CreateListener(address string) (net.Listener, error) {
	if strings.HasPrefix(address, "tcp://") {
		return net.Listen("tcp", strings.TrimPrefix(address, "tcp://"))
	}
	return createLocalListener(address)
}

func unary(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	return grpc_prometheus.UnaryServerInterceptor(ctx, req, info, handler)
}

func stream(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	return grpc_prometheus.StreamServerInterceptor(srv, ss, info, handler)
}

func unaryMeta(clictx context.Context) func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		configuredCtx, err := configureContext(ctx, clictx)
		if err != nil {
			return nil, err
		}

		return handler(configuredCtx, req)
	}
}

func streamMeta(clictx context.Context) func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx, err := configureContext(ss.Context(), clictx)
		if err != nil {
			return err
		}

		nss := newServerStream(ctx, ss)

		return handler(srv, nss)
	}
}

// nolint: golint
func configureContext(ctx context.Context, clictx context.Context) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx, nil
	}

	key, ok := md[apicontext.Key]
	if !ok {
		return ctx, nil
	}

	if len(key) == 1 {
		s := store.ContextStore(clictx)
		ctx = store.WithContextStore(ctx, s)
		ctx = apicontext.WithCurrentContext(ctx, key[0])

		c, err := client.New(ctx)
		if err != nil {
			return nil, err
		}

		ctx, err = proxy.WithClient(ctx, c)
		if err != nil {
			return nil, err
		}
	}

	return ctx, nil
}

type contextServerStream struct {
	s   grpc.ServerStream
	ctx context.Context
}

func newServerStream(ctx context.Context, s grpc.ServerStream) grpc.ServerStream {
	return &contextServerStream{
		s:   s,
		ctx: ctx,
	}
}

func (css *contextServerStream) SetHeader(md metadata.MD) error {
	return css.s.SetHeader(md)
}

func (css *contextServerStream) SendHeader(md metadata.MD) error {
	return css.s.SendHeader(md)
}

func (css *contextServerStream) SetTrailer(md metadata.MD) {
	css.s.SetTrailer(md)
}

func (css *contextServerStream) Context() context.Context {
	return css.ctx
}

func (css *contextServerStream) SendMsg(m interface{}) error {
	return css.s.SendMsg(m)
}

func (css *contextServerStream) RecvMsg(m interface{}) error {
	return css.s.RecvMsg(m)
}
