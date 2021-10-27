module github.com/docker/compose/v2

go 1.16

require (
	github.com/AlecAivazis/survey/v2 v2.2.3
	github.com/buger/goterm v1.0.0
	github.com/cnabio/cnab-to-oci v0.3.1-beta1
	github.com/compose-spec/compose-go v1.0.3
	github.com/containerd/console v1.0.2
	github.com/containerd/containerd v1.5.5
	github.com/distribution/distribution/v3 v3.0.0-20210316161203-a01c71e2477e
	github.com/docker/buildx v0.5.2-0.20210422185057-908a856079fc
	github.com/docker/cli v20.10.7+incompatible
	github.com/docker/cli-docs-tool v0.1.1
	github.com/docker/compose-switch v1.0.2
	github.com/docker/docker v20.10.7+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/docker/go-units v0.4.0
	github.com/gofrs/flock v0.8.0 // indirect
	github.com/golang/mock v1.5.0
	github.com/hashicorp/go-multierror v1.1.0
	github.com/hashicorp/go-version v1.3.0
	github.com/kr/pty v1.1.8 // indirect
	github.com/mattn/go-colorable v0.1.6 // indirect
	github.com/mattn/go-isatty v0.0.12
	github.com/mattn/go-shellwords v1.0.12
	github.com/moby/buildkit v0.8.2-0.20210401015549-df49b648c8bf
	github.com/moby/term v0.0.0-20210619224110-3f7ff695adc6
	github.com/morikuni/aec v1.0.0
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.1
	github.com/pkg/errors v0.9.1
	github.com/sanathkr/go-yaml v0.0.0-20170819195128-ed9d249f429b
	github.com/sergi/go-diff v1.1.0 // indirect
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	gotest.tools v2.2.0+incompatible
	gotest.tools/v3 v3.0.3
	k8s.io/client-go v0.21.0 // indirect
)

// (for buildx)
replace github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305
