.DEFAULT_GOAL := build

.PHONY: build
build:
	go build -o jellyfin-vlc-shim

.PHONY: build-with-docker
build-with-docker:
	docker buildx build --progress=plain --platform linux/amd64 --build-arg ARCH=amd64 -t jellyfin-vlc-shim:linux-amd64 -f Dockerfile .

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

assets/source.mov:
	wget -O assets/source.mov https://download.blender.org/peach/bigbuckbunny_movies/big_buck_bunny_1080p_h264.mov

tests/jellyfin/media/mkv_1080_H264_aac/mkv_1080_H264_aac.mkv:
	mkdir -p tests/jellyfin/media/mkv_1080_H264_aac
	ffmpeg -y \
		-i assets/source.mov \
		-map 0:v \
		-map 0:a \
		-t 10 \
		-c:v libx264 \
		-c:a aac \
		tests/jellyfin/media/mkv_1080_H264_aac/mkv_1080_H264_aac.mkv

tests/jellyfin/media/mkv_1080_H265_aac_srt/mkv_1080_H265_aac_srt.mkv:
	mkdir -p tests/jellyfin/media/mkv_1080_H265_aac_srt
	ffmpeg -y \
		-i assets/source.mov \
		-i assets/common_voice_en_43197809.mp3 \
		-i assets/common_voice_fr_43346067.mp3 \
		-i assets/fr.srt \
		-i assets/en.srt \
		-map 0:v \
		-map 1:a \
		-map 2:a \
		-map 3:0 -map 4:0 \
		-t 10 \
		-c:v libx265 \
		-c:a aac \
		-c:s:0 srt \
		-c:s:1 srt \
		-metadata:s:a:0 language=eng -metadata:s:a:0 title="English" \
		-metadata:s:a:1 language=fre -metadata:s:a:1 title="Français" \
		-metadata:s:s:0 language=fre -metadata:s:s:0 title="Français (SRT)" \
		-metadata:s:s:1 language=eng -metadata:s:s:1 title="English (SRT)" \
		tests/jellyfin/media/mkv_1080_H265_aac_srt/mkv_1080_H265_aac_srt.mkv

tests/jellyfin/media/mkv_1080_H265_aac_ass/mkv_1080_H265_aac_ass.mkv:
	mkdir -p tests/jellyfin/media/mkv_1080_H265_aac_ass
	ffmpeg -y \
		-i assets/source.mov \
		-i assets/common_voice_en_43197809.mp3 \
		-i assets/common_voice_fr_43346067.mp3 \
		-i assets/fr.srt \
		-i assets/en.srt \
		-map 0:v \
		-map 1:a \
		-map 2:a \
		-map 3:0 -map 4:0 \
		-t 10 \
		-c:v libx265 \
		-c:a aac \
		-c:s:0 ass \
		-c:s:1 ass \
		-metadata:s:a:0 language=eng -metadata:s:a:0 title="English" \
		-metadata:s:a:1 language=fre -metadata:s:a:1 title="Français" \
		-metadata:s:s:0 language=fre -metadata:s:s:0 title="Français (ASS)" \
		-metadata:s:s:1 language=eng -metadata:s:s:1 title="English (ASS)" \
		tests/jellyfin/media/mkv_1080_H265_aac_ass/mkv_1080_H265_aac_ass.mkv

tests/jellyfin/media/mp4_1080_H265_aac_mov_text/mp4_1080_H265_aac_mov_text.mp4:
	mkdir -p tests/jellyfin/media/mp4_1080_H265_aac_mov_text
	ffmpeg -y \
		-i assets/source.mov \
		-i assets/common_voice_en_43197809.mp3 \
		-i assets/common_voice_fr_43346067.mp3 \
		-i assets/fr.srt \
		-i assets/en.srt \
		-map 0:v \
		-map 1:a \
		-map 2:a \
		-map 3:0 -map 4:0 \
		-t 10 \
		-c:v libx265 \
		-c:a aac \
		-c:s:0 mov_text \
		-c:s:1 mov_text \
		-metadata:s:a:0 language=eng -metadata:s:a:0 title="English" \
		-metadata:s:a:1 language=fre -metadata:s:a:1 title="Français" \
		-metadata:s:s:0 language=fre -metadata:s:s:0 title="Français (mov_text)" \
		-metadata:s:s:1 language=eng -metadata:s:s:1 title="English (mov_text)" \
		tests/jellyfin/media/mp4_1080_H265_aac_mov_text/mp4_1080_H265_aac_mov_text.mp4

tests/jellyfin/media/mp4_1080_H265_aac_ext_srt/mp4_1080_H265_aac_ext_srt.mp4:
	mkdir -p tests/jellyfin/media/mp4_1080_H265_aac_ext_srt
	ffmpeg -y \
		-i assets/source.mov \
		-i assets/common_voice_en_43197809.mp3 \
		-i assets/common_voice_fr_43346067.mp3 \
		-map 0:v \
		-map 1:a \
		-map 2:a \
		-t 10 \
		-c:v libx265 \
		-c:a aac \
		-metadata:s:a:0 language=eng -metadata:s:a:0 title="English" \
		-metadata:s:a:1 language=fre -metadata:s:a:1 title="Français" \
		tests/jellyfin/media/mp4_1080_H265_aac_ext_srt/mp4_1080_H265_aac_ext_srt.mp4
	cp assets/en.srt tests/jellyfin/media/mp4_1080_H265_aac_ext_srt/mp4_1080_H265_aac_ext_srt.en.srt
	cp assets/fr.srt tests/jellyfin/media/mp4_1080_H265_aac_ext_srt/mp4_1080_H265_aac_ext_srt.fr.srt

.PHONY: samples
samples: tests/jellyfin/media/mkv_1080_H264_aac/mkv_1080_H264_aac.mkv
samples: tests/jellyfin/media/mkv_1080_H265_aac_srt/mkv_1080_H265_aac_srt.mkv
samples: tests/jellyfin/media/mkv_1080_H265_aac_ass/mkv_1080_H265_aac_ass.mkv
samples: tests/jellyfin/media/mp4_1080_H265_aac_mov_text/mp4_1080_H265_aac_mov_text.mp4
samples: tests/jellyfin/media/mp4_1080_H265_aac_ext_srt/mp4_1080_H265_aac_ext_srt.mp4
