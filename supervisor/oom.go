package supervisor

import (
	"reflect"
	"sync"
)

func newNotifier(s *Supervisor) *notifier {
	n := &notifier{
		s:          s,
		channels:   make(map[<-chan struct{}]string),
		controller: make(chan struct{}),
	}
	go n.start()
	return n
}

type notifier struct {
	m          sync.Mutex
	channels   map[<-chan struct{}]string
	controller chan struct{}
	s          *Supervisor
}

func (n *notifier) start() {
	for {
		c := n.createCase()
		i, _, ok := reflect.Select(c)
		if i == 0 {
			continue
		}
		if ok {
			ch := c[i].Chan.Interface().(<-chan struct{})
			id := n.channels[ch]
			e := NewEvent(OOMEventType)
			e.ID = id
			n.s.SendEvent(e)
			continue
		}
		// the channel was closed and we should remove it
		ch := c[i].Chan.Interface().(<-chan struct{})
		n.removeChan(ch)
	}
}

func (n *notifier) Add(ch <-chan struct{}, id string) {
	n.m.Lock()
	n.channels[ch] = id
	n.m.Unlock()
	// signal the main loop to break and add the new
	// channels
	n.controller <- struct{}{}
}

func (n *notifier) createCase() []reflect.SelectCase {
	var out []reflect.SelectCase
	// add controller chan so that we can signal when we need to make
	// changes in the select.  The controller chan will always be at
	// index 0 in the slice
	out = append(out, reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(n.controller),
	})
	n.m.Lock()
	for ch := range n.channels {
		v := reflect.ValueOf(ch)
		out = append(out, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: v,
		})
	}
	n.m.Unlock()
	return out
}

func (n *notifier) removeChan(ch <-chan struct{}) {
	n.m.Lock()
	delete(n.channels, ch)
	n.m.Unlock()
}
