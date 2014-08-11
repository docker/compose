package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	gangstaCli "github.com/codegangsta/cli"
	dockerCli "github.com/docker/docker/api/client"
	"github.com/docker/fig/service"
	apiClient "github.com/fsouza/go-dockerclient"
	"github.com/howeyc/fsnotify"
	yaml "gopkg.in/yaml.v1"
)

type RebootStep struct {
	stepName string
	method   func() error
}

var (
	dockerIgnorePath string
	ignoredFiles     = make(map[string]bool)
)

func ignoreFile(filename string) bool {
	if val, ok := ignoredFiles[filename]; ok && filename != ".dockerignore" {
		return val
	} else {
		// "Cache" invalidated or first time, (re)calculate
		dockerignore, err := ioutil.ReadFile(dockerIgnorePath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error reading .dockerignore file", err)
		}
		buffer := bytes.NewBuffer(dockerignore)
		scanner := bufio.NewScanner(buffer)
		for scanner.Scan() {
			matched, err := path.Match(scanner.Text(), filename)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error matching .dockerignore filename", err)
			}
			if matched {
				ignoredFiles[filename] = true
				return true
			}
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintln(os.Stderr, "error scanning .dockerignore file", err)
		}
	}
	ignoredFiles[filename] = false
	return false
}

func CmdBuild(c *gangstaCli.Context) {
}

func CmdKill(c *gangstaCli.Context) {
}

func CmdLogs(c *gangstaCli.Context) {
}

func CmdPs(c *gangstaCli.Context) {
}

func CmdRm(c *gangstaCli.Context) {
}

func CmdRun(c *gangstaCli.Context) {
}

func CmdScale(c *gangstaCli.Context) {
}

func CmdStart(c *gangstaCli.Context) {
}

func CmdStop(c *gangstaCli.Context) {
}

func CmdUp(c *gangstaCli.Context) {
	var (
		wg               sync.WaitGroup
		buildDir         string
		imageName        string
		baseService      service.Service
		baseServiceIndex int
	)

	dockerHost := os.Getenv("DOCKER_HOST")
	if dockerHost == "" {
		dockerHost = "unix:///var/run/docker.sock"
	}

	api, err := apiClient.NewClient(dockerHost)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error creating client!!", err)
	}

	splitDockerHost := strings.Split(dockerHost, "://")
	protocol := splitDockerHost[0]
	location := splitDockerHost[len(splitDockerHost)-1]

	cli := dockerCli.NewDockerCli(os.Stdin, os.Stdout, os.Stderr, protocol, location, nil)
	servicesRaw, err := ioutil.ReadFile("fig.yml")

	if err != nil {
		fmt.Fprintln(os.Stderr, "Error opening fig.yml file")
	}
	services := make(map[string]service.Service)
	err = yaml.Unmarshal(servicesRaw, &services)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error unmarshalling fig.yml file")
	}

	namedServices := []service.Service{}

	for name, service := range services {
		if service.Image == "" {
			curdir, err := os.Getwd()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error getting name of current directory")
			}
			imageName = fmt.Sprintf("%s_%s", filepath.Base(curdir), name)
			service.Image = imageName
			buildDir = service.BuildDir
			dockerIgnorePath = buildDir + "/.dockerignore"
			err = cli.CmdBuild("-t", imageName, buildDir)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error running build for image", err)
				os.Exit(1)
			}
			service.IsBase = true
		} else {
			service.IsBase = false
		}
		service.Name = name
		service.Api = api
		namedServices = append(namedServices, service)
	}

	coloredServices := setColoredPrefixes(namedServices)

	err = runServices(coloredServices)
	if err != nil {
		fmt.Fprintln(os.Stderr, "There was a problem with the run: ", err)
	}
	err = attachServices(coloredServices)
	if err != nil {
		fmt.Fprintln(os.Stderr, "There was an error with attaching to the services", err)
	}

	err = waitServices(coloredServices, &wg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "there was an error in wait services call", err)
	}

	if c.Bool("watch") {
		for index, service := range coloredServices {
			if service.IsBase {
				baseServiceIndex = index
			}
		}
		baseService = coloredServices[baseServiceIndex]
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error creating fs watcher", err)
		}

		timer := &time.Timer{}

		sequence := []RebootStep{
			{"build", func() error {
				err := cli.CmdBuild("-t", imageName, buildDir)
				return err
			}},
			{"stop", baseService.Stop},
			{"remove", baseService.Remove},
			{"create", baseService.Create},
			{"start", baseService.Start},
			{"attach", baseService.Attach},
		}

		go func() {
			for {
				select {
				case ev := <-watcher.Event:
					fmt.Println("got event", ev)
					times := []TimedEvent{}
					lastStep := time.Now()
					rebuildStartTime := time.Now()
					if !ignoreFile(ev.Name) {
						if ev.IsModify() && ev.IsAttrib() {
							timer.Stop()
							timer = time.AfterFunc(100*time.Millisecond, func() {
								wg.Add(1)
								for _, rebootStep := range sequence {
									lastStep = time.Now()
									err = rebootStep.method()
									if err != nil {
										fmt.Fprintln(os.Stderr, "error attempting container", rebootStep.stepName, err)
									}
									times = append(times, TimedEvent{rebootStep.stepName, time.Since(lastStep)})
								}
								timedEventSorter := timedEventSorter{
									events: times,
									by: func(te1, te2 *TimedEvent) bool {
										return te1.duration > te2.duration
									},
								}
								sort.Sort(&timedEventSorter)
								fmt.Println("-> Rebuild time took", time.Since(rebuildStartTime).Seconds(), "seconds total")
								for _, timedEvent := range times {
									fmt.Printf("\t-> %-6s %s %f seconds\n", timedEvent.eventName, "took", timedEvent.duration.Seconds())
								}
								go baseService.Wait(&wg)
								<-watcher.Event
							})
						}
					}
				case err = <-watcher.Error:
					fmt.Fprintln(os.Stderr, err)
				default:
				}
			}
		}()

		err = watcher.Watch(buildDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error watching directory", buildDir, err)
		}
	}

	wg.Wait()
}
