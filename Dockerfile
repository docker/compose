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

RUN curl -fsSL -o dockerbins.tgz "https://download.docker.com/linux/static/stable/x86_64/docker-17.12.0-ce.tgz" && \
    SHA256=692e1c72937f6214b1038def84463018d8e320c8eaf8530546c84c2f8f9c767d; \
    echo "${SHA256}  dockerbins.tgz" | sha256sum -c - && \
    tar xvf dockerbins.tgz docker/docker --strip-components 1 && \
    mv docker /usr/local/bin/docker && \
    chmod +x /usr/local/bin/docker && \
    rm dockerbins.tgz

# Build Python 2.7.13 from source
RUN set -ex; \
    curl -LO https://www.python.org/ftp/python/2.7.13/Python-2.7.13.tgz && \
    SHA256=a4f05a0720ce0fd92626f0278b6b433eee9a6173ddf2bced7957dfb599a5ece1; \
    echo "${SHA256}  Python-2.7.13.tgz" | sha256sum -c - && \
    tar -xzf Python-2.7.13.tgz; \
    cd Python-2.7.13; \
    ./configure --enable-shared; \
    make; \
    make install; \
    cd ..; \
    rm -rf /Python-2.7.13; \
    rm Python-2.7.13.tgz

# Build python 3.6 from source
RUN set -ex; \
    curl -LO https://www.python.org/ftp/python/3.6.4/Python-3.6.4.tgz && \
    SHA256=9de6494314ea199e3633211696735f65; \
    echo "${SHA256}  Python-3.6.4.tgz" | md5sum -c - && \
    tar -xzf Python-3.6.4.tgz; \
    cd Python-3.6.4; \
    ./configure --enable-shared; \
    make; \
    make install; \
    cd ..; \
    rm -rf /Python-3.6.4; \
    rm Python-3.6.4.tgz

# Make libpython findable
ENV LD_LIBRARY_PATH /usr/local/lib

# Install pip
RUN set -ex; \
    curl -LO https://bootstrap.pypa.io/get-pip.py && \
    SHA256=19dae841a150c86e2a09d475b5eb0602861f2a5b7761ec268049a662dbd2bd0c; \
    echo "${SHA256}  get-pip.py" | sha256sum -c - && \
    python get-pip.py

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
