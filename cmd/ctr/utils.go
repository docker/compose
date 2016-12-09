package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	gocontext "context"

	"github.com/docker/containerd/api/execution"
	"github.com/tonistiigi/fifo"
	"github.com/urfave/cli"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
)

var grpcConn *grpc.ClientConn

func prepareStdio(in, out, err string) (*sync.WaitGroup, error) {
	var (
		wg sync.WaitGroup

		dst   io.Writer
		src   io.Reader
		close func()
	)

	for _, f := range []struct {
		name   string
		flags  int
		src    bool
		reader io.Reader
		writer io.Writer
	}{
		{in, syscall.O_WRONLY | syscall.O_CREAT | syscall.O_NONBLOCK, false, os.Stdin, nil},
		{out, syscall.O_RDONLY | syscall.O_CREAT | syscall.O_NONBLOCK, true, nil, os.Stdout},
		{err, syscall.O_RDONLY | syscall.O_CREAT | syscall.O_NONBLOCK, true, nil, os.Stderr},
	} {
		ff, err := fifo.OpenFifo(gocontext.Background(), f.name, f.flags, 0700)
		if err != nil {
			return nil, err
		}
		defer func(c io.Closer) {
			if err != nil {
				c.Close()
			}
		}(ff)

		if f.src {
			src = ff
			dst = f.writer
			close = func() {
				ff.Close()
				wg.Done()
			}
			wg.Add(1)
		} else {
			src = f.reader
			dst = ff
			close = func() { ff.Close() }
		}

		go func(dst io.Writer, src io.Reader, close func()) {
			io.Copy(dst, src)
			close()
		}(dst, src, close)
	}

	return &wg, nil
}

func getGRPCConnection(context *cli.Context) (*grpc.ClientConn, error) {
	if grpcConn != nil {
		return grpcConn, nil
	}

	bindSocket := context.GlobalString("socket")
	// reset the logger for grpc to log to dev/null so that it does not mess with our stdio
	grpclog.SetLogger(log.New(ioutil.Discard, "", log.LstdFlags))
	dialOpts := []grpc.DialOption{grpc.WithInsecure(), grpc.WithTimeout(100 * time.Second)}
	dialOpts = append(dialOpts,
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", bindSocket, timeout)
		},
		))

	conn, err := grpc.Dial(fmt.Sprintf("unix://%s", bindSocket), dialOpts...)
	if err != nil {
		return nil, err
	}

	grpcConn = conn
	return grpcConn, nil
}

func getExecutionService(context *cli.Context) (execution.ExecutionServiceClient, error) {
	conn, err := getGRPCConnection(context)
	if err != nil {
		return nil, err
	}
	return execution.NewExecutionServiceClient(conn), nil
}

func getTempDir(id string) (string, error) {
	err := os.MkdirAll(filepath.Join(os.TempDir(), "ctr"), 0700)
	if err != nil {
		return "", err
	}

	tmpDir, err := ioutil.TempDir(filepath.Join(os.TempDir(), "ctr"), fmt.Sprintf("%s-", id))
	if err != nil {
		return "", err
	}
	return tmpDir, nil
}
