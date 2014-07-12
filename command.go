package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	gangstaCli "github.com/codegangsta/cli"
	dockerCli "github.com/dotcloud/docker/api/client"
	apiClient "github.com/fsouza/go-dockerclient"
	yaml "gopkg.in/yaml.v1"
)

func CmdUp(c *gangstaCli.Context) {
	api, err := apiClient.NewClient("tcp://boot2docker:2375")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating client!!", err)
	}

	// TODO: set protocol and address properly
	// (default to "unix" and "/var/run/docker.sock", otherwise use $DOCKER_HOST)
	cli := dockerCli.NewDockerCli(os.Stdin, os.Stdout, os.Stderr, "tcp", "boot2docker:2375", nil)
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
		service.api = api
		namedServices = append(namedServices, service)
	}

	err = runServices(namedServices)
	if err != nil {
		fmt.Fprintf(os.Stderr, "There was a problem with the run: ", err)
	}
	err = attachServices(namedServices)
	if err != nil {
		fmt.Fprintf(os.Stderr, "There was an error with attaching to the services", err)
	}
	err = waitServices(namedServices)
	if err != nil {
		fmt.Fprintf(os.Stderr, "there was an error in wait services call", err)
	}
}
