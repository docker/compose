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
	"io"
	"os"

	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/context/docker"
	"github.com/docker/cli/cli/context/store"
	manifeststore "github.com/docker/cli/cli/manifest/store"
	client2 "github.com/docker/cli/cli/registry/client"
	"github.com/docker/cli/cli/streams"
	"github.com/docker/cli/cli/trust"
	"github.com/docker/docker/client"
	notaryclient "github.com/theupdateframework/notary/client"
)

// dockerCli creates a (partial) command.Cli so we can use docker/cli commands
func (s *composeService) dockerCli() command.Cli {
	return dockerCli{
		apiClient:  s.apiClient,
		configFile: s.configFile,
	}
}

type dockerCli struct {
	apiClient  client.APIClient
	configFile *configfile.ConfigFile
}

func (d dockerCli) Client() client.APIClient {
	return d.apiClient
}

func (d dockerCli) ConfigFile() *configfile.ConfigFile {
	return d.configFile
}

func (d dockerCli) Out() *streams.Out {
	return streams.NewOut(os.Stdout)
}

func (d dockerCli) Err() io.Writer {
	return os.Stderr
}

func (d dockerCli) In() *streams.In {
	return streams.NewIn(os.Stdin)
}

func (d dockerCli) NotaryClient(imgRefAndAuth trust.ImageRefAndAuth, actions []string) (notaryclient.Repository, error) {
	//TODO implement me
	panic("implement me")
}

func (d dockerCli) DefaultVersion() string {
	//TODO implement me
	panic("implement me")
}

func (d dockerCli) ManifestStore() manifeststore.Store {
	//TODO implement me
	panic("implement me")
}

func (d dockerCli) RegistryClient(b bool) client2.RegistryClient {
	//TODO implement me
	panic("implement me")
}

func (d dockerCli) ContentTrustEnabled() bool {
	//TODO implement me
	panic("implement me")
}

func (d dockerCli) ContextStore() store.Store {
	//TODO implement me
	panic("implement me")
}

func (d dockerCli) CurrentContext() string {
	//TODO implement me
	panic("implement me")
}

func (d dockerCli) StackOrchestrator(flagValue string) (command.Orchestrator, error) {
	//TODO implement me
	panic("implement me")
}

func (d dockerCli) DockerEndpoint() docker.Endpoint {
	//TODO implement me
	panic("implement me")
}

func (d dockerCli) SetIn(in *streams.In) {
	// Nop
}

func (d dockerCli) Apply(ops ...command.DockerCliOption) error {
	// Nop
	return nil
}

func (d dockerCli) ServerInfo() command.ServerInfo {
	panic("implement me")
}

func (d dockerCli) ClientInfo() command.ClientInfo {
	panic("implement me")
}

var _ command.Cli = dockerCli{}
