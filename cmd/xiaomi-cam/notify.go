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
