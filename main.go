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
	Image    string   `yaml:"image"`
	BuildDir string   `yaml:"build"`
	Commands []string `yaml:"command"`
	Links    []string `yaml:"links"`
	Ports    []string `yaml:"ports"`
	Volumes  []string `yaml:"volumes"`
}

func CmdUp(c *gangstaCli.Context) {
	servicesRaw, err := ioutil.ReadFile("fig.yml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening fig.yml file")
	}
	services := make(map[string]Service)
	err = yaml.Unmarshal(servicesRaw, &services)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error unmarshalling fig.yml file")
	}
	// TODO: set protocol and address properly
	// (default to "unix" and "/var/run/docker.sock", otherwise use $DOCKER_HOST)
	cli := dockerClient.NewDockerCli(os.Stdin, os.Stdout, os.Stderr, "tcp", "localhost:2375", nil)
	for name, service := range services {
		if service.Image == "" {
			curdir, err := os.Getwd()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error getting name of current directory")
			}
			imageName := fmt.Sprintf("%s_%s", filepath.Base(curdir), name)
			service.Image = imageName
			cli.CmdBuild("-t", imageName, service.BuildDir)
		}
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
