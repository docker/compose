package main

import (
	"fmt"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerkit"
	"github.com/docker/containerkit/oci"
	"github.com/docker/containerkit/osutils"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

func runContainer() error {
	// create a new runc runtime that implements the ExecutionDriver interface
	runc, err := oci.New(oci.Opts{
		Root: "/run/runc",
		Name: "runc",
	})
	if err != nil {
		return err
	}
	dockerContainer := &testConfig{}

	// create a new container
	container, err := containerkit.NewContainer(dockerContainer, NewBindDriver(), runc)
	if err != nil {
		return err
	}
	// setup some stdio for our container
	container.Stdin = os.Stdin
	container.Stdout = os.Stdout
	container.Stderr = os.Stderr

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

	container, err = containerkit.LoadContainer(dockerContainer, runc)
	if err != nil {
		return err
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

// "Hooks do optional work. Drivers do mandatory work"
func main() {
	if err := osutils.SetSubreaper(1); err != nil {
		logrus.Fatal(err)
	}
	if err := runContainer(); err != nil {
		logrus.Fatal(err)
	}
}
