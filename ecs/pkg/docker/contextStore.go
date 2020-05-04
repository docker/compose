package docker

import (
	"fmt"

	cliconfig "github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/context/store"
	"github.com/mitchellh/mapstructure"
)

const contextType = "aws"

type TypeContext struct {
	Type string
}

func getter() interface{} {
	return &TypeContext{}
}

type AwsContext struct {
	Profile string
	Cluster string
	Region  string
}

func NewContext(name string, awsContext *AwsContext) error {
	contextStore := initContextStore()
	endpoints := map[string]interface{}{
		"aws":    awsContext,
		"docker": awsContext,
	}

	metadata := store.Metadata{
		Name:      name,
		Endpoints: endpoints,
		Metadata:  TypeContext{Type: contextType},
	}
	return contextStore.CreateOrUpdate(metadata)
}

func initContextStore() store.Store {
	config := store.NewConfig(getter)
	return store.New(cliconfig.ContextStoreDir(), config)
}

func CheckAwsContextExists(contextName string) (*AwsContext, error) {
	contextStore := initContextStore()
	metadata, err := contextStore.GetMetadata(contextName)
	if err != nil {
		return nil, err
	}
	endpoint := metadata.Endpoints["aws"]
	awsContext := AwsContext{}
	err = mapstructure.Decode(endpoint, &awsContext)
	if err != nil {
		return nil, err
	}
	if awsContext == (AwsContext{}) {
		return nil, fmt.Errorf("can't use \"%s\" with ECS command because it is not an AWS context", contextName)
	}
	return &awsContext, nil
}
