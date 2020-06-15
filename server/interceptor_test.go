package server

import (
	"context"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/docker/api/config"
	apicontext "github.com/docker/api/context"
)

type interceptorSuite struct {
	suite.Suite
	dir string
	ctx context.Context
}

func (is *interceptorSuite) BeforeTest(suiteName, testName string) {
	dir, err := ioutil.TempDir("", "example")
	require.Nil(is.T(), err)

	ctx := context.Background()
	ctx = config.WithDir(ctx, dir)
	err = ioutil.WriteFile(path.Join(dir, "config.json"), []byte(`{"currentContext": "default"}`), 0644)
	require.Nil(is.T(), err)

	is.dir = dir
	is.ctx = ctx
}

func (is *interceptorSuite) AfterTest(suiteName, tesName string) {
	err := os.RemoveAll(is.dir)
	require.Nil(is.T(), err)
}

func (is *interceptorSuite) TestUnaryGetCurrentContext() {
	interceptor := unaryServerInterceptor(is.ctx)

	currentContext := is.callUnary(context.Background(), interceptor)

	assert.Equal(is.T(), "default", currentContext)
}

func (is *interceptorSuite) TestUnaryContextFromMetadata() {
	contextName := "test"

	interceptor := unaryServerInterceptor(is.ctx)
	reqCtx := context.Background()
	reqCtx = metadata.NewIncomingContext(reqCtx, metadata.MD{
		(key): []string{contextName},
	})

	currentContext := is.callUnary(reqCtx, interceptor)

	assert.Equal(is.T(), contextName, currentContext)
}

func (is *interceptorSuite) TestStreamGetCurrentContext() {
	interceptor := streamServerInterceptor(is.ctx)

	currentContext := is.callStream(context.Background(), interceptor)

	assert.Equal(is.T(), "default", currentContext)
}

func (is *interceptorSuite) TestStreamContextFromMetadata() {
	contextName := "test"

	interceptor := streamServerInterceptor(is.ctx)
	reqCtx := context.Background()
	reqCtx = metadata.NewIncomingContext(reqCtx, metadata.MD{
		(key): []string{contextName},
	})

	currentContext := is.callStream(reqCtx, interceptor)

	assert.Equal(is.T(), contextName, currentContext)
}

func (is *interceptorSuite) callStream(ctx context.Context, interceptor grpc.StreamServerInterceptor) string {
	currentContext := ""
	err := interceptor(nil, &contextServerStream{
		ctx: ctx,
	}, &grpc.StreamServerInfo{
		FullMethod: "/com.docker.api.protos.context.v1.Contexts/test",
	}, func(srv interface{}, stream grpc.ServerStream) error {
		currentContext = apicontext.CurrentContext(stream.Context())
		return nil
	})

	require.Nil(is.T(), err)

	return currentContext
}

func (is *interceptorSuite) callUnary(ctx context.Context, interceptor grpc.UnaryServerInterceptor) string {
	currentContext := ""
	resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{
		FullMethod: "/com.docker.api.protos.context.v1.Contexts/test",
	}, func(ctx context.Context, req interface{}) (interface{}, error) {
		currentContext = apicontext.CurrentContext(ctx)
		return nil, nil
	})

	require.Nil(is.T(), err)
	require.Nil(is.T(), resp)

	return currentContext
}

func TestInterceptor(t *testing.T) {
	suite.Run(t, new(interceptorSuite))
}
