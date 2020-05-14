package context

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/docker/api/context/store"
)

type createOpts struct {
	description       string
	aciLocation       string
	aciSubscriptionID string
	aciResourceGroup  string
}

func createCommand() *cobra.Command {
	var opts createOpts
	cmd := &cobra.Command{
		Use:   "create CONTEXT BACKEND [OPTIONS]",
		Short: "Create a context",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd.Context(), opts, args[0], args[1])
		},
	}

	cmd.Flags().StringVar(&opts.description, "description", "", "Description of the context")
	cmd.Flags().StringVar(&opts.aciLocation, "aci-location", "eastus", "Location")
	cmd.Flags().StringVar(&opts.aciSubscriptionID, "aci-subscription-id", "", "Location")
	cmd.Flags().StringVar(&opts.aciResourceGroup, "aci-resource-group", "", "Resource group")

	return cmd
}

func runCreate(ctx context.Context, opts createOpts, name string, contextType string) error {
	switch contextType {
	case "aci":
		return createACIContext(ctx, name, opts)
	default:
		s := store.ContextStore(ctx)
		return s.Create(name, store.TypedContext{
			Type:        contextType,
			Description: opts.description,
		})
	}
}
