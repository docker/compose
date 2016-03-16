// +build linux

package main

import (
	"os"
	"runtime"

	"github.com/codegangsta/cli"
	"github.com/coreos/go-systemd/activation"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/specs/specs-go"
)

// default action is to start a container
var startCommand = cli.Command{
	Name:  "start",
	Usage: "create and run a container",
	ArgsUsage: `<container-id>

Where "<container-id>" is your name for the instance of the container that you
are starting. The name you provide for the container instance must be unique on
your host.`,
	Description: `The start command creates an instance of a container for a bundle. The bundle
is a directory with a specification file and a root filesystem.`,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "bundle, b",
			Value: "",
			Usage: `path to the root of the bundle directory, defaults to the current directory`,
		},
		cli.StringFlag{
			Name:  "console",
			Value: "",
			Usage: "specify the pty slave path for use with the container",
		},
		cli.BoolFlag{
			Name:  "detach,d",
			Usage: "detach from the container's process",
		},
		cli.StringFlag{
			Name:  "pid-file",
			Value: "",
			Usage: "specify the file to write the process id to",
		},
	},
	Action: func(context *cli.Context) {
		bundle := context.String("bundle")
		if bundle != "" {
			if err := os.Chdir(bundle); err != nil {
				fatal(err)
			}
		}
		spec, err := loadSpec(specConfig)
		if err != nil {
			fatal(err)
		}

		notifySocket := os.Getenv("NOTIFY_SOCKET")
		if notifySocket != "" {
			setupSdNotify(spec, notifySocket)
		}

		if os.Geteuid() != 0 {
			fatalf("runc should be run as root")
		}

		status, err := startContainer(context, spec)
		if err != nil {
			fatal(err)
		}
		// exit with the container's exit status so any external supervisor is
		// notified of the exit with the correct exit status.
		os.Exit(status)
	},
}

var initCommand = cli.Command{
	Name: "init",
	Usage: `init is used to initialize the containers namespaces and launch the users process.
    This command should not be called outside of runc.
    `,
	Action: func(context *cli.Context) {
		runtime.GOMAXPROCS(1)
		runtime.LockOSThread()
		factory, _ := libcontainer.New("")
		if err := factory.StartInitialization(); err != nil {
			fatal(err)
		}
		panic("libcontainer: container init failed to exec")
	},
}

func startContainer(context *cli.Context, spec *specs.Spec) (int, error) {
	id := context.Args().First()
	if id == "" {
		return -1, errEmptyID
	}
	container, err := createContainer(context, id, spec)
	if err != nil {
		return -1, err
	}

	// ensure that the container is always removed if we were the process
	// that created it.
	detach := context.Bool("detach")
	if !detach {
		defer destroy(container)
	}

	// Support on-demand socket activation by passing file descriptors into the container init process.
	listenFDs := []*os.File{}
	if os.Getenv("LISTEN_FDS") != "" {
		listenFDs = activation.Files(false)
	}

	status, err := runProcess(container, &spec.Process, listenFDs, context.String("console"), context.String("pid-file"), detach)
	if err != nil {
		destroy(container)
		return -1, err
	}
	return status, nil
}
