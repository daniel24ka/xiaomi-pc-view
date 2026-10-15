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
