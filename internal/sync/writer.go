/*
   Copyright 2023 Docker Compose CLI authors

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

package sync

import (
	"errors"
	"io"
)

// lossyMultiWriter attempts to tee all writes to the provided io.PipeWriter
// instances.
//
// If a writer fails during a Write call, the write-side of the pipe is then
// closed with the error and no subsequent attempts are made to write to the
// pipe.
//
// If all writers fail during a write, an error is returned.
//
// On Close, any remaining writers are closed.
type lossyMultiWriter struct {
	writers []*io.PipeWriter
}

// newLossyMultiWriter creates a new writer that *attempts* to tee all data written to it to the provided io.PipeWriter
// instances. Rather than failing a write operation if any writer fails, writes only fail if there are no more valid
// writers. Otherwise, errors for specific writers are propagated via CloseWithError.
func newLossyMultiWriter(writers ...*io.PipeWriter) *lossyMultiWriter {
	// reverse the writers because during the write we iterate
	// backwards, so this way we'll end up writing in the same
	// order as the writers were passed to us
	writers = append([]*io.PipeWriter(nil), writers...)
	for i, j := 0, len(writers)-1; i < j; i, j = i+1, j-1 {
		writers[i], writers[j] = writers[j], writers[i]
	}

	return &lossyMultiWriter{
		writers: writers,
	}
}

// Write writes to each writer that is still active (i.e. has not failed/encountered an error on write).
//
// If a writer encounters an error during the write, the write side of the pipe is closed with the error
// and no subsequent attempts will be made to write to that writer.
//
// An error is only returned from this function if ALL writers have failed.
func (l *lossyMultiWriter) Write(p []byte) (int, error) {
	// NOTE: this function iterates backwards so that it can
	// 	safely remove elements during the loop
	for i := len(l.writers) - 1; i >= 0; i-- {
		written, err := l.writers[i].Write(p)
		if err == nil && written != len(p) {
			err = io.ErrShortWrite
		}
		if err != nil {
			// pipe writer close cannot fail
			_ = l.writers[i].CloseWithError(err)
			l.writers = append(l.writers[:i], l.writers[i+1:]...)
		}
	}

	if len(l.writers) == 0 {
		return 0, errors.New("no writers remaining")
	}

	return len(p), nil
}

// Close closes any still open (non-failed) writers.
//
// Failed writers have already been closed with an error.
func (l *lossyMultiWriter) Close() {
	for i := range l.writers {
		// pipe writer close cannot fail
		_ = l.writers[i].Close()
	}
}
