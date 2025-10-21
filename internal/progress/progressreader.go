/*
   Copyright 2025 Docker Compose CLI authors

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

// Copied from https://github.com/moby/moby/blob/f8215cc266744ef195a50a70d427c345da2acdbb/pkg/progress/progressreader.go

/*
	Copyright 2012-2017 Docker, Inc.

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
	"io"
	"time"

	"golang.org/x/time/rate"
)

// Reader is a Reader with progress bar.
type Reader struct {
	in          io.ReadCloser // Stream to read from
	out         Output        // Where to send progress bar to
	size        int64
	current     int64
	lastUpdate  int64
	id          string
	action      string
	rateLimiter *rate.Limiter
}

// NewProgressReader creates a new ProgressReader.
func NewProgressReader(in io.ReadCloser, out Output, size int64, id, action string) *Reader {
	return &Reader{
		in:          in,
		out:         out,
		size:        size,
		id:          id,
		action:      action,
		rateLimiter: rate.NewLimiter(rate.Every(100*time.Millisecond), 1),
	}
}

func (p *Reader) Read(buf []byte) (int, error) {
	read, err := p.in.Read(buf)
	p.current += int64(read)
	updateEvery := int64(1024 * 512) // 512kB
	if p.size > 0 {
		// Update progress for every 1% read if 1% < 512kB
		if increment := int64(0.01 * float64(p.size)); increment < updateEvery {
			updateEvery = increment
		}
	}
	if p.current-p.lastUpdate > updateEvery || err != nil {
		p.updateProgress(err != nil && read == 0)
		p.lastUpdate = p.current
	}

	return read, err
}

// Close closes the progress reader and its underlying reader.
func (p *Reader) Close() error {
	if p.current < p.size {
		// print a full progress bar when closing prematurely
		p.current = p.size
		p.updateProgress(false)
	}
	return p.in.Close()
}

func (p *Reader) updateProgress(last bool) {
	if last || p.current == p.size || p.rateLimiter.Allow() {
		_ = p.out.WriteProgress(Progress{ID: p.id, Action: p.action, Current: p.current, Total: p.size, LastUpdate: last})
	}
}
