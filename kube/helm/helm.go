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

	"github.com/docker/compose-cli/api/compose"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	env "helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// Actions implements charts installation management
type Actions struct {
	Config     *action.Configuration
	Namespace  string
	initialize func(f func(format string, v ...interface{})) error
}

const helmDriver = "configmap"

// NewActions new helm action
func NewActions(getter genericclioptions.RESTClientGetter) (*Actions, error) {
	if getter == nil {
		settings := env.New()
		getter = settings.RESTClientGetter()
	}

	namespace := "default"
	if ns, _, err := getter.ToRawKubeConfigLoader().Namespace(); err == nil {
		namespace = ns
	}
	actions := &Actions{
		Config: &action.Configuration{
			RESTClientGetter: getter,
		},
		Namespace: namespace,
	}

	actions.initialize = func(f func(format string, v ...interface{})) error {
		err := actions.Config.Init(getter, namespace, helmDriver, f)
		if err != nil {
			return err
		}
		return actions.Config.KubeClient.IsReachable()
	}
	err := actions.initialize(nil) // by default no logger, users might re-initialize with another logger function
	if err != nil {
		return nil, err
	}
	return actions, nil
}

// InstallChart installs chart
func (hc *Actions) InstallChart(name string, chart *chart.Chart, logger func(format string, v ...interface{})) error {
	err := hc.initialize(logger)
	if err != nil {
		return err
	}

	actInstall := action.NewInstall(hc.Config)
	actInstall.ReleaseName = name
	actInstall.Namespace = hc.Namespace
	_, err = actInstall.Run(chart, map[string]interface{}{})
	return err
}

// Uninstall uninstall chart
func (hc *Actions) Uninstall(name string, logger func(format string, v ...interface{})) error {
	err := hc.initialize(logger)
	if err != nil {
		return err
	}

	release, err := hc.Get(name)
	if err != nil {
		return err
	}
	if release == nil {
		return errors.New("no release found with the name provided")
	}
	actUninstall := action.NewUninstall(hc.Config)
	_, err = actUninstall.Run(name)
	return err
}

// Get get released object for a named chart
func (hc *Actions) Get(name string) (*release.Release, error) {
	actGet := action.NewGet(hc.Config)
	return actGet.Run(name)
}

// ListReleases lists chart releases
func (hc *Actions) ListReleases() ([]compose.Stack, error) {
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
