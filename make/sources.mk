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
