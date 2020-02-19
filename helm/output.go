package helm

import (
	"bytes"
	"encoding/json"
	"gopkg.in/yaml.v3"
	"html/template"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/runtime"
	"os"
	"path/filepath"
)

func Write(project string, objects map[string]runtime.Object, target string) error {
	out := Outputer{ target }

	if err := out.Write("README.md", []byte("This chart was created by converting a Compose file")); err != nil {
		return err
	}

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
		return err
	}
	type ChartDetails struct {
		Name string
	}
	var chartData bytes.Buffer
	_ = t.Execute(&chartData, ChartDetails{project})


	if err := out.Write("Chart.yaml", chartData.Bytes()); err != nil {
		return err
	}

	for name, o := range objects {
		j, err := json.Marshal(o)
		if err != nil {
			return err
		}
		b, err := jsonToYaml(j, 2)
		if err != nil {
			return err
		}
		if err := out.Write(filepath.Join("templates", name), b); err != nil {
			return err
		}
	}
	return nil
}

type Outputer struct {
	Dir string
}

func (o Outputer) Write(path string, content []byte) error {
	out := filepath.Join(o.Dir, path)
	os.MkdirAll(filepath.Dir(out), 0744)
	return ioutil.WriteFile(out, content, 0644)
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

	// Marshal this object into YAML.
	// return yaml.Marshal(jsonObj)
}