package xccore

import (
	"image"
	"math"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

// The standard test case of the Imou docs (open.imoulife.com/book/http/develop.html).
func TestImouSign(t *testing.T) {
	got := imouSign(1706511734, "f5a1ae2d-c09c-4d39-a744-83a5c2c653c2", "test123456789test123456789")
	if want := "xjhCQBoJ9hRDsCjyDcHjtDNzRZ3ZJezcawsfWeiaoxU="; got != want {
		t.Errorf("sign = %s, want %s", got, want)
	}
}

func TestUniqueName(t *testing.T) {
	if s := uniqueName("Brama", []string{"Brama", "Brama 2"}); s != "Brama 3" {
		t.Errorf("got %q", s)
	}
	if s := uniqueName("Brama", nil); s != "Brama" {
		t.Errorf("got %q", s)
	}
}

func TestImouNightModes(t *testing.T) {
	c := &imouCam{modes: []string{"Intelligent", "FullColor", "Infrared", "Off"}}
	var ids []string
	for _, m := range c.nightModes() {
		ids = append(ids, m.ID)
	}
	if strings.Join(ids, ",") != "auto,auto-color,smart-color,off" {
		t.Errorf("modes %v", ids)
	}
}

// XC_IMOU="<appId>|<appSecret>|<device serial>" go test -run TestImouLive -v ./internal/xccore
// reads the controls of the real camera, switches the night mode and back, and adds,
// renames, turns to and deletes a camera point at the current position (the camera stays).
// XC_IMOULIGHT=1 also turns the white light on for 3 s, XC_IMOUSIREN=1 the siren for 2 s.
func TestImouLive(t *testing.T) {
	parts := strings.Split(os.Getenv("XC_IMOU"), "|")
	if len(parts) != 3 {
		t.Skip("XC_IMOU not set")
	}
	name := "__xc_imou_test"
	ipCams.Lock()
	ipCams.m[name] = &IPCamera{Kind: "rtsp", Main: "rtsp://127.0.0.1/x", ImouApp: parts[0], ImouSecret: parts[1], ImouDevice: parts[2]}
	ipCams.Unlock()
	defer func() {
		ipCams.Lock()
		delete(ipCams.m, name)
		ipCams.Unlock()
		forgetImou(name)
	}()

	c, err := imouOf(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("light %v, siren %v, points %v, night modes %v", c.Light, c.Siren, c.Points, c.modes)

	if c.Siren {
		m, err := c.iotModel()
		if err != nil {
			t.Log("no things data model:", err)
		} else {
			t.Logf("IoT product %s: SirenStart %s (time %s), SirenStop %s", m.product, m.sirenStart, m.sirenTime, m.sirenStop)
		}
	}

	props, err := GetProps(name, PropLight, PropBrightness, PropSiren, PropNightShot)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range props {
		t.Logf("prop %d.%d = %v (code %d)", p.Siid, p.Piid, p.Value, p.Code)
	}

	if modes := c.nightModes(); len(modes) > 1 {
		was, err := GetNightMode(name)
		if err != nil {
			t.Fatal(err)
		}
		other := modes[0].Value
		if other == was {
			other = modes[1].Value
		}
		if err = SetNightMode(name, other); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Second)
		got, err := GetNightMode(name)
		if err2 := SetNightMode(name, was); err2 != nil {
			t.Errorf("night mode not restored to %d: %v", was, err2)
		}
		if err != nil || got != other {
			t.Errorf("night mode %d after setting %d (%v)", got, other, err)
		}
		t.Logf("night mode %d -> %d -> %d", was, got, was)
	}

	if c.Points {
		before, err := c.collections()
		if err != nil {
			t.Fatal(err)
		}
		coll := uniqueName("xc-test", before)
		if err = c.pointCall("setCollection", map[string]any{"name": coll, "needCruise": false}); err != nil {
			t.Fatal(err)
		}
		p := Preset{Idx: 3, Coll: coll}
		renamed, err := c.renamePoint(p, "xc-test-renamed")
		if err != nil {
			t.Error("rename:", err)
		} else {
			p.Coll = renamed
		}
		after, _ := c.collections()
		if !slices.Contains(after, p.Coll) {
			t.Errorf("point %q not in %v", p.Coll, after)
		}
		if err = c.gotoPoint(p); err != nil {
			t.Error("turn:", err)
		}
		if err = c.deletePoint(p); err != nil {
			t.Error("delete:", err)
		}
		final, _ := c.collections()
		if !slices.Equal(final, before) {
			t.Errorf("points %v after the test, were %v", final, before)
		}
		t.Logf("points %v -> %v -> %v", before, after, final)
	}

	// XC_IMOURTSP=<rtsp url of the camera> (+ XC_FFMPEG=<ffmpeg>): saves a point, turns the
	// camera right for 2 s, turns it back to the point and compares the pictures.
	if src := os.Getenv("XC_IMOURTSP"); src != "" && c.Points {
		ffmpeg := os.Getenv("XC_FFMPEG")
		if ffmpeg == "" {
			ffmpeg = "ffmpeg"
		}
		shot := func() *image.Gray {
			out, err := exec.Command(ffmpeg, "-v", "error", "-rtsp_transport", "tcp", "-i", src,
				"-frames:v", "1", "-vf", "scale=64:36,format=gray", "-f", "rawvideo", "-").Output()
			if err != nil || len(out) < 64*36 {
				t.Fatal("snapshot:", err)
			}
			return &image.Gray{Pix: out[:64*36], Stride: 64, Rect: image.Rect(0, 0, 64, 36)}
		}
		diff := func(a, b *image.Gray) float64 {
			var s float64
			for i := range a.Pix {
				s += math.Abs(float64(a.Pix[i]) - float64(b.Pix[i]))
			}
			return s / float64(len(a.Pix))
		}
		taken, _ := c.collections()
		p := Preset{Idx: 3, Coll: uniqueName("xc-move", taken)}
		if err := c.pointCall("setCollection", map[string]any{"name": p.Coll, "needCruise": false}); err != nil {
			t.Fatal(err)
		}
		defer c.deletePoint(p)
		a := shot()
		if err := c.cl.call("controlMovePTZ", c.channel(map[string]any{"operation": "3", "duration": "2000"}), nil); err != nil {
			t.Fatal("move:", err)
		}
		time.Sleep(4 * time.Second)
		b := shot()
		if err := c.gotoPoint(p); err != nil {
			t.Fatal(err)
		}
		time.Sleep(imouTurnTime)
		back := shot()
		t.Logf("picture difference: moved %.1f, back at the point %.1f", diff(a, b), diff(a, back))
		if diff(a, back) >= diff(a, b) {
			t.Error("the camera didn't turn back to the point")
		}
	}

	toggle := func(env string, p MiotProp, d time.Duration) {
		if os.Getenv(env) != "1" {
			return
		}
		if err := SetProp(name, p, true); err != nil {
			t.Error(env, err)
			return
		}
		time.Sleep(d / 2)
		on, _ := GetProps(name, p)
		time.Sleep(d / 2)
		if err := SetProp(name, p, false); err != nil {
			t.Error(env, "off:", err)
		}
		off, _ := GetProps(name, p)
		t.Logf("%s: on %v, off %v", env, on, off)
	}
	toggle("XC_IMOULIGHT", PropLight, 3*time.Second)
	toggle("XC_IMOUSIREN", PropSiren, 2*time.Second)
}
