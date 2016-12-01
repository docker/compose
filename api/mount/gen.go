package mount

//go:generate protoc -I.:../../../../../github.com/gogo/protobuf:/usr/local/include --gogoctrd_out=plugins=grpc,import_path=github.com/docker/containerd/api/mount,Mgogoproto/gogo.proto=github.com/gogo/protobuf/gogoproto,Mgoogle/protobuf/descriptor.proto=github.com/gogo/protobuf/protoc-gen-gogo/descriptor:. mount.proto
