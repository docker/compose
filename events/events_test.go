package events

import (
	"context"
	"fmt"
	"testing"
)

func TestBasicEvent(t *testing.T) {
	ctx := context.Background()

	// simulate a layer pull with events
	ctx, commit, _ := WithTx(ctx)

	G(ctx).Post(ctx, "pull ubuntu")

	for layer := 0; layer < 4; layer++ {
		// make a subtransaction for each layer
		ctx, commit, _ := WithTx(ctx)

		G(ctx).Post(ctx, fmt.Sprintf("fetch layer %v", layer))

		ctx = WithTopic(ctx, "content")
		// simulate sub-operations with a separate topic, on the content store
		G(ctx).Post(ctx, fmt.Sprintf("received sha:256"))

		G(ctx).Post(ctx, fmt.Sprintf("unpack layer %v", layer))

		commit()
	}

	commit()
}
