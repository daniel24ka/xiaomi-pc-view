# Xiaomi Cam (xiaomi-pc-view) – notes for Claude

Desktop viewer/controller for Xiaomi Mi Home cameras on Linux (user's camera: Xiaomi Outdoor
Camera CW700S, model `isa.camera.700sa`). Repo = fork of go2rtc v1.9.14 + app in
`cmd/xiaomi-cam`. GitHub: `git@github.com:genosajd/xiaomi-pc-view.git` (branch `main`, tags
`v1.0.0`…`v1.17.2`). User talks Polish; README/release notes in English.

## Build, install, release

```sh
make install VERSION=vX.Y.Z   # ~/.local/bin/xiaomi-cam + menu entry + hicolor icon
make deb     VERSION=vX.Y.Z   # dist/xiaomi-cam_X.Y.Z_amd64.deb
make release VERSION=vX.Y.Z   # dist/: .deb, plain binary, .tar.gz
go vet ./cmd/xiaomi-cam && go vet -tags devtest ./cmd/xiaomi-cam && go test ./cmd/xiaomi-cam/
```

Workflow per change: commit (message ends with `Co-Authored-By: Claude …` trailer), tag
`vX.Y.Z`, `make install` + `make release`, `git push origin main --tags`, relaunch the app:
`(setsid nohup ~/.local/bin/xiaomi-cam >/dev/null 2>&1 &)`.

GitHub releases: v1.0.0–v1.17.2 published (gh logged in as genosajd). For a new version:
`gh release create vX.Y.Z --title "Xiaomi Cam vX.Y.Z" --notes-file <notes> --latest dist/<3 files>`;
notes must list the changes since the previous tag (English) and the 3 downloads.

## Self-tests with the real camera

Build with `-tags devtest`; `cmd/xiaomi-cam/devtest_*.go` hooks run when env `XC_*` is set
(e.g. `XC_TEST`, `XC_PTZTEST`, `XC_GRIDTEST`, `XC_AITEST`, `XC_SEAMLESSTEST`, `XC_BUSYTEST`,
`XC_NIGHTTEST`, `XC_WALLTEST` + `XC_WALLPOS`/`XC_WALLSECS`/`XC_LOWERMS`). XTest mouse input doesn't reach the window (GNOME Wayland,
XWayland), so click behaviour is tested with the Fyne test driver (`tooltip_test.go`). Real screenshots
(whole desktop, no prompt): xdg-desktop-portal Screenshot with interactive=false via python gi. Real mouse clicks on
the desktop: a virtual absolute pointer via /dev/uinput (python evdev, the user has an ACL) –
only on a monitor area without the user's windows. The app is
single-instance (ports 127.0.0.1:11984 API, :18554 RTSP), so stop the user's running copy
first with an **anchored** pattern (unanchored pkill kills your own shell):
`pkill -f "^/home/genosajd/(.local/bin|xiaomi-pc-view)/xiaomi-cam$"`. Back up and restore
`~/.config/fyne/pl.lawinski.xiaomi-cam/preferences.json` around tests (window geometry,
language, layout). Relaunch the user's app afterwards. Clean up test files in ~/Videos,
~/Pictures and test camera points.

## Architecture (cmd/xiaomi-cam)

- `main.go` UI (Fyne 2.8), `backend.go` embedded go2rtc (api, streams, rtsp, xiaomi), config
  `~/.config/xiaomi-cam-viewer/go2rtc.yaml` (Mi token, 0600)
- `video.go` ffmpeg → RGBA frames, player with seamless `switchTo` (no camera reconnect)
- `control.go` mover (PTZ), custom widgets with pointer cursor + tooltips (`tooltip.go`),
  GLFW access via reflection (frame, minimize, cursor leave), `wm_x11.go` (_NET_WM_MOVERESIZE),
  desktop window type + empty XShape input for `wallpaper.go`; no _NET_WM_STATE_BELOW, it
  is mutter's BOTTOM layer above the desktop icons (DING); `keepBelow` re-lowers the
  window under DING every 10 ms from its own X connection, because DING lowers itself on
  every window map and every click on the desktop, and the icons blinked), `busy.go` spinners on buttons /
  status / video, `nightmode.go`. Tooltips are a canvas overlay that forwards clicks to the
  owner (a widget.PopUp swallowed the first click); never use PopUp for passive overlays
- `grid.go` multi-camera grid, `caps.go` hide unsupported features, `pointsui.go`/`presets.go`
  points 1–3, `history.go` SD playback, `recorder.go` MP4 (selected/each/grid),
  `ai.go`/`aiui.go` AI tasks (Claude SDK, OpenAI, own LM Studio/Ollama server, cron),
  `notify.go`/`notifyui.go` e-mail, Slack, ntfy, Telegram, `httpapi.go` + `mcp.go`
  (MCP server `xiaomi-cam mcp`, registered in the user's Claude Code), `autostart.go`,
  `settings.go`, `i18n.go` (Polish source strings, `T()` → English)
- go2rtc changes: `pkg/xiaomi/miss/cs2/server.go` (CS2 via Xiaomi servers, `p2p=server`),
  session close 0xF0, `miss` Command/Request/StartPlayback, motor, zoom

## Camera protocol facts (isa.camera.700sa)

- Remote video: CS2 servers 34.34.47.37 / 34.90.51.72 / 34.147.123.47:32100 (EU), the camera
  never answers LAN search. One camera session at a time.
- MISS: motor 0x112 `{"operation":1-4}` starts continuous move (repeat every 200 ms while
  held), stop `{"operation":-1001}`; zoom 530 `{"value":zoom*10}` (1–9×); playback 0x10D needs
  the exact ~61 s file start (search second by second, offset 0).
- MIoT: spotlight 6.16, brightness 6.11, siren 6.10, night-shot 2.3 (0 off, 1 IR on, 2 auto,
  3 auto full color, 4 full color on, 5 auto smart full color; user's default 5), motor position 6.12, points 9.1 (list,
  base64 names) / 9.2 (go to idx), SD status 4.1; missing property → code -704040003.
- Mi Home RN plugin for the camera: cloud `/v2/plugin/fetch_plugin` (api_version ≥ 10100),
  dev tool `cmd/xmtool` calls any cloud API with the app's token.

## Open ideas / not done

- Real tests of Telegram, e-mail SMTP, Slack, LM Studio/Ollama,
  Claude provider (no keys); a second real camera for the grid.
- History has no recordings list (RDT channel not implemented).
- CS2 server list hardcoded for EU (`&servers=` param for other regions).
