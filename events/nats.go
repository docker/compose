package events

import (
	"context"
	"strings"

	"github.com/docker/containerd/log"
	nats "github.com/nats-io/go-nats"
)

type natsPoster struct {
	nec *nats.EncodedConn
}

func GetNATSPoster(nec *nats.EncodedConn) Poster {
	return &natsPoster{nec}
}

func (p *natsPoster) Post(ctx context.Context, e Event) {
	subject := strings.Replace(log.GetModulePath(ctx), "/", ".", -1)
	topic := getTopic(ctx)
	if topic != "" {
		subject = strings.Join([]string{subject, topic}, ".")
	}

	if subject == "" {
		log.GetLogger(ctx).WithField("event", e).Warn("unable to post event, subject is empty")
	}

	p.nec.Publish(subject, e)
}
