package execution

//go:generate protoc -I.:../../../../../github.com/gogo/protobuf --gogoctrd_out=plugins=grpc,import_path=github.com/docker/containerd/api/execution,Mgogoproto/gogo.proto=github.com/gogo/protobuf/gogoproto,Mgoogle/protobuf/descriptor.proto=github.com/gogo/protobuf/protoc-gen-gogo/descriptor:. execution.proto
