FROM golang:alpine3.13 as builder  
RUN mkdir -p /go/src/github.com/cowdude/flapi
WORKDIR /go/src/github.com/cowdude/flapi
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64
COPY . .
RUN apk update && \
    apk add git && \
    go mod init && \
    go get github.com/UnnoTed/fileb0x && \
    rm -rf ./src/static && \
    go generate ./src/main.go && \
    go get ./src/... && \
    go build -o /server ./src

FROM flml/flashlight:cuda-latest@sha256:42ccb7981aa4edaa1d8881ce9711583d046d00db2d80049bf7114e1441417cf9

RUN apt-get update && \
    apt-get install --yes --no-install-recommends ffmpeg && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* /tmp/*

COPY --from=builder /server /usr/local/bin/
COPY config.yml /

WORKDIR /
CMD [ "/usr/local/bin/server", "-config", "config.yml" ]
