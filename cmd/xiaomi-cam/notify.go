package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// Notification channels for AI tasks: e-mail (SMTP, with the snapshot attached) and
// Slack (Incoming Webhook). Settings are in ai.json (mode 600).

type emailConfig struct {
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`     // 587 (STARTTLS), 465 (TLS), 25
	Security string `json:"security,omitempty"` // starttls / tls / none
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"` // comma separated
}

func (c emailConfig) configured() bool { return c.Host != "" && c.From != "" && c.To != "" }

func splitAddrs(s string) []string {
	var out []string
	for _, a := range strings.Split(s, ",") {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

// sendEmail sends a text e-mail with an optional JPEG attachment.
func sendEmail(c emailConfig, subject, text string, jpeg []byte, jpegName string) error {
	if !c.configured() {
		return errors.New("e-mail: SMTP is not configured (Settings → Notifications)")
	}
	to := splitAddrs(c.To)
	port := c.Port
	if port == 0 {
		port = 587
	}
	addr := net.JoinHostPort(c.Host, strconv.Itoa(port))

	var msg bytes.Buffer
	boundary := fmt.Sprintf("xiaomi-cam-%d", time.Now().UnixNano())
	fmt.Fprintf(&msg, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\n",
		c.From, strings.Join(to, ", "), mime.QEncoding.Encode("utf-8", subject), time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&msg, "Content-Type: multipart/mixed; boundary=%q\r\n\r\n", boundary)
	fmt.Fprintf(&msg, "--%s\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n%s\r\n",
		boundary, wrap76(base64.StdEncoding.EncodeToString([]byte(text))))
	if len(jpeg) > 0 {
		fmt.Fprintf(&msg, "--%s\r\nContent-Type: image/jpeg; name=%q\r\nContent-Disposition: attachment; filename=%q\r\nContent-Transfer-Encoding: base64\r\n\r\n%s\r\n",
			boundary, jpegName, jpegName, wrap76(base64.StdEncoding.EncodeToString(jpeg)))
	}
	fmt.Fprintf(&msg, "--%s--\r\n", boundary)

	var auth smtp.Auth
	if c.Username != "" {
		auth = smtp.PlainAuth("", c.Username, c.Password, c.Host)
	}

	var conn net.Conn
	var err error
	dialer := net.Dialer{Timeout: 20 * time.Second}
	if c.Security == "tls" || (c.Security == "" && port == 465) {
		conn, err = tls.DialWithDialer(&dialer, "tcp", addr, &tls.Config{ServerName: c.Host})
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("e-mail: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(60 * time.Second))
	client, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("e-mail: %w", err)
	}
	defer client.Close()
	if c.Security == "starttls" || (c.Security == "" && port != 465) {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err = client.StartTLS(&tls.Config{ServerName: c.Host}); err != nil {
				return fmt.Errorf("e-mail: STARTTLS: %w", err)
			}
		} else if c.Security == "starttls" {
			return errors.New("e-mail: the server doesn't support STARTTLS")
		}
	}
	if auth != nil {
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("e-mail: login: %w", err)
		}
	}
	if err = client.Mail(c.From); err != nil {
		return fmt.Errorf("e-mail: %w", err)
	}
	for _, rcpt := range to {
		if err = client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("e-mail: %s: %w", rcpt, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("e-mail: %w", err)
	}
	if _, err = w.Write(msg.Bytes()); err != nil {
		return fmt.Errorf("e-mail: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("e-mail: %w", err)
	}
	return client.Quit()
}

func wrap76(s string) string {
	var b strings.Builder
	for len(s) > 76 {
		b.WriteString(s[:76])
		b.WriteString("\r\n")
		s = s[76:]
	}
	b.WriteString(s)
	return b.String()
}

// sendSlack posts a message to a Slack Incoming Webhook
// (https://api.slack.com/messaging/webhooks). Webhooks can't upload files.
func sendSlack(webhook, text string) error {
	if !strings.HasPrefix(webhook, "https://") && !strings.HasPrefix(webhook, "http://127.0.0.1") {
		return errors.New("Slack: no webhook URL (Settings → Notifications)")
	}
	body, _ := json.Marshal(map[string]any{"text": text})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", webhook, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("Slack: %w", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Slack %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// --- ntfy (https://ntfy.sh, also self-hosted): push notification to the phone app

type ntfyConfig struct {
	Server   string `json:"server,omitempty"` // default https://ntfy.sh
	Topic    string `json:"topic,omitempty"`
	Token    string `json:"token,omitempty"`    // access token for protected topics/servers
	Priority string `json:"priority,omitempty"` // min, low, default, high, urgent
}

func (c ntfyConfig) configured() bool { return c.Topic != "" }

// sendNtfy publishes a notification; with jpeg the picture is attached (shown in the
// notification). Title and message go in RFC 2047 encoded headers (UTF-8).
func sendNtfy(c ntfyConfig, title, message string, jpeg []byte, jpegName string) error {
	if !c.configured() {
		return errors.New("ntfy: no topic (Settings → Notifications)")
	}
	server := strings.TrimRight(c.Server, "/")
	if server == "" {
		server = "https://ntfy.sh"
	}
	var body io.Reader = strings.NewReader(message)
	method := "POST"
	if len(jpeg) > 0 {
		method, body = "PUT", bytes.NewReader(jpeg)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, server+"/"+c.Topic, body)
	if err != nil {
		return err
	}
	req.Header.Set("Title", mime.BEncoding.Encode("utf-8", title))
	req.Header.Set("Tags", "camera")
	if c.Priority != "" && c.Priority != "default" {
		req.Header.Set("Priority", c.Priority)
	}
	if len(jpeg) > 0 {
		req.Header.Set("Filename", jpegName)
		req.Header.Set("Message", mime.BEncoding.Encode("utf-8", message))
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("ntfy: %w", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("ntfy %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// --- Telegram bot

type telegramConfig struct {
	Token  string `json:"token,omitempty"`   // from @BotFather
	ChatID string `json:"chat_id,omitempty"` // user or group chat
}

func (c telegramConfig) configured() bool { return c.Token != "" && c.ChatID != "" }

var telegramAPI = "https://api.telegram.org" // replaced in tests

func telegramCall(ctx context.Context, token, method string, body io.Reader, contentType string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", telegramAPI+"/bot"+token+"/"+method, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Telegram: %v", strings.ReplaceAll(err.Error(), token, "***"))
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	var r struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal(b, &r)
	if !r.OK {
		return nil, fmt.Errorf("Telegram %d: %s", res.StatusCode, r.Description)
	}
	return b, nil
}

// sendTelegram sends the picture with a caption (or a text message without a picture).
func sendTelegram(c telegramConfig, text string, jpeg []byte, jpegName string) error {
	if !c.configured() {
		return errors.New("Telegram: no bot token or chat ID (Settings → Notifications)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if len(jpeg) == 0 {
		body, _ := json.Marshal(map[string]any{"chat_id": c.ChatID, "text": text})
		_, err := telegramCall(ctx, c.Token, "sendMessage", bytes.NewReader(body), "application/json")
		return err
	}
	if r := []rune(text); len(r) > 1024 { // caption limit
		text = string(r[:1021]) + "..."
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("chat_id", c.ChatID)
	_ = mw.WriteField("caption", text)
	fw, _ := mw.CreateFormFile("photo", jpegName)
	_, _ = fw.Write(jpeg)
	_ = mw.Close()
	_, err := telegramCall(ctx, c.Token, "sendPhoto", &buf, mw.FormDataContentType())
	return err
}

// telegramFindChatID returns the chat of the last message sent to the bot
// (the user writes anything to the bot first).
func telegramFindChatID(token string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	b, err := telegramCall(ctx, token, "getUpdates", nil, "")
	if err != nil {
		return "", "", err
	}
	var r struct {
		Result []struct {
			Message *struct {
				Chat struct {
					ID        int64  `json:"id"`
					FirstName string `json:"first_name"`
					Title     string `json:"title"`
				} `json:"chat"`
			} `json:"message"`
		} `json:"result"`
	}
	_ = json.Unmarshal(b, &r)
	for i := len(r.Result) - 1; i >= 0; i-- {
		if m := r.Result[i].Message; m != nil {
			name := m.Chat.FirstName
			if m.Chat.Title != "" {
				name = m.Chat.Title
			}
			return strconv.FormatInt(m.Chat.ID, 10), name, nil
		}
	}
	return "", "", errors.New("Telegram: no messages yet - send any message to your bot and try again")
}
