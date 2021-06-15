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

package ecs

import (
	"context"
	"fmt"

	"github.com/docker/compose-cli/api/volumes"
	"github.com/docker/compose-cli/pkg/api"

	"github.com/awslabs/goformation/v4/cloudformation"
	"github.com/awslabs/goformation/v4/cloudformation/efs"
	"github.com/compose-spec/compose-go/types"
	"github.com/pkg/errors"
)

func (b *ecsAPIService) createNFSMountTarget(project *types.Project, resources awsResources, template *cloudformation.Template) {
	for volume := range project.Volumes {
		for _, subnet := range resources.subnets {
			name := fmt.Sprintf("%sNFSMountTargetOn%s", normalizeResourceName(volume), normalizeResourceName(subnet.ID()))
			template.Resources[name] = &efs.MountTarget{
				FileSystemId:   resources.filesystems[volume].ID(),
				SecurityGroups: resources.allSecurityGroups(),
				SubnetId:       subnet.ID(),
			}
		}
	}
}

func (b *ecsAPIService) mountTargets(volume string, resources awsResources) []string {
	var refs []string
	for _, subnet := range resources.subnets {
		refs = append(refs, fmt.Sprintf("%sNFSMountTargetOn%s", normalizeResourceName(volume), normalizeResourceName(subnet.ID())))
	}
	return refs
}

func (b *ecsAPIService) createAccessPoints(project *types.Project, r awsResources, template *cloudformation.Template) {
	for name, volume := range project.Volumes {
		n := fmt.Sprintf("%sAccessPoint", normalizeResourceName(name))

		uid := volume.DriverOpts["uid"]
		gid := volume.DriverOpts["gid"]
		permissions := volume.DriverOpts["permissions"]
		path := volume.DriverOpts["root_directory"]

		ap := efs.AccessPoint{
			AccessPointTags: []efs.AccessPoint_AccessPointTag{
				{
					Key:   api.ProjectLabel,
					Value: project.Name,
				},
				{
					Key:   api.VolumeLabel,
					Value: name,
				},
				{
					Key:   "Name",
					Value: volume.Name,
				},
			},
			FileSystemId: r.filesystems[name].ID(),
		}

		if uid != "" {
			ap.PosixUser = &efs.AccessPoint_PosixUser{
				Uid: uid,
				Gid: gid,
			}
		}
		if path != "" {
			root := efs.AccessPoint_RootDirectory{
				Path: path,
			}
			ap.RootDirectory = &root
			if uid != "" {
				root.CreationInfo = &efs.AccessPoint_CreationInfo{
					OwnerUid:    uid,
					OwnerGid:    gid,
					Permissions: permissions,
				}
			}
		}

		template.Resources[n] = &ap
	}
}

// VolumeCreateOptions hold EFS filesystem creation options
type VolumeCreateOptions struct {
	KmsKeyID                     string
	PerformanceMode              string
	ProvisionedThroughputInMibps float64
	ThroughputMode               string
}

type ecsVolumeService struct {
	backend *ecsAPIService
}

func (e ecsVolumeService) List(ctx context.Context) ([]volumes.Volume, error) {
	filesystems, err := e.backend.aws.ListFileSystems(ctx, nil)
	if err != nil {
		return nil, err
	}
	var vol []volumes.Volume
	for _, fs := range filesystems {
		vol = append(vol, volumes.Volume{
			ID:          fs.ID(),
			Description: fs.ARN(),
		})
	}
	return vol, nil
}

func (e ecsVolumeService) Create(ctx context.Context, name string, options interface{}) (volumes.Volume, error) {
	fs, err := e.backend.aws.CreateFileSystem(ctx, map[string]string{
		"Name": name,
	}, options.(VolumeCreateOptions))
	return volumes.Volume{
		ID:          fs.ID(),
		Description: fs.ARN(),
	}, err

}

func (e ecsVolumeService) Delete(ctx context.Context, volumeID string, options interface{}) error {
	return e.backend.aws.DeleteFileSystem(ctx, volumeID)
}

func (e ecsVolumeService) Inspect(ctx context.Context, volumeID string) (volumes.Volume, error) {
	ok, err := e.backend.aws.ResolveFileSystem(ctx, volumeID)
	if ok == nil {
		err = errors.Wrapf(api.ErrNotFound, "filesystem %q does not exists", volumeID)
	}
	return volumes.Volume{
		ID:          volumeID,
		Description: ok.ARN(),
	}, err
}
