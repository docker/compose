# API

The API for containerd is with GRPC over a unix socket located at the default location of `/run/containerd/containerd.sock`.  

At this time please refer to the [proto at](https://github.com/docker/containerd/blob/master/api/grpc/types/api.proto) for the API methods and types.  
There is a Go implementation and types checked into this repository but alternate language implementations can be created using the grpc and protoc toolchain.
