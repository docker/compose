package chanotify

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestNotifier(t *testing.T) {
	s := New()
	ch1 := make(chan struct{}, 1)
	ch2 := make(chan struct{}, 1)
	s.Add("1", ch1)
	s.Add("2", ch2)
	s.m.Lock()
	if len(s.doneCh) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(s.doneCh))
	}
	s.m.Unlock()
	ch1 <- struct{}{}
	id1 := <-s.Chan()
	if id1.(string) != "1" {
		t.Fatalf("1 should be spawned, got %s", id1)
	}
	ch2 <- struct{}{}
	id2 := <-s.Chan()
	if id2.(string) != "2" {
		t.Fatalf("2 should be spawned, got %s", id2)
	}
	close(ch1)
	close(ch2)
	time.Sleep(100 * time.Millisecond)
	s.m.Lock()
	if len(s.doneCh) != 0 {
		t.Fatalf("expected 0 channels, got %d", len(s.doneCh))
	}
	s.m.Unlock()
}

func TestConcurrentNotifier(t *testing.T) {
	s := New()
	var chs []chan struct{}
	for i := 0; i < 8; i++ {
		ch := make(chan struct{}, 2)
		s.Add(strconv.Itoa(i), ch)
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
	go func() {
		// give some time to start first select
		time.Sleep(1 * time.Second)
		s.Add("1", ch)
		ch <- struct{}{}
	}()
	val := <-s.Chan()
	if val != "1" {
		t.Fatalf("Expected 1, got %s", val)
	}
}
