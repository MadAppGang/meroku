package slack_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"madappgang.com/infrastructure/ci_lambda/internal/slack"
	"madappgang.com/infrastructure/ci_lambda/internal/testsupport"
)

// awkwardMessages covers every level and every shape, including values that
// would break a template that interpolated them raw.
func awkwardMessages() []slack.Message {
	nasty := `he said "deploy" now\nand a backslash \ too`

	var out []slack.Message
	for _, level := range []slack.Level{slack.LevelSuccess, slack.LevelError, slack.LevelInfo, slack.Level("unknown")} {
		out = append(out,
			slack.Message{Level: level},
			slack.Message{Level: level, Service: "backend", State: "DEPLOYMENT_STARTED"},
			slack.Message{
				Level: level, Env: "dev", Service: nasty, State: nasty,
				Reason: nasty, DeploymentID: nasty, TaskDef: nasty,
			},
		)
	}
	return out
}

// TestEveryTemplateRendersValidJSON is what the three tracked-but-dead template
// files in the old tree would have failed: they referenced a field that did not
// exist and emitted trailing commas.
func TestEveryTemplateRendersValidJSON(t *testing.T) {
	n := slack.New("https://example.invalid/hook", "dev", testsupport.Logger())
	client, ok := n.(*slack.Client)
	require.True(t, ok)

	for i, m := range awkwardMessages() {
		payload, err := client.Render(m)
		require.NoErrorf(t, err, "message %d", i)
		require.Truef(t, json.Valid(payload), "message %d produced invalid JSON:\n%s", i, payload)

		var decoded struct {
			Blocks []map[string]any `json:"blocks"`
		}
		require.NoError(t, json.Unmarshal(payload, &decoded))
		require.NotEmptyf(t, decoded.Blocks, "message %d produced no blocks", i)
	}
}

func TestRenderFallsBackToTheConfiguredEnvironment(t *testing.T) {
	n := slack.New("https://example.invalid/hook", "staging", testsupport.Logger())
	client := n.(*slack.Client)

	payload, err := client.Render(slack.Message{Level: slack.LevelInfo, Service: "backend"})
	require.NoError(t, err)
	require.Contains(t, string(payload), "staging")
}

func TestNoWebhookMeansNoOp(t *testing.T) {
	n := slack.New("", "dev", testsupport.Logger())
	require.NotNil(t, n)
	// Must not panic and must not need a server.
	n.Notify(context.Background(), slack.Message{Level: slack.LevelInfo})
}

func TestNotifyPostsTheRenderedPayload(t *testing.T) {
	var body atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body.Store(string(b))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	slack.New(srv.URL, "dev", testsupport.Logger()).
		Notify(context.Background(), slack.Message{Level: slack.LevelSuccess, Service: "backend", State: "DONE"})

	got, _ := body.Load().(string)
	require.True(t, json.Valid([]byte(got)))
	require.Contains(t, got, "backend")
}

// TestNotifySwallowsFailures is the type-level guarantee that a Slack outage
// cannot fail a deployment: Notify has no error to return.
func TestNotifySwallowsFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := slack.New(srv.URL, "dev", testsupport.Logger())
	n.Notify(context.Background(), slack.Message{Level: slack.LevelError, Service: "backend"})

	// An unreachable endpoint is equally harmless.
	slack.New("http://127.0.0.1:1/hook", "dev", testsupport.Logger()).
		Notify(context.Background(), slack.Message{Level: slack.LevelInfo})

	// So is a cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n.Notify(ctx, slack.Message{Level: slack.LevelInfo})
}
