package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/shim"
	"github.com/docker/containerkit"
	"github.com/docker/containerkit/osutils"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

func runContainer() error {
	// create a new runtime runtime that implements the ExecutionDriver interface
	runtime, err := shim.New(shim.Opts{
		Root:        "/run/cshim/test",
		Name:        "containerd-shim",
		RuntimeName: "runc",
		RuntimeRoot: "/run/runc",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		return err
	}
	dockerContainer := &testConfig{}

	// create a new container
	container, err := containerkit.NewContainer(dockerContainer, NewBindDriver(), runtime)
	if err != nil {
		return err
	}
	// setup some stdio for our container
	container.Stdin = Stdin()
	container.Stdout = Stdout()
	container.Stderr = Stderr()

	// go ahead and set the container in the create state and have it ready to start
	logrus.Info("create container")
	if err := container.Create(); err != nil {
		return err
	}

	// start the user defined process in the container
	logrus.Info("start container")
	if err := container.Start(); err != nil {
		return err
	}

	if exec {
		// start 10 exec processes giving the go var i to exec to stdout
		for i := 0; i < 10; i++ {
			process, err := container.NewProcess(&specs.Process{
				Args: []string{
					"echo", fmt.Sprintf("sup from itteration %d", i),
				},
				Env:             env,
				Terminal:        false,
				Cwd:             "/",
				NoNewPrivileges: true,
				Capabilities:    caps,
			})

			process.Stdin = os.Stdin
			process.Stdout = os.Stdout
			process.Stderr = os.Stderr

			if err := process.Start(); err != nil {
				return err
			}
			procStatus, err := process.Wait()
			if err != nil {
				return err
			}
			logrus.Infof("process %d returned with %d", i, procStatus)
		}
	}

	if load {
		if container, err = containerkit.LoadContainer(dockerContainer, runtime); err != nil {
			return err
		}
	}

	// wait for it to exit and get the exit status
	logrus.Info("wait container")
	status, err := container.Wait()
	if err != nil {
		return err
	}

	// delete the container after it is done
	logrus.Info("delete container")
	if container.Delete(); err != nil {
		return err
	}
	logrus.Infof("exit status %d", status)
	return nil
}

var (
	exec bool
	load bool
)

// "Hooks do optional work. Drivers do mandatory work"
func main() {
	flag.BoolVar(&exec, "exec", false, "run the execs")
	flag.BoolVar(&load, "load", false, "reload the container")
	flag.Parse()
	if err := osutils.SetSubreaper(1); err != nil {
		logrus.Fatal(err)
	}
	if err := runContainer(); err != nil {
		logrus.Fatal(err)
	}
}
