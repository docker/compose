package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	gangstaCli "github.com/codegangsta/cli"
	dockerCli "github.com/dotcloud/docker/api/client"
	apiClient "github.com/fsouza/go-dockerclient"
	yaml "gopkg.in/yaml.v1"
)

type Service struct {
	Name      string
	Image     string   `yaml:"image"`
	BuildDir  string   `yaml:"build"`
	Command   string   `yaml:"command"`
	Links     []string `yaml:"links"`
	Ports     []string `yaml:"ports"`
	Volumes   []string `yaml:"volumes"`
	Container apiClient.Container
}

var api *apiClient.Client

func (s *Service) Create() error {

	config := apiClient.Config{
		AttachStdout: true,
		AttachStdin:  false,
		Image:        s.Image,
		Cmd:          strings.Fields(s.Command),
	}
	opts := apiClient.CreateContainerOptions{Name: s.Name, Config: &config}
	container, err := api.CreateContainer(opts)
	if err != nil {
		return err
	}
	s.Container = *container
	return nil
}

func (s *Service) Start() error {
	links := []string{}
	// TODO: this should work like pyfig
	for _, link := range s.Links {
		links = append(links, fmt.Sprintf("%s:%s_1", link, link))
	}
	err := api.StartContainer(s.Container.ID, &apiClient.HostConfig{
		Links: links,
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) Stop() error {
	err := api.StopContainer(s.Name, 10)
	if err != nil {
		fmt.Fprintf(os.Stderr, "attempt to stop container ", s.Name, "failed", err)
	}
	return nil
}

func (s *Service) Remove() error {
	err := api.RemoveContainer(apiClient.RemoveContainerOptions{
		ID: s.Name,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "attempt to remove container ", s.Name, "failed", err)
	}
	return nil
}

func (s *Service) IsRunning() bool {
	container, err := api.InspectContainer(s.Name)
	if err != nil {
		if _, ok := err.(apiClient.NoSuchContainer); ok {
			fmt.Fprintf(os.Stderr, "unknown error checking if container is running: ", err)
		}
		return false
	}
	return container.State.Running
}

func (s *Service) Exists() bool {
	_, err := api.InspectContainer(s.Name)
	if err != nil {
		if _, ok := err.(apiClient.NoSuchContainer); ok {
			fmt.Fprintf(os.Stderr, "unknown error checking if container is running: ", err)
		}
		return false
	}
	return true
}

func (s *Service) Attach() (io.Reader, error) {
	r, w := io.Pipe()
	options := apiClient.AttachToContainerOptions{
		Container:    s.Name,
		OutputStream: w,
		ErrorStream:  w,
		Stream:       true,
		Stdout:       true,
		Stderr:       true,
		Logs:         true,
	}
	go api.AttachToContainer(options)
	return r, nil
}

func runServices(services []Service) error {
	started := make(map[string]bool)
	stopped := make(map[string]bool)
	nToStart := len(services)

	for {
		/* Boot services in proper order */
		for _, service := range services {
			shouldStart := true
			if !stopped[service.Name] {
				if service.IsRunning() {
					service.Stop()
				}
				if service.Exists() {
					service.Remove()
				}
				stopped[service.Name] = true
			}
			for _, link := range service.Links {
				if !started[link] {
					shouldStart = false
				}
			}
			if shouldStart {
				err := service.Create()
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error creating service", err)
				}
				err = service.Start()
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error starting service", err)
				}
				started[service.Name] = true
				nToStart--
				if nToStart == 0 {
					return nil
				}
			}
		}
	}

	return nil
}

func attachServices(services []Service) error {

	prefixLength := maxPrefixLength(services)

	// Format string for later logging.
	// This has been an Aanand and Nathan creation.
	// * drops mic *
	prefixFmt := fmt.Sprintf("%%-%ds | ", prefixLength)
	for _, service := range services {

		uncoloredPrefix := fmt.Sprintf(prefixFmt, service.Name)
		coloredPrefix := rainbow(uncoloredPrefix)
		reader, err := service.Attach()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error attaching to container", err)
		}
		go func(reader io.Reader, name string) {
			scanner := bufio.NewScanner(reader)
			for scanner.Scan() {
				fmt.Printf("%s%s \n", coloredPrefix, scanner.Text())
			}
			if err := scanner.Err(); err != nil {
				fmt.Fprintf(os.Stderr, "There was an error with the scanner in attached container", err)
			}
		}(reader, service.Name)
	}
	return nil
}

func waitServices(services []Service) error {
	exited := make(chan int)
	for _, service := range services {
		go func(service Service) {
			exitCode, err := api.WaitContainer(service.Name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "container wait had error", err)
			}
			exited <- exitCode
		}(service)
	}
	<-exited
	return nil
}

func CmdUp(c *gangstaCli.Context) {
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

func maxPrefixLength(services []Service) int {
	maxLength := 0
	for _, service := range services {
		if len(service.Name) > maxLength {
			maxLength = len(service.Name)
		}
	}
	return maxLength
}

func main() {
	var err error
	api, err = apiClient.NewClient("tcp://boot2docker:2375")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating api client!")
		os.Exit(1)
	}

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
