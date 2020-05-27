package amazon

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/docker/ecs-plugin/pkg/console"
)

func (c *client) ComposeLogs(ctx context.Context, projectName string) error {
	err := c.api.GetLogs(ctx, projectName, &logConsumer{
		colors: map[string]console.ColorFunc{},
		width:  0,
	})
	if err != nil {
		return err
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan
	return nil
}

type logConsumer struct {
	colors map[string]console.ColorFunc
	width  int
}

func (l *logConsumer) Log(service, container, message string) {
	cf, ok := l.colors[service]
	if !ok {
		cf = <-console.Rainbow
		l.colors[service] = cf
		l.computeWidth()
	}
	prefix := fmt.Sprintf("%-"+strconv.Itoa(l.width)+"s |", service)
	for _, line := range strings.Split(message, "\n") {
		fmt.Printf("%s %s\n", cf(prefix), line)
	}
}

func (l *logConsumer) computeWidth() {
	width := 0
	for n := range l.colors {
		if len(n) > width {
			width = len(n)
		}
	}
	l.width = width + 3
}

type LogConsumer interface {
	Log(service, container, message string)
}

type logsAPI interface {
	GetLogs(ctx context.Context, name string, consumer LogConsumer) error
}
