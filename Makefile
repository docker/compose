
all:
	mkdir -p bin/
	cd containerd && go build -tags libcontainer -o ../bin/containerd
	cd ctr && go build -o ../bin/ctr

client:
	mkdir -p bin/
	cd ctr && go build -o ../bin/ctr

daemon:
	mkdir -p bin/
	cd containerd && go build -tags libcontainer -o ../bin/containerd

install:
	cp bin/* /usr/local/bin/
