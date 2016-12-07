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

	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"

	gocontext "context"

	"github.com/docker/containerd/api/execution"
	"github.com/tonistiigi/fifo"
	"github.com/urfave/cli"
)

type runConfig struct {
	Image   string `toml:"image"`
	Process struct {
		Args []string `toml:"args"`
		Env  []string `toml:"env"`
		Cwd  string   `toml:"cwd"`
		Uid  int      `toml:"uid"`
		Gid  int      `toml:"gid"`
		Tty  bool     `toml:"tty"`
	} `toml:"process"`
	Network struct {
		Type    string `toml:"type"`
		IP      string `toml:"ip"`
		Gateway string `toml:"gateway"`
	} `toml:"network"`
}

var runCommand = cli.Command{
	Name:  "run",
	Usage: "run a container",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "bundle, b",
			Usage: "path to the container's bundle",
		},
	},
	Action: func(context *cli.Context) error {
		// var config runConfig
		// if _, err := toml.DecodeFile(context.Args().First(), &config); err != nil {
		// 	return err
		// }
		id := context.Args().First()
		if id == "" {
			return fmt.Errorf("container id must be provided")
		}
		executionService, err := getExecutionService(context)
		if err != nil {
			return err
		}
		containerService, err := getContainerService(context)
		if err != nil {
			return err
		}

		tmpDir, err := getTempDir(id)
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)

		crOpts := &execution.CreateContainerRequest{
			ID:         id,
			BundlePath: context.String("bundle"),
			Stdin:      filepath.Join(tmpDir, "stdin"),
			Stdout:     filepath.Join(tmpDir, "stdout"),
			Stderr:     filepath.Join(tmpDir, "stderr"),
		}

		fwg, err := prepareStdio(crOpts.Stdin, crOpts.Stdout, crOpts.Stderr)
		if err != nil {
			return err
		}

		cr, err := executionService.Create(gocontext.Background(), crOpts)
		if err != nil {
			return err
		}

		if _, err := containerService.Start(gocontext.Background(), &execution.StartContainerRequest{
			ID: cr.Container.ID,
		}); err != nil {
			return err
		}

		// wait for it to die
		for {
			gcr, err := containerService.Get(gocontext.Background(), &execution.GetContainerRequest{
				ID: cr.Container.ID,
			})
			if err != nil {
				return err
			}
			if gcr.Container.Status != execution.Status_RUNNING {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		if _, err := executionService.Delete(gocontext.Background(), &execution.DeleteContainerRequest{
			ID: cr.Container.ID,
		}); err != nil {
			return err
		}

		// Ensure we read all io
		fwg.Wait()

		return nil
	},
}

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

func getContainerService(context *cli.Context) (execution.ContainerServiceClient, error) {
	conn, err := getGRPCConnection(context)
	if err != nil {
		return nil, err
	}
	return execution.NewContainerServiceClient(conn), nil
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
