package xccore

import (
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/onvif"
)

// Motion events of ONVIF cameras: while the app runs, a PullPoint subscription per camera
// receives the motion alarms; the start of each alarm is an EventMotion. They are kept in
// memory (the app stores what the event watcher read), so SDEvents returns them like the SD
// card events of Xiaomi cameras.

var onvifEvents = struct {
	sync.Mutex
	running map[string]bool    // camera name -> the subscription loop runs
	list    map[string][]Event // camera name -> events, sorted by time
	motion  map[string]bool    // camera name -> the motion alarm is on
}{running: map[string]bool{}, list: map[string][]Event{}, motion: map[string]bool{}}

const (
	onvifEventGap  = 20 * time.Second // alarms closer than this to the last event are one event
	onvifEventKeep = 31 * 24 * time.Hour
)

var onvifEventsOn atomic.Bool // StartONVIFEvents was called: added cameras subscribe too

// StartONVIFEvents subscribes to the motion events of every IP camera (in the background;
// cameras without ONVIF events are left alone).
func StartONVIFEvents() {
	onvifEventsOn.Store(true)
	ipCams.Lock()
	names := make([]string, 0, len(ipCams.m))
	for name := range ipCams.m {
		names = append(names, name)
	}
	ipCams.Unlock()
	for _, name := range names {
		startONVIFEvents(name)
	}
}

func startONVIFEvents(name string) {
	onvifEvents.Lock()
	defer onvifEvents.Unlock()
	if onvifEvents.running[name] {
		return
	}
	onvifEvents.running[name] = true
	go onvifEventLoop(name)
}

func onvifEventLoop(name string) {
	defer func() {
		onvifEvents.Lock()
		delete(onvifEvents.running, name)
		onvifEvents.Unlock()
	}()
	for IPCam(name) != nil {
		c, err := onvifOf(name)
		switch {
		case errors.Is(err, errNoONVIF):
			return
		case err != nil:
			time.Sleep(time.Minute) // the camera is off or ONVIF is disabled: try later
			continue
		case !c.Motion:
			return
		}
		_ = c.pullEvents(name)
		time.Sleep(10 * time.Second)
	}
}

const (
	onvifPullWait     = 20 * time.Second
	onvifSubscribeFor = "PT120S"
	onvifRenewEvery   = time.Minute

	actionPull        = "http://www.onvif.org/ver10/events/wsdl/PullPointSubscription/PullMessagesRequest"
	actionRenew       = "http://docs.oasis-open.org/wsn/bw-2/SubscriptionManager/RenewRequest"
	actionUnsubscribe = "http://docs.oasis-open.org/wsn/bw-2/SubscriptionManager/UnsubscribeRequest"
)

// pullEvents subscribes and pulls the messages until an error (or the camera is removed).
func (c *onvifCam) pullEvents(name string) error {
	b, err := c.request(c.events, `<tev:CreatePullPointSubscription `+nsEvents+`><tev:InitialTerminationTime>`+
		onvifSubscribeFor+`</tev:InitialTerminationTime></tev:CreatePullPointSubscription>`)
	if err != nil {
		return err
	}
	addr := strings.TrimSpace(onvif.FindTagValue(b, "Address"))
	if addr == "" {
		return errors.New("ONVIF: no subscription address")
	}
	defer func() { // free it: the camera keeps a few subscriptions only
		_, _ = c.requestWith(onvifHTTP, addr, actionUnsubscribe, `<wsnt:Unsubscribe `+nsEvents+`/>`)
	}()
	client := &http.Client{Timeout: onvifPullWait + 15*time.Second}
	renewed := time.Now()
	for IPCam(name) != nil {
		if time.Since(renewed) > onvifRenewEvery {
			if _, err = c.requestWith(onvifHTTP, addr, actionRenew, `<wsnt:Renew `+nsEvents+`><wsnt:TerminationTime>`+
				onvifSubscribeFor+`</wsnt:TerminationTime></wsnt:Renew>`); err != nil {
				return err
			}
			renewed = time.Now()
		}
		b, err = c.requestWith(client, addr, actionPull, `<tev:PullMessages `+nsEvents+`><tev:Timeout>PT20S</tev:Timeout>`+
			`<tev:MessageLimit>20</tev:MessageLimit></tev:PullMessages>`)
		if err != nil {
			return err
		}
		for _, m := range parseMotion(b) {
			addONVIFMotion(name, m)
		}
	}
	return nil
}

type motionMessage struct {
	at          time.Time
	on, initial bool
}

var (
	notifyRe  = regexp.MustCompile(`(?s)<(?:\w+:)?NotificationMessage\b.*?</(?:\w+:)?NotificationMessage>`)
	utcRe     = regexp.MustCompile(`UtcTime="([^"]+)"`)
	stateRe   = regexp.MustCompile(`SimpleItem\s+Name="(?:State|IsMotion|Motion)"\s+Value="(true|false)"`)
	initialRe = regexp.MustCompile(`PropertyOperation="Initialized"`)
)

// parseMotion returns the motion messages of a PullMessages answer.
func parseMotion(b []byte) []motionMessage {
	var out []motionMessage
	for _, msg := range notifyRe.FindAll(b, -1) {
		if !motionTopic.Match([]byte(onvif.FindTagValue(msg, "Topic"))) {
			continue
		}
		s := stateRe.FindSubmatch(msg)
		if s == nil {
			continue
		}
		m := motionMessage{at: time.Now(), on: string(s[1]) == "true", initial: initialRe.Match(msg)}
		if t := utcRe.FindSubmatch(msg); t != nil {
			if at, err := time.Parse(time.RFC3339, string(t[1])); err == nil {
				m.at = at.Local()
			}
		}
		out = append(out, m)
	}
	return out
}

// addONVIFMotion turns the start of a motion alarm into an event.
func addONVIFMotion(name string, m motionMessage) {
	onvifEvents.Lock()
	defer onvifEvents.Unlock()
	was := onvifEvents.motion[name]
	onvifEvents.motion[name] = m.on
	if !m.on || was || m.initial {
		return // the end of an alarm, still the same alarm, or the state at the subscription
	}
	list := onvifEvents.list[name]
	if n := len(list); n > 0 && m.at.Sub(list[n-1].Time) < onvifEventGap {
		return
	}
	from := time.Now().Add(-onvifEventKeep)
	i := sort.Search(len(list), func(i int) bool { return !list[i].Time.Before(from) })
	onvifEvents.list[name] = append(list[i:], Event{Time: m.at, Kind: EventMotion})
}

// onvifDayEvents returns the motion events of a day received while the app runs.
func onvifDayEvents(name string, day time.Time) []Event {
	y, mo, d := day.Date()
	from := time.Date(y, mo, d, 0, 0, 0, 0, time.Local)
	to := from.AddDate(0, 0, 1)
	onvifEvents.Lock()
	defer onvifEvents.Unlock()
	var out []Event
	for _, e := range onvifEvents.list[name] {
		if !e.Time.Before(from) && e.Time.Before(to) {
			out = append(out, e)
		}
	}
	return out
}
