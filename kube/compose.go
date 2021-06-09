// +build kube

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

package kube

import (
	"context"
	"fmt"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/pkg/errors"

	"github.com/docker/compose-cli/api/compose"
	apicontext "github.com/docker/compose-cli/api/context"
	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/api/errdefs"
	"github.com/docker/compose-cli/api/progress"
	"github.com/docker/compose-cli/kube/client"
	"github.com/docker/compose-cli/kube/helm"
	"github.com/docker/compose-cli/kube/resources"
	"github.com/docker/compose-cli/utils"
)

type composeService struct {
	sdk    *helm.Actions
	client *client.KubeClient
}

// NewComposeService create a kubernetes implementation of the compose.Service API
func NewComposeService() (compose.Service, error) {
	contextStore := store.Instance()
	currentContext := apicontext.Current()
	var kubeContext store.KubeContext

	if err := contextStore.GetEndpoint(currentContext, &kubeContext); err != nil {
		return nil, err
	}
	config, err := resources.LoadConfig(kubeContext)
	if err != nil {
		return nil, err
	}
	actions, err := helm.NewActions(config)
	if err != nil {
		return nil, err
	}
	apiClient, err := client.NewKubeClient(config)
	if err != nil {
		return nil, err
	}

	return &composeService{
		sdk:    actions,
		client: apiClient,
	}, nil
}

// Up executes the equivalent to a `compose up`
func (s *composeService) Up(ctx context.Context, project *types.Project, options compose.UpOptions) error {
	return progress.Run(ctx, func(ctx context.Context) error {
		return s.up(ctx, project)
	})
}

func (s *composeService) up(ctx context.Context, project *types.Project) error {
	w := progress.ContextWriter(ctx)

	eventName := "Convert Compose file to Helm charts"
	w.Event(progress.CreatingEvent(eventName))

	chart, err := helm.GetChartInMemory(project)
	if err != nil {
		return err
	}
	w.Event(progress.NewEvent(eventName, progress.Done, ""))

	stack, err := s.sdk.Get(project.Name)
	if err != nil || stack == nil {
		// install stack
		eventName = "Install Compose stack"
		w.Event(progress.CreatingEvent(eventName))

		err = s.sdk.InstallChart(project.Name, chart, func(format string, v ...interface{}) {
			message := fmt.Sprintf(format, v...)
			w.Event(progress.NewEvent(eventName, progress.Done, message))
		})

	} else {
		//update stack
		eventName = "Updating Compose stack"
		w.Event(progress.CreatingEvent(eventName))

		err = s.sdk.UpdateChart(project.Name, chart, func(format string, v ...interface{}) {
			message := fmt.Sprintf(format, v...)
			w.Event(progress.NewEvent(eventName, progress.Done, message))
		})
	}
	if err != nil {
		return err
	}

	w.Event(progress.NewEvent(eventName, progress.Done, ""))

	return s.client.WaitForPodState(ctx, client.WaitForStatusOptions{
		ProjectName: project.Name,
		Services:    project.ServiceNames(),
		Status:      compose.RUNNING,
		Log: func(pod string, stateReached bool, message string) {
			state := progress.Done
			if !stateReached {
				state = progress.Working
			}
			w.Event(progress.NewEvent(pod, state, message))
		},
	})
}

// Down executes the equivalent to a `compose down`
func (s *composeService) Down(ctx context.Context, projectName string, options compose.DownOptions) error {
	if options.Volumes {
		return errors.Wrap(errdefs.ErrNotImplemented, "--volumes option is not supported on Kubernetes")
	}
	if options.Images != "" {
		return errors.Wrap(errdefs.ErrNotImplemented, "--rmi option is not supported on Kubernetes")
	}
	return progress.Run(ctx, func(ctx context.Context) error {
		return s.down(ctx, projectName, options)
	})
}

func (s *composeService) down(ctx context.Context, projectName string, options compose.DownOptions) error {
	w := progress.ContextWriter(ctx)
	eventName := fmt.Sprintf("Remove %s", projectName)
	w.Event(progress.CreatingEvent(eventName))

	logger := func(format string, v ...interface{}) {
		message := fmt.Sprintf(format, v...)
		if strings.Contains(message, "Starting delete") {
			action := strings.Replace(message, "Starting delete for", "Delete", 1)

			w.Event(progress.CreatingEvent(action))
			w.Event(progress.NewEvent(action, progress.Done, ""))
			return
		}
		w.Event(progress.NewEvent(eventName, progress.Working, message))
	}
	err := s.sdk.Uninstall(projectName, logger)
	if err != nil {
		return err
	}

	events := []string{}
	err = s.client.WaitForPodState(ctx, client.WaitForStatusOptions{
		ProjectName: projectName,
		Services:    nil,
		Status:      compose.REMOVING,
		Timeout:     options.Timeout,
		Log: func(pod string, stateReached bool, message string) {
			state := progress.Done
			if !stateReached {
				state = progress.Working
			}
			w.Event(progress.NewEvent(pod, state, message))
			if !utils.StringContains(events, pod) {
				events = append(events, pod)
			}
		},
	})
	if err != nil {
		return err
	}
	for _, e := range events {
		w.Event(progress.NewEvent(e, progress.Done, ""))
	}
	w.Event(progress.NewEvent(eventName, progress.Done, ""))
	return nil
}

// List executes the equivalent to a `docker stack ls`
func (s *composeService) List(ctx context.Context, opts compose.ListOptions) ([]compose.Stack, error) {
	return s.sdk.ListReleases()
}

// Build executes the equivalent to a `compose build`
func (s *composeService) Build(ctx context.Context, project *types.Project, options compose.BuildOptions) error {
	return errdefs.ErrNotImplemented
}

// Push executes the equivalent ot a `compose push`
func (s *composeService) Push(ctx context.Context, project *types.Project, options compose.PushOptions) error {
	return errdefs.ErrNotImplemented
}

// Pull executes the equivalent of a `compose pull`
func (s *composeService) Pull(ctx context.Context, project *types.Project, options compose.PullOptions) error {
	return errdefs.ErrNotImplemented
}

// Create executes the equivalent to a `compose create`
func (s *composeService) Create(ctx context.Context, project *types.Project, opts compose.CreateOptions) error {
	return errdefs.ErrNotImplemented
}

// Start executes the equivalent to a `compose start`
func (s *composeService) Start(ctx context.Context, project *types.Project, options compose.StartOptions) error {
	return errdefs.ErrNotImplemented
}

// Restart executes the equivalent to a `compose restart`
func (s *composeService) Restart(ctx context.Context, project *types.Project, options compose.RestartOptions) error {
	return errdefs.ErrNotImplemented
}

// Stop executes the equivalent to a `compose stop`
func (s *composeService) Stop(ctx context.Context, project *types.Project, options compose.StopOptions) error {
	return errdefs.ErrNotImplemented
}

// Copy copies a file/folder between a service container and the local filesystem
func (s *composeService) Copy(ctx context.Context, project *types.Project, options compose.CopyOptions) error {
	return errdefs.ErrNotImplemented
}

// Logs executes the equivalent to a `compose logs`
func (s *composeService) Logs(ctx context.Context, projectName string, consumer compose.LogConsumer, options compose.LogOptions) error {
	if len(options.Services) > 0 {
		consumer = utils.FilteredLogConsumer(consumer, options.Services)
	}
	return s.client.GetLogs(ctx, projectName, consumer, options.Follow)
}

// Ps executes the equivalent to a `compose ps`
func (s *composeService) Ps(ctx context.Context, projectName string, options compose.PsOptions) ([]compose.ContainerSummary, error) {
	return s.client.GetContainers(ctx, projectName, options.All)
}

// Convert translate compose model into backend's native format
func (s *composeService) Convert(ctx context.Context, project *types.Project, options compose.ConvertOptions) ([]byte, error) {

	chart, err := helm.GetChartInMemory(project)
	if err != nil {
		return nil, err
	}

	if options.Output != "" {
		_, err := helm.SaveChart(chart, options.Output)
		return nil, err
	}

	buff := []byte{}
	for _, f := range chart.Raw {
		header := "\n" + f.Name + "\n" + strings.Repeat("-", len(f.Name)) + "\n"
		buff = append(buff, []byte(header)...)
		buff = append(buff, f.Data...)
		buff = append(buff, []byte("\n")...)
	}
	return buff, nil
}

func (s *composeService) Kill(ctx context.Context, project *types.Project, options compose.KillOptions) error {
	return errdefs.ErrNotImplemented
}

// RunOneOffContainer creates a service oneoff container and starts its dependencies
func (s *composeService) RunOneOffContainer(ctx context.Context, project *types.Project, opts compose.RunOptions) (int, error) {
	return 0, errdefs.ErrNotImplemented
}

func (s *composeService) Remove(ctx context.Context, project *types.Project, options compose.RemoveOptions) error {
	return errdefs.ErrNotImplemented
}

// Exec executes a command in a running service container
func (s *composeService) Exec(ctx context.Context, project *types.Project, opts compose.RunOptions) (int, error) {
	return 0, s.client.Exec(ctx, project.Name, opts)
}

func (s *composeService) Pause(ctx context.Context, project string, options compose.PauseOptions) error {
	return errdefs.ErrNotImplemented
}

func (s *composeService) UnPause(ctx context.Context, project string, options compose.PauseOptions) error {
	return errdefs.ErrNotImplemented
}

func (s *composeService) Top(ctx context.Context, projectName string, services []string) ([]compose.ContainerProcSummary, error) {
	return nil, errdefs.ErrNotImplemented
}

func (s *composeService) Events(ctx context.Context, project string, options compose.EventsOptions) error {
	return errdefs.ErrNotImplemented
}

func (s *composeService) Port(ctx context.Context, project string, service string, port int, options compose.PortOptions) (string, int, error) {
	return "", 0, errdefs.ErrNotImplemented
}

func (s *composeService) Images(ctx context.Context, projectName string, options compose.ImagesOptions) ([]compose.ImageSummary, error) {
	return nil, errdefs.ErrNotImplemented
}
