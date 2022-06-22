FROM alpine

COPY --from=base /hello.txt /hello.txt

CMD [ "cat", "/hello.txt" ]
