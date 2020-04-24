/*
	Copyright (c) 2020 Docker Inc.

	Permission is hereby granted, free of charge, to any person
	obtaining a copy of this software and associated documentation
	files (the "Software"), to deal in the Software without
	restriction, including without limitation the rights to use, copy,
	modify, merge, publish, distribute, sublicense, and/or sell copies
	of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be
	included in all copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
	EXPRESS OR IMPLIED,
	INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
	IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
	HOLDERS BE LIABLE FOR ANY CLAIM,
	DAMAGES OR OTHER LIABILITY,
	WHETHER IN AN ACTION OF CONTRACT,
	TORT OR OTHERWISE,
	ARISING FROM, OUT OF OR IN CONNECTION WITH
	THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package main

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

	v1 "github.com/docker/api/backend/v1"
	"github.com/docker/api/server"
	apiUtil "github.com/docker/api/util"
)

func main() {
	app := cli.NewApp()
	app.Name = "example"
	app.Usage = "example backend"
	app.Description = ""
	app.UseShortOptionHandling = true
	app.EnableBashCompletion = true
	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "enable debug output in the logs",
		},
		&cli.StringFlag{
			Name:  "address,a",
			Usage: "address of the server",
		},
	}
	app.Before = func(clix *cli.Context) error {
		if clix.Bool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
		}
		return nil
	}
	app.Action = func(clix *cli.Context) error {
		ctx, cancel := apiUtil.NewSigContext()
		defer cancel()

		// create a new GRPC server with the provided server package
		s := server.New()

		// listen on a socket to accept connects
		l, err := net.Listen("unix", clix.String("address"))
		if err != nil {
			return errors.Wrap(err, "listen unix socket")
		}
		defer l.Close()

		// create our instance of the backend server implementation
		backend := &backend{}

		// register our instance with the GRPC server
		v1.RegisterBackendServer(s, backend)

		// handle context being closed or canceled
		go func() {
			<-ctx.Done()
			logrus.Info("backend signaled to stop")

			s.Stop()
		}()

		logrus.WithField("address", clix.String("address")).Info("serving daemon API")
		// start the GRPC server to serve on the listener
		return s.Serve(l)
	}
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type backend struct {
}

func (b *backend) BackendInformation(ctx context.Context, _ *empty.Empty) (*v1.BackendInformationResponse, error) {
	return &v1.BackendInformationResponse{
		Id: "com.docker.api.backend.example.v1",
	}, nil
}
