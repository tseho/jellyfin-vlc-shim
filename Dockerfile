FROM golang:1.25

RUN apt-get update && apt-get install -y \
    libvlc-dev \
    gcc \
    libx11-dev \
    libxrandr-dev \
    libgl1-mesa-dev \
    libxcursor-dev \
    libxinerama-dev \
    libxi-dev \
    libxxf86vm-dev \
    && rm -rf /var/lib/apt/lists/*

ARG ARCH=amd64
ENV GOOS=linux
ENV GOARCH=${ARCH}
ENV CGO_ENABLED=1

WORKDIR /jellyfin-vlc-shim

COPY go.mod go.sum ./
RUN go mod download

COPY cmd cmd
COPY internal internal

RUN go build -o bin/jellyfin-vlc-shim cmd/jellyfin-vlc-shim/main.go
