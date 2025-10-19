.DEFAULT_GOAL := build

.PHONY: build
build: jellyfin-vlc-shim

jellyfin-vlc-shim: main.go go.mod go.sum
	go build -o jellyfin-vlc-shim

.PHONY: tests
tests:
	(cd tests && docker compose up -d)
	(cd tests/playwright && npx playwright test)
