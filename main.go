package main

import (
	"bufio"
	"fmt"
	"io"
	"os"

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
				fmt.Println("Creating service", service.Name)
				err := service.Create()
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error creating service", err)
					os.Exit(1)
				}
				err = service.Start()
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error starting service", err)
					os.Exit(1)
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
			exitCode, err := service.Wait(service.Name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "container wait had error", err)
			}
			exited <- exitCode
		}(service)
	}
	<-exited
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
			Name:   "up",
			Usage:  "Initialize a pod of containers based on a fig.yml file",
			Action: CmdUp,
		},
	}

	app.Run(os.Args)
}
