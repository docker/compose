## Codefresh fork

The original fork was made to build the docker image for the Codefresh managed docker-compose [image](https://hub.docker.com/repository/docker/codefresh/compose)

It was needed for an ARM version of the image, as well as for replacing the `--compatibility` flag default value from `false` to `true`.

Since it was created, the official image started to release ARM versions, as well as exposing the flag as an env variable. However - the official image is not a runnable container. instead, it's just a way to deliver the binaries. so the current Dockerfile just copies the binary for docker-compose into our image and sets the `COMPOSE_COMPATIBILITY` env variable to `true`.
