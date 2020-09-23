package proxy

import (
	"context"

	"github.com/docker/compose-cli/api/volumes"
	volumesv1 "github.com/docker/compose-cli/protos/volumes/v1"
)

// VolumesCreate creates a volume.
func (p *proxy) VolumesCreate(ctx context.Context, req *volumesv1.VolumesCreateRequest) (*volumesv1.VolumesCreateResponse, error) {
	v, err := Client(ctx).VolumeService().Create(ctx, req.Options)
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
	err := Client(ctx).VolumeService().Delete(ctx, req.Id, req.Options)
	return &volumesv1.VolumesDeleteResponse{}, err
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
