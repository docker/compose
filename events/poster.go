package events

import (
	"context"

	"github.com/docker/containerd/log"
	"github.com/sirupsen/logrus"
)

var (
	G = GetPoster
)

// Poster posts the event.
type Poster interface {
	Post(ctx context.Context, event Event)
}

type posterKey struct{}

func WithPoster(ctx context.Context, poster Poster) context.Context {
	return context.WithValue(ctx, posterKey{}, poster)
}

func GetPoster(ctx context.Context) Poster {
	poster := ctx.Value(posterKey{})
	if poster == nil {
		logger := log.G(ctx)
		tx, _ := getTx(ctx)
		topic := getTopic(ctx)

		// likely means we don't have a configured event system. Just return
		// the default poster, which merely logs events.
		return posterFunc(func(ctx context.Context, event Event) {
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

type posterFunc func(ctx context.Context, event Event)

func (fn posterFunc) Post(ctx context.Context, event Event) {
	fn(ctx, event)
}
