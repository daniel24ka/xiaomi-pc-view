package main

import (
	"sync"

	"fyne.io/fyne/v2"
)

// Camera capabilities: controls of features the selected camera doesn't have are hidden.
// Detected by reading the MIoT properties of the features (code 0 - the camera has it,
// -704040003 - no such property); zoom of isa.camera.700sa works over P2P (cmd 530), so
// known zoom models are listed.

type camCaps struct {
	PTZ, Zoom, Light, Siren, Points, History, NightMode bool
}

var allCaps = camCaps{true, true, true, true, true, true, true}

var zoomModels = map[string]bool{
	"isa.camera.700sa":  true, // Outdoor Camera CW700S (CW400 Pro OverSea)
	"isa.camera.hlzoom": true, // CW400 Pro
}

var (
	propSDStatus = miotProp{Siid: 4, Piid: 1}
	capsCache    sync.Map // camera name -> camCaps
)

// cameraCaps returns (cached) capabilities of a camera.
func cameraCaps(name string) camCaps {
	if v, ok := capsCache.Load(name); ok {
		return v.(camCaps)
	}
	var c camCaps
	src := cameraSource(name)
	c.Zoom = zoomModels[sourceParam(src, "model")]
	props, err := getProps(name, propLight, propSiren, propMotorLocation, propPresets, propSDStatus, propZoom, propNightShot)
	if err != nil {
		if src == "" {
			return c // not a Xiaomi camera: no controls
		}
		return allCaps // the cloud didn't answer: don't hide anything, try again next time
	}
	for _, p := range props {
		ok := p.Code == 0
		switch {
		case p.Siid == propLight.Siid && p.Piid == propLight.Piid:
			c.Light = ok
		case p.Siid == propSiren.Siid && p.Piid == propSiren.Piid:
			c.Siren = ok
		case p.Siid == propMotorLocation.Siid && p.Piid == propMotorLocation.Piid:
			c.PTZ = ok
		case p.Siid == propPresets.Siid && p.Piid == propPresets.Piid:
			c.Points = ok
		case p.Siid == propSDStatus.Siid && p.Piid == propSDStatus.Piid:
			c.History = ok
		case p.Siid == propZoom.Siid && p.Piid == propZoom.Piid:
			c.Zoom = c.Zoom || ok
		case p.Siid == propNightShot.Siid && p.Piid == propNightShot.Piid:
			c.NightMode = ok
		}
	}
	capsCache.Store(name, c)
	return c
}

// updateCaps detects the capabilities of the selected camera (in the background) and
// shows/hides its controls.
func (u *ui) updateCaps() {
	name := u.camera.Selected
	go func() {
		c := allCaps
		if name != "" {
			c = cameraCaps(name)
		}
		fyne.Do(func() {
			if name == u.camera.Selected {
				u.caps = c
				u.applyCaps()
			}
		})
	}()
}

func (u *ui) applyCaps() {
	show := func(on bool, objs ...fyne.CanvasObject) {
		for _, o := range objs {
			if o == nil {
				continue
			}
			if on {
				o.Show()
			} else {
				o.Hide()
			}
		}
	}
	c := u.caps
	show(c.PTZ, u.capPTZ, u.capOverlayPTZ)
	show(c.Points, u.capPoints)
	show(c.Zoom, u.capZoom, u.capOverlayZoom)
	show(c.Light, u.capLight)
	show(c.Siren, u.siren)
	show(c.NightMode, u.capNight)
	show(c.History, u.hist.button)
}
