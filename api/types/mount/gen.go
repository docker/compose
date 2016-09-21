package mount

//go:generate protoc -I .:../../..:$GOPATH/src --gogo_out=plugins=grpc,import_path=github.com/docker/containerkit/api/types/mount:. mount.proto

//+++go:generate protoc -I .:../../..:$GOPATH/src --gogo_out=plugins=grpc,import_path=github.com/docker/containerkit/api/types/mount,Mgogoproto/gogo.proto=github.com/gogo/protobuf/gogoproto:. mount.proto
