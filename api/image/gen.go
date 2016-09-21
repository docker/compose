package image

//go:generate protoc -I .:../../..:$GOPATH/src --gogo_out=plugins=grpc,import_path=github.com/docker/containerkit/api/image:. image.proto
