package mobycli

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/docker/api/context/store"
)

func TestDelegateContextTypeToMoby(t *testing.T) {

	isDelegated := func(val string) bool {
		for _, ctx := range delegatedContextTypes {
			if ctx == val {
				return true
			}
		}
		return false
	}

	allCtx := []string{store.AciContextType, store.EcsContextType, store.AwsContextType, store.DefaultContextType}
	for _, ctx := range allCtx {
		if isDelegated(ctx) {
			assert.Assert(t, mustDelegateToMoby(ctx))
			continue
		}
		assert.Assert(t, !mustDelegateToMoby(ctx))
	}
}
