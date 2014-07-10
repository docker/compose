package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	gangstaCli "github.com/codegangsta/cli"
	dockerClient "github.com/dotcloud/docker/api/client"
	yaml "gopkg.in/yaml.v1"
)

type Service struct {
	Name     string
	Image    string   `yaml:"image"`
	BuildDir string   `yaml:"build"`
	Command  string   `yaml:"command"`
	Links    []string `yaml:"links"`
	Ports    []string `yaml:"ports"`
	Volumes  []string `yaml:"volumes"`
	Running  bool
	Logger   ServiceLogger
}

type ServiceLogger struct {
	ServiceName string
	Stdout      io.Writer
	Stderr      io.Writer
	Stdin       io.ReadCloser
}

type StdWriter struct {
	ServiceName string
}

func (s StdWriter) Write(p []byte) (int, error) {
	fmt.Println(s.ServiceName)
	fmt.Println(string(p[:]))
	return len(p) + len(s.ServiceName), nil
}

func NewStdWriter(name string) StdWriter {
	return StdWriter{ServiceName: name}
}

func NewServiceLogger(name string) ServiceLogger {
	serviceLogger := ServiceLogger{}
	serviceLogger.ServiceName = name
	stdOut := StdWriter{}
	serviceLogger.Stdout = stdOut
	return serviceLogger
}

func isRunning(serviceName string) bool {
	var (
		err    error
		val    []byte
		reader io.Reader
		writer io.Writer
	)
	cli := dockerClient.NewDockerCli(os.Stdin, writer, os.Stderr, "tcp", "localhost:2375", nil)
	err = cli.CmdInspect([]string{"-f", "'{{ .State.Running }}'", serviceName}...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "something went wrong inspecting in isRunning", err)
		return false
	} else {
		_, err = io.Copy(writer, reader)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error Copy from reader in isRunning")
		}
		val, err = ioutil.ReadAll(reader)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error ReadAll from reader in isRunning")
		}
		return (string(val[:]) == "true")
	}
}

func (s Service) Run(errmsg chan error) error {
	serviceLogger := NewServiceLogger(s.Name)
	cli := dockerClient.NewDockerCli(os.Stdin, serviceLogger.Stdout, os.Stderr, "tcp", "localhost:2375", nil)
	/*
		err := cli.CmdRm("-f", s.Name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error with removing container")
			errmsg <- err
		}
	*/

	cmd := []string{}
	if len(s.Links) > 0 {
		for _, link := range s.Links {
			cmd = append(cmd, []string{"--link", fmt.Sprintf("%s:%s_1", link, link)}...)
		}
	}
	cmd = append(cmd, []string{"--name", s.Name, s.Image}...)
	if s.Command != "" {
		cmd = append(cmd, []string{"sh", "-c", s.Command}...)
	}

	fmt.Println(cmd)
	fmt.Println("running cmd", cmd)

	go func() {
		err := cli.CmdRun(cmd...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error in CmdRun of the container")
			errmsg <- err
		}
	}()

	return nil
}

func runServices(services []Service) error {
	errmsg := make(chan error)

	/* Boot services in proper order */
	for {
		select {
		case err := <-errmsg:
			return err
		default:
			for _, service := range services {
				readyToRun := true
				for _, link := range service.Links {
					readyToRun = readyToRun && isRunning(link)
				}
				fmt.Println("readyToRun", readyToRun)
				if readyToRun && !isRunning(service.Name) {
					go func(service Service) {
						fmt.Println("running ", service)
						service.Run(errmsg)
					}(service)
				}
			}
		}
	}

	return nil
}

func CmdUp(c *gangstaCli.Context) {
	// TODO: set protocol and address properly
	// (default to "unix" and "/var/run/docker.sock", otherwise use $DOCKER_HOST)
	cli := dockerClient.NewDockerCli(os.Stdin, os.Stdout, os.Stderr, "tcp", "boot2docker:2375", nil)
	servicesRaw, err := ioutil.ReadFile("fig.yml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening fig.yml file")
	}
	namedServices := []Service{}
	services := make(map[string]Service)
	err = yaml.Unmarshal(servicesRaw, &services)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error unmarshalling fig.yml file")
	}
	for name, service := range services {
		if service.Image == "" {
			curdir, err := os.Getwd()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error getting name of current directory")
			}
			imageName := fmt.Sprintf("%s_%s", filepath.Base(curdir), name)
			service.Image = imageName
			err = cli.CmdBuild("-t", imageName, service.BuildDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error running build for image")
			}
		}
		service.Name = name
		namedServices = append(namedServices, service)
	}

	err = runServices(namedServices)
	if err != nil {
		fmt.Fprintf(os.Stderr, "There was a problem with the run: ", err)
	}
}

func main() {
	app := gangstaCli.NewApp()
	app.Name = "fig"
	app.Usage = "Orchestrate Docker containers"
	app.Commands = []gangstaCli.Command{
		{
			Name:   "up",
			Usage:  "Initialize a pod of containers based on a fig.yml file",
			Action: CmdUp,
		},
	}

	app.Run(os.Args)

}
