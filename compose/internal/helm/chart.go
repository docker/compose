package helm

import (
	"gopkg.in/yaml.v2"
	action "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	loader "helm.sh/helm/v3/pkg/chart/loader"
)

type HelmChart struct {
	chart        *chart.Chart
	actionConfig *HelmConfig
	Path         string
	Name         string
}

func NewChart(name, chartpath string) *HelmChart {
	chart, err := loader.Load(chartpath)
	if err != nil {
		return nil
	}
	return &HelmChart{
		chart: chart,
		Path:  chartpath,
		Name:  name,
	}
}
func (chart *HelmChart) SetActionConfig(config *HelmConfig) error {
	chart.actionConfig = config
	return nil
}

func (chart *HelmChart) Validate() error {
	_, err := yaml.Marshal(chart.chart.Metadata)
	if err != nil {
		return nil
	}

	return nil
}

func (chart *HelmChart) Install() error {
	err := chart.actionConfig.InitKubeClient()
	if err != nil {
		return err
	}

	actInstall := action.NewInstall(chart.actionConfig.Config)
	actInstall.ReleaseName = chart.Name
	actInstall.Namespace = chart.actionConfig.Settings.Namespace()

	release, err := actInstall.Run(chart.chart, map[string]interface{}{})
	if err != nil {
		return err
	}

	println("Release status: ", release.Info.Status)
	println("Release description: ", release.Info.Description)
	return chart.actionConfig.Config.Releases.Update(release)
}

func (chart *HelmChart) Uninstall() error {
	err := chart.actionConfig.InitKubeClient()
	if err != nil {
		return err
	}
	actUninstall := action.NewUninstall(chart.actionConfig.Config)
	_, err = actUninstall.Run(chart.Name)
	return err
}
