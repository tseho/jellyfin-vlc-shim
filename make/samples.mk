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
