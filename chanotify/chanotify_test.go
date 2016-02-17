package chanotify

import (
	"sync"
	"testing"
	"time"
)

func TestNotifier(t *testing.T) {
	s := New()
	ch1 := make(chan struct{}, 1)
	ch2 := make(chan struct{}, 1)
	id1 := "1"
	id2 := "2"

	if err := s.Add(id1, ch1); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(id2, ch2); err != nil {
		t.Fatal(err)
	}
	s.m.Lock()
	if len(s.doneCh) != 2 {
		t.Fatalf("want 2 channels, got %d", len(s.doneCh))
	}
	s.m.Unlock()
	ch1 <- struct{}{}
	if got, want := <-s.Chan(), id1; got != want {
		t.Fatalf("got %v; want %v", got, want)
	}
	ch2 <- struct{}{}
	if got, want := <-s.Chan(), id2; got != want {
		t.Fatalf("got %v; want %v", got, want)
	}
	close(ch1)
	close(ch2)
	time.Sleep(100 * time.Millisecond)
	s.m.Lock()
	if len(s.doneCh) != 0 {
		t.Fatalf("want 0 channels, got %d", len(s.doneCh))
	}
	s.m.Unlock()
}

func TestConcurrentNotifier(t *testing.T) {
	s := New()
	var chs []chan struct{}
	for i := 0; i < 8; i++ {
		ch := make(chan struct{}, 2)
		if err := s.Add(i, ch); err != nil {
			t.Fatal(err)
		}
		chs = append(chs, ch)
	}
	testCounter := make(map[interface{}]int)
	done := make(chan struct{})
	go func() {
		for id := range s.Chan() {
			testCounter[id]++
		}
		close(done)
	}()
	var wg sync.WaitGroup
	for _, ch := range chs {
		wg.Add(1)
		go func(ch chan struct{}) {
			ch <- struct{}{}
			ch <- struct{}{}
			close(ch)
			wg.Done()
		}(ch)
	}
	wg.Wait()
	// wait for notifications
	time.Sleep(1 * time.Second)
	s.Close()
	<-done
	if len(testCounter) != 8 {
		t.Fatalf("expect to find exactly 8 distinct ids, got %d", len(testCounter))
	}
	for id, c := range testCounter {
		if c != 2 {
			t.Fatalf("Expected to find exactly 2 id %s, but got %d", id, c)
		}
	}
}

func TestAddToBlocked(t *testing.T) {
	s := New()
	ch := make(chan struct{}, 1)
	id := 1
	go func() {
		// give some time to start first select
		time.Sleep(1 * time.Second)
		if err := s.Add(id, ch); err != nil {
			t.Fatal(err)
		}
		ch <- struct{}{}
	}()
	if got, want := <-s.Chan(), id; got != want {
		t.Fatalf("got %v; want %v", got, want)
	}
}

func TestAddDuplicate(t *testing.T) {
	s := New()
	ch1 := make(chan struct{}, 1)
	ch2 := make(chan struct{}, 1)

	if err := s.Add(1, ch1); err != nil {
		t.Fatalf("cannot add; err = %v", err)
	}

	if err := s.Add(1, ch2); err == nil {
		t.Fatalf("duplicate keys are not allowed; but Add succeeded")
	}
}
