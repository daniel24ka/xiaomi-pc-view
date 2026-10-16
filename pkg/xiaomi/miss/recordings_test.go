package miss

import (
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
