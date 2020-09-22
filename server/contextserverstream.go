/*
   Copyright 2020 Docker Compose CLI authors

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
