
BUILDTAGS=libcontainer

all:
	mkdir -p bin/
	cd containerd && go build -tags "$(BUILDTAGS)" -o ../bin/containerd
	cd ctr && go build -o ../bin/ctr

client:
	mkdir -p bin/
	cd ctr && go build -o ../bin/ctr

daemon:
	mkdir -p bin/
	cd containerd && go build -tags "$(BUILDTAGS)" -o ../bin/containerd

install:
	cp bin/* /usr/local/bin/

protoc:
	protoc -I ./api/grpc/types ./api/grpc/types/api.proto --go_out=plugins=grpc:api/grpc/types
