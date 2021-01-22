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

package helm

import (
	"errors"
	"log"

	"github.com/docker/compose-cli/api/compose"
	action "helm.sh/helm/v3/pkg/action"
	chart "helm.sh/helm/v3/pkg/chart"
	loader "helm.sh/helm/v3/pkg/chart/loader"
	env "helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
)

type HelmActions struct {
	Config         *action.Configuration
	Settings       *env.EnvSettings
	kube_conn_init bool
}

func NewHelmActions(settings *env.EnvSettings) *HelmActions {
	if settings == nil {
		settings = env.New()
	}
	return &HelmActions{
		Config:         new(action.Configuration),
		Settings:       settings,
		kube_conn_init: false,
	}
}

func (hc *HelmActions) initKubeClient() error {
	if hc.kube_conn_init {
		return nil
	}
	if err := hc.Config.Init(
		hc.Settings.RESTClientGetter(),
		hc.Settings.Namespace(),
		"configmap",
		log.Printf,
	); err != nil {
		log.Fatal(err)
	}
	if err := hc.Config.KubeClient.IsReachable(); err != nil {
		return err
	}
	hc.kube_conn_init = true
	return nil
}

func (hc *HelmActions) InstallChartFromDir(name string, chartpath string) error {
	chart, err := loader.Load(chartpath)
	if err != nil {
		return err
	}
	return hc.InstallChart(name, chart)
}

func (hc *HelmActions) InstallChart(name string, chart *chart.Chart) error {
	hc.initKubeClient()

	actInstall := action.NewInstall(hc.Config)
	actInstall.ReleaseName = name
	actInstall.Namespace = hc.Settings.Namespace()

	release, err := actInstall.Run(chart, map[string]interface{}{})
	if err != nil {
		return err
	}
	log.Println("Release status: ", release.Info.Status)
	log.Println(release.Info.Description)
	return nil
}

func (hc *HelmActions) Uninstall(name string) error {
	hc.initKubeClient()
	release, err := hc.Get(name)
	if err != nil {
		return err
	}
	if release == nil {
		return errors.New("No release found with the name provided.")
	}
	actUninstall := action.NewUninstall(hc.Config)
	response, err := actUninstall.Run(name)
	if err != nil {
		return err
	}
	log.Println(response.Release.Info.Description)
	return nil
}

func (hc *HelmActions) Get(name string) (*release.Release, error) {
	hc.initKubeClient()

	actGet := action.NewGet(hc.Config)
	return actGet.Run(name)
}

func (hc *HelmActions) ListReleases() ([]compose.Stack, error) {
	hc.initKubeClient()

	actList := action.NewList(hc.Config)
	releases, err := actList.Run()
	if err != nil {
		return nil, err
	}
	result := []compose.Stack{}
	for _, rel := range releases {
		result = append(result, compose.Stack{
			ID:     rel.Name,
			Name:   rel.Name,
			Status: string(rel.Info.Status),
		})
	}
	return result, nil
}
