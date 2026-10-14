# Xiaomi Cam – Xiaomi Mi Home camera viewer for Linux

A desktop app for Ubuntu/Linux to watch and control **Xiaomi Mi Home** cameras, which
officially work only in the Mi Home phone app. Tested with the **Xiaomi Outdoor Camera
CW700S** (`isa.camera.700sa`).

One binary file with an embedded [go2rtc](https://github.com/AlexxIT/go2rtc). It also has an
**MCP server** for AI assistants and a **local HTTP API** for scripts.

## Features

- Live video up to **2.5K** (2560×1440), quality 2.5K / 720p / SD.
- **Remote connection from any network**, like Mi Home, without port forwarding or VPN.
  There is also a local (LAN) mode.
- **Pan/tilt**: buttons, arrow keys, or dragging the picture with the mouse.
- **Native camera zoom up to 9×**: the camera zooms itself, so details stay sharp and the
  date/time printed by the camera keeps its size. Use the mouse wheel, `+`/`−`, or the
  slider in video-only mode.
- **SD card history**: date, timeline of the day, ±1/±10 min, speed 1–16×, and back to live.
- **Spotlight** with brightness, and **siren**.
- **Local recording to MP4**: H.265 video copied as it comes from the camera (no
  re-encoding, almost no CPU), AAC audio. Fragmented MP4, so a file stays playable even if
  the app is closed while recording. Recording works with the window hidden.
- Full resolution (2560×1440) snapshots.
- **Settings**: folders for recordings (default: your Videos folder) and snapshots (default:
  your Pictures folder).
- Camera audio.
- **Video-only mode**: no panels and no window frame. The window keeps the picture's aspect
  ratio and can be resized by any edge or moved with the ✥ button. Pan/tilt buttons and a
  zoom slider with a scale appear when you move the mouse.
- Full screen mode.
- **System tray**: closing or minimizing the window hides the app and stops streaming.
- Window size and position are remembered, separately with and without the frame.
- Only one instance runs; starting the app again shows the window that is already open.
- Polish / English UI.
- Mi Home login with captcha and 2FA code support.

## Requirements

- Linux x86-64 (tested on Ubuntu 26.04, GNOME, X11/XWayland).
- `ffmpeg` for H.265 decoding: `sudo apt install ffmpeg`.
- On GNOME the tray icon needs the AppIndicator extension (enabled by default in Ubuntu).

## Install

Download `xiaomi-cam-<version>-linux-amd64.tar.gz` from [Releases](../../releases):

```sh
tar xzf xiaomi-cam-*-linux-amd64.tar.gz
install -Dm755 xiaomi-cam/xiaomi-cam ~/.local/bin/xiaomi-cam
install -Dm644 xiaomi-cam/xiaomi-cam.desktop ~/.local/share/applications/xiaomi-cam.desktop
```

Or build from source (Go ≥ 1.25, a C compiler, X11/GL headers):

```sh
sudo apt install golang gcc libgl1-mesa-dev xorg-dev libxkbcommon-dev libwayland-dev
make install        # builds and installs to ~/.local
```

## First run

1. Start **Xiaomi Cam** from the applications menu.
2. Log in with your Mi Home account. The password goes only to the embedded go2rtc and to
   Xiaomi servers. Only a token is stored, in `~/.config/xiaomi-cam-viewer/go2rtc.yaml`
   (mode 600).
3. Choose the region of your account (e.g. **Europe**) and the cameras.

## Keyboard and mouse

| Key / mouse | Action |
|---|---|
| arrows, pan/tilt buttons (hold) | pan/tilt the camera |
| drag the picture | pan/tilt (not in video-only mode) |
| `+` / `−`, mouse wheel | camera zoom |
| `0` | zoom 1× |
| `H` | video-only mode; `Esc` or right click to go back |
| `F`, `F11`, double click | full screen |
| `S` | snapshot |
| red dot button | start / stop recording |

## AI control (MCP)

`xiaomi-cam mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) server over
stdio for AI assistants: Claude Code, Claude Desktop and others. If the app isn't running, it
starts it hidden in the tray.

**Claude Code:**

```sh
claude mcp add --scope user xiaomi-cam -- ~/.local/bin/xiaomi-cam mcp
```

**Claude Desktop** (`~/.config/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "xiaomi-cam": { "command": "/home/USER/.local/bin/xiaomi-cam", "args": ["mcp"] }
  }
}
```

**Tools.** All tools take an optional `camera` argument (a name from `list_cameras`); the
default is the first camera.

| Tool | Arguments | Result |
|---|---|---|
| `list_cameras` | – | camera names and models |
| `get_snapshot` | `width` (default 1280, max 2560) | live JPEG image (returned to the model as an image) |
| `get_camera_state` | – | zoom, spotlight on/off and brightness, siren |
| `move_camera` | `direction` (`left`/`right`/`up`/`down`), `steps` (default 10, max 100; 1 step ≈ 1–2°) | pan/tilt |
| `set_zoom` | `zoom` (1.0–9.0) | camera zoom |
| `set_spotlight` | `on` (bool), `brightness` (0–100) | spotlight |
| `set_siren` | `on` (bool) | alarm siren |
| `save_snapshot` | – | saves a 2560×1440 JPEG to the snapshots folder, returns the path |
| `start_recording` | `duration_seconds` (optional, auto stop) | starts an MP4 recording, returns the path |
| `stop_recording` | – | finishes the MP4 file, returns the path and duration |
| `recording_status` | – | recording or not, file path, elapsed seconds |

Example prompts: *"Show me what the camera sees"*, *"Turn the camera left and zoom in on the
gate"*, *"Is there a car parked in front of the house?"*.

## HTTP API

The running app serves a REST API on **`127.0.0.1:11984`**. It is local only and has no
authentication. All endpoints take an optional `camera=<name>`; the default is the first
camera. Errors return a non-200 status with a text message.

| Method | Path | Parameters | Response |
|---|---|---|---|
| GET | `/api/xiaomi-cam/cameras` | – | `[{"name": "...", "model": "..."}]` |
| GET | `/api/xiaomi-cam/snapshot` | `width` (optional) | `image/jpeg` of live video |
| GET | `/api/xiaomi-cam/state` | – | `{"zoom", "spotlight", "spotlight_brightness", "siren"}` |
| POST | `/api/xiaomi-cam/move` | `direction` = `left`/`right`/`up`/`down`, `steps` (1–100) | `{"moved", "steps"}` |
| POST | `/api/xiaomi-cam/zoom` | `value` (1.0–9.0) | `{"zoom"}` |
| POST | `/api/xiaomi-cam/spotlight` | `on` (`true`/`false`), `brightness` (0–100) | `{"ok": true}` |
| POST | `/api/xiaomi-cam/siren` | `on` (`true`/`false`) | `{"siren"}` |
| POST | `/api/xiaomi-cam/snapshot/save` | – | `{"file"}` – JPEG saved to the snapshots folder |
| POST | `/api/xiaomi-cam/record/start` | `duration` (seconds, optional) | `{"recording", "file"}` |
| POST | `/api/xiaomi-cam/record/stop` | – | `{"recording", "file", "duration_s"}` |
| GET | `/api/xiaomi-cam/record` | – | `{"recording", "file", "elapsed_s"}` |

```sh
# snapshot
curl -o camera.jpg "http://127.0.0.1:11984/api/xiaomi-cam/snapshot?width=1920"
# pan left by 20 steps, zoom 3x
curl -X POST "http://127.0.0.1:11984/api/xiaomi-cam/move?direction=left&steps=20"
curl -X POST "http://127.0.0.1:11984/api/xiaomi-cam/zoom?value=3"
# record 60 seconds to the recordings folder
curl -X POST "http://127.0.0.1:11984/api/xiaomi-cam/record/start?duration=60"
# spotlight on at 80 %
curl -X POST "http://127.0.0.1:11984/api/xiaomi-cam/spotlight?on=true&brightness=80"
```

Pan/tilt and zoom use the camera session. When the window is hidden in the tray, the API
opens a session itself and closes it a minute after the last command. If the app is showing
history, the snapshot comes from the recording being played.

The embedded go2rtc also serves the camera stream as RTSP:
`rtsp://127.0.0.1:18554/<camera name>`.

## How it works

Xiaomi cameras don't offer RTSP. The video comes over Xiaomi's **MISS** protocol on **CS2**
P2P, the same way as in Mi Home:

1. The Xiaomi cloud returns the encryption keys and the camera's P2P id.
2. The app asks the Xiaomi CS2 servers to connect it with the camera. The servers send each
   side the address of the other, and both punch through NAT (UDP hole punching).
3. The video goes straight from the camera to the computer, encrypted. go2rtc serves it as
   RTSP on `127.0.0.1`, and ffmpeg decodes it.

This "server" mode was added to go2rtc in this repository, because some cameras (e.g. the
CW700S) don't answer the LAN search that upstream go2rtc uses.

Other controls:

| Function | How it is done |
|---|---|
| Pan/tilt | MISS command `0x112` |
| Zoom | MISS command `530` `{"value": zoom×10}`, found in the camera's Mi Home plugin |
| Spotlight, siren | Xiaomi cloud (MIoT properties) |
| History | MISS `PLAYBACK_REQ` `0x10D` |

For history, the camera needs the exact start of a recorded file (files are about 61 s). The
app searches for it second by second and plays from the start of the file that contains the
chosen time.

## Limitations

- CS2 server addresses are set for the **Europe** region. For other regions, add
  `&servers=ip1,ip2` to the camera source in the config.
- Cameras with the `cs2` P2P protocol are supported (most newer Xiaomi cameras). Spotlight,
  siren and zoom follow the `isa.camera.700sa` specification.
- History has no list of recordings (timeline marks) yet.

## License and credits

MIT. Based on [go2rtc](https://github.com/AlexxIT/go2rtc) by Alexey Khit (MIT); the original
README is in [README.go2rtc.md](README.go2rtc.md). The pan/tilt motor command is based on
AlexxIT/go2rtc#2531, and the command channel fix on AlexxIT/go2rtc#2489.
