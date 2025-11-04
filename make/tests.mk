.PHONY: tests
tests: build
tests: export CI ?= 1
tests:
	(cd tests && docker compose --profile server up -d)
	@killall jellyfin-vlc-shim > /dev/null 2>&1 || true
	@killall -9 jellyfin-vlc-shim > /dev/null 2>&1 || true
	@rm -rf .tmp
	@mkdir -p .tmp
	./bin/jellyfin-vlc-shim --config .tmp/config auth --url http://localhost:8096 --username admin --password admin > /dev/null 2>&1
	@yq -i '.jellyfin_device="tests"' .tmp/config/configuration.json
	@yq -i '.fullscreen=false' .tmp/config/configuration.json
	@yq -i '.log_level="warn"' .tmp/config/configuration.json
	./bin/jellyfin-vlc-shim --config .tmp/config > /dev/null 2>&1 &
	(cd tests/playwright && npm run test)
	@killall jellyfin-vlc-shim > /dev/null 2>&1 || true
	@killall -9 jellyfin-vlc-shim > /dev/null 2>&1 || true

.PHONY: flush-test-databases
flush-test-databases:
	sqlite3 tests/jellyfin/config/data/jellyfin.db "PRAGMA wal_checkpoint(TRUNCATE);"
	sqlite3 tests/jellyfin/config/data/library.db "PRAGMA wal_checkpoint(TRUNCATE);"
