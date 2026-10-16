package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/robfig/cron/v3"
)

// AI tasks: on a schedule (CRON) take a snapshot from a camera and ask an AI model
// (Claude or OpenAI) whether what the user described in the prompt is visible; if it is,
// run actions (system notification, save snapshot; more kinds can be added).
//
// Config with API keys is in ~/.config/xiaomi-cam-viewer/ai.json (mode 600).

const (
	providerClaude = "claude"
	providerOpenAI = "openai"
	providerLocal  = "local" // own OpenAI-compatible server: LM Studio, Ollama...

	defaultClaudeModel = "claude-opus-5-5"
	defaultOpenAIModel = "gpt-5-mini"
)

type aiTask struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
	Camera   string `json:"camera"`   // "" - first camera
	Provider string `json:"provider"` // claude / openai
	Prompt   string `json:"prompt"`
	Schedule string `json:"schedule"` // CRON (5 fields) or @every 5m, @hourly...
	FullRes  bool   `json:"full_res"`
	Point    int    `json:"point,omitempty"` // turn the camera to point 1-3 before the check // send 2560x1440 instead of 1280 px (small details: brands, plates)

	// actions when detected
	Notify       bool `json:"notify"`
	SaveSnapshot bool `json:"save_snapshot"`
	Email        bool `json:"email"`          // e-mail with the snapshot attached
	Slack        bool `json:"slack"`          // Slack message (Incoming Webhook)
	Ntfy         bool `json:"ntfy"`           // push to the phone (ntfy app), with the snapshot
	Telegram     bool `json:"telegram"`       // Telegram bot message, with the snapshot
	Record       bool `json:"record"`         // record an MP4 video
	RecordSec    int  `json:"record_seconds"` // video length
	CooldownMin  int  `json:"cooldown_min"`   // don't repeat actions for N minutes

	// state
	LastRun      time.Time `json:"last_run,omitzero"`
	LastDetected bool      `json:"last_detected"`
	LastAnswer   string    `json:"last_answer,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
	LastAction   time.Time `json:"last_action,omitzero"`
}

type aiConfig struct {
	AnthropicKey   string         `json:"anthropic_key,omitempty"`
	AnthropicModel string         `json:"anthropic_model,omitempty"`
	OpenAIKey      string         `json:"openai_key,omitempty"`
	OpenAIModel    string         `json:"openai_model,omitempty"`
	LocalURL       string         `json:"local_url,omitempty"` // e.g. http://192.168.1.50:1234
	LocalModel     string         `json:"local_model,omitempty"`
	LocalKey       string         `json:"local_key,omitempty"` // optional
	Email          emailConfig    `json:"email,omitzero"`
	SlackWebhook   string         `json:"slack_webhook,omitempty"`
	Ntfy           ntfyConfig     `json:"ntfy,omitzero"`
	Telegram       telegramConfig `json:"telegram,omitzero"`
	Events         eventsConfig   `json:"events,omitzero"` // actions on camera events (events.go)
	Tasks          []*aiTask      `json:"tasks"`
}

// actions are implemented in the app (notification needs the UI)
type aiActions struct {
	notify func(title, text string)
}

type aiEngine struct {
	mu      sync.Mutex
	path    string
	cfg     aiConfig
	cron    *cron.Cron
	entries map[string]cron.EntryID
	running map[string]bool
	actions aiActions

	onChange func() // task state changed (UI refresh)
}

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

func validateSchedule(s string) error {
	_, err := cronParser.Parse(strings.TrimSpace(s))
	return err
}

func newAIEngine(actions aiActions) *aiEngine {
	dir, _ := os.UserConfigDir()
	e := &aiEngine{
		path:    filepath.Join(dir, "xiaomi-cam-viewer", "ai.json"),
		cron:    cron.New(cron.WithParser(cronParser)),
		entries: map[string]cron.EntryID{},
		running: map[string]bool{},
		actions: actions,
	}
	if b, err := os.ReadFile(e.path); err == nil {
		_ = json.Unmarshal(b, &e.cfg)
	}
	if e.cfg.AnthropicModel == "" {
		e.cfg.AnthropicModel = defaultClaudeModel
	}
	if e.cfg.OpenAIModel == "" {
		e.cfg.OpenAIModel = defaultOpenAIModel
	}
	e.reschedule()
	e.cron.Start()
	return e
}

func (e *aiEngine) save() error {
	e.mu.Lock()
	b, err := json.MarshalIndent(e.cfg, "", "  ")
	e.mu.Unlock()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(e.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(e.path, b, 0o600) // contains API keys
}

func (e *aiEngine) config() aiConfig {
	e.mu.Lock()
	defer e.mu.Unlock()
	c := e.cfg
	c.Tasks = nil
	for _, t := range e.cfg.Tasks {
		tc := *t
		c.Tasks = append(c.Tasks, &tc)
	}
	return c
}

func (e *aiEngine) setProviders(anthropicKey, anthropicModel, openaiKey, openaiModel string) error {
	e.mu.Lock()
	e.cfg.AnthropicKey = strings.TrimSpace(anthropicKey)
	e.cfg.AnthropicModel = strings.TrimSpace(anthropicModel)
	e.cfg.OpenAIKey = strings.TrimSpace(openaiKey)
	e.cfg.OpenAIModel = strings.TrimSpace(openaiModel)
	e.mu.Unlock()
	return e.save()
}

func (e *aiEngine) setLocal(url, model, key string) error {
	e.mu.Lock()
	e.cfg.LocalURL = normalizeBaseURL(url)
	e.cfg.LocalModel = strings.TrimSpace(model)
	e.cfg.LocalKey = strings.TrimSpace(key)
	e.mu.Unlock()
	return e.save()
}

// notifyChannels - settings of all notification channels
type notifyChannels struct {
	Email        emailConfig
	SlackWebhook string
	Ntfy         ntfyConfig
	Telegram     telegramConfig
}

func (e *aiEngine) setNotifications(n notifyChannels) error {
	e.mu.Lock()
	e.cfg.Email = n.Email
	e.cfg.SlackWebhook = strings.TrimSpace(n.SlackWebhook)
	e.cfg.Ntfy = n.Ntfy
	e.cfg.Telegram = n.Telegram
	e.mu.Unlock()
	return e.save()
}

func (e *aiEngine) setEvents(c eventsConfig) error {
	e.mu.Lock()
	e.cfg.Events = c
	e.mu.Unlock()
	return e.save()
}

// putTask adds or replaces a task (by ID) and reschedules.
func (e *aiEngine) putTask(t *aiTask) error {
	if err := validateSchedule(t.Schedule); err != nil {
		return fmt.Errorf("schedule: %w", err)
	}
	e.mu.Lock()
	if t.ID == "" {
		t.ID = fmt.Sprintf("t%d", time.Now().UnixNano())
	}
	replaced := false
	for i, old := range e.cfg.Tasks {
		if old.ID == t.ID {
			e.cfg.Tasks[i] = t
			replaced = true
		}
	}
	if !replaced {
		e.cfg.Tasks = append(e.cfg.Tasks, t)
	}
	e.mu.Unlock()
	e.reschedule()
	return e.save()
}

func (e *aiEngine) deleteTask(id string) error {
	e.mu.Lock()
	for i, t := range e.cfg.Tasks {
		if t.ID == id {
			e.cfg.Tasks = append(e.cfg.Tasks[:i], e.cfg.Tasks[i+1:]...)
			break
		}
	}
	e.mu.Unlock()
	e.reschedule()
	return e.save()
}

func (e *aiEngine) setEnabled(id string, on bool) error {
	e.mu.Lock()
	for _, t := range e.cfg.Tasks {
		if t.ID == id {
			t.Enabled = on
		}
	}
	e.mu.Unlock()
	e.reschedule()
	return e.save()
}

func (e *aiEngine) reschedule() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id, entry := range e.entries {
		e.cron.Remove(entry)
		delete(e.entries, id)
	}
	for _, t := range e.cfg.Tasks {
		if !t.Enabled {
			continue
		}
		id := t.ID
		entry, err := e.cron.AddFunc(t.Schedule, func() { e.run(id) })
		if err == nil {
			e.entries[id] = entry
		}
	}
}

// runNow runs a task once (button "run now"), in the background.
func (e *aiEngine) runNow(id string) { go e.run(id) }

func (e *aiEngine) isRunning(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running[id]
}

func (e *aiEngine) run(id string) {
	e.mu.Lock()
	var task aiTask
	found := false
	for _, t := range e.cfg.Tasks {
		if t.ID == id {
			task, found = *t, true
		}
	}
	if !found || e.running[id] {
		e.mu.Unlock()
		return // removed, or the previous run is still going
	}
	e.running[id] = true
	cfg := e.cfg
	e.mu.Unlock()
	if e.onChange != nil {
		e.onChange() // the task list shows it's running
	}

	detected, answer, img, err := e.check(&task, cfg)

	e.mu.Lock()
	delete(e.running, id)
	var t *aiTask
	for _, tt := range e.cfg.Tasks {
		if tt.ID == id {
			t = tt
		}
	}
	if t == nil {
		e.mu.Unlock()
		return
	}
	t.LastRun = time.Now()
	t.LastError = ""
	if err != nil {
		t.LastError = err.Error()
	} else {
		t.LastDetected, t.LastAnswer = detected, answer
	}
	act := err == nil && detected &&
		(t.LastAction.IsZero() || time.Since(t.LastAction) >= time.Duration(t.CooldownMin)*time.Minute)
	if act {
		t.LastAction = time.Now()
	}
	task = *t
	e.mu.Unlock()

	if act {
		if errs := e.runActions(&task, cfg, answer, img); len(errs) > 0 {
			e.mu.Lock()
			if t := e.taskByID(id); t != nil {
				t.LastError = errors.Join(errs...).Error()
			}
			e.mu.Unlock()
		}
	}
	_ = e.save()
	if e.onChange != nil {
		e.onChange()
	}
}

func (e *aiEngine) taskByID(id string) *aiTask {
	for _, t := range e.cfg.Tasks {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// runActions runs the actions of a task after a detection; returns errors of the
// actions that failed (shown in the task status).
func (e *aiEngine) runActions(t *aiTask, cfg aiConfig, answer string, img []byte) []error {
	var errs []error
	title := "Xiaomi Cam: " + t.Name
	camera := taskCamera(t)
	when := time.Now().Format("2006-01-02 15:04:05")
	if t.Notify && e.actions.notify != nil {
		e.actions.notify(title, answer)
	}
	if t.Email {
		body := fmt.Sprintf("%s\n\n%s: %s\n%s: %s\n%s: %s\n", answer,
			"Camera", camera, "Time", when, "Prompt", t.Prompt)
		if err := sendEmail(cfg.Email, title, body, img, fileName(camera, "jpg")); err != nil {
			errs = append(errs, err)
		}
	}
	if t.Slack {
		text := fmt.Sprintf("*%s*\n%s\n_%s · %s_", title, answer, camera, when)
		if err := sendSlack(cfg.SlackWebhook, text); err != nil {
			errs = append(errs, err)
		}
	}
	if t.Ntfy {
		if err := sendNtfy(cfg.Ntfy, title, answer, img, fileName(camera, "jpg")); err != nil {
			errs = append(errs, err)
		}
	}
	if t.Telegram {
		text := fmt.Sprintf("%s\n%s\n%s · %s", title, answer, camera, when)
		if err := sendTelegram(cfg.Telegram, text, img, fileName(camera, "jpg")); err != nil {
			errs = append(errs, err)
		}
	}
	if t.SaveSnapshot {
		_, _ = saveSnapshot(taskCamera(t))
	}
	if t.Record {
		sec := t.RecordSec
		if sec <= 0 {
			sec = 60
		}
		// already recording (e.g. started by hand) - keep that recording
		_, _ = startRecording(taskCamera(t), time.Duration(sec)*time.Second)
	}
	return errs
}

func taskCamera(t *aiTask) string {
	if cameraExists(t.Camera) {
		return t.Camera
	}
	if names := cameraNames(); len(names) > 0 {
		return names[0]
	}
	return ""
}

const aiSystemPrompt = `You analyze a still image from a home security camera (the camera prints the date and time in the top left corner).
The user describes what to look for. Decide whether it is visible in this image.
Reply only with JSON: {"detected": true or false, "description": "one or two sentences: what you see that answers the question, in the language of the user's description"}.`

var aiSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"detected":    map[string]any{"type": "boolean"},
		"description": map[string]any{"type": "string"},
	},
	"required":             []string{"detected", "description"},
	"additionalProperties": false,
}

type aiResult struct {
	Detected    bool   `json:"detected"`
	Description string `json:"description"`
}

// check takes a snapshot and asks the model; returns the snapshot for the actions.
func (e *aiEngine) check(t *aiTask, cfg aiConfig) (bool, string, []byte, error) {
	camera := taskCamera(t)
	if camera == "" {
		return false, "", nil, errors.New("no camera")
	}
	if t.Point > 0 {
		p, err := findPoint(camera, strconv.Itoa(t.Point))
		if err != nil {
			return false, "", nil, err
		}
		if err = gotoPresetAndWait(camera, p.Idx); err != nil {
			return false, "", nil, err
		}
		pointReached(camera, p)
	}
	// keep the camera session open between frequent checks (closed a minute after the last one),
	// otherwise every check connects to the camera again (2-4 s)
	if err := ensureSession(camera); err != nil {
		return false, "", nil, err
	}
	width := 1280
	if t.FullRes {
		width = 0 // full camera resolution
	}
	img, err := snapshotJPEG(camera, width)
	if err != nil {
		return false, "", nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	var text string
	switch t.Provider {
	case providerOpenAI:
		text, err = askOpenAI(ctx, cfg.OpenAIKey, cfg.OpenAIModel, t.Prompt, img)
	case providerLocal:
		text, err = askLocal(ctx, cfg.LocalURL, cfg.LocalKey, cfg.LocalModel, t.Prompt, img)
	default:
		text, err = askClaude(ctx, cfg.AnthropicKey, cfg.AnthropicModel, t.Prompt, img)
	}
	if err != nil {
		return false, "", nil, err
	}
	var res aiResult
	if err = json.Unmarshal([]byte(extractJSON(text)), &res); err != nil {
		return false, "", nil, fmt.Errorf("unexpected answer: %.200s", text)
	}
	return res.Detected, res.Description, img, nil
}

// extractJSON - the JSON object in a text (models may wrap it in ```json)
func extractJSON(s string) string {
	if i, j := strings.Index(s, "{"), strings.LastIndex(s, "}"); i >= 0 && j > i {
		return s[i : j+1]
	}
	return s
}

func askClaude(ctx context.Context, key, model, prompt string, jpeg []byte) (string, error) {
	if key == "" {
		return "", errors.New("Claude: no API key (Settings → AI)")
	}
	if model == "" {
		model = defaultClaudeModel
	}
	client := anthropic.NewClient(option.WithAPIKey(key))
	// on a safety decline the request is re-run on a fallback model (server-side fallbacks)
	opts := []option.RequestOption{
		option.WithHeader("anthropic-beta", string(anthropic.AnthropicBetaServerSideFallback2026_07_01)),
		option.WithJSONSet("fallbacks", "default"),
	}
	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: 4000,
		System:    []anthropic.TextBlockParam{{Text: aiSystemPrompt}},
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(
			anthropic.NewImageBlockBase64("image/jpeg", base64.StdEncoding.EncodeToString(jpeg)),
			anthropic.NewTextBlock(prompt),
		)},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffortLow, // a yes/no look at one picture
			Format: anthropic.JSONOutputFormatParam{Schema: aiSchema},
		},
	}, opts...)
	if err != nil {
		return "", fmt.Errorf("Claude: %w", err)
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return "", fmt.Errorf("Claude declined: %s", resp.StopDetails.Explanation)
	}
	var out strings.Builder
	for _, block := range resp.Content {
		if b, ok := block.AsAny().(anthropic.TextBlock); ok {
			out.WriteString(b.Text)
		}
	}
	return out.String(), nil
}

// askOpenAI - OpenAI Chat Completions with an image and a JSON schema response format.
func askOpenAI(ctx context.Context, key, model, prompt string, jpeg []byte) (string, error) {
	if key == "" {
		return "", errors.New("OpenAI: no API key (Settings → AI)")
	}
	if model == "" {
		model = defaultOpenAIModel
	}
	return chatCompletions(ctx, "OpenAI", "https://api.openai.com/v1", key, model, prompt, jpeg)
}

// askLocal - own OpenAI-compatible server (LM Studio, Ollama, vLLM, LocalAI...) in the
// network; the model must support images (e.g. Qwen2.5-VL, Gemma 3, LLaVA).
func askLocal(ctx context.Context, baseURL, key, model, prompt string, jpeg []byte) (string, error) {
	if baseURL == "" {
		return "", errors.New("local server: no address (Settings → AI)")
	}
	if model == "" {
		return "", errors.New("local server: no model (Settings → AI)")
	}
	return chatCompletions(ctx, "local server", baseURL, key, model, prompt, jpeg)
}

// normalizeBaseURL - "192.168.1.50:1234", "http://host:11434/v1/" -> "http://host:port/v1"
func normalizeBaseURL(s string) string {
	s = strings.TrimRight(strings.TrimSpace(s), "/")
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	if !strings.HasSuffix(s, "/v1") {
		s += "/v1"
	}
	return s
}

func apiRequest(ctx context.Context, method, url, key string, body []byte) ([]byte, int, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, r)
	if err != nil {
		return nil, 0, err
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	return b, res.StatusCode, err
}

// chatCompletions - /chat/completions with the picture; asks for a JSON schema response,
// and when the server doesn't support response_format (400), asks again without it
// (the system prompt asks for JSON too).
func chatCompletions(ctx context.Context, label, baseURL, key, model, prompt string, jpeg []byte) (string, error) {
	req := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{"role": "system", "content": aiSystemPrompt},
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": prompt},
				map[string]any{"type": "image_url", "image_url": map[string]any{
					"url": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpeg),
				}},
			}},
		},
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name": "detection", "strict": true, "schema": aiSchema,
			},
		},
	}
	url := baseURL + "/chat/completions"
	body, _ := json.Marshal(req)
	b, status, err := apiRequest(ctx, "POST", url, key, body)
	if err == nil && status == http.StatusBadRequest && bytes.Contains(bytes.ToLower(b), []byte("response_format")) {
		delete(req, "response_format")
		body, _ = json.Marshal(req)
		b, status, err = apiRequest(ctx, "POST", url, key, body)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("%s %d: %.300s", label, status, b)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
				Refusal string `json:"refusal"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err = json.Unmarshal(b, &out); err != nil || len(out.Choices) == 0 {
		return "", fmt.Errorf("%s: unexpected response: %.200s", label, b)
	}
	if r := out.Choices[0].Message.Refusal; r != "" {
		return "", fmt.Errorf("%s declined: %s", label, r)
	}
	return out.Choices[0].Message.Content, nil
}

// listModels - model ids from an OpenAI-compatible server (GET /v1/models).
func listModels(baseURL, key string) ([]string, error) {
	baseURL = normalizeBaseURL(baseURL)
	if baseURL == "" {
		return nil, errors.New("no server address")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	b, status, err := apiRequest(ctx, "GET", baseURL+"/models", key, nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("%d: %.200s", status, b)
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err = json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("not an OpenAI-compatible server: %.100s", b)
	}
	var ids []string
	for _, m := range out.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}
