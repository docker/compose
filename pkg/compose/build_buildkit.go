/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package compose

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	ctxkube "github.com/docker/buildx/driver/kubernetes/context"
	"github.com/docker/buildx/store"
	"github.com/docker/buildx/store/storeutil"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/context/docker"
	ctxstore "github.com/docker/cli/cli/context/store"
	dockerclient "github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/docker/buildx/build"
	"github.com/docker/buildx/driver"
	_ "github.com/docker/buildx/driver/docker"           //nolint:blank-imports
	_ "github.com/docker/buildx/driver/docker-container" //nolint:blank-imports
	_ "github.com/docker/buildx/driver/kubernetes"       //nolint:blank-imports
	xprogress "github.com/docker/buildx/util/progress"
)

func (s *composeService) doBuildBuildkit(ctx context.Context, opts map[string]build.Options, mode string) (map[string]string, error) {
	dis, err := s.getDrivers(ctx)
	if err != nil {
		return nil, err
	}

	// Progress needs its own context that lives longer than the
	// build one otherwise it won't read all the messages from
	// build and will lock
	progressCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := xprogress.NewPrinter(progressCtx, s.stdout(), os.Stdout, mode)

	response, err := build.Build(ctx, dis, opts, &internalAPI{dockerCli: s.dockerCli}, filepath.Dir(s.configFile().Filename), w)
	errW := w.Wait()
	if err == nil {
		err = errW
	}
	if err != nil {
		return nil, WrapCategorisedComposeError(err, BuildFailure)
	}

	imagesBuilt := map[string]string{}
	for name, img := range response {
		if img == nil || len(img.ExporterResponse) == 0 {
			continue
		}
		digest, ok := img.ExporterResponse["containerimage.digest"]
		if !ok {
			continue
		}
		imagesBuilt[name] = digest
	}

	return imagesBuilt, err
}

func (s *composeService) getDrivers(ctx context.Context) ([]build.DriverInfo, error) { //nolint:gocyclo
	txn, release, err := storeutil.GetStore(s.dockerCli)
	if err != nil {
		return nil, err
	}
	defer release()

	ng, err := storeutil.GetCurrentInstance(txn, s.dockerCli)
	if err != nil {
		return nil, err
	}

	dis := make([]build.DriverInfo, len(ng.Nodes))
	var f driver.Factory
	if ng.Driver != "" {
		factories := driver.GetFactories()
		for _, fac := range factories {
			if fac.Name() == ng.Driver {
				f = fac
				continue
			}
		}
		if f == nil {
			if f = driver.GetFactory(ng.Driver, true); f == nil {
				return nil, fmt.Errorf("failed to find buildx driver %q", ng.Driver)
			}
		}
	} else {
		ep := ng.Nodes[0].Endpoint
		dockerapi, err := clientForEndpoint(s.dockerCli, ep)
		if err != nil {
			return nil, err
		}
		f, err = driver.GetDefaultFactory(ctx, dockerapi, false)
		if err != nil {
			return nil, err
		}
		ng.Driver = f.Name()
	}

	imageopt, err := storeutil.GetImageConfig(s.dockerCli, ng)
	if err != nil {
		return nil, err
	}

	eg, _ := errgroup.WithContext(ctx)
	for i, n := range ng.Nodes {
		func(i int, n store.Node) {
			eg.Go(func() error {
				di := build.DriverInfo{
					Name:        n.Name,
					Platform:    n.Platforms,
					ProxyConfig: storeutil.GetProxyConfig(s.dockerCli),
				}
				defer func() {
					dis[i] = di
				}()

				dockerapi, err := clientForEndpoint(s.dockerCli, n.Endpoint)
				if err != nil {
					di.Err = err
					return nil
				}
				// TODO: replace the following line with dockerclient.WithAPIVersionNegotiation option in clientForEndpoint
				dockerapi.NegotiateAPIVersion(ctx)

				contextStore := s.dockerCli.ContextStore()

				var kcc driver.KubeClientConfig
				kcc, err = configFromContext(n.Endpoint, contextStore)
				if err != nil {
					// err is returned if n.Endpoint is non-context name like "unix:///var/run/docker.sock".
					// try again with name="default".
					// FIXME: n should retain real context name.
					kcc, err = configFromContext("default", contextStore)
					if err != nil {
						logrus.Error(err)
					}
				}

				tryToUseKubeConfigInCluster := false
				if kcc == nil {
					tryToUseKubeConfigInCluster = true
				} else {
					if _, err := kcc.ClientConfig(); err != nil {
						tryToUseKubeConfigInCluster = true
					}
				}
				if tryToUseKubeConfigInCluster {
					kccInCluster := driver.KubeClientConfigInCluster{}
					if _, err := kccInCluster.ClientConfig(); err == nil {
						logrus.Debug("using kube config in cluster")
						kcc = kccInCluster
					}
				}

				d, err := driver.GetDriver(ctx, "buildx_buildkit_"+n.Name, f, dockerapi, imageopt.Auth, kcc, n.Flags, n.Files, n.DriverOpts, n.Platforms, "")
				if err != nil {
					di.Err = err
					return nil
				}
				di.Driver = d
				di.ImageOpt = imageopt
				return nil
			})
		}(i, n)
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return dis, nil
}

func clientForEndpoint(dockerCli command.Cli, name string) (dockerclient.APIClient, error) {
	list, err := dockerCli.ContextStore().List()
	if err != nil {
		return nil, err
	}
	for _, l := range list {
		if l.Name != name {
			continue
		}
		dep, ok := l.Endpoints["docker"]
		if !ok {
			return nil, fmt.Errorf("context %q does not have a Docker endpoint", name)
		}
		epm, ok := dep.(docker.EndpointMeta)
		if !ok {
			return nil, fmt.Errorf("endpoint %q is not of type EndpointMeta, %T", dep, dep)
		}
		ep, err := docker.WithTLSData(dockerCli.ContextStore(), name, epm)
		if err != nil {
			return nil, err
		}
		clientOpts, err := ep.ClientOpts()
		if err != nil {
			return nil, err
		}
		return dockerclient.NewClientWithOpts(clientOpts...)
	}

	ep := docker.Endpoint{
		EndpointMeta: docker.EndpointMeta{
			Host: name,
		},
	}

	clientOpts, err := ep.ClientOpts()
	if err != nil {
		return nil, err
	}

	return dockerclient.NewClientWithOpts(clientOpts...)
}

func configFromContext(endpointName string, s ctxstore.Reader) (clientcmd.ClientConfig, error) {
	if strings.HasPrefix(endpointName, "kubernetes://") {
		u, _ := url.Parse(endpointName)
		if kubeconfig := u.Query().Get("kubeconfig"); kubeconfig != "" {
			_ = os.Setenv(clientcmd.RecommendedConfigPathEnvVar, kubeconfig)
		}
		rules := clientcmd.NewDefaultClientConfigLoadingRules()
		apiConfig, err := rules.Load()
		if err != nil {
			return nil, err
		}
		return clientcmd.NewDefaultClientConfig(*apiConfig, &clientcmd.ConfigOverrides{}), nil
	}
	return ctxkube.ConfigFromContext(endpointName, s)
}

type internalAPI struct {
	dockerCli command.Cli
}

func (a *internalAPI) DockerAPI(name string) (dockerclient.APIClient, error) {
	if name == "" {
		name = a.dockerCli.CurrentContext()
	}
	return clientForEndpoint(a.dockerCli, name)
}
