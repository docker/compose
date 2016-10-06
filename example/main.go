package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/shim"
	"github.com/docker/containerkit"
	"github.com/docker/containerkit/osutils"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

func reloadContainer() error {
	// create a new runtime runtime that implements the ExecutionDriver interface
	runtime, err := shim.Load("/run/cshim/test")
	if err != nil {
		return err
	}
	dockerContainer := &testConfig{}
	container, err := containerkit.LoadContainer(dockerContainer, runtime)
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
	container.Stdin = Stdin("")
	container.Stdout = Stdout("")
	container.Stderr = Stderr("")

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

	for i := 0; i < exec; i++ {
		process, err := container.NewProcess(&specs.Process{
			Args: []string{
				"sh", "-c",
				"echo " + fmt.Sprintf("sup from itteration %d", i),
			},
			Env:             env,
			Terminal:        false,
			Cwd:             "/",
			NoNewPrivileges: true,
			Capabilities:    caps,
		})

		process.Stdin = Stdin(strconv.Itoa(i))
		stdout := Stdout(strconv.Itoa(i))

		stderr := Stderr(strconv.Itoa(i))
		go io.Copy(os.Stdout, stdout)
		go io.Copy(os.Stdout, stderr)
		process.Stdout = stdout
		process.Stderr = stderr

		if err := process.Start(); err != nil {
			return err
		}
		procStatus, err := process.Wait()
		if err != nil {
			return err
		}
		logrus.Infof("process %d returned with %d", i, procStatus)
	}

	if load {
		return nil
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
	exec   int
	load   bool
	reload bool
)

// "Hooks do optional work. Drivers do mandatory work"
func main() {
	flag.IntVar(&exec, "exec", 0, "run n number of execs")
	flag.BoolVar(&load, "load", false, "reload the container")
	flag.BoolVar(&reload, "reload", false, "reload the container live")
	flag.Parse()
	if err := osutils.SetSubreaper(1); err != nil {
		logrus.Fatal(err)
	}
	if reload {
		if err := reloadContainer(); err != nil {
			logrus.Fatal(err)
		}
		return
	}
	if err := runContainer(); err != nil {
		logrus.Fatal(err)
	}
}
