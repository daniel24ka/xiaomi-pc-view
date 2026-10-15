package miss

import "testing"

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
