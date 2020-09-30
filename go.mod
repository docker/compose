module github.com/docker/compose-cli

go 1.15

// the distribution version from ecs plugin is quite old and it breaks containerd
// we need to create a new release tag for docker/distribution
replace github.com/docker/distribution => github.com/docker/distribution v0.0.0-20200708230824-53e18a9d9bfe

replace github.com/awslabs/goformation/v4 => github.com/ndeloof/goformation/v4 v4.8.1-0.20200827081523-b7a7ac375adf

require (
	github.com/AlecAivazis/survey/v2 v2.1.1
	github.com/Azure/azure-sdk-for-go v43.3.0+incompatible
	github.com/Azure/azure-storage-file-go v0.8.0
	github.com/Azure/go-autorest/autorest v0.11.4
	github.com/Azure/go-autorest/autorest/adal v0.9.2
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.1
	github.com/Azure/go-autorest/autorest/azure/cli v0.4.0
	github.com/Azure/go-autorest/autorest/date v0.3.0
	github.com/Azure/go-autorest/autorest/to v0.4.0
	github.com/Azure/go-autorest/autorest/validation v0.2.0 // indirect
	github.com/Microsoft/go-winio v0.4.15-0.20190919025122-fc70bd9a86b5
	github.com/Microsoft/hcsshim v0.8.9 // indirect
	github.com/aws/aws-sdk-go v1.34.8
	github.com/awslabs/goformation/v4 v4.14.0
	github.com/buger/goterm v0.0.0-20200322175922-2f3e71b85129
	github.com/compose-spec/compose-go v0.0.0-20200907084823-057e1edc5b6f
	github.com/containerd/console v1.0.0
	github.com/containerd/containerd v1.3.5 // indirect
	github.com/docker/cli v0.0.0-20200528204125-dd360c7c0de8
	github.com/docker/distribution v0.0.0-00010101000000-000000000000 // indirect
	github.com/docker/docker v17.12.0-ce-rc1.0.20200309214505-aa6a9891b09c+incompatible
	github.com/docker/docker-credential-helpers v0.6.3 // indirect
	github.com/docker/go-connections v0.4.0
	github.com/docker/go-units v0.4.0
	github.com/gobwas/httphead v0.0.0-20180130184737-2c6c146eadee // indirect
	github.com/gobwas/pool v0.2.0 // indirect
	github.com/gobwas/ws v1.0.4
	github.com/golang/protobuf v1.4.2
	github.com/google/go-cmp v0.5.2
	github.com/google/uuid v1.1.2
	github.com/gorilla/mux v1.7.4 // indirect
	github.com/hashicorp/go-multierror v1.1.0
	github.com/joho/godotenv v1.3.0
	github.com/moby/term v0.0.0-20200611042045-63b9a826fb74
	github.com/morikuni/aec v1.0.0
	github.com/onsi/gomega v1.10.1 // indirect
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/runc v0.1.1 // indirect
	github.com/pkg/errors v0.9.1
	github.com/sanathkr/go-yaml v0.0.0-20170819195128-ed9d249f429b
	github.com/sirupsen/logrus v1.6.0
	github.com/smartystreets/goconvey v1.6.4 // indirect
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	go.uber.org/zap v1.10.0
	golang.org/x/mod v0.3.0
	golang.org/x/net v0.0.0-20200822124328-c89045814202
	golang.org/x/oauth2 v0.0.0-20200902213428-5d25da1a8d43
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208
	google.golang.org/grpc v1.32.0
	google.golang.org/protobuf v1.25.0
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/ini.v1 v1.61.0
	gotest.tools v2.2.0+incompatible
	gotest.tools/v3 v3.0.2
)
