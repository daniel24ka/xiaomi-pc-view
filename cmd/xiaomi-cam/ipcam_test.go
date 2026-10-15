package main

import "testing"

func TestIPFormSources(t *testing.T) {
	tests := []struct {
		f         ipForm
		main, sub string
	}{
		{ipForm{Kind: "rtsp", Host: "192.168.1.50", User: "admin", Pass: "p@ss", MainPath: "/Streaming/Channels/101", SubPath: "/Streaming/Channels/102"},
			"rtsp://admin:p%40ss@192.168.1.50/Streaming/Channels/101", "rtsp://admin:p%40ss@192.168.1.50/Streaming/Channels/102"},
		{ipForm{Kind: "rtsp", Host: "192.168.1.50", Port: 8554, MainPath: "stream1"},
			"rtsp://192.168.1.50:8554/stream1", ""},
		{ipForm{Kind: "rtsp", Host: "rtsp://10.0.0.2:554/cam/realmonitor?channel=1&subtype=0", User: "u", Pass: "p", SubPath: "/cam/realmonitor?channel=1&subtype=1"},
			"rtsp://u:p@10.0.0.2:554/cam/realmonitor?channel=1&subtype=0", "rtsp://u:p@10.0.0.2:554/cam/realmonitor?channel=1&subtype=1"},
		{ipForm{Kind: "rtsp", Host: "rtsp://x:y@10.0.0.2/live", User: "u", Pass: "p"},
			"rtsp://x:y@10.0.0.2/live", ""},
		{ipForm{Kind: "http", Host: "10.0.0.3", Port: 8080, MainPath: "/video.mjpg"},
			"http://10.0.0.3:8080/video.mjpg", ""},
		{ipForm{Kind: "tapo", Host: "10.0.0.4", Pass: "cloudpass"},
			"tapo://cloudpass@10.0.0.4", "tapo://cloudpass@10.0.0.4?subtype=1"},
		{ipForm{Kind: "dvrip", Host: "10.0.0.5", User: "admin", Channel: 2},
			"dvrip://admin@10.0.0.5?channel=2&subtype=0", "dvrip://admin@10.0.0.5?channel=2&subtype=1"},
	}
	for _, tt := range tests {
		main, sub, err := tt.f.sources()
		if err != nil || main != tt.main || sub != tt.sub {
			t.Errorf("%+v:\n got %q %q %v\nwant %q %q", tt.f, main, sub, err, tt.main, tt.sub)
		}
	}
	if _, _, err := (&ipForm{Kind: "rtsp"}).sources(); err == nil {
		t.Error("no address: want an error")
	}
}
