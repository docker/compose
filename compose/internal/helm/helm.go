package helm

import (
	"log"

	action "helm.sh/helm/v3/pkg/action"
	env "helm.sh/helm/v3/pkg/cli"
)

type HelmConfig struct {
	Config         *action.Configuration
	Settings       *env.EnvSettings
	kube_conn_init bool
}

func NewHelmConfig(settings *env.EnvSettings) *HelmConfig {
	if settings == nil {
		settings = env.New()
	}
	return &HelmConfig{
		Config:         new(action.Configuration),
		Settings:       settings,
		kube_conn_init: false,
	}
}

func (hc *HelmConfig) InitKubeClient() error {
	if hc.kube_conn_init {
		return nil
	}
	if err := hc.Config.Init(hc.Settings.RESTClientGetter(), hc.Settings.Namespace(), "memory", log.Printf); err != nil {
		log.Fatal(err)
	}
	if err := hc.Config.KubeClient.IsReachable(); err != nil {
		return err
	}
	hc.kube_conn_init = true
	return nil
}
