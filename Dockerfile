FROM docker:18.09.3 AS docker
FROM python:3.7.2-alpine3.9 AS builder
RUN apk --update add \
        gcc \
        libffi-dev \
        musl-dev \
        openssl-dev \
        yaml-dev \
        make \
        python3-dev

WORKDIR /code
COPY setup.py LICENSE README.md MANIFEST.in ./
COPY compose ./compose
RUN python setup.py install --user

FROM python:3.7.2-alpine3.9
COPY --from=builder /root/.local /usr/local
COPY --from=docker /usr/local/bin/docker /usr/local/bin/docker
ENTRYPOINT ["/usr/local/bin/docker-compose"]