# jellyfin-vlc-shim

## Install

1. Download the [latest release](https://github.com/tseho/jellyfin-vlc-shim/releases)
2. Authenticate on Jellyfin with `jellyfin-vlc-shim auth`
3. Start with `jellyfin-vlc-shim`

## Features

| Feature                                | Status |
| -------------------------------------- | ------ |
| Play video (MP4 x264)                  | ❔     |
| Play video (MKV x264)                  | ❔     |
| Play video (MP4 x265)                  | ✅     |
| Play video (MKV x265)                  | ✅     |
| Play video (MP4) + embedded `srt`      | ❔     |
| Play video (MP4) + embedded `mov_text` | ❔     |
| Play video (MKV) + embedded `srt`      | ⚠️     |
| Play video (MKV) + embedded `ass`      | ❔     |
| Play video + external `srt`            | 🔴     |
| Seek during play                       | ✅     |
| Change audio track during play         | 🔴     |
| Change subtitle track during play      | 🔴     |

**Legend:**

❔: _untested_  
✅: _supported_  
⚠️: _experimental_  
🔴: _unsupported_

## Options

Options can be set in `~/.config/jellyfin-vlc-shim/configuration.json`.

- `log_level`: `debug|info|warn|error`, default: `info`.
- `fullscreen`: default: `true`
- `jellyfin_device`: Client name show in jellyfin, default to hostname.
- `jellyfin_client`: Client type shown in jellyfin, default: `jellyfin-vlc-shim`.
