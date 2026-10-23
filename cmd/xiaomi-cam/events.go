package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/xccore"
)

// Camera events (detections: motion, person, car, a person near a car, the virtual fence).
// The camera writes the event type of its SD card recordings (RDT command 11, xccore); the
// app keeps the events it read (per camera, ~/.cache/xiaomi-cam/events.json, 31 days) for
// the history timeline and the events list, and the watcher checks the cameras every few
// minutes and runs the actions chosen in Settings → Events. Without a Xiaomi cloud
// subscription the cloud has no alerts, so this works without one.

type eventKind = xccore.EventKind

// eventNames - the kinds in the UI
var eventNames = map[eventKind]string{
	xccore.EventMotion:   "Ruch",
	xccore.EventPerson:   "Człowiek",
	xccore.EventCar:      "Samochód",
	xccore.EventNearCar:  "Osoba przy samochodzie",
	xccore.EventFenceIn:  "Wejście w strefę",
	xccore.EventFenceOut: "Wyjście ze strefy",
	xccore.EventFace:     "Twarz",
	xccore.EventPet:      "Zwierzę",
	xccore.EventBabyCry:  "Płacz dziecka",
	xccore.EventAI:       "Zdarzenie AI",
	eventSiren:           "Syrena w okolicy",
	eventSky:             "Wykrycie na niebie",
	eventPinned:          "Zachowane ręcznie",
}

func eventName(k eventKind) string {
	if n, ok := eventNames[k]; ok {
		return T(n)
	}
	return string(k)
}

// eventColor - the colors of Mi Home
func eventColor(k eventKind) color.NRGBA {
	switch k {
	case xccore.EventPerson:
		return hex(0x44CECA)
	case xccore.EventCar:
		return hex(0x4195EA)
	case xccore.EventNearCar:
		return hex(0xFFA626)
	case xccore.EventFenceIn, xccore.EventFenceOut:
		return hex(0x6E82FD)
	case xccore.EventFace:
		return hex(0x2DB0FF)
	case xccore.EventPet:
		return hex(0x75DF6E)
	case xccore.EventBabyCry:
		return hex(0x9B91FF)
	case eventSiren:
		return hex(0xEF4444)
	case eventSky:
		return hex(0xFF7A1A)
	case eventPinned:
		return hex(0x60A5FA)
	}
	return hex(0xFDC541) // motion
}

// --- the store

type storedEvent struct {
	T int64     `json:"t"`
	K eventKind `json:"k"`
}

type camEvents struct {
	Events []storedEvent   `json:"events"` // sorted by time
	Days   map[string]bool `json:"days"`   // days read after they ended (complete)
	Seen   int64           `json:"seen"`   // the newest event the watcher handled
}

type eventStore struct {
	mu   sync.Mutex
	dids map[string]*camEvents
}

var evStore = &eventStore{dids: map[string]*camEvents{}}

func (s *eventStore) camera(did string) *camEvents {
	c := s.dids[did]
	if c == nil {
		c = &camEvents{Days: map[string]bool{}}
		s.dids[did] = c
	}
	return c
}

// add puts the events of a day of a camera (all of them: the day is replaced).
func (s *eventStore) add(did string, day time.Time, list []xccore.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.camera(did)
	from := dayStart(day).Unix()
	to := dayStart(day).AddDate(0, 0, 1).Unix()
	keep := c.Events[:0:0]
	for _, e := range c.Events {
		if e.T < from || e.T >= to {
			keep = append(keep, e)
		}
	}
	for _, e := range list {
		keep = append(keep, storedEvent{e.Time.Unix(), e.Kind})
	}
	sort.Slice(keep, func(i, j int) bool { return keep[i].T < keep[j].T })
	c.Events = keep
	if dayStart(time.Now()).After(day) {
		c.Days[day.Format("20060102")] = true
	}
}

// merge adds the events that are not in the store yet.
func (s *eventStore) merge(did string, list []xccore.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.camera(did)
	have := map[storedEvent]bool{}
	for _, e := range c.Events {
		have[e] = true
	}
	for _, e := range list {
		if se := (storedEvent{e.Time.Unix(), e.Kind}); !have[se] {
			c.Events = append(c.Events, se)
		}
	}
	sort.Slice(c.Events, func(i, j int) bool { return c.Events[i].T < c.Events[j].T })
}

// between - the events of a camera in [from, to)
func (s *eventStore) between(did string, from, to time.Time) []xccore.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.dids[did]
	if c == nil {
		return nil
	}
	i := sort.Search(len(c.Events), func(i int) bool { return c.Events[i].T >= from.Unix() })
	var out []xccore.Event
	for ; i < len(c.Events) && c.Events[i].T < to.Unix(); i++ {
		out = append(out, xccore.Event{Time: time.Unix(c.Events[i].T, 0), Kind: c.Events[i].K})
	}
	return out
}

// complete reports whether the events of a past day were read already.
func (s *eventStore) complete(did string, day time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.dids[did]
	return c != nil && c.Days[day.Format("20060102")]
}

// seen returns the newest event handled by the watcher and sets a new one (zero: keep).
func (s *eventStore) seen(did string, t int64) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.camera(did)
	old := c.Seen
	if t > c.Seen {
		c.Seen = t
	}
	return old
}

func eventsFile() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "xiaomi-cam", "events.json")
}

func (s *eventStore) load() {
	b, err := os.ReadFile(eventsFile())
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = json.Unmarshal(b, &s.dids)
	for did, c := range s.dids {
		if c == nil {
			delete(s.dids, did)
		} else if c.Days == nil {
			c.Days = map[string]bool{}
		}
	}
}

// save writes the store, without the events older than 31 days.
func (s *eventStore) save() {
	path := eventsFile()
	if path == "" {
		return
	}
	s.mu.Lock()
	old := time.Now().AddDate(0, 0, -31)
	for _, c := range s.dids {
		i := sort.Search(len(c.Events), func(i int) bool { return c.Events[i].T >= old.Unix() })
		c.Events = c.Events[i:]
		for d := range c.Days {
			if t, err := time.ParseInLocation("20060102", d, time.Local); err == nil && t.Before(old) {
				delete(c.Days, d)
			}
		}
	}
	b, err := json.Marshal(s.dids)
	s.mu.Unlock()
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, b, 0o600)
}

func dayStart(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

// eventKey - the camera in the store: the device id of a Xiaomi camera, "ip:<name>" of an
// IP camera.
func eventKey(name string) string {
	if ipCam(name) != nil {
		return "ip:" + name
	}
	return cameraDid(cameraSource(name))
}

// readDayEvents reads the events of a day from the camera into the store. IP cameras give
// the events received while the app runs, so they are added to the stored ones.
func readDayEvents(name string, day time.Time) ([]xccore.Event, error) {
	list, err := sdEvents(name, day)
	if err != nil {
		return nil, err
	}
	if ipCam(name) != nil {
		evStore.merge(eventKey(name), list)
		return evStore.between(eventKey(name), dayStart(day), dayStart(day).AddDate(0, 0, 1)), nil
	}
	evStore.add(eventKey(name), day, list)
	return list, nil
}

// --- the watcher

// eventsConfig - Settings → Events: checking the cameras and the actions
type eventsConfig struct {
	Watch        bool        `json:"watch"`        // check the cameras in the background
	IntervalMin  int         `json:"interval_min"` // every N minutes (1-30, 0: 2)
	Kinds        []eventKind `json:"kinds"`        // the events with actions (empty: none)
	Notify       bool        `json:"notify"`
	Email        bool        `json:"email"`
	Slack        bool        `json:"slack"`
	Ntfy         bool        `json:"ntfy"`
	Telegram     bool        `json:"telegram"`
	SaveSnapshot bool        `json:"save_snapshot"`
	Record       bool        `json:"record"`
	RecordSec    int         `json:"record_seconds"`
}

func (c eventsConfig) interval() time.Duration {
	if c.IntervalMin <= 0 {
		return 2 * time.Minute
	}
	return time.Duration(min(c.IntervalMin, 30)) * time.Minute
}

func (c eventsConfig) wants(k eventKind) bool {
	for _, w := range c.Kinds {
		if w == k {
			return true
		}
	}
	return false
}

// eventWatch - the state of the watcher (Settings → Events shows it)
type eventWatch struct {
	mu       sync.Mutex
	checked  map[string]time.Time // camera -> last check
	errs     map[string]string    // camera -> last error
	onChange func()               // UI refresh (events list, settings)
}

var watch = &eventWatch{checked: map[string]time.Time{}, errs: map[string]string{}}

// startEventWatcher checks the cameras for new events every few minutes while it is on.
func (u *ui) startEventWatcher() {
	go func() {
		time.Sleep(20 * time.Second) // after the start (the window, the first video)
		for {
			cfg := u.ai.config().Events
			if cfg.Watch {
				for _, name := range cameraNames() {
					if len(cameraEvents(name)) > 0 {
						u.checkEvents(name, cfg)
					}
				}
				evStore.save()
			}
			time.Sleep(cfg.interval())
		}
	}()
}

// checkEvents reads the events of today (and of yesterday after midnight) and runs the
// actions for the new ones; the first check only marks the events as seen.
func (u *ui) checkEvents(name string, cfg eventsConfig) {
	did := eventKey(name)
	now := time.Now()
	seen := evStore.seen(did, 0)
	days := []time.Time{now}
	if seen > 0 && dayStart(time.Unix(seen, 0)).Before(dayStart(now)) {
		days = []time.Time{time.Unix(seen, 0), now}
	}
	var fresh []xccore.Event
	var err error
	for _, day := range days {
		var list []xccore.Event
		if list, err = readDayEvents(name, day); err != nil {
			break
		}
		for _, e := range list {
			if e.Time.Unix() > seen {
				fresh = append(fresh, e)
			}
		}
	}
	watch.mu.Lock()
	watch.checked[name] = now
	if err != nil {
		watch.errs[name] = err.Error()
	} else {
		delete(watch.errs, name)
	}
	onChange := watch.onChange
	watch.mu.Unlock()
	if onChange != nil {
		onChange()
	}
	if err != nil {
		return
	}
	newest := now.Unix()
	if len(fresh) > 0 {
		newest = max(newest, fresh[len(fresh)-1].Time.Unix())
	}
	evStore.seen(did, newest)
	if seen == 0 {
		return // the first check: the events before the app ran are not new
	}
	var wanted []xccore.Event
	for _, e := range fresh {
		if cfg.wants(e.Kind) {
			wanted = append(wanted, e)
		}
	}
	if len(wanted) > 0 {
		u.eventActions(name, cfg, wanted)
	}
}

// eventActions runs the actions of Settings → Events for the new events of a camera.
func (u *ui) eventActions(name string, cfg eventsConfig, list []xccore.Event) {
	var kinds []string
	have := map[eventKind]bool{}
	for _, e := range list {
		if !have[e.Kind] {
			have[e.Kind] = true
			kinds = append(kinds, eventName(e.Kind))
		}
	}
	last := list[len(list)-1].Time
	title := "Xiaomi Cam: " + name
	text := fmt.Sprintf("%s – %s", strings.Join(kinds, ", "), last.Format("15:04:05"))
	if len(list) > 1 {
		text = fmt.Sprintf(T("%s – %d zdarzeń, ostatnie %s"), strings.Join(kinds, ", "), len(list), last.Format("15:04:05"))
	}
	if cfg.Notify && u.ai.actions.notify != nil {
		u.ai.actions.notify(title, text)
	}
	// a picture for the messages (it opens the camera session for a minute)
	var img []byte
	if cfg.Email || cfg.Ntfy || cfg.Telegram {
		if ensureSession(name) == nil {
			img, _ = snapshotJPEG(name, 1280)
		}
	}
	ai := u.ai.config()
	when := last.Format("2006-01-02 15:04:05")
	if cfg.Email {
		_ = sendEmail(ai.Email, title, fmt.Sprintf("%s\n\n%s: %s\n%s: %s\n", text, "Camera", name, "Time", when),
			img, fileName(name, "jpg"))
	}
	if cfg.Slack {
		_ = sendSlack(ai.SlackWebhook, fmt.Sprintf("*%s*\n%s", title, text))
	}
	if cfg.Ntfy {
		_ = sendNtfy(ai.Ntfy, title, text, img, fileName(name, "jpg"))
	}
	if cfg.Telegram {
		_ = sendTelegram(ai.Telegram, title+"\n"+text, img, fileName(name, "jpg"))
	}
	if cfg.SaveSnapshot {
		_, _ = saveSnapshot(name)
	}
	if cfg.Record {
		sec := cfg.RecordSec
		if sec <= 0 {
			sec = 60
		}
		_, _ = startRecording(name, time.Duration(sec)*time.Second)
	}
}

// hasEventCameras reports whether a camera reports events.
func hasEventCameras() bool {
	for _, name := range cameraNames() {
		if len(cameraEvents(name)) > 0 {
			return true
		}
	}
	return false
}

// recentEvents - the newest events of all cameras (the events list)
func recentEvents(limit int) []namedEvent {
	var out []namedEvent
	from := time.Now().AddDate(0, 0, -31)
	for _, name := range cameraNames() {
		if len(cameraEvents(name)) == 0 {
			continue
		}
		for _, e := range evStore.between(eventKey(name), from, time.Now().Add(time.Hour)) {
			out = append(out, namedEvent{name, e})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

type namedEvent struct {
	Camera string
	xccore.Event
}
