
all:
	mkdir -p bin/
	cd containerd && go build -tags libcontainer -o ../bin/containerd
	cd ctr && go build -o ../bin/ctr

