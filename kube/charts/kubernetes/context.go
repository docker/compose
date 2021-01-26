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

package kubernetes

import (
	"fmt"
	"os"

	"k8s.io/client-go/tools/clientcmd"
)

// ListAvailableKubeConfigContexts list kube contexts
func ListAvailableKubeConfigContexts(kubeconfig string) ([]string, error) {
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

	contexts := []string{}
	for k := range config.Contexts {
		contexts = append(contexts, k)
	}
	return contexts, nil
}
