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

# ENV ALL_DOCKER_VERSIONS 1.6.0

# RUN set -ex; \
#     for v in ${ALL_DOCKER_VERSIONS}; do \
#         curl https://get.docker.com/builds/Linux/x86_64/docker-$v -o /usr/local/bin/docker-$v; \
#         chmod +x /usr/local/bin/docker-$v; \
#     done

# Temporarily use dev version of Docker
ENV ALL_DOCKER_VERSIONS dev
ADD ./docker-dev /usr/local/bin/docker-dev
RUN chmod +x /usr/local/bin/docker-dev

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
