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

package provider

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httputil"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/mocks"
)

// TestStreamImageTo locks the wire format of the get-image answer: one JSON
// announce line, then the tar bytes as an HTTP/1.1 chunked body terminated by
// the zero chunk and a final CRLF — decodable with a stock chunked reader.
func TestStreamImageTo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	s := New(&testBackend{apiClient: apiClient})

	payload := bytes.Repeat([]byte("compose-image-bytes\x00\x17\xff"), 100_000) // > one chunk, binary-hostile values included
	apiClient.EXPECT().ImageSave(gomock.Any(), []string{"proj-app"}).
		Return(fakeImageSaveResult{io.NopCloser(bytes.NewReader(payload))}, nil)

	var out bytes.Buffer
	assert.NilError(t, s.streamImageTo(t.Context(), &out, "proj-app", ""))

	rd := bufio.NewReader(&out)
	line, err := rd.ReadString('\n')
	assert.NilError(t, err)
	var announce imageStreamAnswer
	assert.NilError(t, json.Unmarshal([]byte(line), &announce))
	assert.Equal(t, announce.Type, ImageStreamType)
	assert.Equal(t, announce.Encoding, "chunked")
	assert.Equal(t, announce.Error, "")

	body, err := io.ReadAll(httputil.NewChunkedReader(rd))
	assert.NilError(t, err)
	assert.Assert(t, bytes.Equal(body, payload), "chunked round-trip must be byte-identical (%d vs %d bytes)", len(body), len(payload))

	// nothing may follow the zero chunk: a stock chunked reader stops there
	// without consuming further bytes, and any leftover would corrupt the
	// next answer of a provider requesting several images
	rest, err := io.ReadAll(rd)
	assert.NilError(t, err)
	assert.Equal(t, string(rest), "", "no byte may follow the terminating zero chunk")
}

// TestStreamImageTo_SaveError: when the daemon cannot export the image, the
// announce line carries the error and no stream follows — the provider is
// never left waiting.
func TestStreamImageTo_SaveError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	s := New(&testBackend{apiClient: apiClient})

	apiClient.EXPECT().ImageSave(gomock.Any(), []string{"proj-app"}).
		Return(nil, errors.New("no such image"))

	var out bytes.Buffer
	assert.ErrorContains(t, s.streamImageTo(t.Context(), &out, "proj-app", ""), "no such image")

	var announce imageStreamAnswer
	assert.NilError(t, json.Unmarshal(out.Bytes(), &announce))
	assert.Equal(t, announce.Type, ImageStreamType)
	assert.ErrorContains(t, errors.New(announce.Error), "no such image")
	assert.Equal(t, announce.Encoding, "")
}

// TestStreamImageTo_SuccessAnnounceWriteFailureAborts: when the write of the
// success announce itself fails, the channel is broken regardless of
// whether the provider received anything — the caller must close stdin
// (via streamAbortedError) rather than leave the provider blocked reading a
// line that was never delivered.
func TestStreamImageTo_SuccessAnnounceWriteFailureAborts(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	s := New(&testBackend{apiClient: apiClient})

	apiClient.EXPECT().ImageSave(gomock.Any(), []string{"proj-app"}).
		Return(fakeImageSaveResult{io.NopCloser(bytes.NewReader([]byte("tar")))}, nil)

	err := s.streamImageTo(t.Context(), failingWriter{errors.New("broken pipe")}, "proj-app", "")
	var aborted *streamAbortedError
	assert.Assert(t, errors.As(err, &aborted), "want *streamAbortedError, got %T: %v", err, err)
}

// TestStreamImageTo_ErrorAnnounceWriteFailureAborts: same reasoning on the
// error-announce path — if ImageSave fails AND the resulting error line
// cannot be written either, the provider gets no error to read and must be
// unblocked by closing stdin.
func TestStreamImageTo_ErrorAnnounceWriteFailureAborts(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	s := New(&testBackend{apiClient: apiClient})

	apiClient.EXPECT().ImageSave(gomock.Any(), []string{"proj-app"}).
		Return(nil, errors.New("no such image"))

	err := s.streamImageTo(t.Context(), failingWriter{errors.New("broken pipe")}, "proj-app", "")
	var aborted *streamAbortedError
	assert.Assert(t, errors.As(err, &aborted), "want *streamAbortedError, got %T: %v", err, err)
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

type fakeImageSaveResult struct{ io.ReadCloser }

// TestExecutePlugin_GetImage runs the pull command against a fake provider
// (this test binary re-executed, see TestHelperProviderPull): the get-image
// request must be answered with the announced chunked stream, which the
// provider consumes and acknowledges by hashing.
func TestExecutePlugin_GetImage(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	svc := New(&testBackend{apiClient: apiClient})

	payload := bytes.Repeat([]byte{0x17, 0x00, 0xAB}, 500_000) // ETB and NUL laden
	sum := sha256.Sum256(payload)
	apiClient.EXPECT().ImageSave(gomock.Any(), []string{"proj-db"}).
		Return(fakeImageSaveResult{io.NopCloser(bytes.NewReader(payload))}, nil)

	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProviderPull")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	service := types.ServiceConfig{
		Name:     "db",
		Provider: &types.ServiceProviderConfig{Type: "fake"},
	}
	variables, err := svc.executePlugin(t.Context(), &types.Project{Name: "proj"}, cmd, "pull", service)
	assert.NilError(t, err)
	assert.Equal(t, variables.prefixed["SHA"], hex.EncodeToString(sum[:]))
	assert.Equal(t, variables.prefixed["LEN"], strconv.Itoa(len(payload)))
}

// TestHelperProviderPull is not a test: it is the fake provider process
// spawned by TestExecutePlugin_GetImage. It requests the image, decodes the
// chunked stream with the stock reader, and reports what it received.
func TestHelperProviderPull(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		t.Skip("helper process for TestExecutePlugin_GetImage")
	}
	emit := func(msg JsonMessage) {
		if err := json.NewEncoder(os.Stdout).Encode(msg); err != nil {
			os.Exit(1)
		}
	}
	emit(JsonMessage{Type: GetImageType, Message: "proj-db"})

	stdin := bufio.NewReader(os.Stdin)
	line, err := stdin.ReadString('\n')
	if err != nil {
		emit(JsonMessage{Type: ErrorType, Message: "reading announce: " + err.Error()})
		os.Exit(1)
	}
	var announce imageStreamAnswer
	if err := json.Unmarshal([]byte(line), &announce); err != nil || announce.Error != "" || announce.Encoding != "chunked" {
		emit(JsonMessage{Type: ErrorType, Message: fmt.Sprintf("bad announce %q (err %v)", line, err)})
		os.Exit(1)
	}
	h := sha256.New()
	n, err := io.Copy(h, httputil.NewChunkedReader(stdin))
	if err != nil {
		emit(JsonMessage{Type: ErrorType, Message: "reading stream: " + err.Error()})
		os.Exit(1)
	}
	emit(JsonMessage{Type: SetEnvType, Message: "SHA=" + hex.EncodeToString(h.Sum(nil))})
	emit(JsonMessage{Type: SetEnvType, Message: fmt.Sprintf("LEN=%d", n)})
	os.Exit(0)
}

// TestExecutePlugin_GetImageRefusedOutsidePull: image distribution belongs to
// the pull command — a get-image emitted during up is a protocol error, so an
// image export can never stall another lifecycle command.
func TestExecutePlugin_GetImageRefusedOutsidePull(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	svc := New(&testBackend{apiClient: apiClient})

	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProviderPull")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	service := types.ServiceConfig{
		Name:     "db",
		Provider: &types.ServiceProviderConfig{Type: "fake"},
	}
	_, err := svc.executePlugin(t.Context(), &types.Project{Name: "proj"}, cmd, "up", service)
	assert.ErrorContains(t, err, "get-image is only supported during the pull command")
}

// failingReader errors after serving its prefix, simulating a daemon export
// dying mid-stream.
type failingReader struct{ data []byte }

func (f *failingReader) Read(p []byte) (int, error) {
	if len(f.data) == 0 {
		return 0, errors.New("export died mid-stream")
	}
	n := copy(p, f.data)
	f.data = f.data[n:]
	return n, nil
}

func (f *failingReader) Close() error { return nil }

// TestExecutePlugin_GetImageAbortedMidStream: a transfer failing after the
// success announce must end observably — compose closes the answer channel,
// the provider reads EOF mid-chunk and reports, instead of blocking forever
// on a stream nobody will finish.
func TestExecutePlugin_GetImageAbortedMidStream(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	svc := New(&testBackend{apiClient: apiClient})

	apiClient.EXPECT().ImageSave(gomock.Any(), []string{"proj-db"}).
		Return(&failingReader{data: bytes.Repeat([]byte{0xAB}, 4096)}, nil)

	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProviderPull")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	service := types.ServiceConfig{
		Name:     "db",
		Provider: &types.ServiceProviderConfig{Type: "fake"},
	}
	_, err := svc.executePlugin(t.Context(), &types.Project{Name: "proj"}, cmd, "pull", service)
	assert.ErrorContains(t, err, "reading stream")
}

// TestExecutePlugin_GetImageSaveErrorDoesNotAbortChannel: a failure BEFORE
// the success announce (ImageSave itself failing) is a clean, well-formed
// error line — the channel is intact. A provider making a second,
// independent get-image request in the same pull invocation must still get
// served, proving compose did not close stdin on it.
func TestExecutePlugin_GetImageSaveErrorDoesNotAbortChannel(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	svc := New(&testBackend{apiClient: apiClient})

	apiClient.EXPECT().ImageSave(gomock.Any(), []string{"proj-missing"}).
		Return(nil, errors.New("no such image"))
	payload := []byte("second request succeeds")
	sum := sha256.Sum256(payload)
	apiClient.EXPECT().ImageSave(gomock.Any(), []string{"proj-db"}).
		Return(fakeImageSaveResult{io.NopCloser(bytes.NewReader(payload))}, nil)

	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProviderPullTwice")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	service := types.ServiceConfig{
		Name:     "db",
		Provider: &types.ServiceProviderConfig{Type: "fake"},
	}
	variables, err := svc.executePlugin(t.Context(), &types.Project{Name: "proj"}, cmd, "pull", service)
	assert.NilError(t, err)
	assert.Equal(t, variables.prefixed["FIRST_ERROR"], "no such image")
	assert.Equal(t, variables.prefixed["SHA"], hex.EncodeToString(sum[:]))
}

// TestHelperProviderPullTwice is not a test: it is the fake provider process
// spawned by TestExecutePlugin_GetImageSaveErrorDoesNotAbortChannel. It
// requests an image that fails to export, then — on the SAME stdin, proving
// compose never closed it — requests a second image that succeeds.
func TestHelperProviderPullTwice(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		t.Skip("helper process for TestExecutePlugin_GetImageSaveErrorDoesNotAbortChannel")
	}
	emit := func(msg JsonMessage) {
		if err := json.NewEncoder(os.Stdout).Encode(msg); err != nil {
			os.Exit(1)
		}
	}
	stdin := bufio.NewReader(os.Stdin)

	emit(JsonMessage{Type: GetImageType, Message: "proj-missing"})
	line, err := stdin.ReadString('\n')
	if err != nil {
		emit(JsonMessage{Type: ErrorType, Message: "reading first announce: " + err.Error()})
		os.Exit(1)
	}
	var announce imageStreamAnswer
	if err := json.Unmarshal([]byte(line), &announce); err != nil {
		emit(JsonMessage{Type: ErrorType, Message: fmt.Sprintf("bad first announce %q (err %v)", line, err)})
		os.Exit(1)
	}
	emit(JsonMessage{Type: SetEnvType, Message: "FIRST_ERROR=" + announce.Error})

	emit(JsonMessage{Type: GetImageType, Message: "proj-db"})
	line, err = stdin.ReadString('\n')
	if err != nil {
		emit(JsonMessage{Type: ErrorType, Message: "reading second announce: " + err.Error()})
		os.Exit(1)
	}
	announce = imageStreamAnswer{} // JSON unmarshal only overwrites fields present in the new line
	if err := json.Unmarshal([]byte(line), &announce); err != nil || announce.Error != "" || announce.Encoding != "chunked" {
		emit(JsonMessage{Type: ErrorType, Message: fmt.Sprintf("bad second announce %q (err %v)", line, err)})
		os.Exit(1)
	}
	h := sha256.New()
	if _, err := io.Copy(h, httputil.NewChunkedReader(stdin)); err != nil {
		emit(JsonMessage{Type: ErrorType, Message: "reading second stream: " + err.Error()})
		os.Exit(1)
	}
	emit(JsonMessage{Type: SetEnvType, Message: "SHA=" + hex.EncodeToString(h.Sum(nil))})
	os.Exit(0)
}
