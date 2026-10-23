package xccore

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Regions of the Mi Home cloud (name: Polish source string of the UI)
var Regions = []struct{ Code, Name string }{
	{"de", "Europa (Polska)"}, {"cn", "Chiny"}, {"i2", "Indie"},
	{"ru", "Rosja"}, {"sg", "Singapur"}, {"us", "USA"},
}

// Quality - a picture quality of a Xiaomi camera (subtype param of the source)
type Quality struct {
	Name, Subtype string
	W, H          int
}

var XiaomiQualities = []Quality{
	{"2.5K", "3", 2560, 1440},
	{"720p", "", 1280, 720},
	{"SD", "sd", 1280, 720},
}

// OlderQualities - cameras without the 2.5K stream (e.g. Mi 360° 1080p, 2K Pro, C200):
// videoquality 3 is broken on some of them, so only the camera's HD and SD.
var OlderQualities = []Quality{
	{"HD", "", 1920, 1080},
	{"SD", "sd", 640, 360},
}

// models2K - cameras whose 2.5K stream is videoquality 3 (Hualai CW400 / CW700S family)
var models2K = map[string]bool{
	"isa.camera.700sa":  true, // Outdoor Camera CW700S
	"isa.camera.hlzoom": true, // CW400 Pro
	"isa.camera.hlc8":   true, // CW400
}

// QualitiesForModel returns the picture qualities of a Xiaomi camera model.
func QualitiesForModel(model string) []Quality {
	if models2K[model] {
		return XiaomiQualities
	}
	return OlderQualities
}

// CameraQualities returns the picture qualities of a Xiaomi camera of the app.
func CameraQualities(name string) []Quality {
	return QualitiesForModel(SourceParam(CameraSource(name), "model"))
}

// XiaomiQuality returns the index of the selected quality of a Xiaomi camera.
func XiaomiQuality(name string) int {
	qs := CameraQualities(name)
	sub := SourceParam(CameraSource(name), "subtype")
	for i, q := range qs {
		if q.Subtype == sub {
			return i
		}
	}
	for i, q := range qs {
		if q.Subtype == "" {
			return i
		}
	}
	return 0
}

// --- Mi account login (the go2rtc API, the same flow as go2rtc WebUI)

// Accounts returns the logged in Mi accounts.
func Accounts() []string {
	var accounts []string
	if b, code, err := APICall("GET", "/api/xiaomi", nil, nil); err == nil && code == http.StatusOK {
		_ = json.Unmarshal(b, &accounts)
	}
	return accounts
}

// LoginResult - the answer to a login step: OK, or what Xiaomi asks for next.
type LoginResult struct {
	OK          bool   `json:"ok"`
	Captcha     string `json:"captcha,omitempty"` // base64 JPEG: send "captcha"
	VerifyPhone string `json:"verify_phone,omitempty"`
	VerifyEmail string `json:"verify_email,omitempty"` // a code was sent: send "verify"
	QR          string `json:"qr,omitempty"`           // base64 PNG to scan: send "qr"="wait"
	QRURL       string `json:"qr_url,omitempty"`
	QRWait      bool   `json:"qr_wait,omitempty"` // not scanned yet: send "qr"="wait" again
}

// Login sends a login step: {username, password}, then {captcha} or {verify} when asked;
// or {qr: "start"}, then {qr: "wait"} until the code is scanned with the phone.
func Login(form url.Values) (*LoginResult, error) {
	b, code, err := APICall("POST", "/api/xiaomi", nil, form)
	switch {
	case err != nil:
		return nil, err
	case code == http.StatusUnauthorized:
		r := &LoginResult{}
		_ = json.Unmarshal(b, r)
		return r, nil
	case code != http.StatusOK:
		return nil, fmt.Errorf("login failed: %s", strings.TrimSpace(string(b)))
	}
	return &LoginResult{OK: true}, nil
}

// Device - a camera of the Mi account
type Device struct {
	Name string `json:"name"`
	Info string `json:"info"`
	URL  string `json:"url"`
}

// ErrNoDevices - no cameras of the account in the region
var ErrNoDevices = fmt.Errorf("no cameras in this region")

// Devices lists the cameras of the account in the region.
func Devices(account, region string) ([]Device, error) {
	b, status, err := APICall("GET", "/api/xiaomi", url.Values{"id": {account}, "region": {region}}, nil)
	switch {
	case err != nil:
		return nil, err
	case status == http.StatusNotFound:
		return nil, ErrNoDevices
	case status != http.StatusOK:
		return nil, fmt.Errorf("%s", strings.TrimSpace(string(b)))
	}
	var res struct {
		Sources []Device `json:"sources"`
	}
	err = json.Unmarshal(b, &res)
	return res.Sources, err
}

// UniqueName returns name, or "name 2", "name 3"... when a camera has it.
func UniqueName(name string) string {
	existing := map[string]bool{}
	for _, n := range CameraNames() {
		existing[n] = true
	}
	out := name
	for n := 2; existing[out]; n++ {
		out = fmt.Sprintf("%s %d", name, n)
	}
	return out
}

// AddXiaomiCamera adds a camera of the account (a Device URL) under a unique name and
// returns the name. p2p=server works from any network (EU CS2 cameras; TUTK and legacy
// ones connect in the LAN anyway); subtype=3 - 2.5K (2560x1440) on the cameras that have it.
func AddXiaomiCamera(name, deviceURL string) (string, error) {
	name = UniqueName(name)
	var params string
	// the CS2 servers (pkg/xiaomi/miss/cs2) are the EU ones: cameras of other regions
	// connect in the LAN (p2p=server = the Xiaomi servers, from any network)
	if u, err := url.Parse(deviceURL); err == nil {
		if region, _ := u.User.Password(); region == "de" {
			params = "&p2p=server"
		}
	}
	if models2K[SourceParam(deviceURL, "model")] {
		params += "&subtype=3"
	}
	return name, AddCamera(name, deviceURL+params)
}
