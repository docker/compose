package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/codegangsta/cli"
	"github.com/docker/containerd/api/grpc/types"
	"github.com/docker/docker/pkg/term"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/specs"
	netcontext "golang.org/x/net/context"
	"google.golang.org/grpc"
)

// TODO: parse flags and pass opts
func getClient(ctx *cli.Context) types.APIClient {
	dialOpts := []grpc.DialOption{grpc.WithInsecure()}
	dialOpts = append(dialOpts,
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		},
		))
	conn, err := grpc.Dial(ctx.GlobalString("address"), dialOpts...)
	if err != nil {
		fatal(err.Error(), 1)
	}
	return types.NewAPIClient(conn)
}

var ContainersCommand = cli.Command{
	Name:  "containers",
	Usage: "interact with running containers",
	Subcommands: []cli.Command{
		ExecCommand,
		KillCommand,
		ListCommand,
		StartCommand,
		StatsCommand,
	},
	Action: listContainers,
}

var ListCommand = cli.Command{
	Name:   "list",
	Usage:  "list all running containers",
	Action: listContainers,
}

func listContainers(context *cli.Context) {
	c := getClient(context)
	resp, err := c.State(netcontext.Background(), &types.StateRequest{})
	if err != nil {
		fatal(err.Error(), 1)
	}
	w := tabwriter.NewWriter(os.Stdout, 20, 1, 3, ' ', 0)
	fmt.Fprint(w, "ID\tPATH\tSTATUS\tPID1\n")
	for _, c := range resp.Containers {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", c.Id, c.BundlePath, c.Status, c.Processes[0].Pid)
	}
	if err := w.Flush(); err != nil {
		fatal(err.Error(), 1)
	}
}

var StartCommand = cli.Command{
	Name:  "start",
	Usage: "start a container",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "checkpoint,c",
			Value: "",
			Usage: "checkpoint to start the container from",
		},
		cli.BoolFlag{
			Name:  "attach,a",
			Usage: "connect to the stdio of the container",
		},
	},
	Action: func(context *cli.Context) {
		var (
			id   = context.Args().Get(0)
			path = context.Args().Get(1)
		)
		if path == "" {
			fatal("bundle path cannot be empty", 1)
		}
		if id == "" {
			fatal("container id cannot be empty", 1)
		}
		c := getClient(context)
		events, err := c.Events(netcontext.Background(), &types.EventsRequest{})
		if err != nil {
			fatal(err.Error(), 1)
		}
		r := &types.CreateContainerRequest{
			Id:         id,
			BundlePath: path,
			Checkpoint: context.String("checkpoint"),
		}
		if context.Bool("attach") {
			mkterm, err := readTermSetting(path)
			if err != nil {
				fatal(err.Error(), 1)
			}
			if mkterm {
				if err := attachTty(r); err != nil {
					fatal(err.Error(), 1)
				}
			} else {
				if err := attachStdio(r); err != nil {
					fatal(err.Error(), 1)
				}
			}
		}
		if _, err := c.CreateContainer(netcontext.Background(), r); err != nil {
			fatal(err.Error(), 1)
		}
		if context.Bool("attach") {
			go func() {
				io.Copy(stdin, os.Stdin)
				if state != nil {
					term.RestoreTerminal(os.Stdin.Fd(), state)
				}
			}()
			for {
				e, err := events.Recv()
				if err != nil {
					fatal(err.Error(), 1)
				}
				if e.Id == id && e.Type == "exit" {
					os.Exit(int(e.Status))
				}
			}
		}
	},
}

var (
	stdin io.WriteCloser
	state *term.State
)

func readTermSetting(path string) (bool, error) {
	f, err := os.Open(filepath.Join(path, "config.json"))
	if err != nil {
		return false, err
	}
	defer f.Close()
	var spec specs.Spec
	if err := json.NewDecoder(f).Decode(&spec); err != nil {
		return false, err
	}
	return spec.Process.Terminal, nil
}

func attachTty(r *types.CreateContainerRequest) error {
	console, err := libcontainer.NewConsole(os.Getuid(), os.Getgid())
	if err != nil {
		return err
	}
	r.Console = console.Path()
	stdin = console
	go func() {
		io.Copy(os.Stdout, console)
		console.Close()
	}()
	s, err := term.SetRawTerminal(os.Stdin.Fd())
	if err != nil {
		return err
	}
	state = s
	return nil
}

func attachStdio(r *types.CreateContainerRequest) error {
	dir, err := ioutil.TempDir("", "ctr-")
	if err != nil {
		return err
	}
	for _, p := range []struct {
		path string
		flag int
		done func(f *os.File)
	}{
		{
			path: filepath.Join(dir, "stdin"),
			flag: syscall.O_RDWR,
			done: func(f *os.File) {
				r.Stdin = filepath.Join(dir, "stdin")
				stdin = f
			},
		},
		{
			path: filepath.Join(dir, "stdout"),
			flag: syscall.O_RDWR,
			done: func(f *os.File) {
				r.Stdout = filepath.Join(dir, "stdout")
				go io.Copy(os.Stdout, f)
			},
		},
		{
			path: filepath.Join(dir, "stderr"),
			flag: syscall.O_RDWR,
			done: func(f *os.File) {
				r.Stderr = filepath.Join(dir, "stderr")
				go io.Copy(os.Stderr, f)
			},
		},
	} {
		if err := syscall.Mkfifo(p.path, 0755); err != nil {
			return fmt.Errorf("mkfifo: %s %v", p.path, err)
		}
		f, err := os.OpenFile(p.path, p.flag, 0)
		if err != nil {
			return fmt.Errorf("open: %s %v", p.path, err)
		}
		p.done(f)
	}
	return nil
}

var KillCommand = cli.Command{
	Name:  "kill",
	Usage: "send a signal to a container or it's processes",
	Flags: []cli.Flag{
		cli.IntFlag{
			Name:  "pid,p",
			Usage: "pid of the process to signal within the container",
		},
		cli.IntFlag{
			Name:  "signal,s",
			Value: 15,
			Usage: "signal to send to the container",
		},
	},
	Action: func(context *cli.Context) {
		id := context.Args().First()
		if id == "" {
			fatal("container id cannot be empty", 1)
		}
		c := getClient(context)
		if _, err := c.Signal(netcontext.Background(), &types.SignalRequest{
			Id:     id,
			Pid:    uint32(context.Int("pid")),
			Signal: uint32(context.Int("signal")),
		}); err != nil {
			fatal(err.Error(), 1)
		}
	},
}

var ExecCommand = cli.Command{
	Name:  "exec",
	Usage: "exec another process in an existing container",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "id",
			Usage: "container id to add the process to",
		},
		cli.StringFlag{
			Name:  "cwd",
			Usage: "current working directory for the process",
		},
		cli.BoolFlag{
			Name:  "tty,t",
			Usage: "create a terminal for the process",
		},
		cli.StringSliceFlag{
			Name:  "env,e",
			Value: &cli.StringSlice{},
			Usage: "environment variables for the process",
		},
		cli.IntFlag{
			Name:  "uid,u",
			Usage: "user id of the user for the process",
		},
		cli.IntFlag{
			Name:  "gid,g",
			Usage: "group id of the user for the process",
		},
	},
	Action: func(context *cli.Context) {
		p := &types.AddProcessRequest{
			Args:     context.Args(),
			Cwd:      context.String("cwd"),
			Terminal: context.Bool("tty"),
			Id:       context.String("id"),
			Env:      context.StringSlice("env"),
			User: &types.User{
				Uid: uint32(context.Int("uid")),
				Gid: uint32(context.Int("gid")),
			},
		}
		c := getClient(context)
		if _, err := c.AddProcess(netcontext.Background(), p); err != nil {
			fatal(err.Error(), 1)
		}
	},
}

var StatsCommand = cli.Command{
	Name:  "stats",
	Usage: "get stats for running container",
	Action: func(context *cli.Context) {
		req := &types.StatsRequest{
			Id: context.Args().First(),
		}
		c := getClient(context)
		stream, err := c.GetStats(netcontext.Background(), req)
		if err != nil {
			fatal(err.Error(), 1)
		}
		for {
			stats, err := stream.Recv()
			if err != nil {
				fatal(err.Error(), 1)
			}
			fmt.Println(stats)
		}
	},
}
