package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/onvif"
)

// IP cameras (not Xiaomi): any go2rtc source - rtsp://, http(s):// (MJPEG), onvif://,
// tapo://, dvrip://. The go2rtc stream has the source of the selected quality (main or
// sub stream); the app keeps both, and the picture size, in the "xiaomi_cam" section of
// go2rtc.yaml.

type ipCamera struct {
	Kind    string `yaml:"kind" json:"kind"` // ipKinds[].id
	Main    string `yaml:"main" json:"main"`
	Sub     string `yaml:"sub,omitempty" json:"sub,omitempty"`
	Quality int    `yaml:"quality,omitempty" json:"quality,omitempty"` // 0 main, 1 sub
	W       int    `yaml:"w,omitempty" json:"w,omitempty"`
	H       int    `yaml:"h,omitempty" json:"h,omitempty"`
	SubW    int    `yaml:"sub_w,omitempty" json:"sub_w,omitempty"`
	SubH    int    `yaml:"sub_h,omitempty" json:"sub_h,omitempty"`
	// video codecs (ffprobe names: h264, hevc, mjpeg)
	Codec    string `yaml:"codec,omitempty" json:"codec,omitempty"`
	SubCodec string `yaml:"sub_codec,omitempty" json:"sub_codec,omitempty"`
}

// active returns the source of the selected quality.
func (c *ipCamera) active() string {
	if c.Quality == 1 && c.Sub != "" {
		return c.Sub
	}
	return c.Main
}

var ipCams = struct {
	sync.Mutex
	m map[string]*ipCamera
}{m: map[string]*ipCamera{}}

// loadIPCameras reads the IP cameras from the config (after app.Init).
func loadIPCameras() {
	var cfg struct {
		X struct {
			Cameras map[string]*ipCamera `yaml:"cameras"`
		} `yaml:"xiaomi_cam"`
	}
	app.LoadConfig(&cfg)
	ipCams.Lock()
	defer ipCams.Unlock()
	for name, c := range cfg.X.Cameras {
		if c != nil && c.Main != "" {
			ipCams.m[name] = c
		}
	}
}

// ipCam returns a copy of the IP camera settings (nil: no such IP camera).
func ipCam(name string) *ipCamera {
	ipCams.Lock()
	defer ipCams.Unlock()
	if c := ipCams.m[name]; c != nil {
		cc := *c
		return &cc
	}
	return nil
}

func saveIPCamera(name string, c *ipCamera) error {
	ipCams.Lock()
	defer ipCams.Unlock()
	if c == nil {
		delete(ipCams.m, name)
	} else {
		cc := *c
		ipCams.m[name] = &cc
	}
	// all of them: the config patch can't add a key under a missing parent
	all := map[string]any{}
	for n, c := range ipCams.m {
		all[n] = c
	}
	return app.PatchConfig([]string{"xiaomi_cam", "cameras"}, all)
}

// addIPCamera adds the camera (go2rtc stream with the selected source) and saves it.
func addIPCamera(name string, c *ipCamera) error {
	if err := addCamera(name, c.active()); err != nil {
		return err
	}
	return saveIPCamera(name, c)
}

// ipFormOf returns the form fields of a saved IP camera (for editing).
func ipFormOf(c *ipCamera) *ipForm {
	f := &ipForm{Kind: c.Kind}
	u, err := url.Parse(c.Main)
	if err != nil {
		return f
	}
	f.Host = u.Hostname()
	f.Port, _ = strconv.Atoi(u.Port())
	if u.User != nil {
		f.User = u.User.Username()
		f.Pass, _ = u.User.Password()
	}
	pathOf := func(u *url.URL) string {
		p := u.EscapedPath()
		if u.RawQuery != "" {
			p += "?" + u.RawQuery
		}
		return p
	}
	switch c.Kind {
	case "rtsp", "http":
		// the whole address (keeps https://, the port and the path), login and password apart
		full := *u
		full.User = nil
		f.Host, f.Port = full.String(), 0
		if s, err := url.Parse(c.Sub); err == nil && c.Sub != "" {
			f.SubPath = pathOf(s)
		}
	case "tapo":
		if _, ok := u.User.Password(); !ok && u.User != nil {
			f.User, f.Pass = "", u.User.Username() // tapo://password@host
		}
	case "dvrip":
		f.Channel, _ = strconv.Atoi(u.Query().Get("channel"))
	}
	return f
}

// updateIPCamera saves the edited camera (old name: the camera before the edit). The caller
// stops its video first.
func updateIPCamera(old, name string, c *ipCamera) error {
	if prev := ipCam(old); prev != nil && c.Sub != "" {
		c.Quality = prev.Quality // keep main / sub
	}
	if old != name {
		if err := addIPCamera(name, c); err != nil {
			return err
		}
		return removeCamera(old)
	}
	src := c.active()
	streams.Get(name).SetSource(src)
	if err := app.PatchConfig([]string{"streams", name}, []string{src}); err != nil {
		return err
	}
	return saveIPCamera(name, c)
}

// setIPQuality switches the camera to its main (0) or sub (1) stream. The caller stops
// the video first.
func setIPQuality(name string, q int) error {
	c := ipCam(name)
	if c == nil {
		return fmt.Errorf("no camera %q", name)
	}
	c.Quality = q
	src := c.active()
	streams.Get(name).SetSource(src)
	if err := app.PatchConfig([]string{"streams", name}, []string{src}); err != nil {
		return err
	}
	return saveIPCamera(name, c)
}

func cameraExists(name string) bool { return name != "" && streams.Get(name) != nil }

// streamSource returns the (first) source of the camera stream.
func streamSource(name string) string {
	if s := streams.Get(name); s != nil {
		if srcs := s.Sources(); len(srcs) > 0 {
			return srcs[0]
		}
	}
	return ""
}

// cameraInfo - a short description for lists: Xiaomi model or IP camera kind and host.
func cameraInfo(name string) string {
	if c := ipCam(name); c != nil {
		host := ""
		if u, err := url.Parse(c.Main); err == nil {
			host = u.Hostname()
		}
		return strings.TrimSpace(kindName(c.Kind) + " " + host)
	}
	return sourceParam(cameraSource(name), "model")
}

// --- kinds and the source URL

type ipKind struct {
	id, name string
	port     int // default port ("" in the URL)
}

var ipKinds = []ipKind{
	{"onvif", "ONVIF", 80},
	{"rtsp", "RTSP", 554},
	{"http", "HTTP (MJPEG, HLS)", 80},
	{"tapo", "TP-Link Tapo", 0},
	{"dvrip", "DVRIP (XMeye)", 34569},
}

func kindName(id string) string {
	for _, k := range ipKinds {
		if k.id == id {
			return k.name
		}
	}
	return id
}

// rtspBrand - RTSP paths of the main and sub stream of popular cameras
type rtspBrand struct{ name, main, sub string }

var rtspBrands = []rtspBrand{
	{"Hikvision / Annke / Ezviz", "/Streaming/Channels/101", "/Streaming/Channels/102"},
	{"Dahua / Amcrest / Imou", "/cam/realmonitor?channel=1&subtype=0", "/cam/realmonitor?channel=1&subtype=1"},
	{"Reolink", "/h264Preview_01_main", "/h264Preview_01_sub"},
	{"TP-Link Tapo / VIGI", "/stream1", "/stream2"},
	{"Uniview", "/media/video1", "/media/video2"},
	{"Axis", "/axis-media/media.amp", "/axis-media/media.amp?resolution=640x360"},
	{"Foscam", "/videoMain", "/videoSub"},
}

// ipForm - what the user typed in the "IP camera" form
type ipForm struct {
	Kind, Host, User, Pass string
	Port                   int
	MainPath, SubPath      string // RTSP, HTTP
	Channel                int    // DVRIP
}

// hostPort returns host[:port] (no port when it is the default one).
func (f *ipForm) hostPort(def int) string {
	h := strings.TrimSpace(f.Host)
	if f.Port > 0 && f.Port != def {
		return h + ":" + strconv.Itoa(f.Port)
	}
	return h
}

func (f *ipForm) userinfo() *url.Userinfo {
	switch {
	case f.User == "" && f.Pass == "":
		return nil
	case f.Pass == "":
		return url.User(f.User)
	}
	return url.UserPassword(f.User, f.Pass)
}

// fullURL - a whole URL typed in the address field (with the scheme), credentials added
func (f *ipForm) fullURL() (string, bool) {
	h := strings.TrimSpace(f.Host)
	u, err := url.Parse(h)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", false
	}
	if u.User == nil {
		u.User = f.userinfo()
	}
	return u.String(), true
}

func withPath(base, path string) string {
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

// sources returns the main and sub stream URLs (sub may be ""). ONVIF asks the camera for
// its profiles.
func (f *ipForm) sources() (main, sub string, err error) {
	if strings.TrimSpace(f.Host) == "" {
		return "", "", errors.New(T("Podaj adres kamery"))
	}
	switch f.Kind {
	case "rtsp", "http":
		if full, ok := f.fullURL(); ok {
			main = full
			if f.SubPath != "" {
				u, _ := url.Parse(full)
				u.Path, u.RawQuery = "", ""
				sub = withPath(u.String(), f.SubPath)
			}
			return main, sub, nil
		}
		def := 554
		if f.Kind == "http" {
			def = 80
		}
		u := url.URL{Scheme: f.Kind, User: f.userinfo(), Host: f.hostPort(def)}
		base := u.String()
		main = withPath(base, f.MainPath)
		if main == "" {
			main = base
		}
		return main, withPath(base, f.SubPath), nil
	case "onvif":
		u := url.URL{Scheme: "onvif", User: f.userinfo(), Host: f.hostPort(80)}
		tokens, err := onvifProfiles(u.String())
		if err != nil {
			return "", "", err
		}
		main = u.String() + "?subtype=" + url.QueryEscape(tokens[0])
		if len(tokens) > 1 {
			sub = u.String() + "?subtype=" + url.QueryEscape(tokens[1])
		}
		return main, sub, nil
	case "tapo":
		// the camera account password (Tapo app: Advanced settings - Camera account), or
		// the TP-Link cloud password
		u := url.URL{Scheme: "tapo", User: f.userinfo(), Host: f.hostPort(0)}
		if f.User == "" {
			u.User = url.User(f.Pass)
		}
		main = u.String()
		return main, main + "?subtype=1", nil
	case "dvrip":
		u := url.URL{Scheme: "dvrip", User: f.userinfo(), Host: f.hostPort(34569)}
		q := fmt.Sprintf("?channel=%d", f.Channel)
		return u.String() + q + "&subtype=0", u.String() + q + "&subtype=1", nil
	}
	return "", "", fmt.Errorf("unknown camera kind %q", f.Kind)
}

// onvifProfiles returns the media profile tokens of an ONVIF camera, the main stream
// (the largest picture) first and the sub stream (the smallest) second.
func onvifProfiles(src string) ([]string, error) {
	client, err := onvif.NewClient(src)
	if err != nil {
		return nil, err
	}
	tokens, err := client.GetProfilesTokens()
	if err != nil {
		return nil, fmt.Errorf("ONVIF: %w", err)
	}
	if len(tokens) == 0 {
		return nil, errors.New(T("Kamera ONVIF nie ma profili wideo (zły login lub hasło?)"))
	}
	if len(tokens) == 1 {
		return tokens, nil
	}
	pixels := map[string]int{}
	for _, t := range tokens {
		if b, err := client.GetProfile(t); err == nil {
			w, _ := strconv.Atoi(onvif.FindTagValue(b, "Width"))
			h, _ := strconv.Atoi(onvif.FindTagValue(b, "Height"))
			pixels[t] = w * h
		}
	}
	main, sub := tokens[0], tokens[1]
	for _, t := range tokens {
		if pixels[t] > pixels[main] {
			main = t
		}
	}
	for _, t := range tokens {
		if t != main && (sub == main || pixels[t] < pixels[sub]) {
			sub = t
		}
	}
	return []string{main, sub}, nil
}

// --- probing: picture size of a source

type probeResult struct {
	W, H  int
	Codec string
	Audio string // audio codec, "" - none
}

var probeSeq struct {
	sync.Mutex
	n int
}

// probeSource connects to the source through go2rtc (a temporary stream, the same path
// as the camera will use) and reads the picture size with ffprobe.
func probeSource(src string) (*probeResult, error) {
	probeSeq.Lock()
	probeSeq.n++
	name := fmt.Sprintf("%sprobe%d", hiddenPrefix, probeSeq.n)
	probeSeq.Unlock()
	if _, err := streams.New(name, src); err != nil {
		return nil, err
	}
	defer streams.Delete(name)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffprobe", "-hide_banner", "-loglevel", "error",
		"-rtsp_transport", "tcp", "-timeout", "15000000",
		"-show_entries", "stream=codec_type,codec_name,width,height", "-of", "json", rtspURL(name))
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, errors.New(T("Kamera nie odpowiada (przekroczony czas)"))
		}
		msg := strings.TrimSpace(stderr.String())
		if strings.Contains(msg, "404") {
			// go2rtc couldn't open the source: wrong address, login or password
			return nil, errors.New(T("Nie udało się połączyć z kamerą – sprawdź adres, login i hasło"))
		}
		return nil, fmt.Errorf("ffprobe: %v %s", err, msg)
	}
	var res struct {
		Streams []struct {
			Type   string `json:"codec_type"`
			Codec  string `json:"codec_name"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		return nil, err
	}
	r := &probeResult{}
	for _, s := range res.Streams {
		switch {
		case s.Type == "video" && r.W == 0:
			r.W, r.H, r.Codec = s.Width, s.Height, s.Codec
		case s.Type == "audio" && r.Audio == "":
			r.Audio = s.Codec
		}
	}
	if r.W == 0 || r.H == 0 {
		return nil, errors.New(T("Kamera nie wysyła obrazu"))
	}
	return r, nil
}

// hiddenPrefix - temporary go2rtc streams of the app, not cameras
const hiddenPrefix = "__xc_"
