// Package xccore is the camera logic of Xiaomi Cam shared by the desktop app
// (cmd/xiaomi-cam) and the Android app (mobile/): the embedded go2rtc, cameras, camera
// control (PTZ, zoom, MIoT properties), capabilities, points, night modes, IP cameras.
// No UI here.
package xccore

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
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/dvrip"
	gohttp "github.com/AlexxIT/go2rtc/internal/http"
	"github.com/AlexxIT/go2rtc/internal/onvif"
	"github.com/AlexxIT/go2rtc/internal/rtsp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/internal/tapo"
	"github.com/AlexxIT/go2rtc/internal/xiaomi"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/miss"
)

const (
	APIAddr  = "127.0.0.1:11984"
	RTSPAddr = "127.0.0.1:18554"
)

const defaultConfig = `api:
  listen: "` + APIAddr + `"
rtsp:
  listen: "` + RTSPAddr + `"
log:
  level: warn
`

// HiddenPrefix - temporary go2rtc streams of the app, not cameras
const HiddenPrefix = "__xc_"

// Start runs the embedded go2rtc on localhost only: API, RTSP server, Xiaomi source and
// the sources of IP cameras (RTSP client, HTTP/MJPEG, ONVIF, Tapo, DVRIP). dir keeps
// go2rtc.yaml (created when missing, 0600: it has the Mi account token).
func Start(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	cfg := filepath.Join(dir, "go2rtc.yaml")
	if _, err := os.Stat(cfg); errors.Is(err, os.ErrNotExist) {
		if err = os.WriteFile(cfg, []byte(defaultConfig), 0o600); err != nil {
			return err
		}
	}
	_ = os.Chmod(cfg, 0o600)

	os.Args = []string{os.Args[0], "-config", cfg}
	app.Init()
	api.Init()
	streams.Init()
	rtsp.Init()
	xiaomi.Init()
	gohttp.Init()
	onvif.Init()
	tapo.Init()
	dvrip.Init()
	LoadIPCameras()
	return nil
}

// --- local go2rtc HTTP API (used for login, the same flow as go2rtc WebUI)

func APICall(method, path string, query url.Values, form url.Values) ([]byte, int, error) {
	u := "http://" + APIAddr + path
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

// --- cameras: streams with xiaomi:// source, and IP cameras (ipcam.go)

func CameraNames() []string {
	var names []string
	for _, n := range streams.GetAllNames() {
		if !strings.HasPrefix(n, HiddenPrefix) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

// CameraSource returns the xiaomi:// source of the camera ("" - not a Xiaomi camera).
func CameraSource(name string) string {
	if s := streams.Get(name); s != nil {
		for _, src := range s.Sources() {
			if strings.HasPrefix(src, "xiaomi://") {
				return src
			}
		}
	}
	return ""
}

func SourceParam(src, key string) string {
	u, err := url.Parse(src)
	if err != nil {
		return ""
	}
	return u.Query().Get(key)
}

// SetSourceParams changes query params of camera source ("" - remove) and saves config.
// Caller must stop video first, so the camera session is closed and redialed with new params.
func SetSourceParams(name string, params map[string]string) error {
	src := CameraSource(name)
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

// SetPlayback switches the camera source to SD card recordings from unix time at
// (0 - back to live video). Not saved to the config.
func SetPlayback(name string, at int64, speed int) error {
	src := CameraSource(name)
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

func AddCamera(name, src string) error {
	if _, err := streams.New(name, src); err != nil {
		return err
	}
	return app.PatchConfig([]string{"streams", name}, []string{src})
}

func RemoveCamera(name string) error {
	streams.Delete(name)
	if IPCam(name) != nil {
		if err := SaveIPCamera(name, nil); err != nil {
			return err
		}
	}
	CapsCache.Delete(name)
	return app.PatchConfig([]string{"streams", name}, nil)
}

func RTSPURL(name string) string {
	return "rtsp://" + RTSPAddr + "/" + url.PathEscape(name)
}

// --- camera control

const (
	MotorLeft  = 1
	MotorRight = 2
	MotorUp    = 3
	MotorDown  = 4
)

// ErrNotXiaomi - camera controls of IP cameras are not supported (yet)
var ErrNotXiaomi = errors.New("this camera has no such control (only Xiaomi cameras)")

func Motor(name string, op int) error {
	if CameraSource(name) == "" {
		return ErrNotXiaomi
	}
	return xiaomi.Motor(name, op)
}

// SDFiles lists the files on the SD card of a Xiaomi camera over its open session (live video
// or playback) and adds them to the recordings cache (miss.RecordingsBetween).
func SDFiles(name string) ([]miss.SDFile, error) {
	if CameraSource(name) == "" {
		return nil, ErrNotXiaomi
	}
	return xiaomi.SDFiles(name, 10*time.Second)
}

// CameraZoom sets the optical/sensor zoom of the camera (1.0-9.0), like the Mi Home plugin:
// MISS command 530 {"value": zoom*10}; older firmware uses MIoT property 6.23 instead.
func CameraZoom(name string, zoom float64) error {
	if CameraSource(name) == "" {
		return ErrNotXiaomi
	}
	value := int(zoom*10 + 0.5)
	if err := xiaomi.Command(name, cmdZoomSet, fmt.Sprintf(`{"value":%d}`, value)); err != nil {
		return SetProp(name, PropZoom, value)
	}
	return nil
}

const cmdZoomSet = 530

// --- camera zoom state (the camera can be zoomed by the UI, the API and camera points)

var zoomState sync.Map // camera name -> float64

func CurrentZoom(name string) float64 {
	if v, ok := zoomState.Load(name); ok {
		return v.(float64)
	}
	return 1
}

// OnZoomState - the zoom of a camera changed (UI, API, a camera point): the UI shows it.
// Called from any goroutine.
var OnZoomState func(name string, z float64)

func SetZoomState(name string, z float64) {
	zoomState.Store(name, z)
	if OnZoomState != nil {
		OnZoomState(name, z)
	}
}

// --- MIoT properties

type MiotProp struct {
	Siid  int `json:"siid"`
	Piid  int `json:"piid"`
	Value any `json:"value"`
	Code  int `json:"code"`
}

// MIoT properties of isa.camera.700sa (https://home.miot-spec.com/spec/isa.camera.700sa)
var (
	PropLight      = MiotProp{Siid: 6, Piid: 16} // hl-filllight-switch
	PropBrightness = MiotProp{Siid: 6, Piid: 11} // hl-filllight-lum 0-100
	PropSiren      = MiotProp{Siid: 6, Piid: 10} // hl-audible-alarm
	PropZoom       = MiotProp{Siid: 6, Piid: 23} // zoom ratio *10 (not in public spec, from Mi Home plugin)
	PropSDStatus   = MiotProp{Siid: 4, Piid: 1}
)

func CameraDid(src string) string {
	return SourceParam(src, "did")
}

func GetProps(name string, props ...MiotProp) ([]MiotProp, error) {
	src := CameraSource(name)
	if src == "" {
		return nil, ErrNotXiaomi
	}
	var params []map[string]any
	for _, p := range props {
		params = append(params, map[string]any{"did": CameraDid(src), "siid": p.Siid, "piid": p.Piid})
	}
	b, _ := json.Marshal(map[string]any{"params": params})
	res, err := xiaomi.CloudRequest(src, "/miotspec/prop/get", string(b))
	if err != nil {
		return nil, err
	}
	var out []MiotProp
	if err = json.Unmarshal(res, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func SetProp(name string, p MiotProp, value any) error {
	src := CameraSource(name)
	if src == "" {
		return ErrNotXiaomi
	}
	b, _ := json.Marshal(map[string]any{"params": []map[string]any{
		{"did": CameraDid(src), "siid": p.Siid, "piid": p.Piid, "value": value},
	}})
	res, err := xiaomi.CloudRequest(src, "/miotspec/prop/set", string(b))
	if err != nil {
		return err
	}
	var out []MiotProp
	if err = json.Unmarshal(res, &out); err == nil && len(out) > 0 && out[0].Code != 0 {
		return fmt.Errorf("camera returned code %d", out[0].Code)
	}
	return nil
}

// Same reports whether p is the property q (for reading GetProps results).
func (p MiotProp) Same(q MiotProp) bool { return p.Siid == q.Siid && p.Piid == q.Piid }
