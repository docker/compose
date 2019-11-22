# syntax=docker/dockerfile:1.0.0-experimental
FROM busybox

# https://github.com/moby/moby/issues/1996#issuecomment-550020843
ARG CACHEBUST

RUN --mount=type=secret,target=/secret,required cp secret out
CMD cat out
