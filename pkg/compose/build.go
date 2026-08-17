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
	"sort"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose/v5/internal/tracing"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/utils"
)

func (s *composeService) Build(ctx context.Context, project *types.Project, options api.BuildOptions) error {
	err := options.Apply(project)
	if err != nil {
		return err
	}
	return Run(ctx, func(ctx context.Context) error {
		return tracing.SpanWrapFunc("project/build", tracing.ProjectOptions(ctx, project),
			func(ctx context.Context) error {
				builtImages, err := s.build(ctx, project, options, nil)
				if err == nil && len(builtImages) == 0 {
					logrus.Warn("No services to build")
				}
				return err
			})(ctx)
	}, "build", s.events)
}

func (s *composeService) build(ctx context.Context, project *types.Project, options api.BuildOptions, localImages map[string]api.ImageSummary) (map[string]string, error) {
	imageIDs := map[string]string{}
	serviceToBuild := types.Services{}

	var policy types.DependencyOption = types.IgnoreDependencies
	if options.Deps {
		policy = types.IncludeDependencies
	}

	if len(options.Services) == 0 {
		options.Services = project.ServiceNames()
	}

	// also include services used as additional_contexts with service: prefix
	options.Services = addBuildDependencies(options.Services, project)

	// Some build dependencies we just introduced may not be enabled
	var err error
	project, err = project.WithServicesEnabled(options.Services...)
	if err != nil {
		return nil, err
	}

	project, err = project.WithSelectedServices(options.Services)
	if err != nil {
		return nil, err
	}

	err = project.ForEachService(options.Services, func(serviceName string, service *types.ServiceConfig) error {
		if service.Build == nil {
			return nil
		}
		image := api.GetImageNameOrDefault(*service, project.Name)
		_, localImagePresent := localImages[image]
		if localImagePresent && service.PullPolicy != types.PullPolicyBuild {
			return nil
		}
		serviceToBuild[serviceName] = *service
		return nil
	}, policy)
	if err != nil {
		return imageIDs, err
	}

	if len(serviceToBuild) == 0 {
		return imageIDs, nil
	}

	bake, err := buildWithBake(s.dockerCli)
	if err != nil {
		return nil, err
	}
	if bake {
		return s.doBuildBake(ctx, project, serviceToBuild, options)
	}
	return s.doBuildClassic(ctx, project, serviceToBuild, options)
}

func (s *composeService) ensureImagesExists(ctx context.Context, project *types.Project, buildOpts *api.BuildOptions, quietPull bool) error {
	for name, service := range project.Services {
		if service.Provider == nil && service.Image == "" && service.Build == nil {
			return fmt.Errorf("invalid service %q. Must specify either image or build", name)
		}
	}

	images, pinnedDigests, err := s.getLocalImagesDigests(ctx, project)
	if err != nil {
		return err
	}

	err = tracing.SpanWrapFunc("project/pull", tracing.ProjectOptions(ctx, project),
		func(ctx context.Context) error {
			return s.pullRequiredImages(ctx, project, images, quietPull)
		},
	)(ctx)
	if err != nil {
		return err
	}

	if buildOpts != nil {
		err = tracing.SpanWrapFunc("project/build", tracing.ProjectOptions(ctx, project),
			func(ctx context.Context) error {
				builtImages, err := s.build(ctx, project, *buildOpts, images)
				if err != nil {
					return err
				}

				for name, digest := range builtImages {
					images[name] = api.ImageSummary{
						Repository:  name,
						ID:          digest,
						LastTagTime: time.Now(),
					}
				}
				return nil
			},
		)(ctx)
		if err != nil {
			return err
		}
	}

	// set digest as com.docker.compose.image label so we can detect outdated
	// containers — the single writer of that label, so the platform-pinned
	// resolution below can't be overwritten by another code path
	for name, service := range project.Services {
		image := api.GetImageNameOrDefault(service, project.Name)
		img, ok := images[image]
		if ok {
			// a platform-pinned service is labelled with the digest of ITS
			// platform's manifest — see serviceImageDigest
			digest := s.serviceImageDigest(ctx, service, image, img, pinnedDigests)
			service.CustomLabels = service.CustomLabels.Add(api.ImageDigestLabel, digest)
		}

		resolveImageVolumes(&service, images, project.Name)

		project.Services[name] = service
	}
	return nil
}

func resolveImageVolumes(service *types.ServiceConfig, images map[string]api.ImageSummary, projectName string) {
	var digests []string
	for i, vol := range service.Volumes {
		if vol.Type != types.VolumeTypeImage {
			continue
		}
		imgName := vol.Source
		img, ok := images[imgName]
		if !ok {
			// check if source is another service in the project
			imgName = api.GetImageNameOrDefault(types.ServiceConfig{Name: vol.Source}, projectName)
			// If we still can't find it, it might be an external image that wasn't pulled yet or doesn't exist
			if img, ok = images[imgName]; !ok {
				continue
			}
		}
		// The daemon only resolves a `type=image` mount Source that is a name/tag
		// or a top-level image ID, not a per-platform manifest digest (which is
		// what ImageSummary.ID holds to stay stable across attested rebuilds, see
		// localContentDigest). Keep Source as the resolved name so mounting always
		// works, and track the digest separately so mustRecreate can still detect
		// a changed source image.
		//
		// Two accepted tradeoffs: (1) Source feeds ServiceHash, so containers
		// created by a previous release (hashed with Source=<image ID>) are
		// recreated once on upgrade — see the release note; (2) the mount is
		// created from the mutable tag while the digest label comes from a
		// separate inspect, so a retag racing between the two leaves the
		// container mounting the new content under the old recorded digest
		// until the next up recreates it. Pinning Source to a digest would
		// close the race but reintroduce either the mount failure (#14005) or
		// the attestation-churn recreates (#13636).
		service.Volumes[i].Source = imgName
		digests = append(digests, vol.Target+"="+img.ID)
	}
	if len(digests) > 0 {
		sort.Strings(digests)
		service.CustomLabels = service.CustomLabels.Add(api.ImageVolumeDigestLabel, strings.Join(digests, ","))
	}
}

// pinnedImageDigest is the content digest of a platform-pinned service's
// image, resolved for the service's platform rather than the host's.
type pinnedImageDigest struct {
	digest string
	// from is the shared summary digest at resolution time: once a pull or
	// build refreshed the summary, this resolution is stale and
	// serviceImageDigest re-resolves the pinned platform, keeping the
	// refreshed shared value only as fallback.
	from string
}

func (s *composeService) getLocalImagesDigests(ctx context.Context, project *types.Project) (map[string]api.ImageSummary, map[string]pinnedImageDigest, error) {
	imageNames := utils.Set[string]{}
	for _, s := range project.Services {
		imageNames.Add(api.GetImageNameOrDefault(s, project.Name))
		for _, volume := range s.Volumes {
			if volume.Type == types.VolumeTypeImage {
				imageNames.Add(volume.Source)
			}
		}
		for _, img := range api.GetDependentImages(s, project.Name) {
			imageNames.Add(img)
		}
	}
	inspections, err := s.inspectLocalImages(ctx, imageNames.Elements())
	if err != nil {
		return nil, nil, err
	}
	imgs := make(map[string]api.ImageSummary, len(inspections))
	for repoTag, inspect := range inspections {
		imgs[repoTag] = imageSummary(repoTag, inspect)
	}

	pinnedDigests := map[string]pinnedImageDigest{}
	for name, service := range project.Services {
		if service.Platform == "" {
			continue
		}
		imgName := api.GetImageNameOrDefault(service, project.Name)
		img, ok := imgs[imgName]
		if !ok {
			continue
		}
		// digest selection and platform validation share the same manifest
		// resolution (matchLocalManifest) on the inspect we already hold,
		// otherwise the digest recorded and the platform validated could
		// refer to different manifests of the same image
		digest, satisfied, err := localContentDigest(inspections[imgName], service.Platform)
		if err != nil {
			return nil, nil, err
		}
		if !satisfied {
			logrus.Debugf("local image %s doesn't match expected platform %s", service.Image, service.Platform)
			// there is a local image, but it's for the wrong platform, so
			// pretend it doesn't exist so that we can pull/build an image
			// for the correct platform instead
			delete(imgs, imgName)
			continue
		}
		pinnedDigests[name] = pinnedImageDigest{digest: digest, from: img.ID}
	}

	return imgs, pinnedDigests, nil
}

// resolveAndMergeBuildArgs returns the final set of build arguments to use for the service image build.
//
// First, args directly defined via `build.args` in YAML are considered.
// Then, any explicitly passed args in opts (e.g. via `--build-arg` on the CLI) are merged, overwriting any
// keys that already exist.
// Next, any keys without a value are resolved using the project environment.
//
// Finally, standard proxy variables based on the Docker client configuration are added, but will not overwrite
// any values if already present.
func resolveAndMergeBuildArgs(proxyConfig map[string]string, project *types.Project, service types.ServiceConfig, opts api.BuildOptions) types.MappingWithEquals {
	result := make(types.MappingWithEquals).
		OverrideBy(service.Build.Args).
		OverrideBy(opts.Args).
		Resolve(envResolver(project.Environment))

	// proxy arguments do NOT override and should NOT have env resolution applied,
	// so they're handled last
	for k, v := range proxyConfig {
		if _, ok := result[k]; !ok {
			v := v
			result[k] = &v
		}
	}
	return result
}

func getImageBuildLabels(project *types.Project, service types.ServiceConfig) types.Labels {
	ret := make(types.Labels)
	if service.Build != nil {
		for k, v := range service.Build.Labels {
			ret.Add(k, v)
		}
	}

	ret.Add(api.VersionLabel, api.ComposeVersion)
	ret.Add(api.ProjectLabel, project.Name)
	ret.Add(api.ServiceLabel, service.Name)
	return ret
}

func addBuildDependencies(services []string, project *types.Project) []string {
	servicesWithDependencies := utils.NewSet(services...)
	for _, service := range services {
		s, ok := project.Services[service]
		if !ok {
			s = project.DisabledServices[service]
		}
		b := s.Build
		if b != nil {
			for _, target := range b.AdditionalContexts {
				if s, found := strings.CutPrefix(target, types.ServicePrefix); found {
					servicesWithDependencies.Add(s)
				}
			}
		}
	}
	if len(servicesWithDependencies) > len(services) {
		return addBuildDependencies(servicesWithDependencies.Elements(), project)
	}
	return servicesWithDependencies.Elements()
}
