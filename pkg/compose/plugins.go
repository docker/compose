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
	"os/exec"
	"sync"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"

	"github.com/docker/compose/v5/internal/provider"
)

// Running the provider plugins, and the message protocol they speak, is done
// in internal/provider: composeService hands that package the engine it works
// with. The types and constants below stay here as the public face of the
// protocol (see docs/extension.md).

// JsonMessage is a message a provider plugin writes on its standard output.
type JsonMessage = provider.JsonMessage

// ProviderMetadata is what a provider plugin declares about itself and its commands.
type ProviderMetadata = provider.ProviderMetadata

// CommandMetadata is what a provider plugin declares about one of its commands.
type CommandMetadata = provider.CommandMetadata

// ParameterMetadata describes one parameter of a provider plugin command.
type ParameterMetadata = provider.ParameterMetadata

// The types of the messages of the provider protocol, and the variable
// announcing them to the plugin.
const (
	ErrorType                  = provider.ErrorType
	InfoType                   = provider.InfoType
	SetEnvType                 = provider.SetEnvType
	RawSetEnvType              = provider.RawSetEnvType
	DebugType                  = provider.DebugType
	PublishEndpointType        = provider.PublishEndpointType
	GetServiceConfigType       = provider.GetServiceConfigType
	GetRelayInfoType           = provider.GetRelayInfoType
	GetImageType               = provider.GetImageType
	ImageStreamType            = provider.ImageStreamType
	ComposeProviderMessagesEnv = provider.ComposeProviderMessagesEnv
)

func (s *composeService) providerService() *provider.Service {
	return provider.New(providerBackend{relayBackend{s}})
}

func (s *composeService) runPlugin(ctx context.Context, project *types.Project, service types.ServiceConfig, pluginCommand string, extraArgs ...string) error {
	return s.providerService().Run(ctx, project, service, pluginCommand, extraArgs...)
}

// providerBackend is the engine of a composeService, as internal/provider
// sees it: what the relay needs, plus what finds and runs the plugin.
type providerBackend struct {
	relayBackend
}

func (b providerBackend) CLI() command.Cli {
	return b.s.dockerCli
}

func (b providerBackend) PrepareShellOut(ctx context.Context, env types.Mapping, cmd *exec.Cmd) error {
	return b.s.prepareShellOut(ctx, env, cmd)
}

func (b providerBackend) DryRun() bool {
	return b.s.dryRun
}

func (b providerBackend) ProjectLock() sync.Locker {
	return &b.s.pluginMu
}
