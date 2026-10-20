// xc-onvifprobe - developer tool: ask an IP camera of the app's config what it can do over
// ONVIF (profiles, PTZ, presets, imaging / IR cut filter, event topics) and print the answers.
// Usage: xc-onvifprobe [camera name] [raw]  (raw - whole SOAP answers)
package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/onvif"
	"gopkg.in/yaml.v3"
)

func main() {
	dir, _ := os.UserConfigDir()
	b, err := os.ReadFile(filepath.Join(dir, "xiaomi-cam-viewer", "go2rtc.yaml"))
	if err != nil {
		panic(err)
	}
	var cfg struct {
		X struct {
			Cameras map[string]struct{ Main string } `yaml:"cameras"`
		} `yaml:"xiaomi_cam"`
	}
	if err = yaml.Unmarshal(b, &cfg); err != nil {
		panic(err)
	}
	var name string
	if len(os.Args) > 1 {
		name = os.Args[1]
	}
	raw := len(os.Args) > 2 && os.Args[2] == "raw"
	var src string
	for n, c := range cfg.X.Cameras {
		if name == "" || n == name {
			name, src = n, c.Main
			break
		}
	}
	u, err := url.Parse(src)
	if err != nil || src == "" {
		fmt.Println("no IP camera", name)
		os.Exit(1)
	}
	base := "http://" + u.Hostname()
	fmt.Println("camera:", name, base)

	req := func(path, body string) string {
		e := onvif.NewEnvelopeWithUser(u.User)
		e.Append(body)
		c := &http.Client{Timeout: 10 * time.Second}
		res, err := c.Post(base+path, "application/soap+xml;charset=utf-8", bytes.NewReader(e.Bytes()))
		if err != nil {
			return "ERR " + err.Error()
		}
		defer res.Body.Close()
		out, _ := io.ReadAll(res.Body)
		s := string(out)
		if res.StatusCode != http.StatusOK {
			return res.Status + " " + s
		}
		return s
	}
	show := func(title, s string, tags ...string) {
		fmt.Println("==", title)
		if raw {
			fmt.Println(s)
			return
		}
		if strings.HasPrefix(s, "ERR") || !strings.Contains(s, "Envelope") || strings.Contains(s, "Fault") {
			fmt.Println(s[:min(len(s), 600)])
			return
		}
		for _, t := range tags {
			re := regexp.MustCompile(`<(?:\w+:)?` + t + `\b([^>]*)>([^<]*)`)
			for _, m := range re.FindAllStringSubmatch(s, 40) {
				fmt.Printf("  %s%s = %s\n", t, m[1], m[2])
			}
		}
	}

	const ptz = `xmlns:tptz="http://www.onvif.org/ver20/ptz/wsdl"`
	const img = `xmlns:timg="http://www.onvif.org/ver20/imaging/wsdl"`
	const tev = `xmlns:tev="http://www.onvif.org/ver10/events/wsdl"`

	show("device info", req("/onvif/device_service", `<tds:GetDeviceInformation/>`),
		"Manufacturer", "Model", "FirmwareVersion")
	profiles := req("/onvif/media_service", `<trt:GetProfiles/>`)
	show("profiles", profiles, "Profiles", "Name", "SourceToken", "PTZConfiguration", "NodeToken", "Width", "Height", "Encoding")
	profile := regexp.MustCompile(`Profiles[^>]*token="([^"]+)`).FindStringSubmatch(profiles)
	source := regexp.MustCompile(`<(?:\w+:)?SourceToken>([^<]+)`).FindStringSubmatch(profiles)
	if profile == nil || source == nil {
		fmt.Println("no profile / video source")
		return
	}
	pt, st := profile[1], source[1]
	fmt.Println("profile:", pt, "video source:", st)

	show("ptz nodes", req("/onvif/ptz_service", `<tptz:GetNodes `+ptz+`/>`),
		"Name", "MaximumNumberOfPresets", "HomeSupported", "URI", "Min", "Max")
	show("ptz status", req("/onvif/ptz_service", `<tptz:GetStatus `+ptz+`><tptz:ProfileToken>`+pt+`</tptz:ProfileToken></tptz:GetStatus>`),
		"PanTilt", "Zoom", "MoveStatus", "UtcTime")
	show("ptz presets", req("/onvif/ptz_service", `<tptz:GetPresets `+ptz+`><tptz:ProfileToken>`+pt+`</tptz:ProfileToken></tptz:GetPresets>`),
		"Preset", "Name")
	show("imaging settings", req("/onvif/imaging_service", `<timg:GetImagingSettings `+img+`><timg:VideoSourceToken>`+st+`</timg:VideoSourceToken></timg:GetImagingSettings>`),
		"IrCutFilter", "Brightness", "Mode")
	show("imaging options", req("/onvif/imaging_service", `<timg:GetOptions `+img+`><timg:VideoSourceToken>`+st+`</timg:VideoSourceToken></timg:GetOptions>`),
		"IrCutFilterModes", "Mode")
	ev := req("/onvif/event_service", `<tev:GetEventProperties `+tev+`/>`)
	fmt.Println("== event topics")
	if raw {
		fmt.Println(ev)
	} else {
		for _, m := range regexp.MustCompile(`<(\w+:)?(\w+) wstop:topic="true"`).FindAllStringSubmatch(ev, 80) {
			fmt.Println("  ", m[2])
		}
		fmt.Println(ev[:min(len(ev), 300)])
	}
}
