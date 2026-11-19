package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// eventsDeployedProject writes dev.yaml (and, when mainTf is not "", a
// previous env/dev/main.tf) in a fresh project directory and enters it.
func eventsDeployedProject(t *testing.T, yamlBody, mainTf string) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("dev.yaml", []byte(yamlBody), 0o600); err != nil {
		t.Fatal(err)
	}
	if mainTf != "" {
		if err := os.MkdirAll(filepath.Join("env", "dev"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join("env", "dev", "main.tf"), []byte(mainTf), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func getEventsDeployedReport(t *testing.T, query string) (int, eventsDeployedReport, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	getEventsDeployed(rec, httptest.NewRequest(http.MethodGet, "/api/events/deployed?"+query, nil))
	var report eventsDeployedReport
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
			t.Fatalf("invalid JSON: %v; body %s", err, rec.Body.String())
		}
	}
	return rec.Code, report, rec.Body.String()
}

// snapshotTree lists every file under dir with its size, to show that a
// request wrote nothing.
func snapshotTree(t *testing.T, dir string) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		out[path] = info.Size()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestGetEventsDeployed(t *testing.T) {
	yamlBody := freshCheckoutYAML("event_bus: { enabled: true }", freshCheckoutSubscribes)
	noPull := func() ([]byte, error) { t.Error("terraform state pull ran"); return nil, nil }

	t.Run("the state answers on a fresh checkout", func(t *testing.T) {
		dir := eventsDeployedProject(t, yamlBody, "")
		_, remotes := stubEventsReaders(t, noPull, func(Env) ([]byte, error) { return []byte(freshCheckoutState), nil })
		before := snapshotTree(t, dir)
		code, report, body := getEventsDeployedReport(t, "env=dev")
		if code != http.StatusOK {
			t.Fatalf("status %d: %s", code, body)
		}
		want := eventsDeployedReport{Known: true, FifoConsumers: []string{"orders"},
			SharedPrincipals:    []string{"account:000000000001", "account:000000000002"},
			SubscribePrincipals: []string{"account:000000000001"}}
		if !reflect.DeepEqual(report, want) || *remotes != 1 {
			t.Errorf("report %+v after %d S3 reads, want %+v after one", report, *remotes, want)
		}
		if after := snapshotTree(t, dir); !reflect.DeepEqual(before, after) {
			t.Errorf("the request wrote files: before %v, after %v", before, after)
		}
	})

	t.Run("an unreadable state is not known", func(t *testing.T) {
		eventsDeployedProject(t, yamlBody, "")
		stubEventsReaders(t, noPull, func(Env) ([]byte, error) { return nil, errors.New("AccessDenied") })
		code, report, body := getEventsDeployedReport(t, "env=dev")
		want := eventsDeployedReport{FifoConsumers: []string{}, SharedPrincipals: []string{}, SubscribePrincipals: []string{}}
		if code != http.StatusOK || !reflect.DeepEqual(report, want) {
			t.Errorf("status %d report %+v (%s), want 200 and %+v", code, report, body, want)
		}
		if body != `{"known":false,"fifoConsumers":[],"sharedPrincipals":[],"subscribePrincipals":[]}`+"\n" {
			t.Errorf("body %q: the lists must be [] and never null", body)
		}
	})

	t.Run("a previous render without markers answers offline", func(t *testing.T) {
		eventsDeployedProject(t, yamlBody, "# a classic render\n")
		stubEventsReaders(t, noPull, func(Env) ([]byte, error) { t.Error("the S3 state was read"); return nil, nil })
		code, report, body := getEventsDeployedReport(t, "env=dev")
		want := eventsDeployedReport{Known: true, FifoConsumers: []string{}, SharedPrincipals: []string{}, SubscribePrincipals: []string{}}
		if code != http.StatusOK || !reflect.DeepEqual(report, want) {
			t.Errorf("status %d report %+v (%s), want 200 and %+v", code, report, body, want)
		}
	})

	t.Run("markers answer when the state is not needed", func(t *testing.T) {
		eventsDeployedProject(t, yamlBody, "# meroku:events fifo=orders retired\n"+
			"# meroku:events share=account:000000000001:full\n# meroku:events share=organization:-:publish\n")
		stubEventsReaders(t, noPull, func(Env) ([]byte, error) { return nil, errRemoteStateAbsent })
		code, report, body := getEventsDeployedReport(t, "env=dev")
		// fifo_retired is not in this YAML, but the markers make the state
		// needed; an absent object is a verified empty state.
		want := eventsDeployedReport{Known: true, FifoConsumers: []string{}, SharedPrincipals: []string{}, SubscribePrincipals: []string{}}
		if code != http.StatusOK || !reflect.DeepEqual(report, want) {
			t.Errorf("status %d report %+v (%s), want 200 and %+v", code, report, body, want)
		}
	})

	t.Run("offline markers: subscribe-capable shares", func(t *testing.T) {
		d := eventDeployed{hasPrevious: true, previous: parseEventMarkers([]byte(
			"# meroku:events share=account:000000000001:full\n# meroku:events share=organization:-:publish\n# meroku:events share=ou:ou-ab12-cd34ef56:subscribe\n"))}
		if got, want := d.sharedPrincipals(), []string{"account:000000000001", "organization", "ou:ou-ab12-cd34ef56"}; !reflect.DeepEqual(got, want) {
			t.Errorf("sharedPrincipals %v, want %v", got, want)
		}
		if got, want := d.subscribePrincipals(), []string{"account:000000000001", "ou:ou-ab12-cd34ef56"}; !reflect.DeepEqual(got, want) {
			t.Errorf("subscribePrincipals %v, want %v", got, want)
		}
	})

	t.Run("unknown env is 404", func(t *testing.T) {
		eventsDeployedProject(t, yamlBody, "")
		code, _, body := getEventsDeployedReport(t, "env=staging")
		var e map[string]string
		if code != http.StatusNotFound || json.Unmarshal([]byte(body), &e) != nil || e["error"] == "" {
			t.Errorf("status %d body %s, want 404 with an error", code, body)
		}
	})

	t.Run("no env is 400", func(t *testing.T) {
		eventsDeployedProject(t, yamlBody, "")
		if code, _, body := getEventsDeployedReport(t, ""); code != http.StatusBadRequest {
			t.Errorf("status %d body %s, want 400", code, body)
		}
	})

	t.Run("served by mainRouter behind the access guard", func(t *testing.T) {
		eventsDeployedProject(t, yamlBody, "")
		stubEventsReaders(t, noPull, func(Env) ([]byte, error) { return []byte(freshCheckoutState), nil })
		router := mainRouter()

		req := httptest.NewRequest(http.MethodGet, "/api/events/deployed?env=dev", nil)
		req.RemoteAddr = "127.0.0.1:54000"
		req.Host = "127.0.0.1:8080"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("loopback GET: status %d body %s", rec.Code, rec.Body.String())
		}

		req = httptest.NewRequest(http.MethodGet, "/api/events/deployed?env=dev", nil)
		req.RemoteAddr = "192.0.2.10:54000"
		req.Host = "127.0.0.1:8080"
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			t.Errorf("a non-loopback client reached the handler: %s", rec.Body.String())
		}
	})
}
