package cmd

import (
	"context"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/docker/api/context/store"
	containersv1 "github.com/docker/api/protos/containers/v1"
	contextsv1 "github.com/docker/api/protos/contexts/v1"
	"github.com/docker/api/server"
	"github.com/docker/api/server/proxy"

	"github.com/spf13/cobra"
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

	p := proxy.NewContainerAPI()

	containersv1.RegisterContainersServer(s, p)
	contextsv1.RegisterContextsServer(s, &cliServer{})

	go func() {
		<-ctx.Done()
		logrus.Info("stopping server")
		s.Stop()
	}()

	logrus.WithField("address", opts.address).Info("serving daemon API")

	// start the GRPC server to serve on the listener
	return s.Serve(listener)
}

type cliServer struct {
}

func (cs *cliServer) List(ctx context.Context, request *contextsv1.ListRequest) (*contextsv1.ListResponse, error) {
	s := store.ContextStore(ctx)
	contexts, err := s.List()
	if err != nil {
		logrus.Error(err)
		return &contextsv1.ListResponse{}, err
	}
	result := &contextsv1.ListResponse{}
	for _, c := range contexts {
		result.Contexts = append(result.Contexts, &contextsv1.Context{
			Name:        c.Name,
			ContextType: c.Type,
		})
	}
	return result, nil
}
