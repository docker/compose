FROM golang:1.3

ADD . /go/src/github.com/docker/fig
WORKDIR /go/src/github.com/docker/fig
RUN go get ./...
RUN go build
