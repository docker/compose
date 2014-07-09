package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

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

func main() {
	configRaw, err := ioutil.ReadFile("fig.yml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening fig.yml file")
	}
	config := make(map[string]Service)
	err = yaml.Unmarshal(configRaw, &config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error unmarshalling fig.yml file")
	}
	// TODO: set protocol and address properly
	// (default to "unix" and "/var/run/docker.sock", otherwise use $DOCKER_HOST)
	cli := dockerClient.NewDockerCli(os.Stdin, os.Stdout, os.Stderr, "tcp", "localhost:2375", nil)
	fmt.Println(config)
	for name, service := range config {
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
