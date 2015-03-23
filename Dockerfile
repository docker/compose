FROM debian:wheezy

RUN set -ex; \
    apt-get update -qq; \
    apt-get install -y \
        python \
        python-pip \
        python-dev \
        git \
        apt-transport-https \
        ca-certificates \
        curl \
        lxc \
        iptables \
    ; \
    rm -rf /var/lib/apt/lists/*

ENV ALL_DOCKER_VERSIONS 1.3.3 1.4.1 1.5.0 1.6.0-rc2

RUN set -ex; \
    for v in 1.3.3 1.4.1 1.5.0; do \
        curl https://get.docker.com/builds/Linux/x86_64/docker-$v -o /usr/local/bin/docker-$v; \
        chmod +x /usr/local/bin/docker-$v; \
    done; \
    curl https://test.docker.com/builds/Linux/x86_64/docker-1.6.0-rc2 -o /usr/local/bin/docker-1.6.0-rc2; \
    chmod +x /usr/local/bin/docker-1.6.0-rc2

# Set the default Docker to be run
RUN ln -s /usr/local/bin/docker-1.3.3 /usr/local/bin/docker

RUN useradd -d /home/user -m -s /bin/bash user
WORKDIR /code/

ADD requirements.txt /code/
RUN pip install -r requirements.txt

ADD requirements-dev.txt /code/
RUN pip install -r requirements-dev.txt

ADD . /code/
RUN python setup.py install

RUN chown -R user /code/

ENTRYPOINT ["/usr/local/bin/docker-compose"]
