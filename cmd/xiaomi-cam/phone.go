package main

import (
	_ "embed"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"github.com/AlexxIT/go2rtc/internal/xccore"
	"github.com/AlexxIT/go2rtc/www"
)

// The phone page: live view and controls of the cameras for a phone browser. It has its own
// listener on localhost, meant to be published to the user's own devices only (e.g.
// `tailscale serve --bg 11985`: HTTPS inside the tailnet, no open router ports). An
// allowlist stands in front of the local go2rtc API, which must never be reachable from
// outside as a whole: its config holds the Mi account token, and api/ws or api/stream.m3u8
// with an unknown src would create a stream from any source (exec: included). So only the
// cameras' video and a few controls go through. It is off until switched on in the settings,
// and it answers only its own page (phoneAllowed): a web page from elsewhere in a browser on
// this computer can't open the cameras through it.

//go:embed phone.html
var phoneHTML []byte

const (
	PhoneAddr     = "127.0.0.1:11985"
	prefPhonePage = "phone_page" // the phone page is on (off by default)
)

// phoneAPI - the app API calls the page may make (path → methods)
var phoneAPI = map[string]string{
	"api/xiaomi-cam/cameras":     "GET",
	"api/xiaomi-cam/state":       "GET",
	"api/xiaomi-cam/snapshot":    "GET",
	"api/xiaomi-cam/move":        "POST",
	"api/xiaomi-cam/night-mode":  "POST",
	"api/xiaomi-cam/onair":       "GET POST",
	"api/xiaomi-cam/points":      "GET",
	"api/xiaomi-cam/points/goto": "POST",
	// video: MSE over WebSocket, HLS for iPhones without MSE (src checked below)
	"api/ws":                "GET",
	"api/stream.m3u8":       "GET",
	"api/hls/playlist.m3u8": "GET",
	"api/hls/segment.ts":    "GET",
	"api/hls/init.mp4":      "GET",
	"api/hls/segment.m4s":   "GET",
}

var phoneServer struct {
	mu  sync.Mutex
	srv *http.Server
}

// setPhonePage starts or stops the phone page.
func setPhonePage(on bool) error {
	phoneServer.mu.Lock()
	defer phoneServer.mu.Unlock()
	if !on {
		if phoneServer.srv != nil {
			_ = phoneServer.srv.Close()
			phoneServer.srv = nil
		}
		return nil
	}
	if phoneServer.srv != nil {
		return nil
	}
	ln, err := net.Listen("tcp", PhoneAddr)
	if err != nil {
		return err
	}
	phoneServer.srv = &http.Server{Handler: phoneHandler()}
	go func(srv *http.Server) { _ = srv.Serve(ln) }(phoneServer.srv)
	return nil
}

func phoneHandler() http.Handler {
	target := &url.URL{Scheme: "http", Host: xccore.APIAddr}
	proxy := httputil.NewSingleHostReverseProxy(target)
	direct := proxy.Director
	proxy.Director = func(r *http.Request) {
		direct(r)
		r.Host = target.Host
		r.Header.Del("Origin") // checked by phoneAllowed; go2rtc's WebSocket compares it with its own address
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(phoneHTML)
	})
	for _, f := range []string{"video-rtc.js", "video-stream.js"} {
		f := f
		mux.HandleFunc("/"+f, func(w http.ResponseWriter, r *http.Request) {
			b, err := www.Static.ReadFile(f)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write(b)
		})
	}
	mux.HandleFunc("/phone/info", phoneInfo)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		methods, ok := phoneAPI[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok || !strings.Contains(methods, r.Method) {
			http.Error(w, "not available on the phone page", http.StatusForbidden)
			return
		}
		if src := r.URL.Query().Get("src"); src != "" || r.URL.Path == "/api/ws" || r.URL.Path == "/api/stream.m3u8" {
			if !phoneStream(src) {
				http.Error(w, "no such camera", http.StatusForbidden)
				return
			}
		}
		proxy.ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !phoneAllowed(r) {
			http.Error(w, "only the phone page itself", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// phoneAllowed: the request comes from the phone page itself (same origin), opened at this
// computer's address or its Tailscale name (`tailscale serve` keeps the Host). Another name
// is a DNS rebinding page; another origin or Sec-Fetch-Site is a page of another site
// (also another port of localhost) that would use the cameras. Requests without the browser
// headers come from programs, which can reach the local API anyway.
func phoneAllowed(r *http.Request) bool {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	switch {
	case host == "127.0.0.1", host == "localhost", host == "::1", strings.HasSuffix(host, ".ts.net"):
	default:
		return false
	}
	if o := r.Header.Get("Origin"); o != "" {
		u, err := url.Parse(o)
		if err != nil || !strings.EqualFold(u.Host, r.Host) {
			return false
		}
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none": // none: opened from the address bar or a bookmark
		return true
	}
	return false
}

// phoneStream: only the app's cameras and the on-air stream (never a source URL)
func phoneStream(src string) bool {
	if src == xccore.OnAirStream {
		return true
	}
	for _, n := range cameraNames() {
		if n == src {
			return true
		}
	}
	return false
}

// phoneInfo - the cameras with their controls and texts for the page
func phoneInfo(w http.ResponseWriter, r *http.Request) {
	type mode struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	type camera struct {
		Name       string `json:"name"`
		PTZ        bool   `json:"ptz"`
		Points     bool   `json:"points"`
		NightModes []mode `json:"night_modes"`
	}
	cams := []camera{}
	for _, n := range cameraNames() {
		c := cameraCaps(n)
		cam := camera{Name: n, PTZ: c.PTZ, Points: c.Points}
		if c.NightMode {
			for _, m := range cameraNightModes(n) {
				cam.NightModes = append(cam.NightModes, mode{m.ID, TN(m.Name)})
			}
		}
		cams = append(cams, cam)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"cameras": cams, "onair": xccore.OnAirCamera(), "lang": lang,
	})
}
