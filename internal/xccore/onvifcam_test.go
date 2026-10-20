package xccore

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"net/http"
	"regexp"

	"github.com/AlexxIT/go2rtc/pkg/onvif"
	"gopkg.in/yaml.v3"
)

func TestParseMotion(t *testing.T) {
	b := []byte(`<tev:PullMessagesResponse><tev:CurrentTime>2026-10-18T05:00:00Z</tev:CurrentTime>
<wsnt:NotificationMessage><wsnt:Topic Dialect="http://www.onvif.org/ver10/tev/topicExpression/ConcreteSet">tns1:VideoSource/MotionAlarm</wsnt:Topic>
<wsnt:Message><tt:Message UtcTime="2026-10-18T05:00:01Z" PropertyOperation="Initialized"><tt:Source><tt:SimpleItem Name="Source" Value="VideoSource000"/></tt:Source>
<tt:Data><tt:SimpleItem Name="State" Value="false"/></tt:Data></tt:Message></wsnt:Message></wsnt:NotificationMessage>
<wsnt:NotificationMessage><wsnt:Topic>tns1:VideoSource/ImagingService</wsnt:Topic><wsnt:Message><tt:Message UtcTime="2026-10-18T05:00:02Z" PropertyOperation="Changed">
<tt:Data><tt:SimpleItem Name="State" Value="true"/></tt:Data></tt:Message></wsnt:Message></wsnt:NotificationMessage>
<wsnt:NotificationMessage><wsnt:Topic>tns1:RuleEngine/CellMotionDetector/Motion</wsnt:Topic><wsnt:Message><tt:Message UtcTime="2026-10-18T05:00:03Z" PropertyOperation="Changed">
<tt:Data><tt:SimpleItem Name="IsMotion" Value="true"/></tt:Data></tt:Message></wsnt:Message></wsnt:NotificationMessage>
</tev:PullMessagesResponse>`)
	got := parseMotion(b)
	if len(got) != 2 {
		t.Fatalf("got %d messages, want 2: %+v", len(got), got)
	}
	if got[0].on || !got[0].initial {
		t.Errorf("first: %+v, want off, initial", got[0])
	}
	if !got[1].on || got[1].initial || got[1].at.UTC().Format(time.RFC3339) != "2026-10-18T05:00:03Z" {
		t.Errorf("second: %+v, want on at 05:00:03Z", got[1])
	}

	name := "test-motion"
	t0 := time.Now()
	addONVIFMotion(name, motionMessage{at: t0, on: true, initial: true}) // state at subscribe
	addONVIFMotion(name, motionMessage{at: t0.Add(time.Second), on: false})
	addONVIFMotion(name, motionMessage{at: t0.Add(2 * time.Second), on: true})
	addONVIFMotion(name, motionMessage{at: t0.Add(3 * time.Second), on: true}) // same alarm
	addONVIFMotion(name, motionMessage{at: t0.Add(4 * time.Second), on: false})
	addONVIFMotion(name, motionMessage{at: t0.Add(5 * time.Second), on: true}) // < gap
	addONVIFMotion(name, motionMessage{at: t0.Add(6 * time.Second), on: false})
	addONVIFMotion(name, motionMessage{at: t0.Add(time.Minute), on: true})
	if n := len(onvifDayEvents(name, t0)); n != 2 {
		t.Errorf("got %d events, want 2", n)
	}
}

func TestPointOf(t *testing.T) {
	for _, v := range [][2]float64{{0.8, -0.8}, {-1, 1}, {0.12345, 0}} {
		p := pointOf(v[0], v[1])
		if math.Abs(float64(p[0])/pointScale-v[0]) > 1e-4 || math.Abs(float64(p[1])/pointScale-v[1]) > 1e-4 {
			t.Errorf("pointOf(%v) = %v", v, p)
		}
	}
}

// XC_ONVIFCAM=<camera name of the app config> go test -run TestONVIFLive -v ./internal/xccore
// turns the real camera a bit right and back, and waits 30 s for motion events.
func TestONVIFLive(t *testing.T) {
	name := os.Getenv("XC_ONVIFCAM")
	if name == "" {
		t.Skip("XC_ONVIFCAM not set")
	}
	dir, _ := os.UserConfigDir()
	b, err := os.ReadFile(filepath.Join(dir, "xiaomi-cam-viewer", "go2rtc.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		X struct {
			Cameras map[string]*IPCamera `yaml:"cameras"`
		} `yaml:"xiaomi_cam"`
	}
	if err = yaml.Unmarshal(b, &cfg); err != nil || cfg.X.Cameras[name] == nil {
		t.Fatal("no camera", name, err)
	}
	ipCams.Lock()
	ipCams.m[name] = cfg.X.Cameras[name]
	ipCams.Unlock()

	c, err := onvifOf(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("ptz=%v absolute=%v motion=%v ir=%v profile=%s source=%s", c.PTZ, c.Absolute, c.Motion, c.IRModes, c.profile, c.source)
	t.Logf("caps: %+v, events: %v", ipCaps(name), CameraEvents(name))

	if op := os.Getenv("XC_ONVIFOP"); op != "" { // only one movement of ~1 s
		n := int(op[0] - '0')
		for i := 0; i < 5; i++ {
			if err = c.move(n); err != nil {
				t.Fatal(err)
			}
			time.Sleep(200 * time.Millisecond)
		}
		if err = c.move(motorStop); err != nil {
			t.Fatal(err)
		}
		return
	}

	if !c.Absolute {
		t.Log("no absolute positioning: skip the move test")
		goto events
	}
	{
		x0, y0, err := c.position()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("start position %.4f %.4f", x0, y0)
		for _, op := range []int{MotorRight, MotorUp} {
			for i := 0; i < 5; i++ { // like the Mover: repeated every 200 ms
				if err = c.move(op); err != nil {
					t.Fatal(err)
				}
				time.Sleep(200 * time.Millisecond)
			}
			if err = c.move(motorStop); err != nil {
				t.Fatal(err)
			}
			c.waitStill()
			x, y, _ := c.position()
			t.Logf("after op %d: %.4f %.4f", op, x, y)
		}
		if err = c.moveTo(x0, y0); err != nil {
			t.Fatal(err)
		}
		c.waitStill()
		x, y, _ := c.position()
		t.Logf("back: %.4f %.4f", x, y)
		if math.Abs(x-x0) > 0.01 || math.Abs(y-y0) > 0.01 {
			t.Errorf("not back at the start position")
		}
	}
events:

	if c.Motion {
		done := make(chan error, 1)
		go func() { done <- c.pullEvents(name) }()
		t.Log("waiting 30 s for motion events (move in front of the camera)")
		if os.Getenv("XC_ONVIFWIGGLE") != "" { // turn the camera there and back: the picture moves
			go func() {
				time.Sleep(5 * time.Second)
				for _, op := range []int{MotorRight, MotorLeft} {
					for i := 0; i < 4; i++ {
						_ = c.move(op)
						time.Sleep(200 * time.Millisecond)
					}
					_ = c.move(motorStop)
					time.Sleep(3 * time.Second)
				}
			}()
		}
		select {
		case err = <-done:
			t.Errorf("pull ended: %v", err)
		case <-time.After(30 * time.Second):
		}
		t.Logf("events: %v, alarm on: %v", onvifDayEvents(name, time.Now()), onvifEvents.motion[name])
	}
}

// XC_ONVIFCAM=<name> XC_ONVIFRAW=1: the raw event messages of the camera for ~80 s
func TestONVIFPullRaw(t *testing.T) {
	name := os.Getenv("XC_ONVIFCAM")
	if name == "" || os.Getenv("XC_ONVIFRAW") == "" {
		t.Skip()
	}
	dir, _ := os.UserConfigDir()
	b, _ := os.ReadFile(filepath.Join(dir, "xiaomi-cam-viewer", "go2rtc.yaml"))
	var cfg struct {
		X struct {
			Cameras map[string]*IPCamera `yaml:"cameras"`
		} `yaml:"xiaomi_cam"`
	}
	_ = yaml.Unmarshal(b, &cfg)
	ipCams.Lock()
	ipCams.m[name] = cfg.X.Cameras[name]
	ipCams.Unlock()
	c, err := onvifOf(name)
	if err != nil {
		t.Fatal(err)
	}
	b, err = c.request(c.events, `<tev:CreatePullPointSubscription `+nsEvents+`><tev:InitialTerminationTime>PT120S</tev:InitialTerminationTime></tev:CreatePullPointSubscription>`)
	if err != nil {
		t.Fatal(err)
	}
	addr := onvif.FindTagValue(b, "Address")
	defer c.requestWith(onvifHTTP, addr, actionUnsubscribe, `<wsnt:Unsubscribe `+nsEvents+`/>`)
	client := &http.Client{Timeout: time.Minute}
	ns := regexp.MustCompile(`\s?xmlns:[\w-]+="[^"]*"`)
	for i := 0; i < 4; i++ {
		b, err = c.requestWith(client, addr, actionPull, `<tev:PullMessages `+nsEvents+`><tev:Timeout>PT20S</tev:Timeout><tev:MessageLimit>20</tev:MessageLimit></tev:PullMessages>`)
		t.Logf("pull %d %v: %s", i, err, ns.ReplaceAll(b, nil))
	}
}
