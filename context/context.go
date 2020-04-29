package context

import (
	gocontext "context"

	"golang.org/x/net/context"
)

const KEY = "context_key"

type currentContextKey struct{}

func WithCurrentContext(ctx gocontext.Context, contextName string) context.Context {
	return context.WithValue(ctx, currentContextKey{}, contextName)
}

// CurrentContext returns the current context name
func CurrentContext(ctx context.Context) string {
	cc, _ := ctx.Value(currentContextKey{}).(string)
	return cc
}
