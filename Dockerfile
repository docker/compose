FROM python:2.7.10-wheezy

RUN set -ex; \
    apt-get update -qq; \
    apt-get install -y \
        git \
        apt-transport-https \
        ca-certificates \
        curl \
        lxc \
        iptables \
        libsqlite3-dev \
    ; \
    rm -rf /var/lib/apt/lists/*

ENV ALL_DOCKER_VERSIONS 1.7.1 1.8.1

RUN set -ex; \
    curl https://get.docker.com/builds/Linux/x86_64/docker-1.7.1 -o /usr/local/bin/docker-1.7.1; \
    chmod +x /usr/local/bin/docker-1.7.1; \
    curl https://get.docker.com/builds/Linux/x86_64/docker-1.8.1 -o /usr/local/bin/docker-1.8.1; \
    chmod +x /usr/local/bin/docker-1.8.1

# Set the default Docker to be run
RUN ln -s /usr/local/bin/docker-1.7.1 /usr/local/bin/docker

RUN useradd -d /home/user -m -s /bin/bash user
WORKDIR /code/

ADD requirements.txt /code/
RUN pip install -r requirements.txt

ADD requirements-dev.txt /code/
RUN pip install -r requirements-dev.txt

RUN pip install tox==2.1.1

ADD . /code/
RUN python setup.py install

RUN chown -R user /code/

ENTRYPOINT ["/usr/local/bin/docker-compose"]
