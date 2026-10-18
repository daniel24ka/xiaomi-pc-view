package xccore

import (
	"errors"
	"sync"
)

// Camera capabilities: controls of features the selected camera doesn't have are hidden.
// Detected by reading the MIoT properties of the features (code 0 - the camera has it,
// -704040003 - no such property); zoom of isa.camera.700sa works over P2P (cmd 530), so
// known zoom models are listed.

type Caps struct {
	PTZ, Zoom, Light, Siren, Points, History, NightMode bool
}

var AllCaps = Caps{true, true, true, true, true, true, true}

var zoomModels = map[string]bool{
	"isa.camera.700sa":  true, // Outdoor Camera CW700S (CW400 Pro OverSea)
	"isa.camera.hlzoom": true, // CW400 Pro
}

var CapsCache sync.Map // camera name -> Caps

// CameraCaps returns (cached) capabilities of a camera.
func CameraCaps(name string) Caps {
	if v, ok := CapsCache.Load(name); ok {
		return v.(Caps)
	}
	var c Caps
	src := CameraSource(name)
	if src == "" {
		return ipCaps(name) // an IP camera: the controls it has over ONVIF
	}
	c.Zoom = zoomModels[SourceParam(src, "model")]
	props, err := GetProps(name, PropLight, PropSiren, PropMotorLocation, PropPresets, PropSDStatus, PropZoom, PropNightShot)
	if err != nil {
		return AllCaps // the cloud didn't answer: don't hide anything, try again next time
	}
	for _, p := range props {
		ok := p.Code == 0
		switch {
		case p.Same(PropLight):
			c.Light = ok
		case p.Same(PropSiren):
			c.Siren = ok
		case p.Same(PropMotorLocation):
			c.PTZ = ok
		case p.Same(PropPresets):
			c.Points = ok
		case p.Same(PropSDStatus):
			c.History = ok
		case p.Same(PropZoom):
			c.Zoom = c.Zoom || ok
		case p.Same(PropNightShot):
			c.NightMode = ok
		}
	}
	CapsCache.Store(name, c)
	return c
}

// ipCaps - the controls of an IP camera over ONVIF (none when the camera doesn't answer:
// asked again next time).
func ipCaps(name string) Caps {
	var c Caps
	oc, err := onvifOf(name)
	if err != nil {
		if errors.Is(err, errNoONVIF) {
			CapsCache.Store(name, c)
		}
		return c
	}
	c.PTZ = oc.PTZ
	c.Points = oc.PTZ && oc.Absolute
	c.NightMode = len(oc.nightModes()) > 0
	CapsCache.Store(name, c)
	return c
}
