package xccore

import (
	"net/url"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
)

// On air: one RTSP address for a broadcast app (OBS) that always shows the camera chosen
// here, so switching the broadcast camera never touches OBS. Video only: the camera
// microphone is never on air. It re-streams the camera's own stream, so the app and OBS
// share one camera session.

const OnAirStream = HiddenPrefix + "onair"

// OnAirURL - the address for OBS (Media Source).
func OnAirURL() string {
	return RTSPURL(OnAirStream)
}

func onAirSource(name string) string {
	return RTSPURL(name) + "?video"
}

// OnAirCamera returns the camera on air ("" - none).
func OnAirCamera() string {
	s := streams.Get(OnAirStream)
	if s == nil {
		return ""
	}
	for _, src := range s.Sources() {
		u, err := url.Parse(src)
		if err != nil {
			continue
		}
		name, err := url.PathUnescape(strings.TrimPrefix(u.EscapedPath(), "/"))
		if err == nil && name != "" {
			return name
		}
	}
	return ""
}

// SetOnAir puts a camera on air (saved in go2rtc.yaml). A running OBS connection switches
// to the new camera (the source of the stream is replaced).
func SetOnAir(name string) error {
	src := onAirSource(name)
	if s := streams.Get(OnAirStream); s != nil {
		s.SetSource(src)
	} else if _, err := streams.New(OnAirStream, src); err != nil {
		return err
	}
	return app.PatchConfig([]string{"streams", OnAirStream}, []string{src})
}

// EnsureOnAir puts the first camera on air when nothing (or a removed camera) is.
func EnsureOnAir() {
	cur := OnAirCamera()
	names := CameraNames()
	for _, n := range names {
		if n == cur {
			return
		}
	}
	if len(names) > 0 {
		_ = SetOnAir(names[0])
	}
}

// RenameOnAir keeps the broadcast on a renamed camera.
func RenameOnAir(oldName, newName string) {
	if OnAirCamera() == oldName {
		_ = SetOnAir(newName)
	}
}
