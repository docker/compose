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

package charts

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/kube/charts/helm"
	"github.com/docker/compose-cli/kube/charts/kubernetes"
	chart "helm.sh/helm/v3/pkg/chart"
	util "helm.sh/helm/v3/pkg/chartutil"
	helmenv "helm.sh/helm/v3/pkg/cli"
)

type SDK struct {
	h           *helm.HelmActions
	environment map[string]string
}

func NewSDK(ctx store.KubeContext) (SDK, error) {
	return SDK{
		environment: environment(),
		h:           helm.NewHelmActions(nil),
	}, nil
}

// Install deploys a Compose stack
func (s SDK) Install(project *types.Project) error {
	chart, err := s.GetChartInMemory(project)
	if err != nil {
		return err
	}
	return s.h.InstallChart(project.Name, chart)
}

// Uninstall removes a runnign compose stack
func (s SDK) Uninstall(projectName string) error {
	return s.h.Uninstall(projectName)
}

// List returns a list of compose stacks
func (s SDK) List(projectName string) ([]compose.Stack, error) {
	return s.h.ListReleases()
}

// GetDefault initializes Helm EnvSettings
func (s SDK) GetDefaultEnv() *helmenv.EnvSettings {
	return helmenv.New()
}

func (s SDK) GetChartInMemory(project *types.Project) (*chart.Chart, error) {
	// replace _ with - in volume names
	for k, v := range project.Volumes {
		volumeName := strings.ReplaceAll(k, "_", "-")
		if volumeName != k {
			project.Volumes[volumeName] = v
			delete(project.Volumes, k)
		}
	}
	objects, err := kubernetes.MapToKubernetesObjects(project)
	if err != nil {
		return nil, err
	}
	//in memory files
	return helm.ConvertToChart(project.Name, objects)
}

func (s SDK) SaveChart(project *types.Project, dest string) error {
	chart, err := s.GetChartInMemory(project)
	if err != nil {
		return err
	}
	return util.SaveDir(chart, dest)
}

func (s SDK) GenerateChart(project *types.Project, dirname string) error {
	if strings.Contains(dirname, ".") {
		splits := strings.SplitN(dirname, ".", 2)
		dirname = splits[0]
	}

	dirname = filepath.Dir(dirname)
	return s.SaveChart(project, dirname)
}

func environment() map[string]string {
	vars := make(map[string]string)
	env := os.Environ()
	for _, v := range env {
		k := strings.SplitN(v, "=", 2)
		vars[k[0]] = k[1]
	}
	return vars
}
