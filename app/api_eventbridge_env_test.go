package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The env name in a request becomes part of a file path. A name with a
// separator or ".." must be refused before any file is read.
func TestEventBridgeAPI_RefusesUnsafeEnvNames(t *testing.T) {
	loaded := 0
	previous := loadEnvForTestEvent
	loadEnvForTestEvent = func(string) (Env, error) {
		loaded++
		return Env{}, nil
	}
	t.Cleanup(func() { loadEnvForTestEvent = previous })

	for _, name := range []string{"../x", "a/b", "..", `a\b`, "dev.yaml"} {
		t.Run("GET "+name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/eventbridge/event-tasks?env="+url.QueryEscape(name), nil)
			getEventTaskInfo(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status %d, want 400", rec.Code)
			}
		})
		t.Run("POST "+name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			body := `{"env":` + jsonQuote(name) + `,"source":"myapp.api","detailType":"x"}`
			req := httptest.NewRequest(http.MethodPost, "/api/eventbridge/send-test-event", strings.NewReader(body))
			sendTestEvent(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status %d, want 400", rec.Code)
			}
		})
	}
	if loaded != 0 {
		t.Errorf("the loader ran %d times for refused names", loaded)
	}
}

func jsonQuote(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

// An event group longer than the ordered queue accepts, or with characters
// SQS refuses, is answered 400 before any AWS call: PutEvents would accept it
// and the delivery would then fail.
func TestSendTestEventRefusesUndeliverableEventGroups(t *testing.T) {
	enabled := true
	previous := loadEnvForTestEvent
	loadEnvForTestEvent = func(string) (Env, error) {
		return Env{Project: "myapp", Env: "dev", Region: "ap-southeast-2",
			EventBus: &EventBus{Enabled: &enabled, Type: eventBusTypeEnhanced}}, nil
	}
	t.Cleanup(func() { loadEnvForTestEvent = previous })

	for name, group := range map[string]string{
		"116 characters": strings.Repeat("k", eventGroupIDMax+1),
		"a space":        "order 1",
		"non-ASCII":      "ordér-1",
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			body := `{"env":"dev","eventBusName":"myapp-dev","source":"myapp.api","detailType":"x","eventGroupId":` + jsonQuote(group) + `}`
			sendTestEvent(rec, httptest.NewRequest(http.MethodPost, "/api/eventbridge/send-test-event", strings.NewReader(body)))
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "eventGroupId") {
				t.Errorf("status %d body %s, want 400 naming eventGroupId", rec.Code, rec.Body.String())
			}
		})
	}
	if got := eventGroupIDError(strings.Repeat("k", eventGroupIDMax)); got != "" {
		t.Errorf("a %d-character group is refused: %s", eventGroupIDMax, got)
	}
}

// loadEnvReadOnly migrates in memory and never touches the file: a GET must not
// rewrite <env>.yaml or leave a backup next to it.
func TestLoadEnvReadOnly_DoesNotWriteTheFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	old := "schema_version: 1\nproject: myapp\nenv: dev\nregion: us-east-1\naccount_id: \"000000000000\"\n"
	if err := os.WriteFile("dev.yaml", []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	e, err := loadEnvReadOnly("dev")
	if err != nil {
		t.Fatalf("loadEnvReadOnly: %v", err)
	}
	if e.Project != "myapp" {
		t.Errorf("project %q, want myapp", e.Project)
	}

	after, err := os.ReadFile("dev.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != old {
		t.Errorf("dev.yaml was rewritten:\n%s", after)
	}
	entries, _ := filepath.Glob(filepath.Join(dir, "*"))
	if len(entries) != 1 {
		t.Errorf("want only dev.yaml in the directory, got %v", entries)
	}
}
