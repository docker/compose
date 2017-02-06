FROM debian:wheezy

RUN set -ex; \
    apt-get update -qq; \
    apt-get install -y \
        locales \
        gcc \
        make \
        zlib1g \
        zlib1g-dev \
        libssl-dev \
        git \
        ca-certificates \
        curl \
        libsqlite3-dev \
        libbz2-dev \
    ; \
    rm -rf /var/lib/apt/lists/*

RUN curl https://get.docker.com/builds/Linux/x86_64/docker-1.8.3 \
        -o /usr/local/bin/docker && \
    chmod +x /usr/local/bin/docker

# Build Python 2.7.13 from source
RUN set -ex; \
    curl -L https://www.python.org/ftp/python/2.7.13/Python-2.7.13.tgz | tar -xz; \
    cd Python-2.7.13; \
    ./configure --enable-shared; \
    make; \
    make install; \
    cd ..; \
    rm -rf /Python-2.7.13

# Build python 3.4 from source
RUN set -ex; \
    curl -L https://www.python.org/ftp/python/3.4.6/Python-3.4.6.tgz | tar -xz; \
    cd Python-3.4.6; \
    ./configure --enable-shared; \
    make; \
    make install; \
    cd ..; \
    rm -rf /Python-3.4.6

# Make libpython findable
ENV LD_LIBRARY_PATH /usr/local/lib

# Install pip
RUN set -ex; \
    curl -L https://bootstrap.pypa.io/get-pip.py | python

# Python3 requires a valid locale
RUN echo "en_US.UTF-8 UTF-8" > /etc/locale.gen && locale-gen
ENV LANG en_US.UTF-8

RUN useradd -d /home/user -m -s /bin/bash user
WORKDIR /code/

RUN pip install tox==2.1.1

ADD requirements.txt /code/
ADD requirements-dev.txt /code/
ADD .pre-commit-config.yaml /code/
ADD setup.py /code/
ADD tox.ini /code/
ADD compose /code/compose/
RUN tox --notest

ADD . /code/
RUN chown -R user /code/

ENTRYPOINT ["/code/.tox/py27/bin/docker-compose"]
