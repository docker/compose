FROM debian:wheezy

RUN set -ex; \
    apt-get update -qq; \
    apt-get install -y \
        gcc \
        make \
        zlib1g \
        zlib1g-dev \
        libssl-dev \
        git \
        apt-transport-https \
        ca-certificates \
        curl \
        lxc \
        iptables \
    ; \
    rm -rf /var/lib/apt/lists/*

# Build Python 2.7.9 from source
RUN set -ex; \
    curl -LO https://www.python.org/ftp/python/2.7.9/Python-2.7.9.tgz; \
    tar -xzf Python-2.7.9.tgz; \
    cd Python-2.7.9; \
    ./configure --enable-shared; \
    make; \
    make install; \
    cd ..; \
    rm -rf /Python-2.7.9; \
    rm Python-2.7.9.tgz

# Make libpython findable
ENV LD_LIBRARY_PATH /usr/local/lib

# Install setuptools
RUN set -ex; \
    curl -LO https://bootstrap.pypa.io/ez_setup.py; \
    python ez_setup.py; \
    rm ez_setup.py

# Install pip
RUN set -ex; \
    curl -LO https://pypi.python.org/packages/source/p/pip/pip-7.0.1.tar.gz; \
    tar -xzf pip-7.0.1.tar.gz; \
    cd pip-7.0.1; \
    python setup.py install; \
    cd ..; \
    rm -rf pip-7.0.1; \
    rm pip-7.0.1.tar.gz

ENV ALL_DOCKER_VERSIONS 1.7.1

RUN set -ex; \
    curl https://get.docker.com/builds/Linux/x86_64/docker-1.7.1 -o /usr/local/bin/docker-1.7.1; \
    chmod +x /usr/local/bin/docker-1.7.1

# Set the default Docker to be run
RUN ln -s /usr/local/bin/docker-1.7.1 /usr/local/bin/docker

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
