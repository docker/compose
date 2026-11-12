FROM alpine
RUN --mount=type=secret,id=test-secret ls -la /run/secrets/; cp /run/secrets/test-secret /tmp

CMD ["cat", "/tmp/test-secret"]

