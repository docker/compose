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
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"

	"github.com/docker/compose/v5/internal/progress"
	"github.com/docker/compose/v5/pkg/api"
)

// llmman backend for the top-level `models` element.
//
// llmman (https://github.com/llmmanorg/llmman) stores models as OCI images and
// serves them through Ollama, OpenAI (/v1) and Anthropic compatible APIs.
// It is selected, instead of Docker Model Runner, by setting
// COMPOSE_MODEL_PROVIDER=llmman in the shell or in the project's .env file.
// It is also used automatically, as a fallback, when COMPOSE_MODEL_PROVIDER is
// unset or empty, the Docker Model plugin is not installed and an llmman binary
// is found in PATH. Docker Model Runner always wins when its plugin is installed,
// and an explicit COMPOSE_MODEL_PROVIDER=docker-model-runner never falls back.
//
// Compose drives the `llmman` CLI found in PATH, with the project environment
// (so LLMMAN_HOST, LLMMAN_API_KEY, LLMMAN_MODELS... are honored):
//
//   - `llmman ps` checks the daemon is reachable (and the API key accepted).
//     It never starts the daemon: run `llmman serve` first. It is checked
//     before anything else because `llmman pull` would otherwise start one.
//   - `llmman list --format ...` reads the local store to find which models are
//     already pulled. References are normalized by llmman itself
//     (`llmman list <ref>`), so only missing models are pulled.
//   - `llmman pull <ref>` fetches missing models. It is skipped on --dry-run.
//
// Models references follow llmman rules, not Docker Model Runner ones: a bare
// name such as `smollm2` is Docker Hub's `ai/` namespace, `docker.io/ai/smollm2`
// is spelled in full, but `ai/smollm2` (owner/repo without a registry) is a
// Hugging Face repository (`hf.co/ai/smollm2`).
//
// llmman has no per-model configuration: the context size is daemon-wide
// (LLMMAN_CONTEXT_LENGTH) and so are the engine arguments (e.g. LLMMAN_VLLM_ARGS),
// both read when `llmman serve` starts. `context_size` and `runtime_flags` are
// therefore ignored, with a warning.
//
// Services get <NAME>_MODEL (the reference as written in the Compose file) and
// <NAME>_URL (or the model_var and endpoint_var overrides), where the URL is
// the OpenAI compatible base URL of the daemon, as seen from a container:
// scheme://host:port/v1 built from LLMMAN_HOST (default 127.0.0.1:17434). A
// loopback or wildcard host is replaced by host.docker.internal. llmman binds
// 127.0.0.1 by default, which containers cannot reach on a Linux engine: bind
// the daemon on an address reachable from containers (LLMMAN_HOST=0.0.0.0:17434,
// which requires LLMMAN_API_KEYS or LLMMAN_AUTH=off) and map the name with
// `extra_hosts: ["host.docker.internal:host-gateway"]`. When the daemon requires
// an API key, services must be given it explicitly, Compose does not inject it.

const (
	llmmanBinary        = "llmman"
	llmmanHostEnv       = "LLMMAN_HOST"
	llmmanDefaultPort   = "17434"
	llmmanContainerHost = "host.docker.internal"
	llmmanInstallURL    = "https://github.com/llmmanorg/llmman"
)

type llmmanModelAPI struct {
	path     string
	prepare  func(ctx context.Context, cmd *exec.Cmd) error
	endpoint string
	dryRun   bool
}

func (s *composeService) newLLMManModelAPI(project *types.Project) (*llmmanModelAPI, error) {
	path, err := exec.LookPath(llmmanBinary)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("'models' support with %s=%s requires the llmman CLI in PATH, see %s",
				modelProviderEnv, modelProviderLLMMan, llmmanInstallURL)
		}
		return nil, fmt.Errorf("failed to locate llmman: %w", err)
	}
	return &llmmanModelAPI{
		path: path,
		prepare: func(ctx context.Context, cmd *exec.Cmd) error {
			return s.prepareShellOut(ctx, project.Environment, cmd)
		},
		endpoint: llmmanEndpoint(project.Environment[llmmanHostEnv]),
		dryRun:   s.dryRun,
	}, nil
}

func (m *llmmanModelAPI) Close() {}

// run executes llmman with args and returns its stdout. stderr is only used to
// report a failure, as llmman prints warnings and progress there.
func (m *llmmanModelAPI) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, m.path, args...)
	if err := m.prepare(ctx, cmd); err != nil {
		return "", err
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String() + stdout.String()); msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	return stdout.String(), nil
}

// Check verifies the llmman daemon is running, without starting it.
func (m *llmmanModelAPI) Check(ctx context.Context) error {
	if _, err := m.run(ctx, "ps"); err != nil {
		return fmt.Errorf("the llmman daemon is not reachable, start it with `llmman serve` (LLMMAN_HOST and LLMMAN_API_KEY are read from the project environment): %w", err)
	}
	return nil
}

// ListModels returns the references of the models in llmman's local store.
func (m *llmmanModelAPI) ListModels(ctx context.Context) ([]string, error) {
	out, err := m.run(ctx, "list", "--format", "{{.Name}}")
	if err != nil {
		return nil, fmt.Errorf("error checking available models: %w", err)
	}
	return nonEmptyLines(out), nil
}

// HasModel tells whether model is in the local store. An exact match with a stored
// reference is enough. Otherwise llmman is asked to resolve the reference, as the
// stored one is fully qualified (e.g. docker.io/ai/smollm2:latest) while the Compose
// file may use any shorthand llmman accepts. A false negative only costs a redundant pull.
func (m *llmmanModelAPI) HasModel(ctx context.Context, available []string, model string) (bool, error) {
	if slices.Contains(available, model) {
		return true, nil
	}
	if len(available) == 0 || strings.Contains(model, "@") {
		return false, nil
	}
	out, err := m.run(ctx, "list", model, "--format", "{{.Name}}|{{.Tag}}")
	if err != nil {
		return false, fmt.Errorf("error checking model %s: %w", model, err)
	}
	// Without an explicit tag llmman matches every tag of the repository, while
	// pulling the reference fetches :latest.
	explicitTag := strings.Contains(model[strings.LastIndex(model, "/")+1:], ":")
	for _, line := range nonEmptyLines(out) {
		_, tag, _ := strings.Cut(line, "|")
		if explicitTag || tag == "latest" {
			return true, nil
		}
	}
	return false, nil
}

func (m *llmmanModelAPI) PullModel(ctx context.Context, model types.ModelConfig, quietPull bool, events api.EventProcessor) error {
	events.On(api.Resource{
		ID:     model.Name,
		Status: api.Working,
		Text:   api.StatusPulling,
	})
	if m.dryRun {
		// llmman is an external process the dry-run API client cannot intercept
		events.On(api.Resource{ID: model.Name, Status: api.Working, Text: api.StatusPulled})
		return nil
	}

	cmd := exec.CommandContext(ctx, m.path, "pull", model.Model)
	if err := m.prepare(ctx, cmd); err != nil {
		return err
	}
	stream, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		_ = stream.Close()
		return err
	}

	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		if scanner.Text() == "" || quietPull {
			continue
		}
		events.On(api.Resource{
			ID:     model.Name,
			Status: api.Working,
			Text:   api.StatusPulling,
		})
	}
	if scanner.Err() != nil {
		// A line too long for the scanner stops it reading: drain the rest so
		// the child does not block on a full pipe.
		_, _ = io.Copy(io.Discard, stream)
	}

	if err := cmd.Wait(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			err = fmt.Errorf("%w: %s", err, msg)
		}
		if rest, ok := strings.CutPrefix(model.Model, "ai/"); ok {
			// the reference of Docker Model Runner for Docker Hub's ai namespace
			err = fmt.Errorf("%w (llmman resolves %q as a Hugging Face repository, use %q or %q for Docker Hub)",
				err, model.Model, rest, "docker.io/"+model.Model)
		}
		events.On(progress.Error(model.Name, err.Error()))
		return err
	}
	events.On(api.Resource{
		ID:     model.Name,
		Status: api.Working,
		Text:   api.StatusPulled,
	})
	return nil
}

// ConfigureModel has nothing to apply: llmman has no per-model settings, so the
// unsupported ones are reported and ignored rather than approximated.
func (m *llmmanModelAPI) ConfigureModel(_ context.Context, config types.ModelConfig, events api.EventProcessor) error {
	events.On(api.Resource{
		ID:     config.Name,
		Status: api.Working,
		Text:   api.StatusConfiguring,
	})
	if config.ContextSize > 0 {
		events.On(progress.New(config.Name, api.Warning, "Ignored context_size",
			"llmman has no per-model context size, set LLMMAN_CONTEXT_LENGTH when starting llmman serve"))
	}
	if len(config.RuntimeFlags) > 0 {
		events.On(progress.New(config.Name, api.Warning, "Ignored runtime_flags",
			"llmman has no per-model runtime flags, see the engine settings (e.g. LLMMAN_VLLM_ARGS) of llmman serve"))
	}
	events.On(api.Resource{
		ID:     config.Name,
		Status: api.Done,
		Text:   api.StatusConfigured,
	})
	return nil
}

func (m *llmmanModelAPI) SetModelVariables(_ context.Context, project *types.Project) error {
	setModelVariables(project, m.endpoint)
	return nil
}

func nonEmptyLines(out string) []string {
	var lines []string
	for line := range strings.SplitSeq(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// llmmanEndpoint returns the OpenAI compatible base URL of the llmman daemon as
// seen from a container. host is parsed like llmman does for LLMMAN_HOST
// ([scheme://][host][:port]): the scheme is https only when spelled so, a missing
// host is 127.0.0.1 and a missing port is 17434, or 80/443 when a scheme is given.
func llmmanEndpoint(host string) string {
	host = strings.Trim(strings.TrimSpace(host), `"'`)
	scheme, port := "http", llmmanDefaultPort
	if s, rest, ok := strings.Cut(host, "://"); ok {
		host = rest
		switch strings.ToLower(s) {
		case "https":
			scheme, port = "https", "443"
		case "http":
			port = "80"
		}
	}
	// a trailing path is discarded
	host, _, _ = strings.Cut(host, "/")
	if h, p, err := net.SplitHostPort(host); err == nil {
		host = h
		if n, err := strconv.ParseUint(p, 10, 16); err == nil {
			port = strconv.FormatUint(n, 10)
		}
	} else {
		// no port, or a bare IPv6 literal
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	if isLoopbackOrWildcard(host) {
		host = llmmanContainerHost
	}
	return scheme + "://" + net.JoinHostPort(host, port) + "/v1"
}

// isLoopbackOrWildcard tells whether host (or the wildcard llmman rewrites to
// loopback when connecting) designates the machine running the daemon, which
// is not what a container sees when it uses that name.
func isLoopbackOrWildcard(host string) bool {
	if host == "" || strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}
