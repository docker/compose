#   Copyright 2020 Docker Compose CLI authors

#   Licensed under the Apache License, Version 2.0 (the "License");
#   you may not use this file except in compliance with the License.
#   You may obtain a copy of the License at

#       http://www.apache.org/licenses/LICENSE-2.0

#   Unless required by applicable law or agreed to in writing, software
#   distributed under the License is distributed on an "AS IS" BASIS,
#   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#   See the License for the specific language governing permissions and
#   limitations under the License.

FROM ubuntu:20.04 AS base
RUN apt-get update && apt-get install -y \
    curl
RUN curl https://get.docker.com | sh

FROM base AS install
COPY install_linux.sh /scripts/install_linux.sh
RUN chmod +x /scripts/install_linux.sh
ARG DOWNLOAD_URL=
RUN DOWNLOAD_URL=${DOWNLOAD_URL} /scripts/install_linux.sh
RUN docker version | grep Cloud

# check we can update
FROM install AS upgrade
RUN DOWNLOAD_URL=${DOWNLOAD_URL} /scripts/install_linux.sh
RUN docker version | grep Cloud

# To run this test locally, start an HTTP server that serves the dist/ folder
# then run a docker build passing the DOWNLOAD_URL as a build arg:
# $ cd dist/ && python3 -m http.server &
# $ docker build -f test.Dockerfile --build-arg DOWNLOAD_URL=http://192.168.0.22:8000/docker-linux-amd64.tar.gz .
