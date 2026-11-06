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
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/context/docker"
	"github.com/spf13/cobra"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

// fakeLLMMan is a shell script standing in for the llmman CLI. State is driven
// by files next to it:
//   - store: references `llmman list` prints, one per line
//   - resolved: lines "<ref>=<name>|<tag>" answered by `llmman list <ref>`
//   - down: when present, `llmman ps` fails as if no daemon was running
//   - listfail: when present, `llmman list --format` fails
//   - resolvefail: when present, `llmman list <ref>` fails
//   - pullfail: when present, `llmman pull` fails
//   - longline: when present, `llmman pull` prints a line longer than a scanner reads
//
// Every invocation is appended to calls.log, with the LLMMAN_* variables seen.
type fakeLLMMan struct {
	dir   string
	calls string
}

func newFakeLLMMan(t *testing.T) *fakeLLMMan {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake llmman is a shell script")
	}
	dir := t.TempDir()
	f := &fakeLLMMan{dir: dir, calls: filepath.Join(dir, "calls.log")}
	script := `#!/bin/sh
dir="` + dir + `"
echo "$* [host=$LLMMAN_HOST key=$LLMMAN_API_KEY]" >> "` + f.calls + `"
case "$1" in
ps)
	if [ -f "$dir/down" ]; then
		echo "llmman serve is not running (nothing is loaded) - start it with llmman serve" >&2
		exit 1
	fi
	echo "NAME    ID    SIZE    PROCESSOR    CONTEXT    STARTED    UNTIL"
	;;
list)
	if [ "$2" = "--format" ]; then
		if [ -f "$dir/listfail" ]; then
			echo "store unreadable" >&2
			exit 1
		fi
		[ -f "$dir/store" ] && cat "$dir/store"
		exit 0
	fi
	if [ -f "$dir/resolvefail" ]; then
		echo "cannot resolve" >&2
		exit 1
	fi
	[ -f "$dir/resolved" ] && while IFS='=' read -r ref out; do
		[ "$ref" = "$2" ] && echo "$out"
	done < "$dir/resolved"
	exit 0
	;;
pull)
	if [ -f "$dir/pullfail" ]; then
		echo "registry exploded" >&2
		exit 1
	fi
	if [ -f "$dir/longline" ]; then
		head -c 300000 /dev/zero | tr '\0' x
		echo
	fi
	echo "pulling $2"
	echo "Pulled $2"
	;;
*)
	echo "unexpected command $*" >&2
	exit 2
	;;
esac
`
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "llmman"), []byte(script), 0o700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return f
}

func (f *fakeLLMMan) set(t *testing.T, name, content string) {
	t.Helper()
	assert.NilError(t, os.WriteFile(filepath.Join(f.dir, name), []byte(content), 0o600))
}

// log returns the commands run, without the environment suffix.
func (f *fakeLLMMan) log(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(f.calls)
	if os.IsNotExist(err) {
		return nil
	}
	assert.NilError(t, err)
	var cmds []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		cmd, _, _ := strings.Cut(line, " [host=")
		cmds = append(cmds, cmd)
	}
	return cmds
}

type recordedEvents struct {
	mu     sync.Mutex
	events []api.Resource
}

func (r *recordedEvents) Start(context.Context, string) {}
func (r *recordedEvents) Done(string, bool)             {}
func (r *recordedEvents) On(events ...api.Resource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, events...)
}

// count returns how many events with the given status and text were recorded for id.
func (r *recordedEvents) count(id string, status api.EventStatus, text string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.events {
		if e.ID == id && e.Status == status && e.Text == text {
			n++
		}
	}
	return n
}

// has tells whether an event with the given status and text was recorded for id.
func (r *recordedEvents) has(id string, status api.EventStatus, text string) bool {
	return r.count(id, status, text) > 0
}

func (r *recordedEvents) details(id string, status api.EventStatus) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var details []string
	for _, e := range r.events {
		if e.ID == id && e.Status == status {
			details = append(details, e.Details)
		}
	}
	return details
}

// llmmanProject returns a project with one service "app" bound to the given models.
func llmmanProject(env types.Mapping, models types.Models, bindings map[string]*types.ServiceModelConfig) *types.Project {
	// Environment and Models are promoted fields, which struct literals
	// cannot set before go1.27
	var app types.ServiceConfig
	app.Environment = types.MappingWithEquals{}
	app.Models = bindings
	return &types.Project{
		Environment: env,
		Models:      models,
		Services:    types.Services{"app": app},
	}
}

func llmmanEnv(extra ...string) types.Mapping {
	env := types.Mapping{modelProviderEnv: modelProviderLLMMan}
	for i := 0; i+1 < len(extra); i += 2 {
		env[extra[i]] = extra[i+1]
	}
	return env
}

func TestModelProviderSelection(t *testing.T) {
	fake := newFakeLLMMan(t)
	s := &composeService{events: &recordedEvents{}}

	models := types.Models{"llm": {Model: "smollm2"}}
	bindings := map[string]*types.ServiceModelConfig{"llm": nil}

	t.Run("llmman is selected by COMPOSE_MODEL_PROVIDER", func(t *testing.T) {
		m, err := s.newModelAPI(llmmanProject(llmmanEnv(), models, bindings))
		assert.NilError(t, err)
		_, ok := m.(*llmmanModelAPI)
		assert.Assert(t, ok, "got %T", m)
	})

	for _, value := range []string{"LLMMAN", "ollama", "llmman,docker-model-runner"} {
		t.Run("unknown provider "+value, func(t *testing.T) {
			env := types.Mapping{modelProviderEnv: value}
			err := s.ensureModels(t.Context(), llmmanProject(env, models, bindings), false)
			assert.ErrorContains(t, err, "unsupported COMPOSE_MODEL_PROVIDER")
			assert.ErrorContains(t, err, value)
			assert.ErrorContains(t, err, modelProviderDMR)
		})
	}

	t.Run("the default is Docker Model Runner", func(t *testing.T) {
		// llmman is on PATH, yet Docker Model Runner wins when its plugin is installed
		dir := t.TempDir()
		script := "#!/bin/sh\ncase \"$1\" in docker-cli-plugin-metadata) echo '{\"SchemaVersion\":\"0.1.0\",\"Vendor\":\"test\",\"Version\":\"v1.2.6\"}' ;; esac\n"
		assert.NilError(t, os.WriteFile(filepath.Join(dir, "docker-model"), []byte(script), 0o700))
		cli := mocks.NewMockCli(gomock.NewController(t))
		cli.EXPECT().ConfigFile().Return(&configfile.ConfigFile{CLIPluginsExtraDirs: []string{dir}}).AnyTimes()
		cli.EXPECT().CurrentContext().Return("default").AnyTimes()
		cli.EXPECT().DockerEndpoint().Return(docker.Endpoint{}).AnyTimes()
		dmr := &composeService{dockerCli: cli}

		for _, env := range []types.Mapping{{}, {modelProviderEnv: ""}, {modelProviderEnv: " " + modelProviderDMR + " "}} {
			m, err := dmr.newModelAPI(llmmanProject(env, models, bindings))
			assert.NilError(t, err)
			_, ok := m.(*dmrModelAPI)
			assert.Assert(t, ok, "got %T for %v", m, env)
			m.Close()
		}
		assert.Equal(t, len(fake.log(t)), 0, "llmman must not be run when not selected")
	})

	t.Run("nothing is checked without models", func(t *testing.T) {
		env := types.Mapping{modelProviderEnv: "bogus"}
		assert.NilError(t, s.ensureModels(t.Context(), llmmanProject(env, nil, nil), false))
	})
}

// fallbackService returns a service whose Docker CLI looks for plugins in a
// single directory, which holds a fake docker-model plugin running metadata
// when it is not empty. The test is skipped if a real plugin is installed
// in a system directory and the plugin is expected to be absent.
func fallbackService(t *testing.T, metadata string) *composeService {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake plugin is a shell script")
	}
	dir := t.TempDir()
	if metadata != "" {
		script := "#!/bin/sh\ncase \"$1\" in docker-cli-plugin-metadata) " + metadata + " ;; esac\n"
		assert.NilError(t, os.WriteFile(filepath.Join(dir, "docker-model"), []byte(script), 0o700))
	}
	cli := mocks.NewMockCli(gomock.NewController(t))
	cli.EXPECT().ConfigFile().Return(&configfile.ConfigFile{CLIPluginsExtraDirs: []string{dir}}).AnyTimes()
	cli.EXPECT().CurrentContext().Return("default").AnyTimes()
	cli.EXPECT().DockerEndpoint().Return(docker.Endpoint{}).AnyTimes()
	if metadata == "" {
		if _, err := manager.GetPlugin("model", cli, &cobra.Command{}); !errdefs.IsNotFound(err) {
			t.Skip("a Docker Model plugin is installed on this system")
		}
	}
	return &composeService{dockerCli: cli, events: &recordedEvents{}}
}

const (
	validPluginMetadata  = `echo '{"SchemaVersion":"0.1.0","Vendor":"test","Version":"v1.2.6"}'`
	brokenPluginMetadata = `echo 'not json'`
	dmrPluginMissingText = "'models' support requires Docker Model plugin"
)

func TestLLMManFallbackWhenPluginIsMissing(t *testing.T) {
	models := types.Models{"llm": {Model: "smollm2"}}
	bindings := map[string]*types.ServiceModelConfig{"llm": nil}

	for name, env := range map[string]types.Mapping{
		"unset":      {},
		"empty":      {modelProviderEnv: ""},
		"whitespace": {modelProviderEnv: "  "},
	} {
		t.Run("llmman is selected and used when the provider is "+name, func(t *testing.T) {
			fake := newFakeLLMMan(t)
			s := fallbackService(t, "")
			project := llmmanProject(env, models, bindings)

			m, err := s.newModelAPI(project)
			assert.NilError(t, err)
			_, ok := m.(*llmmanModelAPI)
			assert.Assert(t, ok, "got %T", m)

			assert.NilError(t, s.ensureModels(t.Context(), project, false))
			assert.DeepEqual(t, fake.log(t), []string{"ps", "list --format {{.Name}}", "pull smollm2"})
			url := project.Services["app"].Environment["LLM_URL"]
			assert.Assert(t, url != nil && *url == "http://host.docker.internal:17434/v1")
		})
	}

	t.Run("explicit docker-model-runner never falls back", func(t *testing.T) {
		fake := newFakeLLMMan(t)
		s := fallbackService(t, "")
		project := llmmanProject(types.Mapping{modelProviderEnv: modelProviderDMR}, models, bindings)

		err := s.ensureModels(t.Context(), project, false)
		assert.Error(t, err, dmrPluginMissingText)
		assert.ErrorIs(t, err, errDMRPluginNotFound)
		assert.Equal(t, len(fake.log(t)), 0, "llmman must not be run")
	})

	t.Run("no plugin and no llmman keeps the original error", func(t *testing.T) {
		s := fallbackService(t, "")
		t.Setenv("PATH", t.TempDir())
		err := s.ensureModels(t.Context(), llmmanProject(types.Mapping{}, models, bindings), false)
		assert.Error(t, err, dmrPluginMissingText)
	})

	t.Run("Docker Model Runner wins when its plugin is installed", func(t *testing.T) {
		fake := newFakeLLMMan(t)
		s := fallbackService(t, validPluginMetadata)
		m, err := s.newModelAPI(llmmanProject(types.Mapping{}, models, bindings))
		assert.NilError(t, err)
		_, ok := m.(*dmrModelAPI)
		assert.Assert(t, ok, "got %T", m)
		m.Close()
		assert.Equal(t, len(fake.log(t)), 0, "llmman must not be run")
	})

	t.Run("a broken plugin does not fall back", func(t *testing.T) {
		fake := newFakeLLMMan(t)
		s := fallbackService(t, brokenPluginMetadata)
		_, err := s.newModelAPI(llmmanProject(types.Mapping{}, models, bindings))
		assert.ErrorContains(t, err, "failed to load Docker Model plugin")
		assert.Assert(t, !errors.Is(err, errDMRPluginNotFound))
		assert.Equal(t, len(fake.log(t)), 0, "llmman must not be run")
	})
}

func TestLLMManMissingBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH lookup differs on windows")
	}
	t.Setenv("PATH", t.TempDir())
	s := &composeService{events: &recordedEvents{}}
	project := llmmanProject(llmmanEnv(), types.Models{"llm": {Model: "smollm2"}}, map[string]*types.ServiceModelConfig{"llm": nil})
	err := s.ensureModels(t.Context(), project, false)
	assert.ErrorContains(t, err, "requires the llmman CLI in PATH")
	assert.ErrorContains(t, err, "https://github.com/llmmanorg/llmman")
}

func TestLLMManDaemonUnreachable(t *testing.T) {
	fake := newFakeLLMMan(t)
	fake.set(t, "down", "")
	s := &composeService{events: &recordedEvents{}}
	project := llmmanProject(llmmanEnv(), types.Models{"llm": {Model: "smollm2"}}, map[string]*types.ServiceModelConfig{"llm": nil})

	err := s.ensureModels(t.Context(), project, false)
	assert.ErrorContains(t, err, "start it with `llmman serve`")
	assert.ErrorContains(t, err, "llmman serve is not running")
	// the daemon is checked first: `llmman pull` would otherwise start one
	assert.DeepEqual(t, fake.log(t), []string{"ps"})
	assert.Assert(t, project.Services["app"].Environment["LLM_URL"] == nil, "no variable is set on failure")
}

func TestLLMManPullsOnlyMissingModels(t *testing.T) {
	fake := newFakeLLMMan(t)
	fake.set(t, "store", "docker.io/ai/smollm2:latest\nhf.co/unsloth/qwen:Q4_K_M\ndocker.io/ai/other:360M\n")
	fake.set(t, "resolved", strings.Join([]string{
		"smollm2=docker.io/ai/smollm2:latest|latest",
		"hf.co/unsloth/qwen:Q4_K_M=hf.co/unsloth/qwen:Q4_K_M|Q4_K_M",
		// no tag asked for and only another tag stored: :latest is what a pull fetches
		"other=docker.io/ai/other:360M|360M",
		// the explicit tag is stored under the fully qualified name
		"ai/new:1b=",
	}, "\n")+"\n")

	events := &recordedEvents{}
	s := &composeService{events: events}
	project := llmmanProject(llmmanEnv(), types.Models{
		"short":  {Model: "smollm2"},
		"exact":  {Model: "hf.co/unsloth/qwen:Q4_K_M"},
		"tagged": {Model: "docker.io/ai/other:360M"},
		"latest": {Model: "other"},
		"new":    {Model: "ai/new:1b"},
	}, map[string]*types.ServiceModelConfig{"short": nil})
	assert.NilError(t, s.ensureModels(t.Context(), project, false))

	var pulls []string
	for _, call := range fake.log(t) {
		if strings.HasPrefix(call, "pull ") {
			pulls = append(pulls, call)
		}
	}
	slices.Sort(pulls)
	assert.DeepEqual(t, pulls, []string{"pull ai/new:1b", "pull other"})

	assert.Assert(t, events.has("latest", api.Working, api.StatusPulling))
	assert.Assert(t, events.has("latest", api.Working, api.StatusPulled))
	assert.Assert(t, events.has("latest", api.Done, api.StatusConfigured))
	assert.Assert(t, !events.has("short", api.Working, api.StatusPulling), "an available model is not pulled")
	assert.Assert(t, events.has("short", api.Done, api.StatusConfigured))
}

func TestLLMManEmptyStoreSkipsResolution(t *testing.T) {
	fake := newFakeLLMMan(t)
	s := &composeService{events: &recordedEvents{}}
	project := llmmanProject(llmmanEnv(), types.Models{"llm": {Model: "smollm2"}}, map[string]*types.ServiceModelConfig{"llm": nil})
	assert.NilError(t, s.ensureModels(t.Context(), project, false))
	assert.DeepEqual(t, fake.log(t), []string{"ps", "list --format {{.Name}}", "pull smollm2"})
}

func TestLLMManPullFailure(t *testing.T) {
	fake := newFakeLLMMan(t)
	fake.set(t, "pullfail", "")
	events := &recordedEvents{}
	s := &composeService{events: events}
	project := llmmanProject(llmmanEnv(), types.Models{"llm": {Model: "smollm2"}}, map[string]*types.ServiceModelConfig{"llm": nil})

	err := s.ensureModels(t.Context(), project, false)
	assert.ErrorContains(t, err, "registry exploded")
	assert.Assert(t, events.has("llm", api.Error, api.StatusError), "pull error is reported")
	assert.Assert(t, !events.has("llm", api.Working, api.StatusPulled))
	assert.Assert(t, !events.has("llm", api.Done, api.StatusConfigured))
}

// TestLLMManPullDrainsOutputTooLongToScan: a line past the scanner's limit stops
// it reading, and the child must not then block on a full pipe.
func TestLLMManPullDrainsOutputTooLongToScan(t *testing.T) {
	fake := newFakeLLMMan(t)
	fake.set(t, "longline", "")
	s := &composeService{events: &recordedEvents{}}
	project := llmmanProject(llmmanEnv(), types.Models{"llm": {Model: "smollm2"}}, map[string]*types.ServiceModelConfig{"llm": nil})

	done := make(chan error, 1)
	go func() { done <- s.ensureModels(t.Context(), project, false) }()
	select {
	case err := <-done:
		assert.NilError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("pull blocked: the output nobody reads filled the pipe")
	}
}

// TestLLMManPullFailureHintsAtDockerHubReferences: `ai/<name>` is how Docker Model
// Runner spells Docker Hub's ai namespace, which llmman reads as a Hugging Face repository.
func TestLLMManPullFailureHintsAtDockerHubReferences(t *testing.T) {
	fake := newFakeLLMMan(t)
	fake.set(t, "pullfail", "")
	s := &composeService{events: &recordedEvents{}}

	project := llmmanProject(llmmanEnv(), types.Models{"llm": {Model: "ai/smollm2"}}, map[string]*types.ServiceModelConfig{"llm": nil})
	err := s.ensureModels(t.Context(), project, false)
	assert.ErrorContains(t, err, "registry exploded")
	assert.ErrorContains(t, err, `use "smollm2" or "docker.io/ai/smollm2" for Docker Hub`)

	project = llmmanProject(llmmanEnv(), types.Models{"llm": {Model: "unsloth/qwen"}}, map[string]*types.ServiceModelConfig{"llm": nil})
	err = s.ensureModels(t.Context(), project, false)
	assert.ErrorContains(t, err, "registry exploded")
	assert.Assert(t, !strings.Contains(err.Error(), "Docker Hub"), err.Error())
}

// TestLLMManListFailureIsNotFatal: when the store cannot be listed, every model is pulled.
func TestLLMManListFailureIsNotFatal(t *testing.T) {
	fake := newFakeLLMMan(t)
	fake.set(t, "listfail", "")
	s := &composeService{events: &recordedEvents{}}
	project := llmmanProject(llmmanEnv(), types.Models{"llm": {Model: "smollm2"}}, map[string]*types.ServiceModelConfig{"llm": nil})
	assert.NilError(t, s.ensureModels(t.Context(), project, false))
	assert.DeepEqual(t, fake.log(t), []string{"ps", "list --format {{.Name}}", "pull smollm2"})
}

func TestLLMManResolveFailure(t *testing.T) {
	fake := newFakeLLMMan(t)
	fake.set(t, "store", "docker.io/ai/other:latest\n")
	fake.set(t, "resolvefail", "")
	s := &composeService{events: &recordedEvents{}}
	project := llmmanProject(llmmanEnv(), types.Models{"llm": {Model: "smollm2"}}, map[string]*types.ServiceModelConfig{"llm": nil})
	err := s.ensureModels(t.Context(), project, false)
	assert.ErrorContains(t, err, "error checking model smollm2")
	assert.ErrorContains(t, err, "cannot resolve")
}

func TestLLMManQuietPull(t *testing.T) {
	newFakeLLMMan(t)
	model := types.ModelConfig{Name: "llm", Model: "smollm2"}
	for quiet, want := range map[bool]int{false: 3, true: 1} {
		events := &recordedEvents{}
		s := &composeService{events: events}
		m, err := s.newLLMManModelAPI(llmmanProject(llmmanEnv(), nil, nil))
		assert.NilError(t, err)
		assert.NilError(t, m.PullModel(t.Context(), model, quiet, events))
		// one event when the pull starts, then one per line llmman prints
		assert.Equal(t, events.count("llm", api.Working, api.StatusPulling), want, "quiet=%v", quiet)
	}
}

func TestLLMManCommandFailures(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on the false command")
	}
	ctx := t.Context()
	events := &recordedEvents{}
	model := types.ModelConfig{Name: "llm", Model: "smollm2"}
	noop := func(context.Context, *exec.Cmd) error { return nil }
	errBoom := errors.New("boom")

	t.Run("failure without output", func(t *testing.T) {
		// false exits 1 with nothing on stdout and stderr
		m := &llmmanModelAPI{path: "false", prepare: noop}
		assert.ErrorContains(t, m.Check(ctx), "exit status 1")
		_, err := m.ListModels(ctx)
		assert.ErrorContains(t, err, "error checking available models: exit status 1")
		_, err = m.HasModel(ctx, []string{"other"}, "smollm2")
		assert.ErrorContains(t, err, "error checking model smollm2: exit status 1")
		assert.Error(t, m.PullModel(ctx, model, false, events), "exit status 1")
	})

	t.Run("prepare failure", func(t *testing.T) {
		m := &llmmanModelAPI{path: "false", prepare: func(context.Context, *exec.Cmd) error { return errBoom }}
		_, err := m.run(ctx, "ps")
		assert.ErrorIs(t, err, errBoom)
		assert.ErrorIs(t, m.PullModel(ctx, model, false, events), errBoom)
	})

	t.Run("stdout cannot be piped", func(t *testing.T) {
		m := &llmmanModelAPI{path: "false", prepare: func(_ context.Context, cmd *exec.Cmd) error {
			cmd.Stdout = io.Discard
			return nil
		}}
		assert.ErrorContains(t, m.PullModel(ctx, model, false, events), "Stdout already set")
	})

	t.Run("binary cannot start", func(t *testing.T) {
		m := &llmmanModelAPI{path: filepath.Join(t.TempDir(), "missing"), prepare: noop}
		assert.ErrorIs(t, m.PullModel(ctx, model, false, events), fs.ErrNotExist)
	})
}

func TestLLMManVariables(t *testing.T) {
	newFakeLLMMan(t)
	s := &composeService{events: &recordedEvents{}}

	tests := []struct {
		name string
		host string
		url  string
	}{
		{"default", "", "http://host.docker.internal:17434/v1"},
		{"loopback with port", "127.0.0.1:8080", "http://host.docker.internal:8080/v1"},
		{"wildcard", "0.0.0.0:17434", "http://host.docker.internal:17434/v1"},
		{"localhost", "localhost", "http://host.docker.internal:17434/v1"},
		{"ipv6 loopback", "[::1]:9000", "http://host.docker.internal:9000/v1"},
		{"port only", ":9000", "http://host.docker.internal:9000/v1"},
		{"lan address", "192.168.1.20:17434", "http://192.168.1.20:17434/v1"},
		{"remote host with scheme", "http://gpu-box", "http://gpu-box:80/v1"},
		{"tls", "https://gpu.example.com", "https://gpu.example.com:443/v1"},
		{"quoted and with path", `"gpu.example.com:1234/ignored"`, "http://gpu.example.com:1234/v1"},
		{"ipv6 remote", "[2001:db8::1]:17434", "http://[2001:db8::1]:17434/v1"},
		{"invalid port", "gpu:nope", "http://gpu:17434/v1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := llmmanEnv()
			if tt.host != "" {
				env[llmmanHostEnv] = tt.host
			}
			project := llmmanProject(env,
				types.Models{"my-llm": {Model: "smollm2"}, "embed": {Model: "hf.co/org/embed-GGUF"}},
				map[string]*types.ServiceModelConfig{
					"my-llm": nil,
					"embed":  {ModelVariable: "EMBED_NAME", EndpointVariable: "EMBED_BASE_URL"},
				})
			assert.NilError(t, s.ensureModels(t.Context(), project, false))
			envs := project.Services["app"].Environment
			assert.Equal(t, *envs["MY_LLM_MODEL"], "smollm2")
			assert.Equal(t, *envs["MY_LLM_URL"], tt.url)
			assert.Equal(t, *envs["EMBED_NAME"], "hf.co/org/embed-GGUF")
			assert.Equal(t, *envs["EMBED_BASE_URL"], tt.url)
			assert.Assert(t, envs["EMBED_MODEL"] == nil && envs["EMBED_URL"] == nil, "overridden names are not set")
		})
	}
}

func TestLLMManPropagatesProjectEnvironment(t *testing.T) {
	fake := newFakeLLMMan(t)
	s := &composeService{events: &recordedEvents{}}
	project := llmmanProject(llmmanEnv("LLMMAN_HOST", "10.0.0.5:9999", "LLMMAN_API_KEY", "s3cret"),
		types.Models{"llm": {Model: "smollm2"}}, map[string]*types.ServiceModelConfig{"llm": nil})
	assert.NilError(t, s.ensureModels(t.Context(), project, false))

	data, err := os.ReadFile(fake.calls)
	assert.NilError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	assert.Assert(t, len(lines) >= 3)
	for _, line := range lines {
		assert.Assert(t, strings.HasSuffix(line, "[host=10.0.0.5:9999 key=s3cret]"), line)
	}
	// the key is for the CLI only: it is not handed to services
	for _, v := range project.Services["app"].Environment {
		assert.Assert(t, v == nil || *v != "s3cret")
	}
}

// TestLLMManUnsupportedSettingsAreIgnored: llmman has no per-model context size
// nor runtime flags, they are reported and nothing is run for them.
func TestLLMManUnsupportedSettingsAreIgnored(t *testing.T) {
	fake := newFakeLLMMan(t)
	fake.set(t, "store", "docker.io/ai/smollm2:latest\n")
	events := &recordedEvents{}
	s := &composeService{events: events}
	project := llmmanProject(llmmanEnv(), types.Models{
		"tuned": {Model: "docker.io/ai/smollm2:latest", ContextSize: 4096, RuntimeFlags: []string{"--no-mmap"}},
		"plain": {Model: "docker.io/ai/smollm2:latest"},
	}, map[string]*types.ServiceModelConfig{"tuned": nil, "plain": nil})
	assert.NilError(t, s.ensureModels(t.Context(), project, false))

	assert.Assert(t, events.has("tuned", api.Warning, "Ignored context_size"))
	assert.Assert(t, events.has("tuned", api.Warning, "Ignored runtime_flags"))
	details := strings.Join(events.details("tuned", api.Warning), "\n")
	assert.Assert(t, strings.Contains(details, "LLMMAN_CONTEXT_LENGTH"), details)
	assert.Equal(t, len(events.details("plain", api.Warning)), 0)
	assert.Assert(t, events.has("tuned", api.Done, api.StatusConfigured), "the model is still usable")

	for _, call := range fake.log(t) {
		assert.Assert(t, !strings.Contains(call, "4096") && !strings.Contains(call, "no-mmap"), call)
	}
	assert.DeepEqual(t, fake.log(t), []string{"ps", "list --format {{.Name}}"})
}

// TestLLMManDryRunDoesNotPull: llmman is an external process the dry-run API
// client cannot intercept, so only its read-only commands (ps, list) may run.
func TestLLMManDryRunDoesNotPull(t *testing.T) {
	fake := newFakeLLMMan(t)
	events := &recordedEvents{}
	s := &composeService{events: events, dryRun: true}
	project := llmmanProject(llmmanEnv(), types.Models{"llm": {Model: "smollm2"}}, map[string]*types.ServiceModelConfig{"llm": nil})
	assert.NilError(t, s.ensureModels(t.Context(), project, false))

	for _, call := range fake.log(t) {
		assert.Assert(t, strings.HasPrefix(call, "ps") || strings.HasPrefix(call, "list"), "%q is not read-only", call)
	}
	assert.Assert(t, events.has("llm", api.Working, api.StatusPulled), "the pull is still reported")
	url := project.Services["app"].Environment["LLM_URL"]
	assert.Assert(t, url != nil && *url == "http://host.docker.internal:17434/v1", "model variables are still computed")

	// a daemon which is down is reported in dry-run mode too
	fake.set(t, "down", "")
	err := s.ensureModels(t.Context(), project, false)
	assert.ErrorContains(t, err, "start it with `llmman serve`")
}
