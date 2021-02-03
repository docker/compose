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

package resources

import (
	"fmt"
	"os"

	"github.com/docker/compose-cli/api/context/store"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// ListAvailableKubeConfigContexts list kube contexts
func ListAvailableKubeConfigContexts(kubeconfig string) ([]string, error) {
	config, err := getKubeConfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	contexts := []string{}
	for k := range config.Contexts {
		contexts = append(contexts, k)
	}
	return contexts, nil
}

// LoadConfig returns kubeconfig data referenced in the docker context
func LoadConfig(ctx store.KubeContext) (*genericclioptions.ConfigFlags, error) {
	if ctx.FromEnvironment {
		return nil, nil
	}
	config, err := getKubeConfig(ctx.KubeconfigPath)
	if err != nil {
		return nil, err
	}
	contextName := ctx.ContextName
	if contextName == "" {
		contextName = config.CurrentContext
	}

	context, ok := config.Contexts[contextName]
	if !ok {
		return nil, fmt.Errorf("context name %s not found in kubeconfig", contextName)
	}
	cluster, ok := config.Clusters[context.Cluster]
	if !ok {
		return nil, fmt.Errorf("cluster %s not found for context %s", context.Cluster, contextName)
	}
	// bind to kubernetes config flags
	return &genericclioptions.ConfigFlags{
		Context:    &ctx.ContextName,
		KubeConfig: &ctx.KubeconfigPath,

		Namespace:   &context.Namespace,
		ClusterName: &context.Cluster,

		APIServer: &cluster.Server,
		CAFile:    &cluster.CertificateAuthority,
	}, nil
}

func getKubeConfig(kubeconfig string) (*api.Config, error) {
	config, err := clientcmd.NewDefaultPathOptions().GetStartingConfig()
	if err != nil {
		return nil, err
	}
	if kubeconfig != "" {
		f, err := os.Stat(kubeconfig)
		if os.IsNotExist(err) {
			return nil, err
		}
		if f.IsDir() {
			return nil, fmt.Errorf("%s not a config file", kubeconfig)
		}

		config = clientcmd.GetConfigFromFileOrDie(kubeconfig)
	}

	return config, nil
}
