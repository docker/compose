package main

import (
	"os"

	"github.com/codegangsta/cli"
)

var defaultStateDir = os.Getenv("PROGRAMDATA") + `\docker\containerd`

const (
	defaultListenType   = "tcp"
	defaultGRPCEndpoint = "localhost:2377"
)

func appendPlatformFlags() {
}

// TODO Windows: May be able to factor out entirely
func checkLimits() error {
	return nil
}

// No idea how to implement this on Windows.
func reapProcesses() {
}

func setAppBefore(app *cli.App) {
}
