package chanotify

import (
	"sync"
)

// Notifier can effectively notify you about receiving from particular channels.
// It operates with pairs <-chan struct{} <-> string which is notification
// channel and its identificator respectively.
// Notification channel is <-chan struc{}, each send to which is spawn
// notification from Notifier, close doesn't spawn anything and removes channel
// from Notifier.
type Notifier struct {
	c chan interface{}

	m      sync.Mutex // guards doneCh
	doneCh map[interface{}]chan struct{}
}

// New returns a new *Notifier.
func New() *Notifier {
	s := &Notifier{
		c:      make(chan interface{}),
		doneCh: make(map[interface{}]chan struct{}),
	}
	return s
}

// Chan returns channel on which client listen for notifications.
// IDs of notifications is sent to the returned channel.
func (n *Notifier) Chan() <-chan interface{} {
	return n.c
}

func (n *Notifier) killWorker(id interface{}, done chan struct{}) {
	n.m.Lock()
	delete(n.doneCh, id)
	n.m.Unlock()
}

// Add adds new notification channel to Notifier.
func (n *Notifier) Add(id interface{}, ch <-chan struct{}) {
	done := make(chan struct{})
	n.m.Lock()
	n.doneCh[id] = done
	n.m.Unlock()

	go func(ch <-chan struct{}, id interface{}, done chan struct{}) {
		for {
			select {
			case _, ok := <-ch:
				if !ok {
					// If the channel is closed, we don't need the goroutine
					// or the done channel mechanism running anymore.
					n.killWorker(id, done)
					return
				}
				n.c <- id
			case <-done:
				// We don't need this goroutine running anymore, return.
				n.killWorker(id, done)
				return
			}
		}
	}(ch, id, done)
}

// Close closes the notifier and releases its underlying resources.
func (n *Notifier) Close() {
	n.m.Lock()
	defer n.m.Unlock()
	for _, done := range n.doneCh {
		close(done)
	}
	close(n.c)
	// TODO(jbd): Don't allow Add after Close returns.
}
