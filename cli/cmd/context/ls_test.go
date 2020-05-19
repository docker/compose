package context

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gotest.tools/v3/golden"

	apicontext "github.com/docker/api/context"
	"github.com/docker/api/context/store"
)

type ContextSuite struct {
	suite.Suite
	ctx            context.Context
	writer         *os.File
	reader         *os.File
	originalStdout *os.File
	storeRoot      string
}

func (sut *ContextSuite) BeforeTest(suiteName, testName string) {
	ctx := context.Background()
	ctx = apicontext.WithCurrentContext(ctx, "example")
	dir, err := ioutil.TempDir("", "store")
	require.Nil(sut.T(), err)
	s, err := store.New(
		store.WithRoot(dir),
	)
	require.Nil(sut.T(), err)

	err = s.Create("example", store.TypedContext{
		Type: "example",
	})
	require.Nil(sut.T(), err)

	sut.storeRoot = dir

	ctx = store.WithContextStore(ctx, s)
	sut.ctx = ctx

	sut.originalStdout = os.Stdout
	r, w, err := os.Pipe()
	require.Nil(sut.T(), err)

	os.Stdout = w
	sut.writer = w
	sut.reader = r
}

func (sut *ContextSuite) getStdOut() string {
	err := sut.writer.Close()
	require.Nil(sut.T(), err)

	out, _ := ioutil.ReadAll(sut.reader)

	return string(out)
}

func (sut *ContextSuite) AfterTest(suiteName, testName string) {
	os.Stdout = sut.originalStdout
	err := os.RemoveAll(sut.storeRoot)
	require.Nil(sut.T(), err)
}

func (sut *ContextSuite) TestLs() {
	err := runList(sut.ctx)
	require.Nil(sut.T(), err)
	golden.Assert(sut.T(), sut.getStdOut(), "ls-out.golden")
}

func TestPs(t *testing.T) {
	suite.Run(t, new(ContextSuite))
}
