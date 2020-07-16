package amazon

import (
	"context"

	"github.com/docker/api/backend"
	"github.com/docker/api/compose"
	"github.com/docker/api/containers"
	apicontext "github.com/docker/api/context"
	"github.com/docker/api/context/cloud"
	"github.com/docker/api/context/store"
	aws "github.com/docker/ecs-plugin/pkg/amazon/backend"
)

// ContextParams options for creating AWS context
type ContextParams struct {
	Description string
	Region      string
	Profile     string

	AwsID     string
	AwsSecret string
}

func init() {
	backend.Register("aws", "aws", service, getCloudService)
}

func service(ctx context.Context) (backend.Service, error) {
	contextStore := store.ContextStore(ctx)
	currentContext := apicontext.CurrentContext(ctx)
	var awsContext store.AwsContext

	if err := contextStore.GetEndpoint(currentContext, &awsContext); err != nil {
		return nil, err
	}

	return getAwsAPIService(awsContext)
}

func getAwsAPIService(awsCtx store.AwsContext) (*awsAPIService, error) {
	backend, err := aws.NewBackend(awsCtx.Profile, awsCtx.Region)
	if err != nil {
		return nil, err
	}
	return &awsAPIService{
		ctx:            awsCtx,
		composeBackend: backend,
	}, nil
}

type awsAPIService struct {
	ctx            store.AwsContext
	composeBackend *aws.Backend
}

func (a *awsAPIService) ContainerService() containers.Service {
	return nil
}

func (a *awsAPIService) ComposeService() compose.Service {
	return a.composeBackend
}

func getCloudService() (cloud.Service, error) {
	return awsCloudService{}, nil
}

type awsCloudService struct {
}

func (a awsCloudService) Login(ctx context.Context, params interface{}) error {
	return nil
}

func (a awsCloudService) Logout(ctx context.Context) error {
	return nil
}

func (a awsCloudService) CreateContextData(ctx context.Context, params interface{}) (interface{}, string, error) {
	contextHelper := newContextCreateHelper()
	createOpts := params.(ContextParams)
	return contextHelper.createContextData(ctx, createOpts)
}
