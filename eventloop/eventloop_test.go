package eventloop

import (
	"sync"
	"testing"
	"time"
)

type racyEvent struct {
	m  map[int]struct{}
	wg *sync.WaitGroup
}

func (e *racyEvent) Handle() {
	e.m[0] = struct{}{}
	e.wg.Done()
}

func simulateRacyEvents(el EventLoop) {
	wg := &sync.WaitGroup{}
	raceMap := make(map[int]struct{})
	var evs []*racyEvent
	for i := 0; i < 1024; i++ {
		wg.Add(1)
		evs = append(evs, &racyEvent{m: raceMap, wg: wg})
	}
	for _, ev := range evs {
		el.Send(ev)
	}
	wg.Wait()
}

// run with -race
func TestChanRace(t *testing.T) {
	e := NewChanLoop(1024)
	e.Start()
	simulateRacyEvents(e)
}

// run with -race
func TestChanStartTwiceRace(t *testing.T) {
	e := NewChanLoop(1024)
	e.Start()
	e.Start()
	simulateRacyEvents(e)
}

type testEvent struct {
	wg *sync.WaitGroup
}

func (e *testEvent) Handle() {
	e.wg.Done()
}

func TestChanEventSpawn(t *testing.T) {
	e := NewChanLoop(1024)
	e.Start()
	wg := &sync.WaitGroup{}
	wg.Add(2)
	e.Send(&testEvent{wg: wg})
	e.Send(&testEvent{wg: wg})
	waitCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitCh)
	}()
	select {
	case <-waitCh:
	case <-time.After(1 * time.Second):
		t.Fatal("Events was not handled in loop")
	}
}
