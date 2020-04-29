package context

import (
	gocontext "context"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
)

type currentContextKey struct{}

func WithCurrentContext(ctx gocontext.Context, configName string, contextName string) (context.Context, error) {
	config, err := LoadConfigFile(configName, "config.json")
	if err != nil {
		return ctx, err
	}

	currentContext := contextName
	if currentContext == "" {
		currentContext = config.CurrentContext
	}
	if currentContext == "" {
		currentContext = "default"
	}

	logrus.Debugf("Current context %q", currentContext)

	return context.WithValue(ctx, currentContextKey{}, currentContext), nil
}

// CurrentContext returns the current context name
func CurrentContext(ctx context.Context) string {
	cc, _ := ctx.Value(currentContextKey{}).(string)
	return cc
}
