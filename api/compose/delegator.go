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
)

// ServiceDelegator implements Service by delegating to another implementation. This allows lazy init
type ServiceDelegator struct {
	Delegate Service
}

//Build implements Service interface
func (s *ServiceDelegator) Build(ctx context.Context, project *types.Project, options BuildOptions) error {
	return s.Delegate.Build(ctx, project, options)
}

//Push implements Service interface
func (s *ServiceDelegator) Push(ctx context.Context, project *types.Project, options PushOptions) error {
	return s.Delegate.Push(ctx, project, options)
}

//Pull implements Service interface
func (s *ServiceDelegator) Pull(ctx context.Context, project *types.Project, options PullOptions) error {
	return s.Delegate.Pull(ctx, project, options)
}

//Create implements Service interface
func (s *ServiceDelegator) Create(ctx context.Context, project *types.Project, options CreateOptions) error {
	return s.Delegate.Create(ctx, project, options)
}

//Start implements Service interface
func (s *ServiceDelegator) Start(ctx context.Context, project *types.Project, options StartOptions) error {
	return s.Delegate.Start(ctx, project, options)
}

//Restart implements Service interface
func (s *ServiceDelegator) Restart(ctx context.Context, project *types.Project, options RestartOptions) error {
	return s.Delegate.Restart(ctx, project, options)
}

//Stop implements Service interface
func (s *ServiceDelegator) Stop(ctx context.Context, project *types.Project, options StopOptions) error {
	return s.Delegate.Stop(ctx, project, options)
}

//Up implements Service interface
func (s *ServiceDelegator) Up(ctx context.Context, project *types.Project, options UpOptions) error {
	return s.Delegate.Up(ctx, project, options)
}

//Down implements Service interface
func (s *ServiceDelegator) Down(ctx context.Context, project string, options DownOptions) error {
	return s.Delegate.Down(ctx, project, options)
}

//Logs implements Service interface
func (s *ServiceDelegator) Logs(ctx context.Context, project string, consumer LogConsumer, options LogOptions) error {
	return s.Delegate.Logs(ctx, project, consumer, options)
}

//Ps implements Service interface
func (s *ServiceDelegator) Ps(ctx context.Context, project string, options PsOptions) ([]ContainerSummary, error) {
	return s.Delegate.Ps(ctx, project, options)
}

//List implements Service interface
func (s *ServiceDelegator) List(ctx context.Context, options ListOptions) ([]Stack, error) {
	return s.Delegate.List(ctx, options)
}

//Convert implements Service interface
func (s *ServiceDelegator) Convert(ctx context.Context, project *types.Project, options ConvertOptions) ([]byte, error) {
	return s.Delegate.Convert(ctx, project, options)
}

//Kill implements Service interface
func (s *ServiceDelegator) Kill(ctx context.Context, project *types.Project, options KillOptions) error {
	return s.Delegate.Kill(ctx, project, options)
}

//RunOneOffContainer implements Service interface
func (s *ServiceDelegator) RunOneOffContainer(ctx context.Context, project *types.Project, options RunOptions) (int, error) {
	return s.Delegate.RunOneOffContainer(ctx, project, options)
}

//Remove implements Service interface
func (s *ServiceDelegator) Remove(ctx context.Context, project *types.Project, options RemoveOptions) ([]string, error) {
	return s.Delegate.Remove(ctx, project, options)
}

//Exec implements Service interface
func (s *ServiceDelegator) Exec(ctx context.Context, project *types.Project, options RunOptions) error {
	return s.Delegate.Exec(ctx, project, options)
}

//Pause implements Service interface
func (s *ServiceDelegator) Pause(ctx context.Context, project string, options PauseOptions) error {
	return s.Delegate.Pause(ctx, project, options)
}

//UnPause implements Service interface
func (s *ServiceDelegator) UnPause(ctx context.Context, project string, options PauseOptions) error {
	return s.Delegate.UnPause(ctx, project, options)
}

//Top implements Service interface
func (s *ServiceDelegator) Top(ctx context.Context, project string, services []string) ([]ContainerProcSummary, error) {
	return s.Delegate.Top(ctx, project, services)
}

//Events implements Service interface
func (s *ServiceDelegator) Events(ctx context.Context, project string, options EventsOptions) error {
	return s.Delegate.Events(ctx, project, options)
}

//Port implements Service interface
func (s *ServiceDelegator) Port(ctx context.Context, project string, service string, port int, options PortOptions) (string, int, error) {
	return s.Delegate.Port(ctx, project, service, port, options)
}

//Images implements Service interface
func (s *ServiceDelegator) Images(ctx context.Context, project string, options ImagesOptions) ([]ImageSummary, error) {
	return s.Delegate.Images(ctx, project, options)
}
