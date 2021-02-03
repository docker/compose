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
	"fmt"

	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/api/errdefs"
	"github.com/docker/compose-cli/utils/prompt"

	"github.com/docker/compose-cli/kube/charts/kubernetes"
)

// ContextParams options for creating a Kubernetes context
type ContextParams struct {
	KubeContextName string
	Description     string
	KubeConfigPath  string
	FromEnvironment bool
}

// CreateContextData create Docker context data
func (cp ContextParams) CreateContextData() (interface{}, string, error) {
	if cp.FromEnvironment {
		// we use the current kubectl context from a $KUBECONFIG path
		return store.KubeContext{
			FromEnvironment: cp.FromEnvironment,
		}, cp.getDescription(), nil
	}
	user := prompt.User{}
	selectContext := func() error {
		contexts, err := kubernetes.ListAvailableKubeConfigContexts(cp.KubeConfigPath)
		if err != nil {
			return err
		}

		selected, err := user.Select("Select kubeconfig context", contexts)
		if err != nil {
			if err == terminal.InterruptErr {
				return errdefs.ErrCanceled
			}
			return err
		}
		cp.KubeContextName = contexts[selected]
		return nil
	}

	if cp.KubeConfigPath != "" {
		if cp.KubeContextName != "" {
			return store.KubeContext{
				ContextName:     cp.KubeContextName,
				KubeconfigPath:  cp.KubeConfigPath,
				FromEnvironment: cp.FromEnvironment,
			}, cp.getDescription(), nil
		}
		err := selectContext()
		if err != nil {
			return nil, "", err
		}
	} else {

		// interactive
		var options []string
		var actions []func() error

		options = append(options, "Context from kubeconfig file")
		actions = append(actions, selectContext)

		options = append(options, "Kubernetes environment variables")
		actions = append(actions, func() error {
			cp.FromEnvironment = true
			return nil
		})

		selected, err := user.Select("Create a Docker context using:", options)
		if err != nil {
			if err == terminal.InterruptErr {
				return nil, "", errdefs.ErrCanceled
			}
			return nil, "", err
		}

		err = actions[selected]()
		if err != nil {
			return nil, "", err
		}
	}
	return store.KubeContext{
		ContextName:     cp.KubeContextName,
		KubeconfigPath:  cp.KubeConfigPath,
		FromEnvironment: cp.FromEnvironment,
	}, cp.getDescription(), nil
}

func (cp ContextParams) getDescription() string {
	if cp.Description != "" {
		return cp.Description
	}
	if cp.FromEnvironment {
		return "From environment variables"
	}
	configFile := "default kube config"
	if cp.KubeConfigPath != "" {
		configFile = cp.KubeConfigPath
	}
	return fmt.Sprintf("%s (in %s)", cp.KubeContextName, configFile)
}
