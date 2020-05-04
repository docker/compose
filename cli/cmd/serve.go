package cmd

import (
	"context"
	"net"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	cliv1 "github.com/docker/api/cli/v1"
	containersv1 "github.com/docker/api/containers/v1"
	"github.com/docker/api/context/store"
	"github.com/docker/api/server"
	"github.com/docker/api/server/proxy"

	"github.com/spf13/cobra"
)

type serveOpts struct {
	address string
}

func ServeCommand() *cobra.Command {
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
	s := server.New()

	listener, err := net.Listen("unix", opts.address)
	if err != nil {
		return errors.Wrap(err, "listen unix socket")
	}
	defer listener.Close()

	p := proxy.NewContainerApi()

	containersv1.RegisterContainersServer(s, p)
	cliv1.RegisterCliServer(s, &cliServer{
		ctx,
	})

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
	ctx context.Context
}

func (cs *cliServer) Contexts(ctx context.Context, request *cliv1.ContextsRequest) (*cliv1.ContextsResponse, error) {
	s, err := store.New()
	if err != nil {
		logrus.Error(err)
		return &cliv1.ContextsResponse{}, err
	}
	contexts, err := s.List()
	if err != nil {
		logrus.Error(err)
		return &cliv1.ContextsResponse{}, err
	}
	result := &cliv1.ContextsResponse{}
	for _, c := range contexts {
		result.Contexts = append(result.Contexts, &cliv1.Context{
			Name:        c.Name,
			ContextType: c.Metadata.Type,
		})
	}
	return result, nil
}
