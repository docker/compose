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
	c chan string

	m      sync.Mutex // guards doneCh
	doneCh map[string]chan struct{}
}

// New returns a new *Notifier.
func New() *Notifier {
	s := &Notifier{
		c:      make(chan string),
		doneCh: make(map[string]chan struct{}),
	}
	return s
}

// Chan returns channel on which client listen for notifications.
// IDs of notifications is sent to the returned channel.
func (s *Notifier) Chan() <-chan string {
	return s.c
}

func (s *Notifier) killWorker(id string, done chan struct{}) {
	s.m.Lock()
	delete(s.doneCh, id)
	s.m.Unlock()
}

// Add adds new notification channel to Notifier.
func (s *Notifier) Add(ch <-chan struct{}, id string) {
	done := make(chan struct{})
	s.m.Lock()
	s.doneCh[id] = done
	s.m.Unlock()

	go func(ch <-chan struct{}, id string, done chan struct{}) {
		for {
			select {
			case _, ok := <-ch:
				if !ok {
					// If the channel is closed, we don't need the goroutine
					// or the done channel mechanism running anymore.
					s.killWorker(id, done)
					return
				}
				s.c <- id
			case <-done:
				// We don't need this goroutine running anymore, return.
				s.killWorker(id, done)
				return
			}
		}
	}(ch, id, done)
}

// Close closes the notifier and releases its underlying resources.
func (s *Notifier) Close() {
	s.m.Lock()
	defer s.m.Unlock()
	for _, done := range s.doneCh {
		close(done)
	}
	close(s.c)
	// TODO(jbd): Don't allow Add after Close returns.
}
