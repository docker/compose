package context

import (
	"context"

	"github.com/docker/api/context/store"
)

func createACIContext(ctx context.Context, name string, opts createOpts) error {
	s := store.ContextStore(ctx)
	return s.Create(name, store.TypedContext{
		Type:        "aci",
		Description: opts.description,
		Data: store.AciContext{
			SubscriptionID: opts.aciSubscriptionID,
			Location:       opts.aciLocation,
			ResourceGroup:  opts.aciResourceGroup,
		},
	})
}
