package server

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// A gRPC server stream will only let you get its context but
// there is no way to set a new (augmented context) to the next
// handler (like we do for a unary request). We need to wrap the grpc.ServerSteam
// to be able to set a new context that will be sent to the next stream interceptor.
type contextServerStream struct {
	ss  grpc.ServerStream
	ctx context.Context
}

func (css *contextServerStream) SetHeader(md metadata.MD) error {
	return css.ss.SetHeader(md)
}

func (css *contextServerStream) SendHeader(md metadata.MD) error {
	return css.ss.SendHeader(md)
}

func (css *contextServerStream) SetTrailer(md metadata.MD) {
	css.ss.SetTrailer(md)
}

func (css *contextServerStream) Context() context.Context {
	return css.ctx
}

func (css *contextServerStream) SendMsg(m interface{}) error {
	return css.ss.SendMsg(m)
}

func (css *contextServerStream) RecvMsg(m interface{}) error {
	return css.ss.RecvMsg(m)
}
