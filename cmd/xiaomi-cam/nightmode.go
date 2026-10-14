package main

import (
	"errors"
	"strings"
)

// Night vision mode (MIoT camera-control night-shot 2.3): infrared (black and white) or
// full color with the spotlight; automatic modes switch by the light level.

var propNightShot = miotProp{Siid: 2, Piid: 3}

type nightMode struct {
	value int
	id    string // HTTP API / MCP
	name  string // UI, Polish source string
}

// nightModes in the order of the UI list
var nightModes = []nightMode{
	{2, "auto", "Auto – podczerwień"},
	{3, "auto-color", "Auto – w kolorze"},
	{5, "smart-color", "Auto – inteligentny kolor"},
	{1, "on", "Zawsze podczerwień"},
	{4, "color", "Zawsze w kolorze"},
	{0, "off", "Wyłączony"},
}

func nightModeByValue(v int) int {
	for i, m := range nightModes {
		if m.value == v {
			return i
		}
	}
	return -1
}

func nightModeByID(id string) (nightMode, error) {
	for _, m := range nightModes {
		if m.id == id {
			return m, nil
		}
	}
	var ids []string
	for _, m := range nightModes {
		ids = append(ids, m.id)
	}
	return nightMode{}, errors.New("mode must be one of: " + strings.Join(ids, ", "))
}
