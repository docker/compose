package env

import (
	"os"
	"strings"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/helm-prototype/pkg/compose/internal/helm"
	"github.com/docker/helm-prototype/pkg/compose/internal/kube"
	"github.com/docker/helm-prototype/pkg/compose/internal/utils"
	chart "helm.sh/helm/v3/pkg/chart"
	util "helm.sh/helm/v3/pkg/chartutil"
	helmenv "helm.sh/helm/v3/pkg/cli"
)

func GetDefault() *helmenv.EnvSettings {
	return helmenv.New()
}

func Environment() map[string]string {
	vars := make(map[string]string)
	env := os.Environ()
	for _, v := range env {
		k := strings.SplitN(v, "=", 2)
		vars[k[0]] = k[1]
	}
	return vars
}

func GetConfig(name string, configPaths []string) (*types.Config, string, error) {
	if name == "" {
		name = "docker-compose"
	}
	workingDir, configs, err := utils.GetConfigs(
		name,
		configPaths,
	)
	if err != nil {
		return nil, "", err
	}
	config, err := loader.Load(types.ConfigDetails{
		WorkingDir:  workingDir,
		ConfigFiles: configs,
		Environment: Environment(),
	})
	if err != nil {
		return nil, "", err
	}
	return config, workingDir, nil
}

func GetChartInMemory(config *types.Config, name string) (*chart.Chart, error) {
	for k, v := range config.Volumes {
		volumeName := strings.ReplaceAll(k, "_", "-")
		if volumeName != k {
			config.Volumes[volumeName] = v
			delete(config.Volumes, k)
		}
	}
	objects, err := kube.MapToKubernetesObjects(config, name)
	if err != nil {
		return nil, err
	}
	//in memory files
	return helm.ConvertToChart(name, objects)
}

func SaveChart(config *types.Config, name, dest string) error {
	chart, err := GetChartInMemory(config, name)
	if err != nil {
		return err
	}
	return util.SaveDir(chart, dest)
}
