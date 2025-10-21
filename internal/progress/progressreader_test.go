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

// Copied from https://github.com/moby/moby/blob/f8215cc266744ef195a50a70d427c345da2acdbb/pkg/progress/progressreader_test.go

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
	"bytes"
	"io"
	"testing"
)

func TestOutputOnPrematureClose(t *testing.T) {
	content := []byte("TESTING")
	reader := io.NopCloser(bytes.NewReader(content))
	progressChan := make(chan Progress, 10)

	pr := NewProgressReader(reader, ChanOutput(progressChan), int64(len(content)), "Test", "Read")

	part := make([]byte, 4)
	_, err := io.ReadFull(pr, part)
	if err != nil {
		_ = pr.Close()
		t.Fatal(err)
	}

drainLoop:
	for {
		select {
		case <-progressChan:
		default:
			break drainLoop
		}
	}

	_ = pr.Close()

	select {
	case <-progressChan:
	default:
		t.Fatalf("Expected some output when closing prematurely")
	}
}

func TestCompleteSilently(t *testing.T) {
	content := []byte("TESTING")
	reader := io.NopCloser(bytes.NewReader(content))
	progressChan := make(chan Progress, 10)

	pr := NewProgressReader(reader, ChanOutput(progressChan), int64(len(content)), "Test", "Read")

	out, err := io.ReadAll(pr)
	if err != nil {
		_ = pr.Close()
		t.Fatal(err)
	}
	if string(out) != "TESTING" {
		_ = pr.Close()
		t.Fatalf("Unexpected output %q from reader", string(out))
	}

drainLoop:
	for {
		select {
		case <-progressChan:
		default:
			break drainLoop
		}
	}

	_ = pr.Close()

	select {
	case <-progressChan:
		t.Fatalf("Should have closed silently when read is complete")
	default:
	}
}
