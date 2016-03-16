// +build !linux

package main

import "github.com/codegangsta/cli"

var (
	checkpointCommand cli.Command
	eventsCommand     cli.Command
	restoreCommand    cli.Command
	specCommand       cli.Command
	killCommand       cli.Command
)

func runAction(*cli.Context) {
	fatalf("Current OS is not supported yet")
}
