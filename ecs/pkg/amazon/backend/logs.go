package backend

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/cli"

	"github.com/docker/ecs-plugin/pkg/console"
)

func (b *Backend) Logs(ctx context.Context, options *cli.ProjectOptions) error {
	name := options.Name
	if name == "" {
		project, err := cli.ProjectFromOptions(options)
		if err != nil {
			return err
		}
		name = project.Name
	}

	err := b.api.GetLogs(ctx, name, &logConsumer{
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
