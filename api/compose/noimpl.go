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

	"github.com/compose-spec/compose-go/types"

	"github.com/docker/compose-cli/api/errdefs"
)

// NoImpl implements Service to return ErrNotImplemented
type NoImpl struct{}

//Build implements Service interface
func (s NoImpl) Build(ctx context.Context, project *types.Project, options BuildOptions) error {
	return errdefs.ErrNotImplemented
}

//Push implements Service interface
func (s NoImpl) Push(ctx context.Context, project *types.Project, options PushOptions) error {
	return errdefs.ErrNotImplemented
}

//Pull implements Service interface
func (s NoImpl) Pull(ctx context.Context, project *types.Project, options PullOptions) error {
	return errdefs.ErrNotImplemented
}

//Create implements Service interface
func (s NoImpl) Create(ctx context.Context, project *types.Project, options CreateOptions) error {
	return errdefs.ErrNotImplemented
}

//Start implements Service interface
func (s NoImpl) Start(ctx context.Context, project *types.Project, options StartOptions) error {
	return errdefs.ErrNotImplemented
}

//Restart implements Service interface
func (s NoImpl) Restart(ctx context.Context, project *types.Project, options RestartOptions) error {
	return errdefs.ErrNotImplemented
}

//Stop implements Service interface
func (s NoImpl) Stop(ctx context.Context, project *types.Project, options StopOptions) error {
	return errdefs.ErrNotImplemented
}

//Up implements Service interface
func (s NoImpl) Up(ctx context.Context, project *types.Project, options UpOptions) error {
	return errdefs.ErrNotImplemented
}

//Down implements Service interface
func (s NoImpl) Down(ctx context.Context, project string, options DownOptions) error {
	return errdefs.ErrNotImplemented
}

//Logs implements Service interface
func (s NoImpl) Logs(ctx context.Context, project string, consumer LogConsumer, options LogOptions) error {
	return errdefs.ErrNotImplemented
}

//Ps implements Service interface
func (s NoImpl) Ps(ctx context.Context, project string, options PsOptions) ([]ContainerSummary, error) {
	return nil, errdefs.ErrNotImplemented
}

//List implements Service interface
func (s NoImpl) List(ctx context.Context, options ListOptions) ([]Stack, error) {
	return nil, errdefs.ErrNotImplemented
}

//Convert implements Service interface
func (s NoImpl) Convert(ctx context.Context, project *types.Project, options ConvertOptions) ([]byte, error) {
	return nil, errdefs.ErrNotImplemented
}

//Kill implements Service interface
func (s NoImpl) Kill(ctx context.Context, project *types.Project, options KillOptions) error {
	return errdefs.ErrNotImplemented
}

//RunOneOffContainer implements Service interface
func (s NoImpl) RunOneOffContainer(ctx context.Context, project *types.Project, options RunOptions) (int, error) {
	return 0, errdefs.ErrNotImplemented
}

//Remove implements Service interface
func (s NoImpl) Remove(ctx context.Context, project *types.Project, options RemoveOptions) ([]string, error) {
	return nil, errdefs.ErrNotImplemented
}

//Exec implements Service interface
func (s NoImpl) Exec(ctx context.Context, project *types.Project, options RunOptions) error {
	return errdefs.ErrNotImplemented
}

//Pause implements Service interface
func (s NoImpl) Pause(ctx context.Context, project string, options PauseOptions) error {
	return errdefs.ErrNotImplemented
}

//UnPause implements Service interface
func (s NoImpl) UnPause(ctx context.Context, project string, options PauseOptions) error {
	return errdefs.ErrNotImplemented
}

//Top implements Service interface
func (s NoImpl) Top(ctx context.Context, project string, services []string) ([]ContainerProcSummary, error) {
	return nil, errdefs.ErrNotImplemented
}

//Events implements Service interface
func (s NoImpl) Events(ctx context.Context, project string, options EventsOptions) error {
	return errdefs.ErrNotImplemented
}

//Port implements Service interface
func (s NoImpl) Port(ctx context.Context, project string, service string, port int, options PortOptions) (string, int, error) {
	return "", 0, errdefs.ErrNotImplemented
}

//Images implements Service interface
func (s NoImpl) Images(ctx context.Context, project string, options ImagesOptions) ([]ImageSummary, error) {
	return nil, errdefs.ErrNotImplemented
}
