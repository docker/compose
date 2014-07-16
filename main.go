package main

import (
	"fmt"
	"os"
	"sync"

	gangstaCli "github.com/codegangsta/cli"
)

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

func setColoredPrefixes(services []Service) []Service {
	servicesWithColoredPrefixes := []Service{}

	prefixLength := maxPrefixLength(services)

	// Format string for later logging.
	// This has been an Aanand and Nathan creation.
	// * drops mic *
	prefixFmt := fmt.Sprintf("%%-%ds | ", prefixLength)

	for _, service := range services {
		uncoloredPrefix := fmt.Sprintf(prefixFmt, service.Name)
		coloredPrefix := rainbow(uncoloredPrefix)
		service.LogPrefix = coloredPrefix
		servicesWithColoredPrefixes = append(servicesWithColoredPrefixes, service)
	}

	return servicesWithColoredPrefixes
}

func attachServices(services []Service) error {

	for _, service := range services {
		err := service.Attach()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error attaching to container", err)
		}
	}
	return nil
}

func waitServices(services []Service, wg *sync.WaitGroup) error {
	// Add one counter to the waitgroup for the group of services.
	// If even one of the services exits (without a restart), we should exit the
	// root process.
	wg.Add(1)
	for _, service := range services {
		go service.Wait(wg)
	}
	return nil
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
	app := gangstaCli.NewApp()
	app.Name = "fig"
	app.Usage = "Orchestrate Docker containers"
	app.Commands = []gangstaCli.Command{
		{
			Name: "up",
			Flags: []gangstaCli.Flag{
				gangstaCli.BoolFlag{Name: "watch", Usage: "Watch build directory for changes and auto-rebuild/restart"},
			},
			Usage:  "Initialize a pod of containers based on a fig.yml file",
			Action: CmdUp,
		},
	}

	app.Run(os.Args)
}
