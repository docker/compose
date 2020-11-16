module github.com/docker/compose-cli

go 1.15

// the distribution version from ecs plugin is quite old and it breaks containerd
// we need to create a new release tag for docker/distribution
replace github.com/docker/distribution => github.com/docker/distribution v0.0.0-20200708230824-53e18a9d9bfe

require (
	github.com/AlecAivazis/survey/v2 v2.1.1
	github.com/Azure/azure-sdk-for-go v43.3.0+incompatible
	github.com/Azure/azure-storage-file-go v0.8.0
	github.com/Azure/go-autorest/autorest v0.11.10
	github.com/Azure/go-autorest/autorest/adal v0.9.5
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.3
	github.com/Azure/go-autorest/autorest/azure/cli v0.4.2
	github.com/Azure/go-autorest/autorest/date v0.3.0
	github.com/Azure/go-autorest/autorest/to v0.4.0
	github.com/Azure/go-autorest/autorest/validation v0.2.0 // indirect
	github.com/Microsoft/go-winio v0.4.15-0.20200908182639-5b44b70ab3ab
	github.com/aws/aws-sdk-go v1.35.15
	github.com/awslabs/goformation/v4 v4.15.3
	github.com/buger/goterm v0.0.0-20200322175922-2f3e71b85129
	github.com/compose-spec/compose-go v0.0.0-20201116112017-777513ca88e2
	github.com/containerd/console v1.0.1
	github.com/containerd/containerd v1.3.5 // indirect
	github.com/docker/cli v0.0.0-20200528204125-dd360c7c0de8
	github.com/docker/distribution v0.0.0-00010101000000-000000000000 // indirect
	github.com/docker/docker v20.10.0-beta1.0.20201113105859-b6bfff2a628f+incompatible
	github.com/docker/docker-credential-helpers v0.6.3 // indirect
	github.com/docker/go-connections v0.4.0
	github.com/docker/go-units v0.4.0
	github.com/gobwas/httphead v0.0.0-20180130184737-2c6c146eadee // indirect
	github.com/gobwas/pool v0.2.0 // indirect
	github.com/gobwas/ws v1.0.4
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/golang/mock v1.4.4
	github.com/golang/protobuf v1.4.3
	github.com/google/go-cmp v0.5.3
	github.com/google/uuid v1.1.2
	github.com/gorilla/mux v1.7.4 // indirect
	github.com/hashicorp/go-multierror v1.1.0
	github.com/hashicorp/go-uuid v1.0.1
	github.com/iancoleman/strcase v0.1.2
	github.com/joho/godotenv v1.3.0
	github.com/labstack/echo v3.3.10+incompatible
	github.com/labstack/gommon v0.3.0 // indirect
	github.com/moby/term v0.0.0-20200915141129-7f0af18e79f2
	github.com/morikuni/aec v1.0.0
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.1
	github.com/pkg/errors v0.9.1
	github.com/prometheus/tsdb v0.10.0
	github.com/sanathkr/go-yaml v0.0.0-20170819195128-ed9d249f429b
	github.com/sirupsen/logrus v1.7.0
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	github.com/valyala/fasttemplate v1.2.1 // indirect
	golang.org/x/mod v0.3.0
	golang.org/x/net v0.0.0-20201026091529-146b70c837a4
	golang.org/x/oauth2 v0.0.0-20200902213428-5d25da1a8d43
	golang.org/x/sync v0.0.0-20201020160332-67f06af15bc9
	google.golang.org/grpc v1.33.1
	google.golang.org/protobuf v1.25.0
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/ini.v1 v1.62.0
	gotest.tools v2.2.0+incompatible
	gotest.tools/v3 v3.0.3
)
