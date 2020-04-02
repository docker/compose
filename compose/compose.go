package compose

import (
	"log"

	chartloader "helm.sh/helm/v3/pkg/chart/loader"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/helm-prototype/pkg/compose/internal/convert"
	"github.com/docker/helm-prototype/pkg/compose/internal/helm"
	utils "github.com/docker/helm-prototype/pkg/compose/internal/utils"
	"helm.sh/helm/v3/pkg/action"
	env "helm.sh/helm/v3/pkg/cli"
	k "helm.sh/helm/v3/pkg/kube"
)

// Orchestrator is "kubernetes" or "swarm"
type Orchestrator string

const (
	// Kubernetes specifies to use kubernetes.
	Kubernetes Orchestrator = "kubernetes"
	// Swarm specifies to use Docker swarm.
	Swarm Orchestrator = "swarm"
)

type ProjectOptions struct {
	ConfigPaths []string
	Name        string
}

type Project struct {
	Config       *types.Config
	ProjectDir   string
	Name         string `yaml:"-" json:"-"`
	Orchestrator Orchestrator
}

func NewProject(config types.ConfigDetails, name string) (*Project, error) {
	model, err := loader.Load(config)
	if err != nil {
		return nil, err
	}

	p := Project{
		Config:     model,
		ProjectDir: config.WorkingDir,
		Name:       name,
	}
	return &p, nil
}

// projectFromOptions load a compose project based on command line options
func ProjectFromOptions(options *ProjectOptions) (*Project, error) {
	if options.Name == "" {
		options.Name = "docker-compose"
	}

	workingDir, configs, err := utils.GetConfigs(
		options.Name,
		options.ConfigPaths,
	)
	if err != nil {
		return nil, err
	}

	return NewProject(types.ConfigDetails{
		WorkingDir:  workingDir,
		ConfigFiles: configs,
		Environment: utils.Environment(),
	}, options.Name)
}

func (p *Project) GenerateChart(path string) error {
	objects, err := convert.MapToKubernetesObjects(p.Config, p.Name)
	if err != nil {
		return err
	}
	err = helm.Write(p.Name, objects, path)
	if err != nil {
		return err
	}
	return nil
}

func (p *Project) InstallChart(n, path string) error {

	if path == "" {
		err := p.GenerateChart(path)
		if err != nil {
			return err
		}
	}

	settings := env.New()
	actionConfig := new(action.Configuration)
	println(".......... here ............")
	if err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), "memory", nil); err != nil {
		log.Fatal(err)
	}
	println(settings.EnvVars())
	client := action.NewInstall(actionConfig)
	println("Original chart version:", client.Version)
	client.Version = ">0.0.0-0"

	name, chart, err := client.NameAndChart([]string{n, path})
	if err != nil {
		return nil
	}

	client.ReleaseName = name
	client.Namespace = settings.Namespace()
	cp, err := client.ChartPathOptions.LocateChart(chart, settings)
	if err != nil {
		return err
	}
	println("CHART PATH: ", cp)

	chartRequested, err := chartloader.Load(cp)
	if err != nil {
		return err
	}
	kclient := k.New(settings.RESTClientGetter())
	println(kclient.Namespace)
	if err = actionConfig.KubeClient.IsReachable(); err != nil {
		println("Kube API is not reachable")
		return err
	}
	println("....Running.....")
	println("Chart description: ", chartRequested.Metadata.Description)
	release, err := client.Run(chartRequested, map[string]interface{}{})

	println(release.Name)
	return err
}
