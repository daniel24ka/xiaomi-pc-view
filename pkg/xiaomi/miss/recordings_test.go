package miss

import (
	"encoding/binary"
	"fmt"
	"testing"
)

func TestRecordingCandidates(t *testing.T) {
	r := &recordingCache{files: map[string][]recording{}}
	// nothing known: every second back from the target
	p, c := r.candidates("cam", 1000)
	if len(p) != 0 || len(c) != playbackSearch || c[0] != 1000 || c[1] != 999 {
		t.Fatalf("empty cache: %v %v", p, c[:3])
	}
	// a known file: the target inside it -> its start first
	r.add("cam", recording{start: 900, duration: 60})
	if p, _ = r.candidates("cam", 930); p[0] != 900 {
		t.Fatalf("inside a known file: %v", p)
	}
	// the next files follow it: 960, 1020, ...
	if p, _ = r.candidates("cam", 1035); p[0] != 1020 {
		t.Fatalf("after a known file: %v", p)
	}
	// before a known file: 840, 780, ...
	p, c = r.candidates("cam", 800)
	if p[0] != 780 {
		t.Fatalf("before a known file: %v", p)
	}
	// every second of the window is still tried, once
	seen := map[int64]bool{}
	for _, s := range append(p, c...) {
		if seen[s] {
			t.Fatalf("%d twice", s)
		}
		seen[s] = true
	}
	if len(p)+len(c) != playbackSearch {
		t.Fatalf("%d candidates", len(p)+len(c))
	}
}

func TestParseSDFiles(t *testing.T) {
	b := []byte{
		0x10, 0, 0, 0, 0, 1, 0, 0, // start 16, duration 0 = 60, motion
		0x08, 0, 0, 0, 30, 0, 1, 0, // start 8, 30 s, saved
		0x20, 0, 0, 0, 60, 0, 0, 1, // being deleted: skipped
		0, 0, 0, 0, 60, 0, 0, 0, // start 0: skipped
	}
	got := parseSDFiles(b)
	want := []SDFile{{8, 30, false, true}, {16, 60, true, false}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestRecordingsMerge(t *testing.T) {
	r := &recordingCache{files: map[string][]recording{}}
	for _, s := range []int64{100, 500, 900} {
		r.add("cam", recording{s, 60})
	}
	// the SD card list covers 400..700: 500 is gone (overwritten), 100 and 900 stay
	r.merge("cam", []recording{{400, 60}, {460, 60}, {700, 60}})
	var starts []int64
	for _, f := range r.files["cam"] {
		starts = append(starts, f.start)
	}
	if fmt.Sprint(starts) != "[100 400 460 700 900]" {
		t.Fatalf("got %v", starts)
	}
}

func TestParseSDEvents(t *testing.T) {
	// a page as the camera sends it: [?][total][index][32 bytes] + 32 bytes per event
	page := func(total, index uint32, ev ...SDEvent) []byte {
		b := make([]byte, 44+32*len(ev)+32) // an empty record at the end
		binary.LittleEndian.PutUint32(b[4:], total)
		binary.LittleEndian.PutUint32(b[8:], index)
		for i, e := range ev {
			binary.LittleEndian.PutUint64(b[44+32*i:], uint64(e.Start))
			binary.LittleEndian.PutUint32(b[44+32*i+8:], uint32(e.Type))
		}
		return b
	}
	ev, last := parseSDEvents(page(2, 1, SDEvent{1792149617, SDEventPeopleMotion}, SDEvent{1792149500, SDEventCarDetected}))
	if last || len(ev) != 2 || ev[0].Start != 1792149617 || ev[0].Type != SDEventPeopleMotion {
		t.Fatalf("page 1: %v %v", ev, last)
	}
	ev2, last := parseSDEvents(page(2, 2, SDEvent{1792149617, SDEventObjectMotion}))
	if !last || len(ev2) != 1 {
		t.Fatalf("page 2: %v %v", ev2, last)
	}
	// sorted, one per start: the highest code (like Mi Home)
	m := mergeSDEvents(append(ev, ev2...))
	if len(m) != 2 || m[0].Start != 1792149500 || m[1].Type != SDEventPeopleMotion {
		t.Fatalf("merged: %v", m)
	}
	if ev, last := parseSDEvents(nil); ev != nil || !last {
		t.Fatalf("empty answer: %v %v", ev, last)
	}
}
