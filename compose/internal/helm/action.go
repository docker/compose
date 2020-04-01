package helm

import (
	"os"
	"sync"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

type KubeConfig struct {
	namespace  string
	config     genericclioptions.RESTClientGetter
	configOnce sync.Once

	// KubeConfig is the path to the kubeconfig file
	KubeConfig string
	// KubeContext is the name of the kubeconfig context.
	KubeContext string
	// Bearer KubeToken used for authentication
	KubeToken string
	// Kubernetes API Server Endpoint for authentication
	KubeAPIServer string
}

func New() *KubeConfig {

	env := KubeConfig{
		namespace:     "",
		KubeContext:   os.Getenv("COMPOSE_KUBECONTEXT"),
		KubeToken:     os.Getenv("COMPOSE_KUBETOKEN"),
		KubeAPIServer: os.Getenv("COMPOSE_KUBEAPISERVER"),
	}
	return &env
}
