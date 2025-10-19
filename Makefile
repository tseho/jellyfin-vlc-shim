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
jellyfin-vlc-shim: player/events.go
jellyfin-vlc-shim: player/player.go
jellyfin-vlc-shim: 
	go build -o jellyfin-vlc-shim

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
