package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/streams"
)

// Local HTTP API for scripts and AI (the MCP server uses it). Listens only on
// 127.0.0.1 (go2rtc API server). All endpoints take optional ?camera=<name>,
// default is the first camera.
//
//	GET  /api/xiaomi-cam/cameras
//	GET  /api/xiaomi-cam/snapshot?width=1280          image/jpeg
//	GET  /api/xiaomi-cam/state                       spotlight, brightness, siren, zoom
//	POST /api/xiaomi-cam/move?direction=left&steps=5 left/right/up/down, 1 step = 0.1 s of movement
//	POST /api/xiaomi-cam/zoom?value=2.5              1.0-9.0 (camera zoom)
//	POST /api/xiaomi-cam/spotlight?on=true&brightness=80
//	POST /api/xiaomi-cam/siren?on=true
//	POST /api/xiaomi-cam/snapshot/save               saves JPEG to the snapshots folder
//	POST /api/xiaomi-cam/record/start?duration=60&mode=each&cameras=a,b  MP4 (H.265); mode: selected/each/grid
//	POST /api/xiaomi-cam/record/stop                 stops all recordings
//	GET  /api/xiaomi-cam/record                      recording status
//	GET  /api/xiaomi-cam/points                      camera points 1-3
//	POST /api/xiaomi-cam/points/goto?point=1         turn to a point (number or name)
//	POST /api/xiaomi-cam/points/save?point=1&name=auto  save the current position
func initHTTPAPI(zoomChanged func(name string, zoom float64)) {
	handle := func(path, method string, f func(r *http.Request, name string) (any, error)) {
		api.HandleFunc("api/xiaomi-cam/"+path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != method {
				http.Error(w, method+" only", http.StatusMethodNotAllowed)
				return
			}
			name := r.URL.Query().Get("camera")
			if name == "" {
				if names := cameraNames(); len(names) > 0 {
					name = names[0]
				}
			}
			if cameraSource(name) == "" {
				http.Error(w, fmt.Sprintf("no camera %q", name), http.StatusNotFound)
				return
			}
			res, err := f(r, name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if b, ok := res.([]byte); ok {
				w.Header().Set("Content-Type", "image/jpeg")
				_, _ = w.Write(b)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(res)
		})
	}

	api.HandleFunc("api/xiaomi-cam/cameras", func(w http.ResponseWriter, r *http.Request) {
		type camera struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		}
		cams := []camera{}
		for _, n := range cameraNames() {
			cams = append(cams, camera{n, sourceParam(cameraSource(n), "model")})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cams)
	})

	handle("snapshot", "GET", func(r *http.Request, name string) (any, error) {
		width, _ := strconv.Atoi(r.URL.Query().Get("width"))
		return snapshotJPEG(name, width)
	})

	handle("state", "GET", func(r *http.Request, name string) (any, error) {
		props, err := getProps(name, propLight, propBrightness, propSiren)
		if err != nil {
			return nil, err
		}
		state := map[string]any{"camera": name, "zoom": currentZoom(name)}
		for _, p := range props {
			switch p.Piid {
			case propLight.Piid:
				state["spotlight"] = p.Value
			case propBrightness.Piid:
				state["spotlight_brightness"] = p.Value
			case propSiren.Piid:
				state["siren"] = p.Value
			}
		}
		return state, nil
	})

	handle("move", "POST", func(r *http.Request, name string) (any, error) {
		ops := map[string]int{"left": motorLeft, "right": motorRight, "up": motorUp, "down": motorDown}
		dir := r.URL.Query().Get("direction")
		op, ok := ops[dir]
		if !ok {
			return nil, errors.New("direction must be left, right, up or down")
		}
		steps, _ := strconv.Atoi(r.URL.Query().Get("steps"))
		steps = min(max(steps, 1), 100)
		if err := ensureSession(name); err != nil {
			return nil, err
		}
		// the camera moves continuously until stopped: one step = 100 ms of movement
		var moveErr error
		m := newMover(func() string { return name }, func(err error) { moveErr = err })
		m.pulse(op, time.Duration(steps)*100*time.Millisecond)
		time.Sleep(300 * time.Millisecond) // let the stop command go out
		if moveErr != nil {
			return nil, moveErr
		}
		return map[string]any{"moved": dir, "steps": steps}, nil
	})

	handle("zoom", "POST", func(r *http.Request, name string) (any, error) {
		z, err := strconv.ParseFloat(r.URL.Query().Get("value"), 64)
		if err != nil || z < 1 || z > maxZoom {
			return nil, fmt.Errorf("value must be 1.0-%.1f", maxZoom)
		}
		if err = ensureSession(name); err != nil {
			return nil, err
		}
		if err = cameraZoom(name, z); err != nil {
			return nil, err
		}
		setZoomState(name, z)
		if zoomChanged != nil {
			zoomChanged(name, z)
		}
		return map[string]any{"zoom": z}, nil
	})

	handle("spotlight", "POST", func(r *http.Request, name string) (any, error) {
		q := r.URL.Query()
		if b := q.Get("brightness"); b != "" {
			v, err := strconv.Atoi(b)
			if err != nil || v < 0 || v > 100 {
				return nil, errors.New("brightness must be 0-100")
			}
			if err = setProp(name, propBrightness, v); err != nil {
				return nil, err
			}
		}
		if on := q.Get("on"); on != "" {
			if err := setProp(name, propLight, on == "true" || on == "1"); err != nil {
				return nil, err
			}
		}
		return map[string]any{"ok": true}, nil
	})

	handle("snapshot/save", "POST", func(r *http.Request, name string) (any, error) {
		path, err := saveSnapshot(name)
		if err != nil {
			return nil, err
		}
		return map[string]any{"file": path}, nil
	})

	handle("record/start", "POST", func(r *http.Request, name string) (any, error) {
		secs, _ := strconv.Atoi(r.URL.Query().Get("duration"))
		mode := recordMode()
		switch m := r.URL.Query().Get("mode"); m {
		case "selected", "camera":
			mode = recordSelected
		case recordEach, recordGrid:
			mode = m
		}
		var cams []string // nil - cameras from the settings
		if c := r.URL.Query().Get("cameras"); c != "" {
			for _, n := range strings.Split(c, ",") {
				if n = strings.TrimSpace(n); cameraSource(n) != "" {
					cams = append(cams, n)
				} else if n != "" {
					return nil, fmt.Errorf("no camera %q", n)
				}
			}
		}
		recs, err := startRecordings(mode, name, cams, time.Duration(secs)*time.Second)
		if len(recs) == 0 && err != nil {
			return nil, err
		}
		var files []string
		for _, rec := range recs {
			files = append(files, rec.path)
		}
		res := map[string]any{"recording": true, "files": files}
		if secs > 0 {
			res["duration_s"] = secs
		}
		if err != nil {
			res["errors"] = err.Error()
		}
		return res, nil
	})

	handle("record/stop", "POST", func(r *http.Request, name string) (any, error) {
		stopped := stopAll()
		if len(stopped) == 0 {
			return nil, errors.New("not recording")
		}
		var files []map[string]any
		for _, rec := range stopped {
			files = append(files, map[string]any{"file": rec.path, "duration_s": int(time.Since(rec.started).Seconds())})
		}
		return map[string]any{"recording": false, "files": files}, nil
	})

	handle("record", "GET", func(r *http.Request, name string) (any, error) {
		active := activeRecordings()
		if len(active) == 0 {
			return map[string]any{"recording": false}, nil
		}
		var files []map[string]any
		for _, rec := range active {
			files = append(files, map[string]any{"file": rec.path, "elapsed_s": int(time.Since(rec.started).Seconds())})
		}
		return map[string]any{"recording": true, "files": files}, nil
	})

	handle("points", "GET", func(r *http.Request, name string) (any, error) {
		list, err := getPresets(name)
		if err != nil {
			return nil, err
		}
		points := []map[string]any{}
		for i, p := range presetSlotsOf(list) {
			item := map[string]any{"point": i + 1, "defined": p != nil}
			if p != nil {
				item["name"] = p.Label()
			}
			points = append(points, item)
		}
		return points, nil
	})

	handle("points/goto", "POST", func(r *http.Request, name string) (any, error) {
		p, err := findPoint(name, r.URL.Query().Get("point"))
		if err != nil {
			return nil, err
		}
		if err = gotoPresetAndWait(name, p.Idx); err != nil {
			return nil, err
		}
		return map[string]any{"point": p.Idx, "name": p.Label()}, nil
	})

	handle("points/save", "POST", func(r *http.Request, name string) (any, error) {
		slot, _ := strconv.Atoi(r.URL.Query().Get("point"))
		label := r.URL.Query().Get("name")
		if err := savePresetSlot(name, slot, label); err != nil {
			return nil, err
		}
		if pointsChanged != nil {
			pointsChanged()
		}
		return map[string]any{"point": slot, "name": label}, nil
	})

	handle("siren", "POST", func(r *http.Request, name string) (any, error) {
		on := r.URL.Query().Get("on")
		if err := setProp(name, propSiren, on == "true" || on == "1"); err != nil {
			return nil, err
		}
		return map[string]any{"siren": on == "true" || on == "1"}, nil
	})
}

// pointsChanged - points saved through the API (UI refresh)
var pointsChanged func()

// --- camera zoom state (the camera can be zoomed by the UI and the API)

var zoomState sync.Map // camera name -> float64

func currentZoom(name string) float64 {
	if v, ok := zoomState.Load(name); ok {
		return v.(float64)
	}
	return 1
}

func setZoomState(name string, z float64) { zoomState.Store(name, z) }

// --- snapshot

// snapshotJPEG grabs one frame of live video. It shares the camera session with the
// app window (go2rtc has one producer per stream), so it works while the app shows video.
func snapshotJPEG(name string, width int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	vf := "null"
	if width > 0 {
		vf = fmt.Sprintf("scale=%d:-2", width)
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-nostdin",
		"-rtsp_transport", "tcp", "-i", rtspURL(name),
		"-map", "0:v:0", "-frames:v", "1", "-vf", vf, "-q:v", "3", "-f", "image2", "-c:v", "mjpeg", "pipe:1")
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("snapshot: %v %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	return out.Bytes(), nil
}

// --- session for commands

// Pan/tilt and zoom are sent over the camera session. When the app window doesn't show
// video (hidden to tray), a background consumer keeps the session open for a minute
// after the last command.

type keeper struct {
	cancel context.CancelFunc
	timer  *time.Timer
}

var (
	keepMu  sync.Mutex
	keepers = map[string]*keeper{}
)

func hasSession(name string) bool {
	s := streams.Get(name)
	return s != nil && len(s.Conns()) > 0
}

func ensureSession(name string) error {
	keepMu.Lock()
	k := keepers[name]
	if k != nil {
		k.timer.Reset(time.Minute)
	}
	keepMu.Unlock()

	if hasSession(name) {
		return nil
	}
	if k == nil {
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "quiet", "-nostdin",
			"-rtsp_transport", "tcp", "-i", rtspURL(name), "-map", "0:v:0", "-c", "copy", "-f", "null", "-")
		if err := cmd.Start(); err != nil {
			cancel()
			return err
		}
		go func() { _ = cmd.Wait() }()
		k = &keeper{cancel: cancel}
		k.timer = time.AfterFunc(time.Minute, func() {
			keepMu.Lock()
			delete(keepers, name)
			keepMu.Unlock()
			cancel()
		})
		keepMu.Lock()
		keepers[name] = k
		keepMu.Unlock()
	}
	for i := 0; i < 60; i++ {
		if hasSession(name) {
			time.Sleep(500 * time.Millisecond) // let the session start media
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return errors.New("can't connect to the camera")
}
