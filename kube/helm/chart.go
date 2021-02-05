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
	"os"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/compose-cli/kube/resources"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	chart "helm.sh/helm/v3/pkg/chart"
	loader "helm.sh/helm/v3/pkg/chart/loader"
	"k8s.io/apimachinery/pkg/runtime"
)

//ConvertToChart convert Kube objects to helm chart
func ConvertToChart(name string, objects map[string]runtime.Object) (*chart.Chart, error) {

	files := []*loader.BufferedFile{
		{
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

// GetChartInMemory get memory representation of helm chart
func GetChartInMemory(project *types.Project) (*chart.Chart, error) {
	// replace _ with - in volume names
	for k, v := range project.Volumes {
		volumeName := strings.ReplaceAll(k, "_", "-")
		if volumeName != k {
			project.Volumes[volumeName] = v
			delete(project.Volumes, k)
		}
	}
	objects, err := resources.MapToKubernetesObjects(project)
	if err != nil {
		return nil, err
	}
	//in memory files
	return ConvertToChart(project.Name, objects)
}

// SaveChart saves the chart to directory
func SaveChart(c *chart.Chart, dest string) (string, error) {
	dir, err := filepath.Abs(dest)
	if err != nil {
		return "", err
	}
	for _, file := range c.Raw {
		filename := filepath.Join(dir, file.Name)
		filedir := filepath.Dir(filename)

		stat, err := os.Stat(filedir)

		if err != nil {
			if os.IsNotExist(err) {
				if err2 := os.MkdirAll(filedir, 0755); err2 != nil {
					return "", err2
				}
			} else {
				return "", err
			}
		} else if !stat.IsDir() {
			return "", errors.Errorf("%s: not a directory", dest)
		}

		f, err := os.Create(filename)
		if err != nil {
			return "", err
		}
		_, err = f.Write(file.Data)
		if err != nil {
			return "", err
		}
	}
	return dir, nil
}
