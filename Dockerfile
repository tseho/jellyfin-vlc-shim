FROM golang:1.25

RUN apt-get update && apt-get install -y libvlc-dev gcc

ARG ARCH=amd64
ENV GOOS=linux
ENV GOARCH=${ARCH}
ENV CGO_ENABLED=1

WORKDIR /jellyfin-vlc-shim

COPY go.mod go.sum main.go ./
COPY commands commands
COPY config config
COPY jellyfin jellyfin
COPY player player
COPY logger logger

RUN go build -o jellyfin-vlc-shim
