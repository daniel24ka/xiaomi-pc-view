package xccore

import (
	"errors"
	"strings"
)

// Night vision mode (MIoT camera-control night-shot 2.3): infrared (black and white) or
// full color with the spotlight; automatic modes switch by the light level.

var PropNightShot = MiotProp{Siid: 2, Piid: 3}

type NightMode struct {
	Value int
	ID    string // HTTP API / MCP
	Name  string // UI, Polish source string
}

// NightModes in the order of the UI list
var NightModes = []NightMode{
	{2, "auto", "Auto – podczerwień"},
	{3, "auto-color", "Auto – w kolorze"},
	{5, "smart-color", "Auto – inteligentny kolor"},
	{1, "on", "Zawsze podczerwień"},
	{4, "color", "Zawsze w kolorze"},
	{0, "off", "Wyłączony"},
}

func NightModeByValue(v int) int {
	for i, m := range NightModes {
		if m.Value == v {
			return i
		}
	}
	return -1
}

func NightModeByID(id string) (NightMode, error) {
	for _, m := range NightModes {
		if m.ID == id {
			return m, nil
		}
	}
	var ids []string
	for _, m := range NightModes {
		ids = append(ids, m.ID)
	}
	return NightMode{}, errors.New("mode must be one of: " + strings.Join(ids, ", "))
}

// CameraNightModes - the night modes of the camera (Xiaomi: all; IP cameras: the night
// vision modes of the Imou cloud or the IR cut filter modes over ONVIF, once detected; nil -
// none).
func CameraNightModes(name string) []NightMode {
	if src := CameraSource(name); src != "" {
		spec := specOf(src)
		var modes []NightMode
		for _, m := range NightModes {
			if spec.hasValue(PropNightShot, m.Value) {
				modes = append(modes, m)
			}
		}
		return modes
	}
	if c := imouCached(name); c != nil && len(c.nightModes()) > 0 {
		return c.nightModes()
	}
	if c := onvifCached(name); c != nil {
		return c.nightModes()
	}
	return nil
}

// GetNightMode returns the night mode value of the camera.
func GetNightMode(name string) (int, error) {
	if CameraSource(name) == "" {
		if c, err := imouOf(name); err == nil && len(c.nightModes()) > 0 {
			return c.nightMode()
		}
		c, err := ipControl(name)
		if err != nil {
			return 0, err
		}
		return c.nightMode()
	}
	props, err := GetProps(name, PropNightShot)
	if err != nil {
		return 0, err
	}
	if len(props) == 0 || props[0].Code != 0 {
		return 0, ErrNotXiaomi
	}
	v, _ := props[0].Value.(float64)
	return int(v), nil
}

// SetNightMode sets the night mode (NightMode.Value).
func SetNightMode(name string, value int) error {
	if CameraSource(name) == "" {
		if c, err := imouOf(name); err == nil && len(c.nightModes()) > 0 {
			return c.setNightMode(value)
		}
		c, err := ipControl(name)
		if err != nil {
			return err
		}
		return c.setNightMode(value)
	}
	return SetProp(name, PropNightShot, value)
}
