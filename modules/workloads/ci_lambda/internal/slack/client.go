// Package slack posts deployment notifications.
//
// Notify returns nothing, on purpose. A webhook outage must never fail a
// deployment or an invocation: when handleECSEvent returned the webhook error,
// EventBridge retried the event and Slack got the notification several times
// over.
package slack

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"text/template"
	"time"
)

//go:embed templates/*.json.tmpl
var templateFS embed.FS

// notifyTimeout bounds one webhook call. The whole invocation has 60s and the
// deployment matters more than the notification.
const notifyTimeout = 5 * time.Second

// Level selects the message template.
type Level string

const (
	LevelSuccess Level = "success"
	LevelError   Level = "error"
	LevelInfo    Level = "info"
)

// Message is everything a notification can say.
type Message struct {
	Level        Level
	Env          string
	Service      string
	State        string
	Reason       string
	DeploymentID string
	TaskDef      string
}

// Notifier posts messages. There is no error to discard.
type Notifier interface {
	Notify(ctx context.Context, m Message)
}

// Client posts to a Slack incoming webhook.
type Client struct {
	url  string
	env  string
	http *http.Client
	tmpl *template.Template
	log  *slog.Logger
}

type noop struct{}

func (noop) Notify(context.Context, Message) {}

// New returns a Notifier. With no webhook URL configured it returns a no-op, so
// callers never need to check whether Slack is enabled.
func New(webhookURL, env string, log *slog.Logger) Notifier {
	if webhookURL == "" {
		return noop{}
	}

	tmpl, err := parseTemplates()
	if err != nil {
		// Templates are embedded, so this can only be a build defect. Degrade
		// to no notifications rather than taking deployments down with us.
		log.Error("slack templates failed to parse; notifications disabled", "error", err)
		return noop{}
	}

	return &Client{
		url:  webhookURL,
		env:  env,
		http: &http.Client{Timeout: notifyTimeout},
		tmpl: tmpl,
		log:  log,
	}
}

func parseTemplates() (*template.Template, error) {
	funcs := template.FuncMap{
		// json renders a value as a JSON literal. Every interpolation in the
		// templates goes through it, so no service name, reason or ARN can
		// produce a malformed payload.
		"json": func(v any) (string, error) {
			b, err := json.Marshal(v)
			if err != nil {
				return "", err
			}
			return string(b), nil
		},
	}
	return template.New("slack").Funcs(funcs).ParseFS(templateFS, "templates/*.json.tmpl")
}

// Render produces the webhook payload for a message. Exported for tests, which
// assert that every template renders valid JSON for every message shape.
func (c *Client) Render(m Message) ([]byte, error) {
	if m.Env == "" {
		m.Env = c.env
	}
	name := templateName(m.Level)

	var buf bytes.Buffer
	if err := c.tmpl.ExecuteTemplate(&buf, name, m); err != nil {
		return nil, fmt.Errorf("slack: render %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

func templateName(l Level) string {
	switch l {
	case LevelSuccess:
		return "success.json.tmpl"
	case LevelError:
		return "error.json.tmpl"
	default:
		return "info.json.tmpl"
	}
}

// Notify posts a message and swallows every failure into a log line.
func (c *Client) Notify(ctx context.Context, m Message) {
	payload, err := c.Render(m)
	if err != nil {
		c.log.Error("slack notification not sent", "error", err, "service", m.Service, "level", string(m.Level))
		return
	}

	// Bounded by the smaller of the notify timeout and whatever is left of the
	// invocation deadline.
	ctx, cancel := context.WithTimeout(ctx, notifyTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(payload))
	if err != nil {
		c.log.Error("slack notification not sent", "error", err, "service", m.Service)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		c.log.Error("slack notification not sent", "error", err, "service", m.Service)
		return
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.log.Error("slack webhook rejected the notification",
			"status_code", resp.StatusCode, "service", m.Service)
		return
	}

	c.log.Debug("slack notification sent", "service", m.Service, "level", string(m.Level))
}
