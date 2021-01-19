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
	"bytes"
	"encoding/json"
	"html/template"
	"path/filepath"

	"gopkg.in/yaml.v3"

	chart "helm.sh/helm/v3/pkg/chart"
	loader "helm.sh/helm/v3/pkg/chart/loader"
	"k8s.io/apimachinery/pkg/runtime"
)

func ConvertToChart(name string, objects map[string]runtime.Object) (*chart.Chart, error) {

	files := []*loader.BufferedFile{
		&loader.BufferedFile{
			Name: "README.md",
			Data: []byte("This chart was created by converting a Compose file"),
		}}

	chart := `name: {{.Name}}
description: A generated Helm Chart for {{.Name}} from Skippbox Kompose
version: 0.0.1
apiVersion: v1
keywords:
  - {{.Name}}
sources:
home:
`

	t, err := template.New("ChartTmpl").Parse(chart)
	if err != nil {
		return nil, err
	}
	type ChartDetails struct {
		Name string
	}
	var chartData bytes.Buffer
	err = t.Execute(&chartData, ChartDetails{Name: name})
	if err != nil {
		return nil, err
	}
	files = append(files, &loader.BufferedFile{
		Name: "Chart.yaml",
		Data: chartData.Bytes(),
	})

	for name, o := range objects {
		j, err := json.Marshal(o)
		if err != nil {
			return nil, err
		}
		buf, err := jsonToYaml(j, 2)
		if err != nil {
			return nil, err
		}
		files = append(files, &loader.BufferedFile{
			Name: filepath.Join("templates", name),
			Data: buf,
		})

	}
	return loader.LoadFiles(files)
}

// Convert JSON to YAML.
func jsonToYaml(j []byte, spaces int) ([]byte, error) {
	// Convert the JSON to an object.
	var jsonObj interface{}
	// We are using yaml.Unmarshal here (instead of json.Unmarshal) because the
	// Go JSON library doesn't try to pick the right number type (int, float,
	// etc.) when unmarshling to interface{}, it just picks float64
	// universally. go-yaml does go through the effort of picking the right
	// number type, so we can preserve number type throughout this process.
	err := yaml.Unmarshal(j, &jsonObj)
	if err != nil {
		return nil, err
	}

	var b bytes.Buffer
	encoder := yaml.NewEncoder(&b)
	encoder.SetIndent(spaces)
	if err := encoder.Encode(jsonObj); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
