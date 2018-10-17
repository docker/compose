FROM docker:18.06.1 as docker
FROM python:3.6

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

ENTRYPOINT ["/code/.tox/py36/bin/docker-compose"]
