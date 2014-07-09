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
	var err error

	err = cli.CmdRm("-f", s.Name)

	cmd := []string{}
	if len(s.Links) > 0 {
		for _, link := range s.Links {
			cmd = append(cmd, []string{"--link", fmt.Sprintf("%s:%s_1", link, link)}...)
		}
	}
	cmd = append(cmd, []string{"-d", "--name", s.Name, s.Image}...)
	if s.Command != "" {
		cmd = append(cmd, []string{"sh", "-c", s.Command}...)
	}

	err = cli.CmdRun(cmd...)
	if err != nil {
		return err
	}

	return nil
}

func runServices(services []Service) error {
	nRun := len(services)
	linkResolve := make(map[string]bool)

	/* Boot services in proper order */
	for {
		for _, service := range services {
			readyToRun := true
			for _, link := range service.Links {
				readyToRun = readyToRun && linkResolve[link]
			}
			if readyToRun {
				err := service.Run()
				if err != nil {
					return err
				}
				linkResolve[service.Name] = true
				nRun--
				if nRun == 0 {
					return nil
				}
			}
		}
	}

	return nil
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
