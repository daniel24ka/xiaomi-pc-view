package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/rtsp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/internal/xiaomi"
)

const (
	apiAddr  = "127.0.0.1:11984"
	rtspAddr = "127.0.0.1:18554"
)

const defaultConfig = `api:
  listen: "` + apiAddr + `"
rtsp:
  listen: "` + rtspAddr + `"
log:
  level: warn
`

// showRunningInstance asks an already running app to show its window.
// Only one instance can run: the camera accepts one session and ports are fixed.
func showRunningInstance() bool {
	client := http.Client{Timeout: time.Second}
	res, err := client.Post("http://"+apiAddr+"/api/xiaomi-cam/show", "", nil)
	if err != nil {
		return false
	}
	_ = res.Body.Close()
	return res.StatusCode == http.StatusOK
}

// startBackend runs embedded go2rtc (API, RTSP server and Xiaomi source) on localhost only.
func startBackend() error {
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	dir = filepath.Join(dir, "xiaomi-cam-viewer")
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	cfg := filepath.Join(dir, "go2rtc.yaml")
	if _, err = os.Stat(cfg); errors.Is(err, os.ErrNotExist) {
		if err = os.WriteFile(cfg, []byte(defaultConfig), 0o600); err != nil {
			return err
		}
	}
	_ = os.Chmod(cfg, 0o600) // contains Mi account token

	os.Args = []string{os.Args[0], "-config", cfg}
	app.Init()
	api.Init()
	streams.Init()
	rtsp.Init()
	xiaomi.Init()
	return nil
}

// onShowRequest is called when another instance of the app was started.
func onShowRequest(f func()) {
	api.HandleFunc("api/xiaomi-cam/show", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			f()
		}
	})
}

// --- local go2rtc HTTP API (used for login, the same flow as go2rtc WebUI)

func apiCall(method, path string, query url.Values, form url.Values) ([]byte, int, error) {
	u := "http://" + apiAddr + path
	if query != nil {
		u += "?" + query.Encode()
	}
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, 0, err
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	client := http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	return b, res.StatusCode, err
}

// --- cameras (streams with xiaomi:// source)

func cameraNames() []string {
	names := streams.GetAllNames()
	sort.Strings(names)
	return names
}

func cameraSource(name string) string {
	if s := streams.Get(name); s != nil {
		for _, src := range s.Sources() {
			if strings.HasPrefix(src, "xiaomi://") {
				return src
			}
		}
	}
	return ""
}

func sourceParam(src, key string) string {
	u, err := url.Parse(src)
	if err != nil {
		return ""
	}
	return u.Query().Get(key)
}

// setSourceParams changes query params of camera source ("" - remove) and saves config.
// Caller must stop video first, so the camera session is closed and redialed with new params.
func setSourceParams(name string, params map[string]string) error {
	src := cameraSource(name)
	if src == "" {
		return fmt.Errorf("no camera %q", name)
	}
	u, err := url.Parse(src)
	if err != nil {
		return err
	}
	q := u.Query()
	for k, v := range params {
		if v == "" {
			q.Del(k)
		} else {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	src = u.String()
	streams.Get(name).SetSource(src)
	// playback params are only for the running session, never saved
	q.Del("playback")
	q.Del("speed")
	u.RawQuery = q.Encode()
	return app.PatchConfig([]string{"streams", name}, []string{u.String()})
}

// setPlayback switches the camera source to SD card recordings from unix time at
// (0 - back to live video). Not saved to the config.
func setPlayback(name string, at int64, speed int) error {
	src := cameraSource(name)
	if src == "" {
		return fmt.Errorf("no camera %q", name)
	}
	u, err := url.Parse(src)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Del("playback")
	q.Del("speed")
	if at > 0 {
		q.Set("playback", strconv.FormatInt(at, 10))
		q.Set("speed", strconv.Itoa(speed))
	}
	u.RawQuery = q.Encode()
	streams.Get(name).SetSource(u.String())
	return nil
}

func addCamera(name, src string) error {
	if _, err := streams.New(name, src); err != nil {
		return err
	}
	return app.PatchConfig([]string{"streams", name}, []string{src})
}

func removeCamera(name string) error {
	streams.Delete(name)
	return app.PatchConfig([]string{"streams", name}, nil)
}

func rtspURL(name string) string {
	return "rtsp://" + rtspAddr + "/" + url.PathEscape(name)
}

// --- camera control

const (
	motorLeft  = 1
	motorRight = 2
	motorUp    = 3
	motorDown  = 4
)

func motor(name string, op int) error {
	return xiaomi.Motor(name, op)
}

// cameraZoom sets the optical/sensor zoom of the camera (1.0-9.0), like the Mi Home plugin:
// MISS command 530 {"value": zoom*10}; older firmware uses MIoT property 6.23 instead.
func cameraZoom(name string, zoom float64) error {
	value := int(zoom*10 + 0.5)
	if err := xiaomi.Command(name, cmdZoomSet, fmt.Sprintf(`{"value":%d}`, value)); err != nil {
		return setProp(name, propZoom, value)
	}
	return nil
}

const cmdZoomSet = 530

type miotProp struct {
	Siid  int `json:"siid"`
	Piid  int `json:"piid"`
	Value any `json:"value"`
	Code  int `json:"code"`
}

// MIoT properties of isa.camera.700sa (https://home.miot-spec.com/spec/isa.camera.700sa)
var (
	propLight      = miotProp{Siid: 6, Piid: 16} // hl-filllight-switch
	propBrightness = miotProp{Siid: 6, Piid: 11} // hl-filllight-lum 0-100
	propSiren      = miotProp{Siid: 6, Piid: 10} // hl-audible-alarm
	propZoom       = miotProp{Siid: 6, Piid: 23} // zoom ratio *10 (not in public spec, from Mi Home plugin)
)

func cameraDid(src string) string {
	return sourceParam(src, "did")
}

func getProps(name string, props ...miotProp) ([]miotProp, error) {
	src := cameraSource(name)
	var params []map[string]any
	for _, p := range props {
		params = append(params, map[string]any{"did": cameraDid(src), "siid": p.Siid, "piid": p.Piid})
	}
	b, _ := json.Marshal(map[string]any{"params": params})
	res, err := xiaomi.CloudRequest(src, "/miotspec/prop/get", string(b))
	if err != nil {
		return nil, err
	}
	var out []miotProp
	if err = json.Unmarshal(res, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func setProp(name string, p miotProp, value any) error {
	src := cameraSource(name)
	b, _ := json.Marshal(map[string]any{"params": []map[string]any{
		{"did": cameraDid(src), "siid": p.Siid, "piid": p.Piid, "value": value},
	}})
	res, err := xiaomi.CloudRequest(src, "/miotspec/prop/set", string(b))
	if err != nil {
		return err
	}
	var out []miotProp
	if err = json.Unmarshal(res, &out); err == nil && len(out) > 0 && out[0].Code != 0 {
		return fmt.Errorf("camera returned code %d", out[0].Code)
	}
	return nil
}
