package main

import (
	"fmt"
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
}

// TODO: set protocol and address properly
// (default to "unix" and "/var/run/docker.sock", otherwise use $DOCKER_HOST)
var cli = dockerClient.NewDockerCli(os.Stdin, os.Stdout, os.Stderr, "tcp", "boot2docker:2375", nil)

func (s *Service) Run() error {
	fmt.Println("running service ", s)

	err := cli.CmdRun("-d", "--name", s.Name, s.Image, s.Command)
	if err != nil {
		return err
	}

	return nil
}

func startServices(services []Service) {
	fmt.Println(services)

	for _, service := range services {
		fmt.Println(service)
		err := service.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "something went wrong in the run", err)
		}
	}
}

func CmdUp(c *gangstaCli.Context) {
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
		fmt.Println(name, service)
		namedServices = append(namedServices, service)
	}

	startServices(namedServices)
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
