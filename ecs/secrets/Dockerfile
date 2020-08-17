FROM golang:1.14.4-alpine AS builder
WORKDIR $GOPATH/src/github.com/docker/ecs-plugin/secrets
COPY . .
RUN GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /go/bin/secrets main/main.go

FROM scratch
COPY --from=builder /go/bin/secrets /secrets
ENTRYPOINT ["/secrets"]
