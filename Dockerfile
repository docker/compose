FROM debian:jessie

RUN apt-get update && apt-get install -y \
	build-essential \
	ca-certificates \
	curl \
	git \
	make \
	jq \
	apparmor \
	libapparmor-dev \
	--no-install-recommends \
	&& rm -rf /var/lib/apt/lists/*

# Install Go
ENV GO_VERSION 1.5.3
RUN curl -sSL  "https://storage.googleapis.com/golang/go${GO_VERSION}.linux-amd64.tar.gz" | tar -v -C /usr/local -xz
ENV PATH /go/bin:/usr/local/go/bin:$PATH
ENV GOPATH /go:/go/src/github.com/docker/containerd/vendor

WORKDIR /go/src/github.com/docker/containerd

# install seccomp: the version shipped in trusty is too old
ENV SECCOMP_VERSION 2.3.0
RUN set -x \
	&& export SECCOMP_PATH="$(mktemp -d)" \
	&& curl -fsSL "https://github.com/seccomp/libseccomp/releases/download/v${SECCOMP_VERSION}/libseccomp-${SECCOMP_VERSION}.tar.gz" \
		| tar -xzC "$SECCOMP_PATH" --strip-components=1 \
	&& ( \
		cd "$SECCOMP_PATH" \
		&& ./configure --prefix=/usr/local \
		&& make \
		&& make install \
		&& ldconfig \
	) \
	&& rm -rf "$SECCOMP_PATH"

# Install runc
ENV RUNC_COMMIT d49ece5a83da3dcb820121d6850e2b61bd0a5fbe
RUN set -x \
	&& export GOPATH="$(mktemp -d)" \
    && git clone git://github.com/opencontainers/runc.git "$GOPATH/src/github.com/opencontainers/runc" \
	&& cd "$GOPATH/src/github.com/opencontainers/runc" \
	&& git checkout -q "$RUNC_COMMIT" \
	&& make BUILDTAGS="seccomp apparmor selinux" && make install

COPY . /go/src/github.com/docker/containerd

WORKDIR /go/src/github.com/docker/containerd

RUN make all install
