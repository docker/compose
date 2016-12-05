package events

import (
	"context"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/log"
)

var (
	G = GetPoster
)

// Poster posts the event.
type Poster interface {
	Post(event Event)
}

type posterKey struct{}

func GetPoster(ctx context.Context) Poster {
	poster := ctx.Value(ctx)
	if poster == nil {
		logger := log.G(ctx)
		tx, _ := getTx(ctx)
		topic := getTopic(ctx)

		// likely means we don't have a configured event system. Just return
		// the default poster, which merely logs events.
		return posterFunc(func(event Event) {
			fields := logrus.Fields{"event": event}

			if topic != "" {
				fields["topic"] = topic
			}

			if tx != nil {
				fields["tx.id"] = tx.id
				if tx.parent != nil {
					fields["tx.parent.id"] = tx.parent.id
				}
			}

			logger.WithFields(fields).Info("event posted")
		})
	}

	return poster.(Poster)
}

type posterFunc func(event Event)

func (fn posterFunc) Post(event Event) {
	fn(event)
}
