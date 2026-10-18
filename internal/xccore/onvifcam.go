package xccore

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/onvif"
)

// Controls of IP cameras over ONVIF: pan/tilt (PTZ ContinuousMove), the position (GetStatus,
// AbsoluteMove) for the camera points, which the app keeps in its config (many cameras, e.g.
// Imou, have no ONVIF presets), the IR cut filter (Imaging) as the night mode, and motion
// events (PullPoint subscription). A camera added as RTSP or HTTP is asked on port 80 with
// the same login (Dahua / Imou, Hikvision, Reolink… answer ONVIF there).

type onvifCam struct {
	user                         *url.Userinfo
	device, ptz, imaging, events string // service URLs
	profile, source              string // media profile token, its video source token
	PTZ, Absolute, Motion        bool
	IRModes                      []string // IrCutFilter modes of the camera: ON, OFF, AUTO
}

var errNoONVIF = errors.New("the camera has no ONVIF")

type onvifEntry struct {
	cam *onvifCam
	err error
	at  time.Time
}

var onvifCams sync.Map // camera name -> onvifEntry

// onvifOf returns the ONVIF services of an IP camera (detected once; a failure is asked
// again after a minute). errNoONVIF: not an IP camera, or a kind without ONVIF.
func onvifOf(name string) (*onvifCam, error) {
	if v, ok := onvifCams.Load(name); ok {
		e := v.(onvifEntry)
		if e.cam != nil || time.Since(e.at) < time.Minute {
			return e.cam, e.err
		}
	}
	c, err := detectONVIF(name)
	onvifCams.Store(name, onvifEntry{c, err, time.Now()})
	return c, err
}

// onvifCached returns the ONVIF services when they were detected already (never waits).
func onvifCached(name string) *onvifCam {
	if v, ok := onvifCams.Load(name); ok {
		return v.(onvifEntry).cam
	}
	return nil
}

// forgetONVIF - the camera was edited or removed: detect it again.
func forgetONVIF(name string) {
	onvifCams.Delete(name)
	CapsCache.Delete(name)
}

func detectONVIF(name string) (*onvifCam, error) {
	ip := IPCam(name)
	if ip == nil {
		return nil, errNoONVIF
	}
	u, err := url.Parse(ip.Main)
	if err != nil {
		return nil, errNoONVIF
	}
	host := u.Host
	switch u.Scheme {
	case "onvif":
		if u.Port() == "" {
			host = u.Hostname() + ":80"
		}
	case "rtsp", "rtsps", "http", "https":
		host = u.Hostname() + ":80"
	default:
		return nil, errNoONVIF
	}
	base := "http://" + host
	c := &onvifCam{user: u.User, device: base + "/onvif/device_service"}

	b, err := c.request(c.device, `<tds:GetCapabilities><tds:Category>All</tds:Category></tds:GetCapabilities>`)
	if err != nil {
		return nil, err
	}
	service := func(tag, def string) string {
		s := onvif.FindTagValue(b, tag+".+?XAddr")
		if s == "" {
			return ""
		}
		if p, err := url.Parse(strings.TrimSpace(s)); err == nil && p.Path != "" {
			return base + p.Path
		}
		return base + def
	}
	media := service("Media", "/onvif/media_service")
	c.ptz = service("PTZ", "/onvif/ptz_service")
	c.imaging = service("Imaging", "/onvif/imaging_service")
	c.events = service("Events", "/onvif/event_service")
	if media == "" {
		return nil, errNoONVIF
	}

	b, err = c.request(media, `<trt:GetProfiles/>`)
	if err != nil {
		return nil, err
	}
	if m := regexp.MustCompile(`Profiles[^>]*token="([^"]+)`).FindSubmatch(b); m != nil {
		c.profile = string(m[1])
	}
	c.source = onvif.FindTagValue(b, "SourceToken")
	if c.profile == "" {
		return nil, errors.New("ONVIF: no media profile")
	}

	if c.ptz != "" && bytes.Contains(b, []byte("PTZConfiguration")) {
		if b, err := c.request(c.ptz, `<tptz:GetNodes `+nsPTZ+`/>`); err == nil {
			c.PTZ = bytes.Contains(b, []byte("ContinuousPanTiltVelocitySpace"))
			c.Absolute = c.PTZ && bytes.Contains(b, []byte("AbsolutePanTiltPositionSpace")) && c.absoluteWorks()
		}
	}
	if c.imaging != "" && c.source != "" {
		if b, err := c.request(c.imaging, `<timg:GetOptions `+nsImaging+`><timg:VideoSourceToken>`+c.source+
			`</timg:VideoSourceToken></timg:GetOptions>`); err == nil {
			for _, m := range regexp.MustCompile(`<(?:\w+:)?IrCutFilterModes>([^<]+)`).FindAllSubmatch(b, -1) {
				c.IRModes = append(c.IRModes, strings.TrimSpace(string(m[1])))
			}
		}
	}
	if c.events != "" {
		if b, err := c.request(c.events, `<tev:GetEventProperties `+nsEvents+`/>`); err == nil {
			c.Motion = motionTopic.Match(b)
		}
	}
	return c, nil
}

const (
	nsPTZ     = `xmlns:tptz="http://www.onvif.org/ver20/ptz/wsdl"`
	nsImaging = `xmlns:timg="http://www.onvif.org/ver20/imaging/wsdl"`
	nsEvents  = `xmlns:tev="http://www.onvif.org/ver10/events/wsdl" xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"`
)

// motionTopic - the motion topics of the cameras (Dahua / Imou: VideoSource/MotionAlarm,
// others: RuleEngine/CellMotionDetector/Motion, RuleEngine/MotionDetector/Motion)
var motionTopic = regexp.MustCompile(`MotionAlarm|CellMotionDetector|MotionDetector`)

var onvifHTTP = &http.Client{Timeout: 10 * time.Second}

func (c *onvifCam) request(url, body string) ([]byte, error) {
	return c.requestWith(onvifHTTP, url, "", body)
}

// requestWith sends a request; action (not ""): with the WS-Addressing Action and To headers,
// which the subscription manager of Dahua / Imou needs ("Resource Unknown" without them).
func (c *onvifCam) requestWith(client *http.Client, url, action, body string) ([]byte, error) {
	res, err := client.Post(url, "application/soap+xml;charset=utf-8", bytes.NewReader(c.envelope(url, action, body)))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusOK && !bytes.Contains(b, []byte(":Fault>")) {
		return b, nil
	}
	if res.StatusCode == http.StatusUnauthorized || bytes.Contains(b, []byte("NotAuthorized")) {
		return nil, errors.New("ONVIF: wrong login or password")
	}
	if s := onvif.FindTagValue(b, "Text"); s != "" {
		return nil, errors.New("ONVIF: " + strings.TrimSpace(s))
	}
	return nil, errors.New("ONVIF: " + res.Status)
}

// envelope - the SOAP request with the login (WS-Security UsernameToken digest, like
// go2rtc pkg/onvif) and the WS-Addressing headers.
func (c *onvifCam) envelope(to, action, body string) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"` +
		` xmlns:tt="http://www.onvif.org/ver10/schema" xmlns:tds="http://www.onvif.org/ver10/device/wsdl"` +
		` xmlns:trt="http://www.onvif.org/ver10/media/wsdl" xmlns:wsa="http://www.w3.org/2005/08/addressing"><s:Header>`)
	if action != "" {
		b.WriteString(`<wsa:Action>` + action + `</wsa:Action><wsa:To>` + html.EscapeString(to) + `</wsa:To>`)
	}
	if c.user != nil {
		nonce := core.RandString(16, 36)
		created := time.Now().UTC().Format(time.RFC3339Nano)
		pass, _ := c.user.Password()
		h := sha1.Sum([]byte(nonce + created + pass))
		b.WriteString(`<wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">` +
			`<wsse:UsernameToken><wsse:Username>` + html.EscapeString(c.user.Username()) + `</wsse:Username>` +
			`<wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">` +
			base64.StdEncoding.EncodeToString(h[:]) + `</wsse:Password>` +
			`<wsse:Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">` +
			base64.StdEncoding.EncodeToString([]byte(nonce)) + `</wsse:Nonce>` +
			`<wsu:Created xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">` +
			created + `</wsu:Created></wsse:UsernameToken></wsse:Security>`)
	}
	b.WriteString(`</s:Header><s:Body>` + body + `</s:Body></s:Envelope>`)
	return []byte(b.String())
}

// --- pan/tilt

const onvifSpeed = 0.4

// move starts a continuous movement in a direction (MotorLeft…MotorDown) or stops it
// (motorStop). The camera stops by itself after a second, so a lost stop does no harm;
// the Mover repeats the command while the button is held.
func (c *onvifCam) move(op int) error {
	if !c.PTZ {
		return ErrNotXiaomi
	}
	if op == motorStop {
		_, err := c.request(c.ptz, `<tptz:Stop `+nsPTZ+`><tptz:ProfileToken>`+c.profile+
			`</tptz:ProfileToken><tptz:PanTilt>true</tptz:PanTilt><tptz:Zoom>false</tptz:Zoom></tptz:Stop>`)
		return err
	}
	var x, y float64
	switch op {
	case MotorLeft:
		x = -onvifSpeed
	case MotorRight:
		x = onvifSpeed
	case MotorUp:
		y = onvifSpeed
	case MotorDown:
		y = -onvifSpeed
	default:
		return fmt.Errorf("unknown motor operation %d", op)
	}
	_, err := c.request(c.ptz, fmt.Sprintf(`<tptz:ContinuousMove %s><tptz:ProfileToken>%s</tptz:ProfileToken>`+
		`<tptz:Velocity><tt:PanTilt x="%g" y="%g"/></tptz:Velocity><tptz:Timeout>PT1S</tptz:Timeout></tptz:ContinuousMove>`,
		nsPTZ, c.profile, x, y))
	return err
}

var panTiltRe = regexp.MustCompile(`PanTilt[^>]*?\sx="([-0-9.eE]+)"[^>]*?\sy="([-0-9.eE]+)"`)

// position returns the pan/tilt position (-1…1).
func (c *onvifCam) position() (x, y float64, err error) {
	b, err := c.request(c.ptz, `<tptz:GetStatus `+nsPTZ+`><tptz:ProfileToken>`+c.profile+`</tptz:ProfileToken></tptz:GetStatus>`)
	if err != nil {
		return 0, 0, err
	}
	m := panTiltRe.FindSubmatch(b)
	if m == nil {
		return 0, 0, errors.New("ONVIF: no pan/tilt position")
	}
	x, _ = strconv.ParseFloat(string(m[1]), 64)
	y, _ = strconv.ParseFloat(string(m[2]), 64)
	return x, y, nil
}

func (c *onvifCam) moveTo(x, y float64) error {
	_, err := c.request(c.ptz, fmt.Sprintf(`<tptz:AbsoluteMove %s><tptz:ProfileToken>%s</tptz:ProfileToken>`+
		`<tptz:Position><tt:PanTilt x="%g" y="%g"/></tptz:Position></tptz:AbsoluteMove>`, nsPTZ, c.profile, x, y))
	return err
}

// absoluteWorks checks that the camera really turns to a position: AbsoluteMove to where it
// is (it doesn't move). Some cameras (Imou) list the space, but don't implement the method
// and report a fixed position.
func (c *onvifCam) absoluteWorks() bool {
	x, y, err := c.position()
	return err == nil && c.moveTo(x, y) == nil
}

// waitStill waits until the position stops changing (max 15 s).
func (c *onvifCam) waitStill() {
	lastX, lastY := 9.0, 9.0
	for i := 0; i < 30; i++ {
		time.Sleep(500 * time.Millisecond)
		x, y, err := c.position()
		if err != nil {
			continue
		}
		if i >= 1 && x == lastX && y == lastY {
			return
		}
		lastX, lastY = x, y
	}
}

// --- camera points of ONVIF cameras: Preset.Pos keeps the position * pointScale

const pointScale = 10000

func pointOf(x, y float64) [2]int {
	return [2]int{int(x*pointScale + sign(x)*0.5), int(y*pointScale + sign(y)*0.5)}
}

func sign(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}

// --- night mode: the IR cut filter (ON - day picture, OFF - infrared, AUTO)

var irNightValues = map[string]int{"AUTO": 2, "OFF": 1, "ON": 0} // NightModes values

func (c *onvifCam) nightModes() []NightMode {
	var out []NightMode
	for _, m := range NightModes {
		for _, ir := range c.IRModes {
			if v, ok := irNightValues[strings.ToUpper(ir)]; ok && v == m.Value {
				out = append(out, m)
			}
		}
	}
	return out
}

func (c *onvifCam) nightMode() (int, error) {
	b, err := c.request(c.imaging, `<timg:GetImagingSettings `+nsImaging+`><timg:VideoSourceToken>`+c.source+
		`</timg:VideoSourceToken></timg:GetImagingSettings>`)
	if err != nil {
		return 0, err
	}
	if v, ok := irNightValues[strings.ToUpper(strings.TrimSpace(onvif.FindTagValue(b, "IrCutFilter")))]; ok {
		return v, nil
	}
	return 0, errors.New("ONVIF: no IR cut filter mode")
}

func (c *onvifCam) setNightMode(value int) error {
	for ir, v := range irNightValues {
		if v == value {
			_, err := c.request(c.imaging, `<timg:SetImagingSettings `+nsImaging+`><timg:VideoSourceToken>`+c.source+
				`</timg:VideoSourceToken><timg:ImagingSettings><tt:IrCutFilter>`+ir+
				`</tt:IrCutFilter></timg:ImagingSettings><timg:ForcePersistence>true</timg:ForcePersistence></timg:SetImagingSettings>`)
			return err
		}
	}
	return errors.New("the camera has no such night mode")
}
