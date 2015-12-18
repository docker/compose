package chanotify

import (
	"reflect"
	"sync"
)

type dataPair struct {
	id         string
	selectCase reflect.SelectCase
}

// Notifier can effectively notify you about receiving from particular channels.
// It operates with pairs <-chan struct{} <-> string which is notification
// channel and its identificator respectively.
// Notification channel is <-chan struc{}, each send to which is spawn
// notification from Notifier, close doesn't spawn anything and removes channel
// from Notifier.
type Notifier struct {
	c     chan string
	chMap map[<-chan struct{}]*dataPair
	exit  chan struct{}
	m     sync.Mutex
}

// New returns already running *Notifier.
func New() *Notifier {
	s := &Notifier{
		c:     make(chan string),
		chMap: make(map[<-chan struct{}]*dataPair),
		exit:  make(chan struct{}),
	}
	go s.start()
	return s
}

// Chan returns channel on which client listen for notifications.
// Ids of notifications is sent to that channel.
func (s *Notifier) Chan() <-chan string {
	return s.c
}

// Add adds new notification channel to Notifier.
func (s *Notifier) Add(ch <-chan struct{}, id string) {
	s.m.Lock()
	s.chMap[ch] = &dataPair{
		id: id,
		selectCase: reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(ch),
		},
	}
	s.m.Unlock()
}

// Close stops Notifier to listen for any notifications and closes its
// "client-side" channel.
func (s *Notifier) Close() {
	close(s.exit)
}

func (s *Notifier) start() {
	for {
		c := s.createCase()
		i, _, ok := reflect.Select(c)
		if i == 0 {
			// exit was closed, we can safely close output
			close(s.c)
			return
		}
		ch := c[i].Chan.Interface().(<-chan struct{})
		if ok {
			s.c <- s.chMap[ch].id
			continue
		}
		// the channel was closed and we should remove it
		s.m.Lock()
		delete(s.chMap, ch)
		s.m.Unlock()
	}
}

func (s *Notifier) createCase() []reflect.SelectCase {
	// put exit channel as 0 element of select
	out := []reflect.SelectCase{
		reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(s.exit),
		},
	}
	s.m.Lock()
	for _, pair := range s.chMap {
		out = append(out, pair.selectCase)
	}
	s.m.Unlock()
	return out
}
