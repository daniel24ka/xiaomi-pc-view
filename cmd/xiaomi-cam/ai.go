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
	FullRes  bool   `json:"full_res"` // send 2560x1440 instead of 1280 px (small details: brands, plates)

	// actions when detected
	Notify       bool `json:"notify"`
	SaveSnapshot bool `json:"save_snapshot"`
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
	AnthropicKey   string    `json:"anthropic_key,omitempty"`
	AnthropicModel string    `json:"anthropic_model,omitempty"`
	OpenAIKey      string    `json:"openai_key,omitempty"`
	OpenAIModel    string    `json:"openai_model,omitempty"`
	Tasks          []*aiTask `json:"tasks"`
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

	detected, answer, err := e.check(&task, cfg)

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
		e.runActions(&task, answer)
	}
	_ = e.save()
	if e.onChange != nil {
		e.onChange()
	}
}

func (e *aiEngine) runActions(t *aiTask, answer string) {
	if t.Notify && e.actions.notify != nil {
		e.actions.notify("Xiaomi Cam: "+t.Name, answer)
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
}

func taskCamera(t *aiTask) string {
	if t.Camera != "" && cameraSource(t.Camera) != "" {
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

// check takes a snapshot and asks the model.
func (e *aiEngine) check(t *aiTask, cfg aiConfig) (bool, string, error) {
	camera := taskCamera(t)
	if camera == "" {
		return false, "", errors.New("no camera")
	}
	width := 1280
	if t.FullRes {
		width = 0 // full camera resolution
	}
	img, err := snapshotJPEG(camera, width)
	if err != nil {
		return false, "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	var text string
	switch t.Provider {
	case providerOpenAI:
		text, err = askOpenAI(ctx, cfg.OpenAIKey, cfg.OpenAIModel, t.Prompt, img)
	default:
		text, err = askClaude(ctx, cfg.AnthropicKey, cfg.AnthropicModel, t.Prompt, img)
	}
	if err != nil {
		return false, "", err
	}
	var res aiResult
	if err = json.Unmarshal([]byte(extractJSON(text)), &res); err != nil {
		return false, "", fmt.Errorf("unexpected answer: %.200s", text)
	}
	return res.Detected, res.Description, nil
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

// askOpenAI - Chat Completions with an image and a JSON schema response format.
func askOpenAI(ctx context.Context, key, model, prompt string, jpeg []byte) (string, error) {
	if key == "" {
		return "", errors.New("OpenAI: no API key (Settings → AI)")
	}
	if model == "" {
		model = defaultOpenAIModel
	}
	body, _ := json.Marshal(map[string]any{
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
	})
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("OpenAI: %w", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI %d: %.300s", res.StatusCode, b)
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
		return "", fmt.Errorf("OpenAI: unexpected response: %.200s", b)
	}
	if r := out.Choices[0].Message.Refusal; r != "" {
		return "", fmt.Errorf("OpenAI declined: %s", r)
	}
	return out.Choices[0].Message.Content, nil
}
