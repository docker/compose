package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/codegangsta/cli"
	"github.com/docker/containerd/api/grpc/types"
	"github.com/docker/docker/pkg/term"
	"github.com/opencontainers/specs"
	netcontext "golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
)

// TODO: parse flags and pass opts
func getClient(ctx *cli.Context) types.APIClient {
	// reset the logger for grpc to log to dev/null so that it does not mess with our stdio
	grpclog.SetLogger(log.New(ioutil.Discard, "", log.LstdFlags))
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

var containersCommand = cli.Command{
	Name:  "containers",
	Usage: "interact with running containers",
	Subcommands: []cli.Command{
		execCommand,
		killCommand,
		listCommand,
		startCommand,
		statsCommand,
	},
	Action: listContainers,
}

var stateCommand = cli.Command{
	Name:  "state",
	Usage: "get a raw dump of the containerd state",
	Action: func(context *cli.Context) {
		c := getClient(context)
		resp, err := c.State(netcontext.Background(), &types.StateRequest{
			Id: context.Args().First(),
		})
		if err != nil {
			fatal(err.Error(), 1)
		}
		data, err := json.Marshal(resp)
		if err != nil {
			fatal(err.Error(), 1)
		}
		fmt.Print(string(data))
	},
}

var listCommand = cli.Command{
	Name:   "list",
	Usage:  "list all running containers",
	Action: listContainers,
}

func listContainers(context *cli.Context) {
	c := getClient(context)
	resp, err := c.State(netcontext.Background(), &types.StateRequest{
		Id: context.Args().First(),
	})
	if err != nil {
		fatal(err.Error(), 1)
	}
	w := tabwriter.NewWriter(os.Stdout, 20, 1, 3, ' ', 0)
	fmt.Fprint(w, "ID\tPATH\tSTATUS\tPROCESSES\n")
	for _, c := range resp.Containers {
		procs := []string{}
		for _, p := range c.Processes {
			procs = append(procs, p.Pid)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", c.Id, c.BundlePath, c.Status, strings.Join(procs, ","))
	}
	if err := w.Flush(); err != nil {
		fatal(err.Error(), 1)
	}
}

var startCommand = cli.Command{
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
		bpath, err := filepath.Abs(path)
		if err != nil {
			fatal(fmt.Sprintf("cannot get the absolute path of the bundle: %v", err), 1)
		}
		s, err := createStdio()
		if err != nil {
			fatal(err.Error(), 1)
		}
		var (
			tty bool
			c   = getClient(context)
			r   = &types.CreateContainerRequest{
				Id:         id,
				BundlePath: bpath,
				Checkpoint: context.String("checkpoint"),
				Stdin:      s.stdin,
				Stdout:     s.stdout,
				Stderr:     s.stderr,
			}
		)
		if context.Bool("attach") {
			mkterm, err := readTermSetting(bpath)
			if err != nil {
				fatal(err.Error(), 1)
			}
			tty = mkterm
			if mkterm {
				s, err := term.SetRawTerminal(os.Stdin.Fd())
				if err != nil {
					fatal(err.Error(), 1)
				}
				state = s
			}
			if err := attachStdio(s); err != nil {
				fatal(err.Error(), 1)
			}
		}
		events, err := c.Events(netcontext.Background(), &types.EventsRequest{})
		if err != nil {
			fatal(err.Error(), 1)
		}
		if _, err := c.CreateContainer(netcontext.Background(), r); err != nil {
			fatal(err.Error(), 1)
		}
		if context.Bool("attach") {
			restoreAndCloseStdin := func() {
				if state != nil {
					term.RestoreTerminal(os.Stdin.Fd(), state)
				}
				stdin.Close()
			}
			go func() {
				io.Copy(stdin, os.Stdin)
				if _, err := c.UpdateProcess(netcontext.Background(), &types.UpdateProcessRequest{
					Id:         id,
					Pid:        "init",
					CloseStdin: true,
				}); err != nil {
					fatal(err.Error(), 1)
				}
				restoreAndCloseStdin()
			}()
			if tty {
				resize(id, "init", c)
				go func() {
					s := make(chan os.Signal, 64)
					signal.Notify(s, syscall.SIGWINCH)
					for range s {
						if err := resize(id, "init", c); err != nil {
							log.Println(err)
						}
					}
				}()
			}
			if err := waitForExit(c, events, id, "init", restoreAndCloseStdin); err != nil {
				fatal(err.Error(), 1)
			}
		}
	},
}

func resize(id, pid string, c types.APIClient) error {
	ws, err := term.GetWinsize(os.Stdin.Fd())
	if err != nil {
		return err
	}
	if _, err := c.UpdateProcess(netcontext.Background(), &types.UpdateProcessRequest{
		Id:     id,
		Pid:    "init",
		Width:  uint32(ws.Width),
		Height: uint32(ws.Height),
	}); err != nil {
		return err
	}
	return nil
}

var (
	stdin io.WriteCloser
	state *term.State
)

// readTermSetting reads the Terminal option out of the specs configuration
// to know if ctr should allocate a pty
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

func attachStdio(s stdio) error {
	stdinf, err := os.OpenFile(s.stdin, syscall.O_RDWR, 0)
	if err != nil {
		return err
	}
	// FIXME: assign to global
	stdin = stdinf
	stdoutf, err := os.OpenFile(s.stdout, syscall.O_RDWR, 0)
	if err != nil {
		return err
	}
	go io.Copy(os.Stdout, stdoutf)
	stderrf, err := os.OpenFile(s.stderr, syscall.O_RDWR, 0)
	if err != nil {
		return err
	}
	go io.Copy(os.Stderr, stderrf)
	return nil
}

var killCommand = cli.Command{
	Name:  "kill",
	Usage: "send a signal to a container or its processes",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "pid,p",
			Value: "init",
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
			Pid:    context.String("pid"),
			Signal: uint32(context.Int("signal")),
		}); err != nil {
			fatal(err.Error(), 1)
		}
	},
}

var execCommand = cli.Command{
	Name:  "exec",
	Usage: "exec another process in an existing container",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "id",
			Usage: "container id to add the process to",
		},
		cli.StringFlag{
			Name:  "pid",
			Usage: "process id for the new process",
		},
		cli.BoolFlag{
			Name:  "attach,a",
			Usage: "connect to the stdio of the container",
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
			Id:       context.String("id"),
			Pid:      context.String("pid"),
			Args:     context.Args(),
			Cwd:      context.String("cwd"),
			Terminal: context.Bool("tty"),
			Env:      context.StringSlice("env"),
			User: &types.User{
				Uid: uint32(context.Int("uid")),
				Gid: uint32(context.Int("gid")),
			},
		}
		s, err := createStdio()
		if err != nil {
			fatal(err.Error(), 1)
		}
		p.Stdin = s.stdin
		p.Stdout = s.stdout
		p.Stderr = s.stderr
		if context.Bool("attach") {
			if context.Bool("tty") {
				s, err := term.SetRawTerminal(os.Stdin.Fd())
				if err != nil {
					fatal(err.Error(), 1)
				}
				state = s
			}
			if err := attachStdio(s); err != nil {
				fatal(err.Error(), 1)
			}
		}
		c := getClient(context)
		events, err := c.Events(netcontext.Background(), &types.EventsRequest{})
		if err != nil {
			fatal(err.Error(), 1)
		}
		if _, err := c.AddProcess(netcontext.Background(), p); err != nil {
			fatal(err.Error(), 1)
		}
		if context.Bool("attach") {
			restoreAndCloseStdin := func() {
				if state != nil {
					term.RestoreTerminal(os.Stdin.Fd(), state)
				}
				stdin.Close()
			}
			go func() {
				io.Copy(stdin, os.Stdin)
				if _, err := c.UpdateProcess(netcontext.Background(), &types.UpdateProcessRequest{
					Id:         p.Id,
					Pid:        p.Pid,
					CloseStdin: true,
				}); err != nil {
					log.Println(err)
				}
				restoreAndCloseStdin()
			}()
			if context.Bool("tty") {
				resize(p.Id, p.Pid, c)
				go func() {
					s := make(chan os.Signal, 64)
					signal.Notify(s, syscall.SIGWINCH)
					for range s {
						if err := resize(p.Id, p.Pid, c); err != nil {
							log.Println(err)
						}
					}
				}()
			}
			if err := waitForExit(c, events, context.String("id"), context.String("pid"), restoreAndCloseStdin); err != nil {
				fatal(err.Error(), 1)
			}
		}
	},
}

var statsCommand = cli.Command{
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

func waitForExit(c types.APIClient, events types.API_EventsClient, id, pid string, closer func()) error {
	for {
		e, err := events.Recv()
		if err != nil {
			time.Sleep(1 * time.Second)
			events, _ = c.Events(netcontext.Background(), &types.EventsRequest{})
			continue
		}
		if e.Id == id && e.Type == "exit" && e.Pid == pid {
			closer()
			os.Exit(int(e.Status))
		}
	}
	return nil
}

type stdio struct {
	stdin  string
	stdout string
	stderr string
}

func createStdio() (s stdio, err error) {
	tmp, err := ioutil.TempDir("", "ctr-")
	if err != nil {
		return s, err
	}
	// create fifo's for the process
	for name, fd := range map[string]*string{
		"stdin":  &s.stdin,
		"stdout": &s.stdout,
		"stderr": &s.stderr,
	} {
		path := filepath.Join(tmp, name)
		if err := syscall.Mkfifo(path, 0755); err != nil && !os.IsExist(err) {
			return s, err
		}
		*fd = path
	}
	return s, nil
}
