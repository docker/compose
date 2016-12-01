package container

//go:generate protoc -I.:../../../../../github.com/gogo/protobuf --gogoswarm_out=plugins=grpc,import_path=github.com/docker/containerd/api/registry,Mgogoproto/gogo.proto=github.com/gogo/protobuf/gogoproto,Mgoogle/protobuf/descriptor.proto=github.com/gogo/protobuf/protoc-gen-gogo/descriptor:. registry.proto
