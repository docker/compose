package main

import (
	"fmt"
	"os"
	"sync"

	gangstaCli "github.com/codegangsta/cli"
	"github.com/docker/fig/service"
)

func runService(srv *service.Service) error {
	fmt.Println("Creating srv", srv.Name)
	err := srv.Create()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating service", err)
		os.Exit(1)
	}
	err = srv.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting service", err)
		os.Exit(1)
	}
	return nil
}

func runServices(services []service.Service) error {
	var err error
	started := make(map[string]bool)
	stopped := make(map[string]bool)
	nToStart := len(services)

	for {
		/* Boot services in proper order */
		for _, srv := range services {
			shouldStart := true
			if !stopped[srv.Name] {
				err = srv.Stop()
				if err != nil {
					return err
				}
				err = srv.Remove()
				if err != nil {
					return err
				}
				stopped[srv.Name] = true
			}
			fmt.Println(srv.Links)
			for _, link := range srv.Links {
				if !started[link] {
					shouldStart = false
				}
			}
			if shouldStart {
				err := runService(&srv)
				if err != nil {
					return err
				}
				started[srv.Name] = true
				nToStart--
				if nToStart == 0 {
					return nil
				}
			}
		}
	}

	return nil
}

func setColoredPrefixes(services []service.Service) []service.Service {
	servicesWithColoredPrefixes := []service.Service{}

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

func attachServices(services []service.Service) error {

	for _, service := range services {
		err := service.Attach()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error attaching to container", err)
		}
	}
	return nil
}

func waitServices(services []service.Service, wg *sync.WaitGroup) error {
	// Add one counter to the waitgroup for the group of services.
	// If even one of the services exits (without a restart), we should exit the
	// root process.
	wg.Add(len(services))
	for _, service := range services {
		go service.Wait(wg)
	}
	return nil
}

func maxPrefixLength(services []service.Service) int {
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
	app.Usage = "Punctual, lightweight development environments using Docker."
	app.Flags = []gangstaCli.Flag{
		gangstaCli.BoolFlag{
			Name:  "verbose",
			Usage: "Show more output",
		},
		gangstaCli.StringFlag{
			Name:  "f, file",
			Usage: "Specify an alternate fig file (default: fig.yml)",
		},
		gangstaCli.StringFlag{
			Name:  "p, project-name",
			Usage: "Specify an alternate project name (default: directory name)",
		},
	}
	app.Commands = []gangstaCli.Command{
		{
			Name: "build",
			Flags: []gangstaCli.Flag{
				gangstaCli.BoolFlag{
					Name:  "no-cache",
					Usage: "Do not use cache when building the image.",
				},
			},
			Usage:  "Build or rebuild services",
			Action: CmdBuild,
		},
		{
			Name:   "kill",
			Flags:  []gangstaCli.Flag{},
			Usage:  "Kill containers",
			Action: CmdKill,
		},
		{
			Name:   "logs",
			Flags:  []gangstaCli.Flag{},
			Usage:  "View output from containers",
			Action: CmdLogs,
		},
		{
			Name: "ps",
			Flags: []gangstaCli.Flag{
				gangstaCli.BoolFlag{
					Name:  "q",
					Usage: "Only display IDs",
				},
			},
			Usage:  "List containers",
			Action: CmdPs,
		},
		{
			Name: "rm",
			Flags: []gangstaCli.Flag{
				gangstaCli.BoolFlag{
					Name:  "force",
					Usage: "Don't ask to confirm removal",
				},
				gangstaCli.BoolFlag{
					Name:  "v",
					Usage: "Remove volumes associated with containers",
				},
			},
			Usage:  "Remove stopped containers",
			Action: CmdRm,
		},
		{
			Name: "run",
			Flags: []gangstaCli.Flag{
				gangstaCli.BoolFlag{
					Name:  "d",
					Usage: "Detached mode: Run container in the background, print new container name.",
				},
				gangstaCli.BoolFlag{
					Name:  "T",
					Usage: "Disables psuedo-tty allocation. By default `fig run` allocates a TTY.",
				},
				gangstaCli.BoolFlag{
					Name:  "rm",
					Usage: "Remove container after run.  Ignored in detached mode.",
				},
				gangstaCli.BoolFlag{
					Name:  "no-deps",
					Usage: "Don't start linked services.",
				},
			},
			Usage:  "Run a one-off command",
			Action: CmdRm,
		},
		{
			Name:   "scale",
			Flags:  []gangstaCli.Flag{},
			Usage:  "Set number of containers for a service",
			Action: CmdRm,
		},
		{
			Name:   "start",
			Flags:  []gangstaCli.Flag{},
			Usage:  "Start services",
			Action: CmdRm,
		},
		{
			Name:   "stop",
			Flags:  []gangstaCli.Flag{},
			Usage:  "Stop services",
			Action: CmdStop,
		},
		{
			Name: "up",
			Flags: []gangstaCli.Flag{
				gangstaCli.BoolFlag{
					Name:  "watch",
					Usage: "Watch build directory for changes and auto-rebuild/restart",
				},
				gangstaCli.BoolFlag{
					Name:  "d",
					Usage: "Detached mode: Run containers in the background, print new container names.",
				},
				gangstaCli.BoolFlag{
					Name:  "no-clean",
					Usage: "Don't remove containers after signal interrupt (CTRL+C)",
				},
				gangstaCli.BoolFlag{
					Name:  "no-deps",
					Usage: "Don't start linked services.",
				},
				gangstaCli.BoolFlag{
					Name:  "no-recreate",
					Usage: "If containers already exist, don't recreate them.",
				},
			},
			Usage:  "Create and start containers",
			Action: CmdUp,
		},
	}

	app.Run(os.Args)
}
