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

// providerSourceService builds a ServiceConfig by assignment: Image and
// PullPolicy are fields promoted from the embedded ContainerSpec, which
// struct literals cannot set before go1.27.
func providerSourceService(build *types.BuildConfig, image, pullPolicy string) types.ServiceConfig {
	var s types.ServiceConfig
	s.Build = build
	s.Image = image
	s.PullPolicy = pullPolicy
	return s
}

func TestProviderImageSource(t *testing.T) {
	build := &types.BuildConfig{Context: "."}
	tests := []struct {
		name      string
		service   types.ServiceConfig
		justBuilt bool
		want      string
	}{
		{"no build: registry authority", providerSourceService(nil, "nginx", ""), false, providerImageSourceRegistry},
		{"build without image: local-only name", providerSourceService(build, "", ""), false, providerImageSourceLocal},
		{"build just run: local wins for this invocation", providerSourceService(build, "repo/app", ""), true, providerImageSourceLocal},
		{"build as CI recipe, default policy: registry authority", providerSourceService(build, "repo/app", ""), false, providerImageSourceRegistry},
		{"pull_policy build: local authority", providerSourceService(build, "repo/app", types.PullPolicyBuild), false, providerImageSourceLocal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, providerImageSource(tc.service, tc.justBuilt), tc.want)
		})
	}
}

// TestStreamImageTo locks the wire format of the get-image answer: one JSON
// announce line, then the tar bytes as an HTTP/1.1 chunked body terminated by
// the zero chunk and a final CRLF — decodable with a stock chunked reader.
func TestStreamImageTo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	cli := mocks.NewMockCli(mockCtrl)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	svc, err := NewComposeService(cli, WithEventProcessor(noopEventProcessor{}))
	assert.NilError(t, err)
	s := svc.(*composeService)

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
	cli := mocks.NewMockCli(mockCtrl)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	svc, err := NewComposeService(cli, WithEventProcessor(noopEventProcessor{}))
	assert.NilError(t, err)
	s := svc.(*composeService)

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

type fakeImageSaveResult struct{ io.ReadCloser }

// TestExecutePlugin_GetImage runs the pull command against a fake provider
// (this test binary re-executed, see TestHelperProviderPull): the get-image
// request must be answered with the announced chunked stream, which the
// provider consumes and acknowledges by hashing.
func TestExecutePlugin_GetImage(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	cli := mocks.NewMockCli(mockCtrl)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	svc, err := NewComposeService(cli, WithEventProcessor(noopEventProcessor{}))
	assert.NilError(t, err)

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
	variables, err := svc.(*composeService).executePlugin(t.Context(), &types.Project{Name: "proj"}, cmd, "pull", service)
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
	cli := mocks.NewMockCli(mockCtrl)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	svc, err := NewComposeService(cli, WithEventProcessor(noopEventProcessor{}))
	assert.NilError(t, err)

	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProviderPull")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	service := types.ServiceConfig{
		Name:     "db",
		Provider: &types.ServiceProviderConfig{Type: "fake"},
	}
	_, err = svc.(*composeService).executePlugin(t.Context(), &types.Project{Name: "proj"}, cmd, "up", service)
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
	cli := mocks.NewMockCli(mockCtrl)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	svc, err := NewComposeService(cli, WithEventProcessor(noopEventProcessor{}))
	assert.NilError(t, err)

	apiClient.EXPECT().ImageSave(gomock.Any(), []string{"proj-db"}).
		Return(&failingReader{data: bytes.Repeat([]byte{0xAB}, 4096)}, nil)

	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProviderPull")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	service := types.ServiceConfig{
		Name:     "db",
		Provider: &types.ServiceProviderConfig{Type: "fake"},
	}
	_, err = svc.(*composeService).executePlugin(t.Context(), &types.Project{Name: "proj"}, cmd, "pull", service)
	assert.ErrorContains(t, err, "reading stream")
}
