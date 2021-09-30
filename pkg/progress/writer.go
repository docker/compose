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

package progress

import (
	"context"
	"os"
	"sync"

	"github.com/containerd/console"
	"github.com/moby/term"
	"golang.org/x/sync/errgroup"
)

// Writer can write multiple progress events
type Writer interface {
	Start(context.Context) error
	Stop()
	Event(Event)
	TailMsgf(string, ...interface{})
}

type writerKey struct{}

// WithContextWriter adds the writer to the context
func WithContextWriter(ctx context.Context, writer Writer) context.Context {
	return context.WithValue(ctx, writerKey{}, writer)
}

// ContextWriter returns the writer from the context
func ContextWriter(ctx context.Context) Writer {
	s, ok := ctx.Value(writerKey{}).(Writer)
	if !ok {
		return &noopWriter{}
	}
	return s
}

type progressFunc func(context.Context) error

type progressFuncWithStatus func(context.Context) (string, error)

// Run will run a writer and the progress function in parallel
func Run(ctx context.Context, pf progressFunc) error {
	_, err := RunWithStatus(ctx, func(ctx context.Context) (string, error) {
		return "", pf(ctx)
	})
	return err
}

// RunWithStatus will run a writer and the progress function in parallel and return a status
func RunWithStatus(ctx context.Context, pf progressFuncWithStatus) (string, error) {
	eg, _ := errgroup.WithContext(ctx)
	w, err := NewWriter(os.Stderr)
	var result string
	if err != nil {
		return "", err
	}
	eg.Go(func() error {
		return w.Start(context.Background())
	})

	ctx = WithContextWriter(ctx, w)

	eg.Go(func() error {
		defer w.Stop()
		s, err := pf(ctx)
		if err == nil {
			result = s
		}
		return err
	})

	err = eg.Wait()
	return result, err
}

// NewWriter returns a new multi-progress writer
func NewWriter(out console.File) (Writer, error) {
	_, isTerminal := term.GetFdInfo(out)

	if isTerminal {
		con, err := console.ConsoleFromFile(out)
		if err != nil {
			return nil, err
		}

		return &ttyWriter{
			out:      con,
			eventIDs: []string{},
			events:   map[string]Event{},
			repeated: false,
			done:     make(chan bool),
			mtx:      &sync.Mutex{},
		}, nil
	}

	return &plainWriter{
		out:  out,
		done: make(chan bool),
	}, nil
}
