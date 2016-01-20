FROM debian:jessie

RUN apt-get update -qq && \
    apt-get install -y \
        libpython2.7 \
        libpython3.4 \
        locales \
        python2.7 \
        python2.7-dev \
        python-setuptools \
        python3.4 \
        python3.4-dev \
        python3-setuptools \
        git \
        ca-certificates \
        curl \
    && \
    rm -rf /var/lib/apt/lists/*

RUN curl https://get.docker.com/builds/Linux/x86_64/docker-1.8.3 \
        -o /usr/local/bin/docker && \
    chmod +x /usr/local/bin/docker

# Install pip
RUN curl -L https://bootstrap.pypa.io/get-pip.py | python

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
