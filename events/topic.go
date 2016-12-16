package events

import "context"

type topicKey struct{}

// WithTopic returns a context with a new topic set, such that events emitted
// from the resulting context will be marked with the topic.
//
// A topic groups events by the target module they operate on. This is
// primarily designed to support multi-module log compaction of events. In
// typical journaling systems, the entries operate on a single data structure.
// When compacting the journal, we can replace all former log entries with  a
// summary data structure that will result in the same state.
//
// By providing a compaction mechanism by topic, we can prune down to a data
// structure oriented towards a single topic, leaving unrelated messages alone.
func WithTopic(ctx context.Context, topic string) context.Context {
	return context.WithValue(ctx, topicKey{}, topic)
}

func getTopic(ctx context.Context) string {
	topic := ctx.Value(topicKey{})

	if topic == nil {
		return ""
	}

	return topic.(string)
}

// RegisterCompactor sets the compacter for the given topic.
func RegisterCompactor(topic string, compactor interface{}) {
	panic("not implemented")
}
