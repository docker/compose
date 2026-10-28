FROM alpine

RUN echo "foo" > /tmp/expected
RUN --mount=type=secret,id=mysecret cat /run/secrets/mysecret > /tmp/actual
RUN diff /tmp/expected /tmp/actual

RUN echo "bar" > /tmp/expected
RUN --mount=type=secret,id=build_secret cat /run/secrets/build_secret > tmp/actual
RUN diff --ignore-all-space /tmp/expected /tmp/actual

RUN echo "zot" > /tmp/expected
RUN --mount=type=secret,id=dotenvsecret cat /run/secrets/dotenvsecret > tmp/actual
RUN diff --ignore-all-space /tmp/expected /tmp/actual

