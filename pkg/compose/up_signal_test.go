//go:build !windows

/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package compose

import (
	"errors"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

// TestUpInteractiveDoesNotHandleSignalsDuringTheCreatePhase pins when an
// interactive up starts to handle SIGINT and SIGTERM itself: once its Create
// phase is over, not before. A signal met during the Create phase belongs to
// its caller, which interrupts the phase through the context; were the
// session already registered, the signal would be waiting in its channel when
// it starts and tear the application down at once, a graceful stop nobody
// asked for after the interruption had been dealt with.
//
// The signal is real: the test takes it too, so that it doesn't end the test
// process, and waits for it to be delivered.
func TestUpInteractiveDoesNotHandleSignalsDuringTheCreatePhase(t *testing.T) {
	mine := make(chan os.Signal, 1)
	signal.Notify(mine, syscall.SIGINT)
	defer signal.Stop(mine)

	h := newInteractiveUpHarness(t)
	h.daemon.onCreate = func() {
		if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
			t.Error(err)
			return
		}
		select {
		case <-mine:
		case <-time.After(10 * time.Second):
			t.Error("the signal was never delivered")
		}
	}

	assert.NilError(t, h.up(t))
	assert.Equal(t, h.daemon.indexOf("stop"), -1, "the application was stopped after a signal met during the Create phase: %v", h.daemon.recorded())
}

// TestUpInteractiveStopsAContainerThatStartsAfterTheTeardownSweep pins what a
// graceful teardown does with a container whose start was already in flight
// when it stopped the application: the one-shot sweep of the teardown misses
// it, since it was not up yet, and it comes up right after. The session stops
// it as it appears, rather than staying up for as long as it runs.
//
// The interruption is a real signal, the one the session itself handles (a
// canceled context would end the session's monitor with it): the test takes it
// too, so that it doesn't end the test process.
func TestUpInteractiveStopsAContainerThatStartsAfterTheTeardownSweep(t *testing.T) {
	mine := make(chan os.Signal, 1)
	signal.Notify(mine, syscall.SIGINT)
	defer signal.Stop(mine)

	h := newInteractiveUpHarness(t)
	h.daemon.startGate = make(chan struct{})
	h.daemon.neverExits = true
	h.daemon.stopEndsStartedContainer = true
	go func() {
		select {
		case <-h.daemon.callSignal("start-requested"):
		case <-t.Context().Done():
			return
		}
		if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
			t.Error(err)
			return
		}
		// the sweep of the teardown, which finds the container not up yet
		select {
		case <-h.daemon.callSignal("stop"):
		case <-t.Context().Done():
			return
		}
		// the start goes through right after it
		close(h.daemon.startGate)
	}()

	assert.NilError(t, h.up(t))
	assert.Equal(t, callsOf(h.daemon, "stop"), 2, "the container that started after the sweep must be stopped too: %v", h.daemon.recorded())
}

// TestUpInteractiveIgnoresTheStartPhaseErrorOfAnInterruptedUp pins that a
// Start phase failing while the up is being torn down is not the up's error:
// the Start phase runs on a context that outlives the interruption, so the
// graceful stop the interruption triggers can land on a container whose start
// is still in flight, and the start then fails because of it.
//
// The interruption is a real signal (see
// TestUpInteractiveStopsAContainerThatStartsAfterTheTeardownSweep).
func TestUpInteractiveIgnoresTheStartPhaseErrorOfAnInterruptedUp(t *testing.T) {
	mine := make(chan os.Signal, 1)
	signal.Notify(mine, syscall.SIGINT)
	defer signal.Stop(mine)

	h := newInteractiveUpHarness(t)
	h.daemon.startGate = make(chan struct{})
	h.daemon.startErr = errors.New("container start interrupted by the stop")
	h.daemon.stopEndsContainer = true
	go func() {
		// the container start is in flight when the interruption comes, and
		// fails only once the graceful stop it triggered went through
		select {
		case <-h.daemon.callSignal("start-requested"):
		case <-t.Context().Done():
			return
		}
		if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
			t.Error(err)
			return
		}
		select {
		case <-h.daemon.callSignal("stop"):
		case <-t.Context().Done():
			return
		}
		close(h.daemon.startGate)
	}()

	assert.NilError(t, h.up(t))
}
