FROM docker:18.06.1 as docker
FROM python:3.7.2-stretch

RUN set -ex \
    && apt-get update -qq \
    && apt-get install -y \
        locales \
        python-dev \
        git \
    && pip install virtualenv==16.2.0 tox==2.9.1
# FIXME(chris-crone): virtualenv 16.3.0 breaks build, force 16.2.0 until fixed

# Python3 requires a valid locale
RUN echo 'en_US.UTF-8 UTF-8' > /etc/locale.gen && locale-gen
ENV LANG en_US.UTF-8

WORKDIR /code/

COPY --from=docker /usr/local/bin/docker /usr/local/bin/docker
COPY . /code/

RUN useradd -d /home/user -m -s /bin/bash user
RUN tox --notest

RUN chown -R user /code/

ENTRYPOINT ["/code/.tox/py37/bin/docker-compose"]
