FROM docker:18.09.3 as docker
FROM python:3.7

RUN set -ex; \
    apt-get update -qq; \
    apt-get install -y \
        locales \
        python-dev \
        git

COPY --from=docker /usr/local/bin/docker /usr/local/bin/docker

# Python3 requires a valid locale
RUN echo "en_US.UTF-8 UTF-8" > /etc/locale.gen && locale-gen
ENV LANG en_US.UTF-8

RUN useradd -d /home/user -m -s /bin/bash user
WORKDIR /code/

# FIXME(chris-crone): virtualenv 16.3.0 breaks build, force 16.2.0 until fixed
RUN pip install virtualenv==16.2.0
RUN pip install tox==3.7.0

COPY requirements.txt requirements-dev.txt .pre-commit-config.yaml setup.py tox.ini README.md /code/
ADD compose/ /code/compose/
RUN tox --notest

RUN chown -R user /code/

ENTRYPOINT ["/code/.tox/py37/bin/docker-compose"]
