.DEFAULT_GOAL := build

.PHONY: build
build: jellyfin-vlc-shim

jellyfin-vlc-shim: main.go go.mod go.sum
	go build -o jellyfin-vlc-shim

.PHONY: tests
tests: build
tests:
	(cd tests && docker compose --profile server up -d)
	@kill `cat .tmp/pid` > /dev/null 2>&1 || true
	@rm -rf .tmp
	@mkdir -p .tmp
	./jellyfin-vlc-shim --config .tmp/config auth --url http://localhost:8096 --username admin --password admin --device-name TV
	./jellyfin-vlc-shim --config .tmp/config run & echo $$! > .tmp/pid
	(cd tests/playwright && CI=1 npx playwright test)
	@kill `cat .tmp/pid` > /dev/null 2>&1 || true

.PHONY: sources
sources: sources/jellyfin-mpv-shim
sources: sources/jellyfin-apiclient-python

sources/jellyfin-mpv-shim:
	git clone https://github.com/jellyfin/jellyfin-mpv-shim.git sources/jellyfin-mpv-shim
	rm -rf sources/jellyfin-mpv-shim/.git

sources/jellyfin-apiclient-python:
	git clone https://github.com/jellyfin/jellyfin-apiclient-python.git sources/jellyfin-apiclient-python
	rm -rf sources/jellyfin-apiclient-python/.git
