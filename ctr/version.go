package main

import (
	"fmt"

	"go.pedge.io/proto/version"

	"github.com/codegangsta/cli"
	"github.com/docker/containerd"
)

var VersionCommand = cli.Command{
	Name:  "version",
	Usage: "get the containerd version",
	Action: func(context *cli.Context) {
		serverVersion, err := protoversion.GetServerVersion(getClientConn(context))
		if err != nil {
			fatal(err.Error(), 1)
		}
		fmt.Printf("Client: %s\nServer: %s\n", containerd.Version.VersionString(), serverVersion.VersionString())
	},
}
