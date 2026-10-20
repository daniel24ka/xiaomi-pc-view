package xccore

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Controls of Imou cameras over the Imou Open API (open.imoulife.com, the cloud of the Imou
// Life app): the white light, the siren, the night vision mode and the camera points
// ("collection points", kept in the camera under a name, also seen in the Imou Life app).
// Imou cameras have no ONVIF presets, absolute positions or IR cut filter, so these come
// from the cloud; pan/tilt, motion events and the video stay local (ONVIF, RTSP). The video
// over the cloud (bindDeviceLive HLS, SD playback) is never used: it costs media flow.
//
// The developer app (AppId, AppSecret from the console, free: 30,000 requests a month) of
// the Imou Life account sees the devices of that account. Every request is a POST of
// {system:{ver,appId,sign,time,nonce},id,params} to https://openapi-<dc>.easy4ip.com/openapi/<method>,
// sign = Base64(HMAC-SHA256("time:T,nonce:N,appSecret:S", key=hex(SHA-256(S)))); the
// accessToken lasts 3 days (TK1002: expired).

const imouDefaultDC = "fk" // Europe (Frankfurt); "sg" Asia, "or" America

var ImouDataCenters = []struct{ ID, Name string }{
	{"fk", "Europa"}, {"sg", "Azja"}, {"or", "Ameryka"},
}

var errNoImou = errors.New("the camera has no Imou cloud settings")

// ImouError - an error code of the Imou Open API (e.g. DV1026: the device doesn't support it)
type ImouError struct{ Code, Msg string }

func (e *ImouError) Error() string { return "Imou: " + e.Code + ": " + strings.TrimSpace(e.Msg) }

type imouClient struct {
	app, secret, dc string
	mu              sync.Mutex
	token           string
	expires         time.Time
}

var imouClients sync.Map // dc + app + secret -> *imouClient

func imouClientOf(app, secret, dc string) *imouClient {
	if dc == "" {
		dc = imouDefaultDC
	}
	key := dc + "\x00" + app + "\x00" + secret
	if v, ok := imouClients.Load(key); ok {
		return v.(*imouClient)
	}
	v, _ := imouClients.LoadOrStore(key, &imouClient{app: app, secret: secret, dc: dc})
	return v.(*imouClient)
}

func imouSign(t int64, nonce, secret string) string {
	h := sha256.Sum256([]byte(secret))
	m := hmac.New(sha256.New, []byte(hex.EncodeToString(h[:])))
	m.Write([]byte("time:" + strconv.FormatInt(t, 10) + ",nonce:" + nonce + ",appSecret:" + secret))
	return base64.StdEncoding.EncodeToString(m.Sum(nil))
}

func imouNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

var imouHTTP = &http.Client{Timeout: 15 * time.Second}

// post sends one request (token: added to params when not "").
func (c *imouClient) post(method, token string, params map[string]any) (json.RawMessage, error) {
	p := map[string]any{}
	for k, v := range params {
		p[k] = v
	}
	if token != "" {
		p["token"] = token
	}
	t := time.Now().Unix()
	nonce := imouNonce()
	body, _ := json.Marshal(map[string]any{
		"system": map[string]any{"ver": "1.0", "appId": c.app, "sign": imouSign(t, nonce, c.secret), "time": t, "nonce": nonce},
		"id":     imouNonce(),
		"params": p,
	})
	res, err := imouHTTP.Post("https://openapi-"+c.dc+".easy4ip.com/openapi/"+method, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	var r struct {
		Result struct {
			Code string          `json:"code"`
			Msg  string          `json:"msg"`
			Data json.RawMessage `json:"data"`
		} `json:"result"`
	}
	if err = json.Unmarshal(raw, &r); err != nil || r.Result.Code == "" {
		return nil, fmt.Errorf("Imou %s: HTTP %d", method, res.StatusCode)
	}
	if r.Result.Code != "0" {
		return nil, &ImouError{r.Result.Code, r.Result.Msg}
	}
	return r.Result.Data, nil
}

func (c *imouClient) accessToken(renew bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !renew && c.token != "" && time.Now().Before(c.expires) {
		return c.token, nil
	}
	data, err := c.post("accessToken", "", nil)
	if err != nil {
		return "", err
	}
	var tk struct {
		AccessToken string `json:"accessToken"`
		ExpireTime  int64  `json:"expireTime"`
	}
	if err = json.Unmarshal(data, &tk); err != nil || tk.AccessToken == "" {
		return "", errors.New("Imou: no accessToken")
	}
	c.token = tk.AccessToken
	c.expires = time.Now().Add(time.Duration(tk.ExpireTime)*time.Second - time.Hour)
	return c.token, nil
}

// call sends a request with the token (renewed once when it expired) and decodes data
// into out (nil: ignored).
func (c *imouClient) call(method string, params map[string]any, out any) error {
	token, err := c.accessToken(false)
	if err != nil {
		return err
	}
	data, err := c.post(method, token, params)
	var ie *ImouError
	if errors.As(err, &ie) && ie.Code == "TK1002" {
		if token, err = c.accessToken(true); err != nil {
			return err
		}
		data, err = c.post(method, token, params)
	}
	if err != nil || out == nil || len(data) == 0 {
		return err
	}
	return json.Unmarshal(data, out)
}

// ImouDevice - a camera of the Imou account (for picking it in the IP camera form)
type ImouDevice struct {
	ID, Name, Model, Status string
}

// ImouDevices lists the devices of the Imou account of the developer app (bound and shared).
func ImouDevices(app, secret, dc string) ([]ImouDevice, error) {
	cl := imouClientOf(app, secret, dc)
	var list []ImouDevice
	for page := 1; page <= 10; page++ {
		var r struct {
			DeviceList []struct {
				DeviceID     string `json:"deviceId"`
				DeviceName   string `json:"deviceName"`
				DeviceModel  string `json:"deviceModel"`
				DeviceStatus string `json:"deviceStatus"`
			} `json:"deviceList"`
		}
		if err := cl.call("listDeviceDetailsByPage", map[string]any{"page": page, "pageSize": 50, "source": "bindAndShare"}, &r); err != nil {
			return nil, err
		}
		for _, d := range r.DeviceList {
			list = append(list, ImouDevice{d.DeviceID, d.DeviceName, d.DeviceModel, d.DeviceStatus})
		}
		if len(r.DeviceList) < 50 {
			break
		}
	}
	return list, nil
}

// --- an Imou camera of the app

type imouCam struct {
	cl                   *imouClient
	device               string
	Light, Siren, Points bool
	modes                []string // night vision modes of the camera (Intelligent, FullColor, Infrared, Off…)

	iotOnce sync.Once
	iot     *imouIoT
	iotErr  error
}

// imouIoT - services of the things data model of an IoT device (refs), for what the
// device enable switches don't do (Cruiser: the siren answers DV1026 to setDeviceCameraStatus)
type imouIoT struct {
	product                          string
	sirenStart, sirenTime, sirenStop string // SirenStart, its clientLocalTime input, SirenStop
}

type imouEntry struct {
	cam *imouCam
	err error
	at  time.Time
}

var imouCams sync.Map // camera name -> imouEntry

// HasImou reports whether the IP camera has Imou cloud settings.
func HasImou(name string) bool {
	ip := IPCam(name)
	return ip != nil && ip.ImouApp != "" && ip.ImouSecret != "" && ip.ImouDevice != ""
}

// imouOf returns the Imou cloud controls of an IP camera (detected once; a failure is asked
// again after a minute). errNoImou: no Imou settings.
func imouOf(name string) (*imouCam, error) {
	if !HasImou(name) {
		return nil, errNoImou
	}
	if v, ok := imouCams.Load(name); ok {
		e := v.(imouEntry)
		if e.cam != nil || time.Since(e.at) < time.Minute {
			return e.cam, e.err
		}
	}
	c, err := detectImou(name)
	imouCams.Store(name, imouEntry{c, err, time.Now()})
	return c, err
}

// imouCached returns the Imou controls when they were detected already (never waits).
func imouCached(name string) *imouCam {
	if v, ok := imouCams.Load(name); ok {
		return v.(imouEntry).cam
	}
	return nil
}

func forgetImou(name string) {
	imouCams.Delete(name)
}

// detectImou reads the capability set of the device (and its night vision modes).
func detectImou(name string) (*imouCam, error) {
	ip := IPCam(name)
	if ip == nil {
		return nil, errNoImou
	}
	c := &imouCam{cl: imouClientOf(ip.ImouApp, ip.ImouSecret, ip.ImouDC), device: ip.ImouDevice}
	var r struct {
		DeviceList []struct {
			Ability  string `json:"ability"`
			Channels []struct {
				Ability string `json:"ability"`
			} `json:"channels"`
		} `json:"deviceList"`
	}
	err := c.cl.call("deviceBaseDetailList", map[string]any{
		"deviceList": []map[string]any{{"deviceId": c.device, "channelList": "0"}},
	}, &r)
	if err != nil {
		return nil, err
	}
	if len(r.DeviceList) == 0 {
		return nil, fmt.Errorf("Imou: no device %s on the account", c.device)
	}
	abilities := map[string]bool{}
	add := func(s string) {
		for _, a := range strings.Split(s, ",") {
			abilities[strings.TrimSpace(a)] = true
		}
	}
	add(r.DeviceList[0].Ability)
	for _, ch := range r.DeviceList[0].Channels {
		add(ch.Ability)
	}
	c.Light = abilities["WhiteLight"] || abilities["ChnWhiteLight"]
	c.Siren = abilities["Siren"]
	c.Points = abilities["CollectionPoint"]
	if abilities["NVM"] {
		var nv struct {
			Modes []string `json:"modes"`
		}
		if err := c.cl.call("getNightVisionMode", c.channel(nil), &nv); err == nil {
			c.modes = nv.Modes
		}
	}
	return c, nil
}

// channel returns the params with the device and channel 0.
func (c *imouCam) channel(params map[string]any) map[string]any {
	p := map[string]any{"deviceId": c.device, "channelId": "0"}
	for k, v := range params {
		p[k] = v
	}
	return p
}

// --- white light and siren (device enable switches)

func (c *imouCam) switchState(enableType string) (bool, error) {
	var r struct {
		Status string `json:"status"`
	}
	err := c.cl.call("getDeviceCameraStatus", c.channel(map[string]any{"enableType": enableType}), &r)
	return r.Status == "on", err
}

func (c *imouCam) setSwitch(enableType string, on bool) error {
	return c.cl.call("setDeviceCameraStatus", c.channel(map[string]any{"enableType": enableType, "enable": on}), nil)
}

// setSiren turns the siren on or off: the device switch, or the SirenStart / SirenStop
// services of the things data model when the device doesn't take the switch (DV1026).
func (c *imouCam) setSiren(on bool) error {
	err := c.setSwitch("siren", on)
	var ie *ImouError
	if !errors.As(err, &ie) || ie.Code != "DV1026" {
		return err
	}
	m, err := c.iotModel()
	if err != nil {
		return err
	}
	if on && m.sirenStart != "" {
		content := map[string]any{}
		if m.sirenTime != "" {
			content[m.sirenTime] = time.Now().Format("2006-01-02 15:04:05")
		}
		return c.iotControl(m, m.sirenStart, content)
	}
	if !on && m.sirenStop != "" {
		return c.iotControl(m, m.sirenStop, map[string]any{})
	}
	return ErrNotXiaomi
}

func (c *imouCam) iotControl(m *imouIoT, ref string, content map[string]any) error {
	return c.cl.call("iotDeviceControl", map[string]any{
		"productId": m.product, "deviceId": c.device, "ref": ref, "content": content,
	}, nil)
}

// iotModel reads the product of the device and its things data model (once).
func (c *imouCam) iotModel() (*imouIoT, error) {
	c.iotOnce.Do(func() {
		var r struct {
			DeviceList []struct {
				ProductID string `json:"productId"`
			} `json:"deviceList"`
		}
		err := c.cl.call("listDeviceDetailsByIds", map[string]any{
			"deviceList": []map[string]any{{"deviceId": c.device}},
		}, &r)
		if err == nil && (len(r.DeviceList) == 0 || r.DeviceList[0].ProductID == "") {
			err = errors.New("Imou: not an IoT device")
		}
		if err != nil {
			c.iotErr = err
			return
		}
		var model struct {
			Services []struct {
				Identifier string `json:"identifier"`
				Ref        string `json:"ref"`
				InputData  []struct {
					Identifier string `json:"identifier"`
					Ref        string `json:"ref"`
				} `json:"inputData"`
			} `json:"services"`
		}
		m := &imouIoT{product: r.DeviceList[0].ProductID}
		if c.iotErr = c.cl.call("getProductModel", map[string]any{"productId": m.product}, &model); c.iotErr != nil {
			return
		}
		for _, s := range model.Services {
			switch s.Identifier {
			case "SirenStart":
				m.sirenStart = s.Ref
				for _, in := range s.InputData {
					if in.Identifier == "clientLocalTime" {
						m.sirenTime = in.Ref
					}
				}
			case "SirenStop":
				m.sirenStop = s.Ref
			}
		}
		c.iot = m
	})
	return c.iot, c.iotErr
}

// --- night vision: the Imou modes as the app's night modes (day/night switches by itself)

var imouNightModes = []struct {
	mode  string
	value int // NightMode.Value
}{
	{"Intelligent", 5}, // smart night vision: infrared, color with the light on motion
	{"FullColor", 3},   // full color: the white light at night
	{"Infrared", 2},
	{"Off", 0},
}

func (c *imouCam) nightModes() []NightMode {
	var out []NightMode
	for _, m := range NightModes {
		for _, im := range imouNightModes {
			if im.value == m.Value && slices.Contains(c.modes, im.mode) {
				out = append(out, m)
			}
		}
	}
	return out
}

func (c *imouCam) nightMode() (int, error) {
	var r struct {
		Mode string `json:"mode"`
	}
	if err := c.cl.call("getNightVisionMode", c.channel(nil), &r); err != nil {
		return 0, err
	}
	for _, im := range imouNightModes {
		if im.mode == r.Mode {
			return im.value, nil
		}
	}
	return 0, fmt.Errorf("Imou: unknown night vision mode %q", r.Mode)
}

func (c *imouCam) setNightMode(value int) error {
	for _, im := range imouNightModes {
		if im.value == value && slices.Contains(c.modes, im.mode) {
			return c.cl.call("setNightVisionMode", c.channel(map[string]any{"mode": im.mode}), nil)
		}
	}
	return ErrNotXiaomi
}

// --- MIoT-like properties of Imou cameras (the UI and the API read the light, the siren
// and the night mode as properties); a property the camera hasn't: code -704040003

const codeNoProp = -704040003

func imouGetProps(name string, props []MiotProp) ([]MiotProp, error) {
	c, err := imouControl(name)
	if err != nil {
		return nil, err
	}
	out := make([]MiotProp, 0, len(props))
	for _, p := range props {
		q := MiotProp{Siid: p.Siid, Piid: p.Piid, Code: codeNoProp}
		switch {
		case p.Same(PropLight) && c.Light:
			on, err := c.switchState("whiteLight")
			if err != nil {
				return nil, err
			}
			q.Value, q.Code = on, 0
		case p.Same(PropSiren) && c.Siren:
			on, err := c.switchState("siren")
			if err != nil {
				return nil, err
			}
			q.Value, q.Code = on, 0
		case p.Same(PropNightShot) && len(c.nightModes()) > 0:
			v, err := c.nightMode()
			if err != nil {
				return nil, err
			}
			q.Value, q.Code = float64(v), 0
		}
		out = append(out, q)
	}
	return out, nil
}

func imouSetProp(name string, p MiotProp, value any) error {
	c, err := imouControl(name)
	if err != nil {
		return err
	}
	on, _ := value.(bool)
	// the cloud reports the new state of a switch about a second later (the UI reads it back)
	settle := func(err error) error {
		if err == nil {
			time.Sleep(1500 * time.Millisecond)
		}
		return err
	}
	switch {
	case p.Same(PropLight) && c.Light:
		return settle(c.setSwitch("whiteLight", on))
	case p.Same(PropSiren) && c.Siren:
		return settle(c.setSiren(on))
	case p.Same(PropNightShot):
		v, ok := value.(int)
		if !ok {
			return ErrNotXiaomi
		}
		return c.setNightMode(v)
	}
	return ErrNotXiaomi
}

// imouControl returns the Imou controls of an IP camera (ErrNotXiaomi: no Imou settings).
func imouControl(name string) (*imouCam, error) {
	c, err := imouOf(name)
	if errors.Is(err, errNoImou) {
		return nil, ErrNotXiaomi
	}
	return c, err
}

// --- camera points: collection points of the camera (Preset.Coll: the name in the camera)

// imouPoints returns the Imou controls when the camera keeps its points in the cloud.
func imouPoints(name string) *imouCam {
	if c, err := imouOf(name); err == nil && c.Points {
		return c
	}
	return nil
}

func (c *imouCam) collections() ([]string, error) {
	var r struct {
		Collections []struct {
			Name string `json:"name"`
		} `json:"collections"`
	}
	if err := c.cl.call("getCollection", c.channel(nil), &r); err != nil {
		return nil, err
	}
	var names []string
	for _, x := range r.Collections {
		names = append(names, x.Name)
	}
	return names, nil
}

// pointCall sends a collection point request; the camera answers 21201 ("Unknown error")
// to one that comes right after another point change: tried again a few times.
func (c *imouCam) pointCall(method string, params map[string]any) error {
	var err error
	for try := 0; try < 4; try++ {
		if try > 0 {
			time.Sleep(2 * time.Second)
		}
		err = c.cl.call(method, c.channel(params), nil)
		var ie *ImouError
		if !errors.As(err, &ie) || ie.Code != "21201" {
			return err
		}
	}
	return err
}

// uniqueName returns name, or "name 2", "name 3"… when the camera has it already.
func uniqueName(name string, taken []string) string {
	s := name
	for n := 2; slices.Contains(taken, s); n++ {
		s = fmt.Sprintf("%s %d", name, n)
	}
	return s
}

// savePoint stores the current position as point slot under name (the old position of the
// slot is removed from the camera).
func (c *imouCam) savePoint(camera string, slot int, name string) error {
	list, err := GetPresets(camera)
	if err != nil {
		return err
	}
	out := []Preset{}
	for _, p := range list {
		if p.Idx != slot {
			out = append(out, p)
		} else if p.Coll != "" {
			_ = c.pointCall("deleteCollection", map[string]any{"names": []string{p.Coll}})
		}
	}
	taken, err := c.collections()
	if err != nil {
		return err
	}
	coll := uniqueName(name, taken)
	if err = c.pointCall("setCollection", map[string]any{"name": coll, "needCruise": false}); err != nil {
		return err
	}
	out = append(out, Preset{Idx: slot, Name: base64.StdEncoding.EncodeToString([]byte(name)), Coll: coll})
	slices.SortFunc(out, func(a, b Preset) int { return a.Idx - b.Idx })
	return PutPresets(camera, out)
}

func (c *imouCam) renamePoint(p Preset, name string) (string, error) {
	if p.Coll == "" {
		return "", nil
	}
	taken, err := c.collections()
	if err != nil {
		return "", err
	}
	taken = slices.DeleteFunc(taken, func(s string) bool { return s == p.Coll })
	coll := uniqueName(name, taken)
	if coll == p.Coll {
		return coll, nil
	}
	return coll, c.pointCall("modifyCollection", map[string]any{"oldName": p.Coll, "newName": coll})
}

func (c *imouCam) deletePoint(p Preset) error {
	if p.Coll == "" {
		return nil
	}
	return c.pointCall("deleteCollection", map[string]any{"names": []string{p.Coll}})
}

func (c *imouCam) gotoPoint(p Preset) error {
	if p.Coll == "" {
		return fmt.Errorf("point %d is not in the camera", p.Idx)
	}
	return c.pointCall("turnCollection", map[string]any{"name": p.Coll})
}

// imouTurnTime - how long GotoPresetAndWait waits for an Imou camera to turn to a point
const imouTurnTime = 6 * time.Second
