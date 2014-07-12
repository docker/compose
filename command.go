package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	gangstaCli "github.com/codegangsta/cli"
	dockerCli "github.com/dotcloud/docker/api/client"
	apiClient "github.com/fsouza/go-dockerclient"
	yaml "gopkg.in/yaml.v1"
)

func CmdUp(c *gangstaCli.Context) {

	dockerHost := os.Getenv("DOCKER_HOST")
	if dockerHost == "" {
		dockerHost = "unix:///var/run/docker.sock"
	}

	api, err := apiClient.NewClient(dockerHost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating client!!", err)
	}

	splitDockerHost := strings.Split(dockerHost, "://")
	protocol := splitDockerHost[0]
	location := splitDockerHost[len(splitDockerHost)-1]

	cli := dockerCli.NewDockerCli(os.Stdin, os.Stdout, os.Stderr, protocol, location, nil)
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
				fmt.Fprintf(os.Stderr, "Error getting name of current directory\n")
			}
			imageName := fmt.Sprintf("%s_%s", filepath.Base(curdir), name)
			service.Image = imageName
			fmt.Println("Building service", imageName)
			err = cli.CmdBuild("-t", imageName, service.BuildDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error running build for image\n", err)
				os.Exit(1)
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
