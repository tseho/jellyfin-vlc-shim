.PHONY: prerelease
prerelease:
	$(MAKE) -B bin/jellyfin-vlc-shim-linux-arm64
	scp bin/jellyfin-vlc-shim-linux-arm64 rpi28:/tmp/jellyfin-vlc-shim
	ssh rpi28 "sudo mv /tmp/jellyfin-vlc-shim /usr/local/bin/jellyfin-vlc-shim && sudo systemctl restart jellyfin-vlc-shim"
