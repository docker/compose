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

package proxy

import (
	"context"

	"github.com/docker/compose-cli/aci"
	"github.com/docker/compose-cli/api/volumes"
	volumesv1 "github.com/docker/compose-cli/cli/server/protos/volumes/v1"
)

// VolumesCreate creates a volume.
func (p *proxy) VolumesCreate(ctx context.Context, req *volumesv1.VolumesCreateRequest) (*volumesv1.VolumesCreateResponse, error) {
	storageAccount := ""
	aciReq := req.GetAciOption()
	if aciReq != nil {
		storageAccount = aciReq.StorageAccount
	}
	aciOpts := aci.VolumeCreateOptions{
		Account: storageAccount,
	}

	v, err := Client(ctx).VolumeService().Create(ctx, req.Name, aciOpts)
	if err != nil {
		return &volumesv1.VolumesCreateResponse{}, err
	}

	return &volumesv1.VolumesCreateResponse{
		Volume: toGrpcVolume(v),
	}, nil
}

// VolumesList lists the volumes.
func (p *proxy) VolumesList(ctx context.Context, req *volumesv1.VolumesListRequest) (*volumesv1.VolumesListResponse, error) {
	volumeList, err := Client(ctx).VolumeService().List(ctx)
	if err != nil {
		return &volumesv1.VolumesListResponse{}, err
	}

	return &volumesv1.VolumesListResponse{
		Volumes: toGrpcVolumeList(volumeList),
	}, nil
}

// VolumesDelete deletes a volume.
func (p *proxy) VolumesDelete(ctx context.Context, req *volumesv1.VolumesDeleteRequest) (*volumesv1.VolumesDeleteResponse, error) {
	err := Client(ctx).VolumeService().Delete(ctx, req.Id, nil)
	return &volumesv1.VolumesDeleteResponse{}, err
}

// VolumesInspect inspects a volume.
func (p *proxy) VolumesInspect(ctx context.Context, req *volumesv1.VolumesInspectRequest) (*volumesv1.VolumesInspectResponse, error) {
	v, err := Client(ctx).VolumeService().Inspect(ctx, req.Id)
	return &volumesv1.VolumesInspectResponse{
		Volume: toGrpcVolume(v),
	}, err
}

func toGrpcVolumeList(volumeList []volumes.Volume) []*volumesv1.Volume {
	var ret []*volumesv1.Volume
	for _, v := range volumeList {
		ret = append(ret, toGrpcVolume(v))
	}
	return ret
}

func toGrpcVolume(v volumes.Volume) *volumesv1.Volume {
	return &volumesv1.Volume{
		Id:          v.ID,
		Description: v.Description,
	}
}
