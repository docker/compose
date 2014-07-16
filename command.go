package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	gangstaCli "github.com/codegangsta/cli"
	dockerCli "github.com/dotcloud/docker/api/client"
	apiClient "github.com/fsouza/go-dockerclient"
	"github.com/howeyc/fsnotify"
	yaml "gopkg.in/yaml.v1"
)

func CmdUp(c *gangstaCli.Context) {
	var (
		wg               sync.WaitGroup
		buildDir         string
		imageName        string
		baseService      Service
		baseServiceIndex int
	)

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
	services := make(map[string]Service)
	err = yaml.Unmarshal(servicesRaw, &services)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error unmarshalling fig.yml file")
	}

	namedServices := []Service{}

	for name, service := range services {
		if service.Image == "" {
			curdir, err := os.Getwd()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error getting name of current directory")
			}
			imageName = fmt.Sprintf("%s_%s", filepath.Base(curdir), name)
			service.Image = imageName
			buildDir = service.BuildDir
			err = cli.CmdBuild("-t", imageName, buildDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error running build for image")
			}
			service.IsBase = true
		} else {
			service.IsBase = false
		}
		service.Name = name
		service.api = api
		namedServices = append(namedServices, service)
	}

	coloredServices := setColoredPrefixes(namedServices)

	err = runServices(coloredServices)
	if err != nil {
		fmt.Fprintf(os.Stderr, "There was a problem with the run: ", err)
	}
	err = attachServices(coloredServices)
	if err != nil {
		fmt.Fprintf(os.Stderr, "There was an error with attaching to the services", err)
	}

	err = waitServices(coloredServices, &wg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "there was an error in wait services call", err)
	}

	if c.Bool("watch") {
		for index, service := range coloredServices {
			if service.IsBase {
				baseServiceIndex = index
			}
		}
		baseService = coloredServices[baseServiceIndex]
		fmt.Println("printing baseService", baseService)
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating fs watcher", err)
		}

		lastEvent := time.Now()

		go func() {
			for {
				select {
				case ev := <-watcher.Event:
					if time.Since(lastEvent) > (time.Millisecond * 100) {
						if ev.IsModify() {
							fmt.Println("event detected in fsnotify", ev)
							err = cli.CmdBuild("-t", imageName, buildDir)
							if err != nil {
								fmt.Fprintf(os.Stderr, "error running build for image")
							}
							wg.Add(1)
							err = baseService.Stop()
							if err != nil {
								fmt.Fprintf(os.Stderr, "error attempting container stop", err)
							}
							err = baseService.Remove()
							if err != nil {
								fmt.Fprintf(os.Stderr, "error attempting container remove", err)
							}
							err = baseService.Create()
							if err != nil {
								fmt.Fprintf(os.Stderr, "error attempting container create", err)
							}
							err = baseService.Start()
							if err != nil {
								fmt.Fprintf(os.Stderr, "error attempting container start", err)
							}
							err = baseService.Attach()
							if err != nil {
								fmt.Fprintf(os.Stderr, "error attaching coloredServices[0]", err)
							}
							go baseService.Wait(&wg)
						}
					}
					lastEvent = time.Now()
				default:
					//case err := <-watcher.Event:
					//fmt.Fprintf(os.Stderr, "error detected in fsnotify", err, "\n")
				}
			}
		}()

		err = watcher.Watch(buildDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error watching directory", buildDir, err)
		}
	}

	wg.Wait()
}
