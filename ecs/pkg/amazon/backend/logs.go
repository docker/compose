package backend

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/docker/ecs-plugin/pkg/console"
)

func (b *Backend) ComposeLogs(ctx context.Context, projectName string) error {
	err := b.api.GetLogs(ctx, projectName, &logConsumer{
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

type logConsumer struct {
	colors map[string]console.ColorFunc
	width  int
}
