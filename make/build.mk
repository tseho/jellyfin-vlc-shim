.PHONY: build
build:
	go build -o bin/jellyfin-vlc-shim cmd/jellyfin-vlc-shim/main.go

.PHONY: build-with-docker
build-with-docker:
	docker buildx build --progress=plain --platform linux/amd64 --build-arg ARCH=amd64 -t jellyfin-vlc-shim:linux-amd64 -f Dockerfile .

bin/jellyfin-vlc-shim-linux-amd64: export GOOS = linux
bin/jellyfin-vlc-shim-linux-amd64: export GOARCH = amd64
bin/jellyfin-vlc-shim-linux-amd64: export CGO_ENABLED = 1
bin/jellyfin-vlc-shim-linux-amd64:
	go build -o bin/jellyfin-vlc-shim-linux-amd64 cmd/jellyfin-vlc-shim/main.go

bin/jellyfin-vlc-shim-linux-arm64:
	docker buildx build --load --progress=plain --platform linux/arm64 --build-arg ARCH=arm64 -t jellyfin-vlc-shim:linux-arm64 -f Dockerfile .
	docker rm jellyfin-vlc-shim-linux-arm64-container || true
	docker create --name jellyfin-vlc-shim-linux-arm64-container jellyfin-vlc-shim:linux-arm64
	docker cp jellyfin-vlc-shim-linux-arm64-container:/jellyfin-vlc-shim/bin/jellyfin-vlc-shim ./bin/jellyfin-vlc-shim-linux-arm64
	docker rm jellyfin-vlc-shim-linux-arm64-container
