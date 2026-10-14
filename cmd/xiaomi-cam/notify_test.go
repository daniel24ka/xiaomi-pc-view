package main

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSendNtfy(t *testing.T) {
	var got *http.Request
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, body = r, must(io.ReadAll(r.Body))
	}))
	defer srv.Close()

	err := sendNtfy(ntfyConfig{Server: srv.URL, Topic: "topic1", Priority: "urgent", Token: "tk"},
		"Xiaomi Cam: samochód", "Biały Citroën przed bramą", []byte{0xff, 0xd8, 1, 2}, "a.jpg")
	if err != nil {
		t.Fatal(err)
	}
	dec := new(mime.WordDecoder)
	title, _ := dec.DecodeHeader(got.Header.Get("Title"))
	msg, _ := dec.DecodeHeader(got.Header.Get("Message"))
	switch {
	case got.Method != "PUT" || got.URL.Path != "/topic1":
		t.Fatalf("request %s %s", got.Method, got.URL.Path)
	case title != "Xiaomi Cam: samochód" || msg != "Biały Citroën przed bramą":
		t.Fatalf("title %q message %q", title, msg)
	case got.Header.Get("Priority") != "urgent" || got.Header.Get("Filename") != "a.jpg":
		t.Fatalf("headers %v", got.Header)
	case got.Header.Get("Authorization") != "Bearer tk" || len(body) != 4:
		t.Fatalf("auth/body")
	}
}

func TestSendTelegram(t *testing.T) {
	var path, chat, caption string
	var photo []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if r.URL.Path == "/botTOKEN/getUpdates" {
			_, _ = io.WriteString(w, `{"ok":true,"result":[{"message":{"chat":{"id":111,"first_name":"A"}}},{"message":{"chat":{"id":4242,"first_name":"Darek"}}}]}`)
			return
		}
		_ = r.ParseMultipartForm(1 << 20)
		chat, caption = r.FormValue("chat_id"), r.FormValue("caption")
		if f, _, err := r.FormFile("photo"); err == nil {
			photo = must(io.ReadAll(f))
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()
	telegramAPI = srv.URL

	if err := sendTelegram(telegramConfig{Token: "TOKEN", ChatID: "4242"}, "Wykryto: człowiek", []byte{0xff, 0xd8, 9}, "a.jpg"); err != nil {
		t.Fatal(err)
	}
	if path != "/botTOKEN/sendPhoto" || chat != "4242" || caption != "Wykryto: człowiek" || len(photo) != 3 {
		t.Fatalf("path %s chat %s caption %q photo %d", path, chat, caption, len(photo))
	}
	id, name, err := telegramFindChatID("TOKEN")
	if err != nil || id != "4242" || name != "Darek" {
		t.Fatalf("chat id %s %s %v", id, name, err)
	}
}

// XC_NTFY_LIVE=1 go test -run Live: publish a test picture to ntfy.sh and read it back.
func TestNtfyLive(t *testing.T) {
	if os.Getenv("XC_NTFY_LIVE") == "" {
		t.Skip("set XC_NTFY_LIVE=1")
	}
	topic := randomTopic()
	if err := sendNtfy(ntfyConfig{Topic: topic, Priority: "high"}, "Xiaomi Cam: test zażółć", "Wiadomość testowa ąę", testJPEG(), "test.jpg"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	res, err := http.Get("https://ntfy.sh/" + topic + "/json?poll=1")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var m struct {
		Title, Message string
		Priority       int
		Attachment     struct{ Name, Type, URL string }
	}
	line := strings.SplitN(string(must(io.ReadAll(res.Body))), "\n", 2)[0]
	if err = json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatal(err, line)
	}
	t.Logf("received: %+v", m)
	if m.Title != "Xiaomi Cam: test zażółć" || m.Message != "Wiadomość testowa ąę" || m.Attachment.Type != "image/jpeg" || m.Priority != 4 {
		t.Fatalf("unexpected: %+v", m)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
