package main

import (
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/internal/xccore"
)

func TestEventStore(t *testing.T) {
	s := &eventStore{dids: map[string]*camEvents{}}
	yesterday := dayStart(time.Now()).AddDate(0, 0, -1)
	at := func(h int) time.Time { return yesterday.Add(time.Duration(h) * time.Hour) }
	s.add("cam", yesterday, []xccore.Event{{Time: at(10), Kind: xccore.EventPerson}, {Time: at(8), Kind: xccore.EventCar}})
	got := s.between("cam", yesterday, yesterday.AddDate(0, 0, 1))
	if len(got) != 2 || got[0].Kind != xccore.EventCar || got[1].Kind != xccore.EventPerson {
		t.Fatalf("between: %v", got)
	}
	if !s.complete("cam", yesterday) {
		t.Fatal("a past day read is complete")
	}
	// the day is replaced, other days stay
	today := dayStart(time.Now())
	s.add("cam", today, []xccore.Event{{Time: today, Kind: xccore.EventMotion}})
	s.add("cam", yesterday, []xccore.Event{{Time: at(9), Kind: xccore.EventFenceIn}})
	got = s.between("cam", yesterday, today.AddDate(0, 0, 1))
	if len(got) != 2 || got[0].Kind != xccore.EventFenceIn || got[1].Kind != xccore.EventMotion {
		t.Fatalf("after replace: %v", got)
	}
	if s.complete("cam", today) {
		t.Fatal("today is never complete")
	}
	if s.seen("cam", 100) != 0 || s.seen("cam", 50) != 100 || s.seen("cam", 0) != 100 {
		t.Fatal("seen keeps the newest")
	}
}

func TestEventsConfig(t *testing.T) {
	c := eventsConfig{Kinds: []eventKind{xccore.EventPerson}}
	if !c.wants(xccore.EventPerson) || c.wants(xccore.EventMotion) {
		t.Fatal("wants")
	}
	if c.interval() != 2*time.Minute || (eventsConfig{IntervalMin: 99}).interval() != 30*time.Minute {
		t.Fatal("interval limits")
	}
}
