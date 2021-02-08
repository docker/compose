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
	"io/ioutil"
	"strings"

	"github.com/compose-spec/compose-go/types"

	corev1 "k8s.io/api/core/v1"
)

func toSecretSpecs(project *types.Project) ([]corev1.Secret, error) {
	var secrets []corev1.Secret

	for _, s := range project.Secrets {
		if s.External.External {
			continue
		}
		name := strings.ReplaceAll(s.Name, "_", "-")
		// load secret file content
		sensitiveData, err := ioutil.ReadFile(s.File)
		if err != nil {
			return nil, err
		}

		readOnly := true
		secret := corev1.Secret{}
		secret.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
		secret.Name = name
		secret.Type = "compose"
		secret.Data = map[string][]byte{
			name: sensitiveData,
		}
		secret.Immutable = &readOnly

		secrets = append(secrets, secret)
	}

	return secrets, nil
}
