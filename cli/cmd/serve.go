/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package cmd

import (
	"context"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/cli/server"
	containersv1 "github.com/docker/compose-cli/cli/server/protos/containers/v1"
	contextsv1 "github.com/docker/compose-cli/cli/server/protos/contexts/v1"
	streamsv1 "github.com/docker/compose-cli/cli/server/protos/streams/v1"
	volumesv1 "github.com/docker/compose-cli/cli/server/protos/volumes/v1"
	"github.com/docker/compose-cli/cli/server/proxy"
)

type serveOpts struct {
	address string
}

// ServeCommand returns the command to serve the API
func ServeCommand() *cobra.Command {
	// FIXME(chris-crone): Should warn that specified context is ignored
	var opts serveOpts
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start an api server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.address, "address", "", "The address to listen to")

	return cmd
}

func runServe(ctx context.Context, opts serveOpts) error {
	s := server.New(ctx)

	listener, err := server.CreateListener(opts.address)
	if err != nil {
		return errors.Wrap(err, "listen address "+opts.address)
	}
	// nolint errcheck
	defer listener.Close()

	p := proxy.New(ctx)

	containersv1.RegisterContainersServer(s, p)
	contextsv1.RegisterContextsServer(s, p.ContextsProxy())
	streamsv1.RegisterStreamingServer(s, p)
	volumesv1.RegisterVolumesServer(s, p)

	go func() {
		<-ctx.Done()
		logrus.Info("stopping server")
		s.Stop()
	}()

	logrus.WithField("address", opts.address).Info("serving daemon API")

	// start the GRPC server to serve on the listener
	return s.Serve(listener)
}
