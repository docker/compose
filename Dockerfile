# syntax = docker/dockerfile:experimental
ARG GO_VERSION=1.14.2

FROM golang:${GO_VERSION} AS fs
ARG TARGET_OS=unknown
ARG TARGET_ARCH=unknown
ARG PWD=$GOPATH/src/github.com/docker/api
RUN apt-get update && apt-get install --no-install-recommends -y \
    make \
    git \
    protobuf-compiler \
    libprotobuf-dev
RUN go get github.com/golang/protobuf/protoc-gen-go && \
    go get golang.org/x/tools/cmd/goimports && \
    go get gotest.tools/gotestsum
WORKDIR ${PWD}
ADD go.* ${PWD}
RUN go mod download
ADD . ${PWD}

FROM fs AS make-protos
RUN make protos

FROM make-protos AS make-bins
RUN --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGET_OS} \
    GOARCH=${TARGET_ARCH} \
    make bins

FROM make-protos as make-test
RUN make test

FROM make-protos AS make-xbins
RUN --mount=type=cache,target=/root/.cache/go-build \
    make xbins

FROM scratch AS protos
COPY --from=make-protos /go/src/github.com/docker/api .

FROM scratch AS bins
COPY --from=make-bins /go/src/github.com/docker/api/bin/* .

FROM scratch AS xbins
COPY --from=make-xbins /go/src/github.com/docker/api/bin/* .
