package docker

import (
	"fmt"

	"github.com/docker/cli/cli/command"
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
	Region  string
}

func NewContext(name string, awsContext interface{}) error {
	_, err := NewContextWithStore(name, awsContext.(AwsContext), cliconfig.ContextStoreDir())
	return err
}

func NewContextWithStore(name string, awsContext AwsContext, contextDirectory string) (store.Store, error) {
	contextStore := initContextStore(contextDirectory)
	endpoints := map[string]interface{}{
		"aws":    awsContext,
		"docker": awsContext,
	}

	metadata := store.Metadata{
		Name:      name,
		Endpoints: endpoints,
		Metadata:  TypeContext{Type: contextType},
	}
	return contextStore, contextStore.CreateOrUpdate(metadata)
}

func initContextStore(contextDir string) store.Store {
	config := store.NewConfig(getter)
	return store.New(contextDir, config)
}

func checkAwsContextExists(contextName string) (*AwsContext, error) {
	if contextName == command.DefaultContextName {
		return nil, fmt.Errorf("can't use \"%s\" with ECS command because it is not an AWS context", contextName)
	}
	contextStore := initContextStore(cliconfig.ContextStoreDir())
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

func GetAwsContext(dockerCli command.Cli) (*AwsContext, error) {
	contextName := dockerCli.CurrentContext()
	return checkAwsContextExists(contextName)
}
