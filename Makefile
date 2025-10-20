.DEFAULT_GOAL := build

.PHONY: build
build: jellyfin-vlc-shim

jellyfin-vlc-shim: go.mod
jellyfin-vlc-shim: go.sum
jellyfin-vlc-shim: main.go
jellyfin-vlc-shim: commands/auth.go
jellyfin-vlc-shim: commands/play.go
jellyfin-vlc-shim: commands/start.go
jellyfin-vlc-shim: config/config.go
jellyfin-vlc-shim: jellyfin/auth.go
jellyfin-vlc-shim: jellyfin/client.go
jellyfin-vlc-shim: jellyfin/types.go
jellyfin-vlc-shim: logger/logger.go
jellyfin-vlc-shim: player/events.go
jellyfin-vlc-shim: player/player.go
jellyfin-vlc-shim:
	go build -o jellyfin-vlc-shim

jellyfin-vlc-shim-linux-amd64: export GOOS = linux
jellyfin-vlc-shim-linux-amd64: export GOARCH = amd64
jellyfin-vlc-shim-linux-amd64: export CGO_ENABLED = 1
jellyfin-vlc-shim-linux-amd64:
	go build -o jellyfin-vlc-shim-linux-amd64

jellyfin-vlc-shim-linux-arm64:
	docker buildx build --load --progress=plain --platform linux/arm64 --build-arg ARCH=arm64 -t jellyfin-vlc-shim:linux-arm64 -f Dockerfile .
	docker rm jellyfin-vlc-shim-linux-arm64-container || true
	docker create --name jellyfin-vlc-shim-linux-arm64-container jellyfin-vlc-shim:linux-arm64
	docker cp jellyfin-vlc-shim-linux-arm64-container:/jellyfin-vlc-shim/jellyfin-vlc-shim ./jellyfin-vlc-shim-linux-arm64
	docker rm jellyfin-vlc-shim-linux-arm64-container

.PHONY: tests
tests: build
tests: export CI ?= 1
tests:
	(cd tests && docker compose --profile server up -d)
	@killall jellyfin-vlc-shim > /dev/null 2>&1 || true
	@killall -9 jellyfin-vlc-shim > /dev/null 2>&1 || true
	@rm -rf .tmp
	@mkdir -p .tmp
	./jellyfin-vlc-shim --config .tmp/config auth --url http://localhost:8096 --username admin --password admin --device-name TV
	./jellyfin-vlc-shim --config .tmp/config &
	(cd tests/playwright && npm run test)
	@killall jellyfin-vlc-shim > /dev/null 2>&1 || true
	@killall -9 jellyfin-vlc-shim > /dev/null 2>&1 || true

.PHONY: flush-test-databases
flush-test-databases:
	sqlite3 tests/jellyfin/config/data/jellyfin.db "PRAGMA wal_checkpoint(TRUNCATE);"
	sqlite3 tests/jellyfin/config/data/library.db "PRAGMA wal_checkpoint(TRUNCATE);"

.PHONY: sources
sources: sources/jellyfin-mpv-shim
sources: sources/jellyfin-apiclient-python
sources: sources/libvlc-go

sources/jellyfin-mpv-shim:
	git clone https://github.com/jellyfin/jellyfin-mpv-shim.git sources/jellyfin-mpv-shim
	rm -rf sources/jellyfin-mpv-shim/.git

sources/jellyfin-apiclient-python:
	git clone https://github.com/jellyfin/jellyfin-apiclient-python.git sources/jellyfin-apiclient-python
	rm -rf sources/jellyfin-apiclient-python/.git

sources/libvlc-go:
	git clone https://github.com/adrg/libvlc-go.git sources/libvlc-go
	rm -rf sources/libvlc-go/.git
