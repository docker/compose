FROM debian:jessie

RUN apt-get update && apt-get install -y \
	build-essential \
	ca-certificates \
	curl \
	git \
	make \
	--no-install-recommends \
	&& rm -rf /var/lib/apt/lists/*

# Install Go
ENV GO_VERSION 1.5.3
RUN curl -sSL  "https://storage.googleapis.com/golang/go${GO_VERSION}.linux-amd64.tar.gz" | tar -v -C /usr/local -xz
ENV PATH /go/bin:/usr/local/go/bin:$PATH
ENV GOPATH /go:/go/src/github.com/docker/containerd/vendor

# install golint/vet
RUN go get github.com/golang/lint/golint \
	&& go get golang.org/x/tools/cmd/vet

COPY . /go/src/github.com/docker/containerd

WORKDIR /go/src/github.com/docker/containerd
