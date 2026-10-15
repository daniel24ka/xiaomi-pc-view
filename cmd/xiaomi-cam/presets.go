package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Preset positions ("often viewed positions" in Mi Home), stored in the camera, so they are
// shared with the Mi Home app:
//   6.12 - current motor position {"x":637,"y":63,"scale":10}
//   9.1  - list [{"location":1,"idx":1,"pos":[x,y],"scale":10,"name":"<base64>"}], up to 6
//   9.2  - write idx: the camera turns to that position

var (
	propMotorLocation = miotProp{Siid: 6, Piid: 12}
	propPresets       = miotProp{Siid: 9, Piid: 1}
	propPresetActive  = miotProp{Siid: 9, Piid: 2}
)

const maxPresets = 6

type preset struct {
	Location int    `json:"location"`
	Idx      int    `json:"idx"`
	Pos      [2]int `json:"pos"`
	Scale    int    `json:"scale,omitempty"`
	Name     string `json:"name"` // base64 in the camera
}

// Label - decoded name
func (p preset) Label() string {
	if b, err := base64.StdEncoding.DecodeString(p.Name); err == nil && len(b) > 0 {
		return string(b)
	}
	return fmt.Sprintf("%d", p.Idx)
}

func getPresets(camera string) ([]preset, error) {
	props, err := getProps(camera, propPresets)
	if err != nil {
		return nil, err
	}
	if len(props) == 0 {
		return nil, errors.New("no answer from the camera")
	}
	s, _ := props[0].Value.(string)
	var list []preset
	if strings.TrimSpace(s) != "" {
		if err = json.Unmarshal([]byte(s), &list); err != nil {
			return nil, fmt.Errorf("presets: %w", err)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Location < list[j].Location })
	return list, nil
}

func putPresets(camera string, list []preset) error {
	for i := range list {
		list[i].Location = i + 1
	}
	b, _ := json.Marshal(list)
	if len(list) == 0 {
		b = []byte("[]")
	}
	return setProp(camera, propPresets, string(b))
}

func deletePreset(camera string, idx int) error {
	list, err := getPresets(camera)
	if err != nil {
		return err
	}
	out := list[:0]
	for _, p := range list {
		if p.Idx != idx {
			out = append(out, p)
		}
	}
	return putPresets(camera, out)
}

// gotoPreset turns the camera to a stored position.
func gotoPreset(camera string, idx int) error {
	return setProp(camera, propPresetActive, idx)
}

// pointReached: the camera turned to the point p, and zoomed to its zoom (scale = zoom*10):
// the zoom shown in the app follows.
func pointReached(camera string, p preset) {
	if p.Scale >= 10 {
		setZoomState(camera, float64(p.Scale)/10)
	}
}

// gotoPresetAndWait turns to a position and waits until the camera stops (the motor
// position doesn't change), max 15 s.
func gotoPresetAndWait(camera string, idx int) error {
	if err := gotoPreset(camera, idx); err != nil {
		return err
	}
	var last string
	for i := 0; i < 15; i++ {
		time.Sleep(time.Second)
		props, err := getProps(camera, propMotorLocation)
		if err != nil || len(props) == 0 {
			continue
		}
		s, _ := props[0].Value.(string)
		if s == last && i >= 1 {
			return nil
		}
		last = s
	}
	return nil
}

// UI, API and MCP use 3 points; point n is the preset with idx n.
const presetSlots = 3

// savePresetSlot stores the current position as point slot (1-3) with a name.
func savePresetSlot(camera string, slot int, name string) error {
	if slot < 1 || slot > presetSlots {
		return fmt.Errorf("point must be 1-%d", presetSlots)
	}
	props, err := getProps(camera, propMotorLocation)
	if err != nil {
		return err
	}
	if len(props) == 0 {
		return errors.New("no position from the camera")
	}
	var loc struct{ X, Y, Scale int }
	s, _ := props[0].Value.(string)
	if err = json.Unmarshal([]byte(s), &loc); err != nil {
		return fmt.Errorf("position: %w (%s)", err, s)
	}
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("%d", slot)
	}
	list, err := getPresets(camera)
	if err != nil {
		return err
	}
	p := preset{Idx: slot, Pos: [2]int{loc.X, loc.Y}, Scale: loc.Scale, Name: base64.StdEncoding.EncodeToString([]byte(name))}
	out := []preset{}
	for _, q := range list {
		if q.Idx != slot {
			out = append(out, q)
		}
	}
	if len(out) >= maxPresets {
		return fmt.Errorf("the camera keeps up to %d positions", maxPresets)
	}
	out = append(out, p)
	sort.Slice(out, func(i, j int) bool { return out[i].Idx < out[j].Idx })
	return putPresets(camera, out)
}

// renamePresetSlot changes the name of point slot (1-3); its position stays.
func renamePresetSlot(camera string, slot int, name string) error {
	list, err := getPresets(camera)
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("%d", slot)
	}
	for i := range list {
		if list[i].Idx == slot {
			list[i].Name = base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(name)))
			return putPresets(camera, list)
		}
	}
	return fmt.Errorf("no point %d", slot)
}

// presetSlotsOf returns points 1-3 (nil - not defined).
func presetSlotsOf(list []preset) [presetSlots]*preset {
	var slots [presetSlots]*preset
	for i := range list {
		if n := list[i].Idx; n >= 1 && n <= presetSlots {
			slots[n-1] = &list[i]
		}
	}
	return slots
}

// findPoint - point by number 1-3 or by name
func findPoint(camera, key string) (preset, error) {
	list, err := getPresets(camera)
	if err != nil {
		return preset{}, err
	}
	key = strings.TrimSpace(key)
	slots := presetSlotsOf(list)
	var n int
	if _, err = fmt.Sscan(key, &n); err == nil && n >= 1 && n <= presetSlots && slots[n-1] != nil {
		return *slots[n-1], nil
	}
	for _, p := range slots {
		if p != nil && strings.EqualFold(p.Label(), key) {
			return *p, nil
		}
	}
	return preset{}, fmt.Errorf("no point %q", key)
}
