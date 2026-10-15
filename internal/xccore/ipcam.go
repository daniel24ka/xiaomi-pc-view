package xccore

import (
	"fmt"
	"sync"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
)

// IP cameras (not Xiaomi): any go2rtc source - rtsp://, http(s):// (MJPEG), onvif://,
// tapo://, dvrip://. The go2rtc stream has the source of the selected quality (main or
// sub stream); the app keeps both, and the picture size, in the "xiaomi_cam" section of
// go2rtc.yaml.

type IPCamera struct {
	Kind    string `yaml:"kind" json:"kind"` // ipKinds[].id of the desktop app
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

// Active returns the source of the selected quality.
func (c *IPCamera) Active() string {
	if c.Quality == 1 && c.Sub != "" {
		return c.Sub
	}
	return c.Main
}

var ipCams = struct {
	sync.Mutex
	m map[string]*IPCamera
}{m: map[string]*IPCamera{}}

// LoadIPCameras reads the IP cameras from the config (after app.Init).
func LoadIPCameras() {
	var cfg struct {
		X struct {
			Cameras map[string]*IPCamera `yaml:"cameras"`
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

// IPCam returns a copy of the IP camera settings (nil: no such IP camera).
func IPCam(name string) *IPCamera {
	ipCams.Lock()
	defer ipCams.Unlock()
	if c := ipCams.m[name]; c != nil {
		cc := *c
		return &cc
	}
	return nil
}

func SaveIPCamera(name string, c *IPCamera) error {
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

// AddIPCamera adds the camera (go2rtc stream with the selected source) and saves it.
func AddIPCamera(name string, c *IPCamera) error {
	if err := AddCamera(name, c.Active()); err != nil {
		return err
	}
	return SaveIPCamera(name, c)
}

// UpdateIPCamera saves the edited camera (old name: the camera before the edit). The caller
// stops its video first.
func UpdateIPCamera(old, name string, c *IPCamera) error {
	if prev := IPCam(old); prev != nil && c.Sub != "" {
		c.Quality = prev.Quality // keep main / sub
	}
	if old != name {
		if err := AddIPCamera(name, c); err != nil {
			return err
		}
		return RemoveCamera(old)
	}
	src := c.Active()
	streams.Get(name).SetSource(src)
	if err := app.PatchConfig([]string{"streams", name}, []string{src}); err != nil {
		return err
	}
	return SaveIPCamera(name, c)
}

// SetIPQuality switches the camera to its main (0) or sub (1) stream. The caller stops
// the video first.
func SetIPQuality(name string, q int) error {
	c := IPCam(name)
	if c == nil {
		return fmt.Errorf("no camera %q", name)
	}
	c.Quality = q
	src := c.Active()
	streams.Get(name).SetSource(src)
	if err := app.PatchConfig([]string{"streams", name}, []string{src}); err != nil {
		return err
	}
	return SaveIPCamera(name, c)
}

func CameraExists(name string) bool { return name != "" && streams.Get(name) != nil }

// StreamSource returns the (first) source of the camera stream.
func StreamSource(name string) string {
	if s := streams.Get(name); s != nil {
		if srcs := s.Sources(); len(srcs) > 0 {
			return srcs[0]
		}
	}
	return ""
}
