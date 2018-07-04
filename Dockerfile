FROM python:3.6

RUN set -ex; \
    apt-get update -qq; \
    apt-get install -y \
        locales \
        curl \
        python-dev \
        git

# Python3 requires a valid locale
RUN echo "en_US.UTF-8 UTF-8" > /etc/locale.gen && locale-gen
ENV LANG en_US.UTF-8

RUN useradd -d /home/user -m -s /bin/bash user
RUN mkdir /code/ && chown -R user:user /code/
RUN pip install tox==2.1.1

# Install the docker cli, which is used for docker-compose exec by default
ARG DOCKERBINS_VERSION=18.03.1
ARG DOCKERBINS_SHA=0e245c42de8a21799ab11179a4fce43b494ce173a8a2d6567ea6825d6c5265aa
RUN curl -fsSL -o dockerbins.tgz "https://download.docker.com/linux/static/stable/x86_64/docker-${DOCKERBINS_VERSION}-ce.tgz" && \
    echo "${DOCKERBINS_SHA} dockerbins.tgz" | sha256sum -c - && \
    tar xvf dockerbins.tgz docker/docker --strip-components 1 && \
    mv docker /usr/local/bin/docker && \
    chmod +x /usr/local/bin/docker && \
    rm dockerbins.tgz

WORKDIR /code/

COPY --chown=user:user . /code/

# Run tox as "user" so that generated files are owned by user
USER user:user
RUN tox --notest

USER root:root

ENTRYPOINT ["/code/.tox/py36/bin/docker-compose"]
