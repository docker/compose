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

# This Dockerfile is used to build a service that runs indefinitely.
# It uses the busybox image and runs the 'sleep infinity' command.

# Use a smaller base image to reduce the overall image size
FROM busybox:1.32.0-uclibc

# Add metadata about the image
LABEL maintainer="Docker Compose CLI authors"
LABEL version="1.0"

# Run the 'sleep infinity' command to keep the container running indefinitely
RUN sleep infinity

# Add a health check to monitor the health of the container
HEALTHCHECK CMD pgrep sleep || exit 1
