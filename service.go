package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	apiClient "github.com/fsouza/go-dockerclient"
)

type Service struct {
	Name         string
	Image        string   `yaml:"image"`
	BuildDir     string   `yaml:"build"`
	Command      string   `yaml:"command"`
	Links        []string `yaml:"links"`
	Ports        []string `yaml:"ports"`
	Volumes      []string `yaml:"volumes"`
	ExposedPorts map[apiClient.Port]struct{}
	Container    apiClient.Container
	api          *apiClient.Client
}

/**
  This is weird looking but Docker API expects JSON such as :

	 "PortBindings": {
		"80/tcp": [
			{
				"HostIp": "0.0.0.0",
				"HostPort": "49153"
			}
		]
	 },

  	to define port bindings, so this function creates the data structure
	that gets marshalled into that JSON.
*/
func (s *Service) createPortBindings() map[apiClient.Port][]apiClient.PortBinding {
	bindingsToMarshal := make(map[apiClient.Port][]apiClient.PortBinding)
	for _, portBinding := range s.Ports {
		ports := strings.Split(portBinding, ":")
		val := []apiClient.PortBinding{}
		key := apiClient.Port(fmt.Sprintf("%s/tcp", ports[0]))
		if len(ports) > 1 {
			val = append(val, apiClient.PortBinding{
				HostIp:   "0.0.0.0",
				HostPort: ports[1],
			})
		}
		bindingsToMarshal[key] = val
	}
	return bindingsToMarshal
}

func (s *Service) configureExposedPorts() {
	s.ExposedPorts = make(map[apiClient.Port]struct{})
	for _, binding := range s.Ports {
		ports := strings.Split(binding, ":")
		if len(ports) > 1 {
			exposedPortKey := apiClient.Port(fmt.Sprintf("%s/tcp", ports[1]))
			s.ExposedPorts[exposedPortKey] = struct{}{}
		}
	}
}

func (s *Service) Create() error {
	s.configureExposedPorts()

	config := apiClient.Config{
		AttachStdout: true,
		AttachStdin:  false,
		Image:        s.Image,
		Cmd:          strings.Fields(s.Command),
		ExposedPorts: s.ExposedPorts,
	}
	createOpts := apiClient.CreateContainerOptions{
		Name:   s.Name,
		Config: &config,
	}
	container, err := s.api.CreateContainer(createOpts)
	if err != nil {
		if err == apiClient.ErrNoSuchImage {
			pullOpts := apiClient.PullImageOptions{
				Repository: s.Image,
			}
			fmt.Println("Unable to find image", s.Image, "locally, pulling...")
			err = s.api.PullImage(pullOpts, apiClient.AuthConfiguration{})
			if err != nil {
				return err
			}
			s.Create()
		}
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
	err := s.api.StartContainer(s.Container.ID, &apiClient.HostConfig{
		Links:        links,
		PortBindings: s.createPortBindings(),
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) Stop() error {
	err := s.api.StopContainer(s.Name, 10)
	if err != nil {
		fmt.Fprintf(os.Stderr, "attempt to stop container ", s.Name, "failed", err)
	}
	return nil
}

func (s *Service) Remove() error {
	err := s.api.RemoveContainer(apiClient.RemoveContainerOptions{
		ID: s.Name,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "attempt to remove container ", s.Name, "failed", err)
	}
	return nil
}

func (s *Service) IsRunning() bool {
	container, err := s.api.InspectContainer(s.Name)
	if err != nil {
		if _, ok := err.(*apiClient.NoSuchContainer); !ok {
			fmt.Fprintf(os.Stderr, "non-NoSuchContainer error checking if container is running: ", err)
		}
		return false
	}
	return container.State.Running
}

func (s *Service) Exists() bool {
	_, err := s.api.InspectContainer(s.Name)
	if err != nil {
		if _, ok := err.(*apiClient.NoSuchContainer); !ok {
			fmt.Fprintf(os.Stderr, "non-NoSuchContainer error checking if container is running: ", err)
		}
		return false
	}
	return true
}

func (s *Service) Wait(name string) (int, error) {
	exitCode, err := s.api.WaitContainer(name)
	return exitCode, err
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
	go s.api.AttachToContainer(options)
	return r, nil
}
