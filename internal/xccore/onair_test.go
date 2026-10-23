package xccore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

// The broadcast address is made only by SetOnAir and goes away with its camera.
func TestOnAirOnDemand(t *testing.T) {
	app.ConfigPath = filepath.Join(t.TempDir(), "go2rtc.yaml")
	defer func() { app.ConfigPath = "" }()
	config := func() string { b, _ := os.ReadFile(app.ConfigPath); return string(b) }
	if !streams.HasProducer("rtsp://x") { // the cameras and the on-air stream are RTSP (never dialed here)
		streams.HandleFunc("rtsp", func(string) (core.Producer, error) { return nil, errors.New("test") })
	}

	for _, n := range []string{"cam A", "cam B"} {
		if _, err := streams.New(n, "rtsp://127.0.0.1:1/x"); err != nil {
			t.Fatal(err)
		}
		defer streams.Delete(n)
	}
	OffAirIfRemoved() // nothing on air: nothing made
	if streams.Get(OnAirStream) != nil || strings.Contains(config(), OnAirStream) {
		t.Fatalf("on air without a choice: %q", config())
	}

	if err := SetOnAir("cam B"); err != nil {
		t.Fatal(err)
	}
	if OnAirCamera() != "cam B" || !strings.Contains(config(), OnAirStream) {
		t.Fatalf("on air %q, config %q", OnAirCamera(), config())
	}
	OffAirIfRemoved() // its camera is there: stays
	if OnAirCamera() != "cam B" {
		t.Fatalf("on air %q after a check", OnAirCamera())
	}

	streams.Delete("cam B")
	OffAirIfRemoved()
	if streams.Get(OnAirStream) != nil || strings.Contains(config(), OnAirStream) {
		t.Fatalf("a removed camera stays on air: %q", config())
	}
}
