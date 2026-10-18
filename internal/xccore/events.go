package xccore

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/internal/xiaomi"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/miss"
)

// Camera events (detections), from the Mi Home plugin of the Hualai cameras: the kinds a
// camera reports, its detection settings (MIoT) and the events of the SD card recordings
// (RDT command 11). The cloud keeps the alerts only with a Xiaomi cloud subscription, so
// the events are read from the camera.

type EventKind string

const (
	EventMotion   EventKind = "ObjectMotion"      // any motion
	EventPerson   EventKind = "PeopleMotion"      // a person
	EventCar      EventKind = "CarDetected"       // a car
	EventNearCar  EventKind = "PersonApproaching" // a person near a car
	EventFenceIn  EventKind = "FenceIn"           // a person entered the virtual fence
	EventFenceOut EventKind = "FenceOut"          // a person left the virtual fence
	EventFace     EventKind = "Face"
	EventPet      EventKind = "Pet"
	EventBabyCry  EventKind = "BabyCry"
	EventAI       EventKind = "AI"
)

// AllEventKinds - every kind, in the order of the UI
var AllEventKinds = []EventKind{EventPerson, EventNearCar, EventCar, EventFenceIn, EventFenceOut,
	EventFace, EventPet, EventBabyCry, EventAI, EventMotion}

// hualaiEvents - the AI cameras of Hualai: motion, person, car, person near a car, fence
var hualaiEvents = []EventKind{EventMotion, EventPerson, EventCar, EventNearCar, EventFenceIn, EventFenceOut}

var eventModels = map[string][]EventKind{
	"isa.camera.700sa":  hualaiEvents, // Outdoor Camera CW700S (CW400 Pro OverSea)
	"isa.camera.hlzoom": hualaiEvents, // CW400 Pro
	"isa.camera.hlc8":   hualaiEvents, // CW400
}

// CameraEvents returns the event kinds the camera reports (nil: no event support known).
func CameraEvents(name string) []EventKind {
	if IPCam(name) != nil {
		if c := onvifCached(name); c != nil && c.Motion {
			return []EventKind{EventMotion} // ONVIF motion alarms
		}
		return nil
	}
	return eventModels[SourceParam(CameraSource(name), "model")]
}

// SDEventKind maps the event code of an SD card recording to a kind ("" - unknown code).
func SDEventKind(model string, code int) EventKind {
	if model == "isa.camera.hlc8" { // CW400: other codes of the car events
		switch code {
		case 12:
			return EventNearCar
		case 14:
			return EventCar
		}
	}
	switch code {
	case miss.SDEventObjectMotion:
		return EventMotion
	case miss.SDEventPeopleMotion:
		return EventPerson
	case miss.SDEventCat, miss.SDEventDog:
		return EventPet
	case miss.SDEventFace, miss.SDEventKnownFace:
		return EventFace
	case miss.SDEventBabyCry:
		return EventBabyCry
	case miss.SDEventAI:
		return EventAI
	case miss.SDEventPersonApproaching:
		return EventNearCar
	case miss.SDEventCarDetected:
		return EventCar
	case miss.SDEventFenceIn:
		return EventFenceIn
	case miss.SDEventFenceOut:
		return EventFenceOut
	}
	return ""
}

// Event - a detection of a camera
type Event struct {
	Time time.Time
	Kind EventKind
}

var (
	rdtMu       sync.Mutex // one short session at a time
	sessionBusy sync.Map   // camera name -> true while a short session is open
)

// SDEvents returns the events of the SD card recordings of a day, over the open session
// of the stream (live video or playback) or, when there is none, over a short session of
// its own (RDT only, no video: ~1-3 s; the camera allows one session, so it fails while
// someone else watches it).
func SDEvents(name string, day time.Time) ([]Event, error) {
	src := CameraSource(name)
	if src == "" {
		if IPCam(name) != nil {
			return onvifDayEvents(name, day), nil // received while the app runs
		}
		return nil, ErrNotXiaomi
	}
	model := SourceParam(src, "model")
	d := day.Format("20060102")
	var list []miss.SDEvent
	var err error
	if hasStreamSession(name) {
		list, err = xiaomi.SDEvents(name, d, 10*time.Second)
	} else {
		list, err = shortSession(name, src, func(c *miss.Client) ([]miss.SDEvent, error) {
			return c.SDEvents(d, 10*time.Second)
		})
	}
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(list))
	for _, e := range list {
		if k := SDEventKind(model, e.Type); k != "" {
			out = append(out, Event{Time: time.Unix(e.Start, 0), Kind: k})
		}
	}
	return out, nil
}

// ShortSessionOpen reports whether a short session of SDEvents is open to the camera (the
// app waits with its video until it ends).
func ShortSessionOpen(name string) bool {
	_, ok := sessionBusy.Load(name)
	return ok
}

func hasStreamSession(name string) bool {
	s := streams.Get(name)
	return s != nil && len(s.Conns()) > 0
}

func shortSession(name, src string, f func(c *miss.Client) ([]miss.SDEvent, error)) ([]miss.SDEvent, error) {
	rdtMu.Lock()
	defer rdtMu.Unlock()
	rawURL, err := xiaomi.MissURL(src)
	if err != nil {
		return nil, err
	}
	// the session parameters of the source (p2p=server, servers) are not in the cloud answer
	if u, err := url.Parse(rawURL); err == nil {
		q := u.Query()
		for _, k := range []string{"p2p", "servers"} {
			if v := SourceParam(src, k); v != "" && q.Get(k) == "" {
				q.Set(k, v)
			}
		}
		u.RawQuery = q.Encode()
		rawURL = u.String()
	}
	sessionBusy.Store(name, true)
	defer sessionBusy.Delete(name)
	c, err := miss.NewClient(rawURL)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	return f(c)
}

// --- detection settings (MIoT properties of the Hualai cameras)

var (
	PropDetection     = MiotProp{Siid: 5, Piid: 1}  // detection (alerts) on
	PropAlarmInterval = MiotProp{Siid: 5, Piid: 2}  // minutes between alerts, 1-30
	PropSensitivity   = MiotProp{Siid: 5, Piid: 3}  // 1 low, 2 middle, 3 high
	PropAIArea        = MiotProp{Siid: 13, Piid: 1} // car detection and virtual fence (JSON)
	PropPersonDetect  = MiotProp{Siid: 13, Piid: 4} // person detection
	PropMotionDetect  = MiotProp{Siid: 13, Piid: 5} // motion detection
	PropAreaOSD       = MiotProp{Siid: 13, Piid: 6} // the camera draws the car area / fence on the picture
	PropLightAlarm    = MiotProp{Siid: 6, Piid: 20} // events turning on the light/siren alarm (JSON)
	PropMotionTrack   = MiotProp{Siid: 2, Piid: 8}  // the camera follows motion
	PropHumanTrack    = MiotProp{Siid: 2, Piid: 10} // the camera follows a person
	PropCruise        = MiotProp{Siid: 10, Piid: 1} // patrol
)

// AIArea - an entry of property 13.1: type 2 car detection (event bits: 1 car, 2 a person
// near a car), type 1 virtual fence (1 entering, 2 leaving). The area is a rectangle in
// 640x360 (0,0 0,0 on new firmware: set by the camera), pos - the camera position.
type AIArea struct {
	ID          int   `json:"id"`
	Type        int   `json:"type"`
	Switch      bool  `json:"switch"`
	Event       int   `json:"event"`
	Pos         []int `json:"pos"`
	LeftTop     []int `json:"left_top"`
	RightBottom []int `json:"right_bottom"`
}

const (
	AreaFence = 1
	AreaCar   = 2
)

// Detection - the detection settings of a camera; Has tells which the camera has.
type Detection struct {
	Has map[MiotProp]bool

	On          bool
	Interval    int
	Sensitivity int
	Person      bool
	Motion      bool
	Car, Fence  AIArea                     // Switch: the detection is on, Event: its events
	LightAlarm  map[EventKind]bool         // events turning on the light/siren alarm
	Tracking    bool                       // follows motion
	HumanTrack  bool                       // follows a person
	Cruise      bool                       // patrol (car/fence detection turns it off)
	AreaOSD     bool                       // the camera draws the areas on the picture
	lightAlarmV map[string]json.RawMessage // other fields of 6.20, kept
}

// lightAlarmKeys - the field names of property 6.20
var lightAlarmKeys = map[EventKind]string{
	EventMotion: "objectmotion", EventPerson: "peoplemotion", EventCar: "cardetected",
	EventNearCar: "personapproaching", EventFenceIn: "fencein", EventFenceOut: "fenceout",
}

// GetDetection reads the detection settings of a camera (one cloud request).
func GetDetection(name string) (*Detection, error) {
	props, err := GetProps(name, PropDetection, PropAlarmInterval, PropSensitivity, PropAIArea,
		PropPersonDetect, PropMotionDetect, PropLightAlarm, PropMotionTrack, PropHumanTrack, PropCruise,
		PropAreaOSD)
	if err != nil {
		return nil, err
	}
	d := &Detection{Has: map[MiotProp]bool{},
		Car:   AIArea{ID: 1, Type: AreaCar, Pos: []int{0, 0}, LeftTop: []int{0, 0}, RightBottom: []int{0, 0}},
		Fence: AIArea{ID: 2, Type: AreaFence, Pos: []int{0, 0}, LeftTop: []int{0, 0}, RightBottom: []int{0, 0}},
	}
	for _, p := range props {
		if p.Code != 0 {
			continue
		}
		key := MiotProp{Siid: p.Siid, Piid: p.Piid}
		d.Has[key] = true
		b, _ := p.Value.(bool)
		n, _ := p.Value.(float64)
		s, _ := p.Value.(string)
		switch key {
		case PropDetection:
			d.On = b
		case PropAlarmInterval:
			d.Interval = int(n)
		case PropSensitivity:
			d.Sensitivity = int(n)
		case PropPersonDetect:
			d.Person = b
		case PropMotionDetect:
			d.Motion = b
		case PropMotionTrack:
			d.Tracking = b
		case PropHumanTrack:
			d.HumanTrack = b
		case PropCruise:
			d.Cruise = b
		case PropAreaOSD:
			d.AreaOSD = b
		case PropAIArea:
			var areas []AIArea
			_ = json.Unmarshal([]byte(s), &areas)
			for _, a := range areas {
				switch a.Type {
				case AreaCar:
					d.Car = a
				case AreaFence:
					d.Fence = a
				}
			}
		case PropLightAlarm:
			_ = json.Unmarshal([]byte(s), &d.lightAlarmV)
			d.LightAlarm = map[EventKind]bool{}
			for k, f := range lightAlarmKeys {
				var on bool
				_ = json.Unmarshal(d.lightAlarmV[f], &on)
				d.LightAlarm[k] = on
			}
		}
	}
	return d, nil
}

// SetArea writes the car detection or virtual fence entry (like the Mi Home plugin: one
// entry, its area and position as read). The camera may answer "processing" (code 1).
func SetArea(name string, a AIArea) error {
	b, _ := json.Marshal([]AIArea{a})
	return setPropProcessing(name, PropAIArea, string(b))
}

// SetLightAlarm sets the events turning on the light/siren alarm.
func SetLightAlarm(name string, d *Detection, on map[EventKind]bool) error {
	v := map[string]any{}
	for k, raw := range d.lightAlarmV {
		v[k] = raw
	}
	for k, f := range lightAlarmKeys {
		v[f] = on[k]
	}
	b, _ := json.Marshal(v)
	if err := setPropProcessing(name, PropLightAlarm, string(b)); err != nil {
		return err
	}
	d.LightAlarm = on
	return nil
}

// setPropProcessing - SetProp that accepts code 1 (the camera applies it and confirms later)
func setPropProcessing(name string, p MiotProp, value any) error {
	err := SetProp(name, p, value)
	if code, ok := err.(CodeError); ok && code == 1 {
		return nil
	}
	return err
}

// EventKindsOn - the kinds the camera detects now with these settings (nil: detection off)
func (d *Detection) EventKindsOn() []EventKind {
	if !d.On {
		return nil
	}
	var out []EventKind
	if d.Motion {
		out = append(out, EventMotion)
	}
	if d.Person {
		out = append(out, EventPerson)
	}
	if d.Car.Switch {
		if d.Car.Event&1 != 0 {
			out = append(out, EventCar)
		}
		if d.Car.Event&2 != 0 {
			out = append(out, EventNearCar)
		}
	}
	if d.Fence.Switch {
		if d.Fence.Event&1 != 0 {
			out = append(out, EventFenceIn)
		}
		if d.Fence.Event&2 != 0 {
			out = append(out, EventFenceOut)
		}
	}
	return out
}

func (d *Detection) String() string {
	return fmt.Sprintf("on=%v kinds=%v", d.On, d.EventKindsOn())
}
