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
	"io/ioutil"
	"os"
	"path"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/docker/compose-cli/api/config"
	apicontext "github.com/docker/compose-cli/api/context"
)

func testContext(t *testing.T) context.Context {
	dir, err := ioutil.TempDir("", "example")
	assert.NilError(t, err)

	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})

	ctx := context.Background()
	config.WithDir(dir)
	err = ioutil.WriteFile(path.Join(dir, "config.json"), []byte(`{"currentContext": "default"}`), 0644)
	assert.NilError(t, err)

	return ctx
}

func TestUnaryGetCurrentContext(t *testing.T) {
	ctx := testContext(t)
	interceptor := unaryServerInterceptor(ctx)

	currentContext := callUnary(context.Background(), t, interceptor)
	assert.Equal(t, currentContext, "default")
}

func TestUnaryContextFromMetadata(t *testing.T) {
	ctx := testContext(t)
	contextName := "test"

	interceptor := unaryServerInterceptor(ctx)
	reqCtx := context.Background()
	reqCtx = metadata.NewIncomingContext(reqCtx, metadata.MD{
		(key): []string{contextName},
	})

	currentContext := callUnary(reqCtx, t, interceptor)
	assert.Equal(t, contextName, currentContext)
}

func TestStreamGetCurrentContext(t *testing.T) {
	ctx := testContext(t)
	interceptor := streamServerInterceptor(ctx)

	currentContext := callStream(context.Background(), t, interceptor)

	assert.Equal(t, currentContext, "default")
}

func TestStreamContextFromMetadata(t *testing.T) {
	ctx := testContext(t)
	contextName := "test"

	interceptor := streamServerInterceptor(ctx)
	reqCtx := context.Background()
	reqCtx = metadata.NewIncomingContext(reqCtx, metadata.MD{
		(key): []string{contextName},
	})

	currentContext := callStream(reqCtx, t, interceptor)
	assert.Equal(t, currentContext, contextName)
}

func callStream(ctx context.Context, t *testing.T, interceptor grpc.StreamServerInterceptor) string {
	currentContext := ""
	err := interceptor(nil, &contextServerStream{
		ctx: ctx,
	}, &grpc.StreamServerInfo{
		FullMethod: "/com.docker.api.protos.context.v1.Contexts/test",
	}, func(srv interface{}, stream grpc.ServerStream) error {
		currentContext = apicontext.Current()
		return nil
	})

	assert.NilError(t, err)

	return currentContext
}

func callUnary(ctx context.Context, t *testing.T, interceptor grpc.UnaryServerInterceptor) string {
	currentContext := ""
	resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{
		FullMethod: "/com.docker.api.protos.context.v1.Contexts/test",
	}, func(ctx context.Context, req interface{}) (interface{}, error) {
		currentContext = apicontext.Current()
		return nil, nil
	})

	assert.NilError(t, err)
	assert.Assert(t, cmp.Nil(resp))

	return currentContext
}
