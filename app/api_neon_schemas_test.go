package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

type schemaQueryCall struct {
	cred     PGCredential
	database string
}

func schemaLookupDeps(t *testing.T, api *qaNeonAPI, query func(context.Context, PGCredential, string) ([]string, error)) (neonLookupDependencies, *[]schemaQueryCall) {
	t.Helper()
	var calls []schemaQueryCall
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return neonLookupDependencies{
		Auth:   qaNeonAuthDeps(now, "fake-schema-token-do-not-use", ""),
		Tokens: newNeonWebTokenCache(),
		NewAPI: func(string) neonAPI { return api },
		QuerySchemas: func(ctx context.Context, cred PGCredential, database string) ([]string, error) {
			calls = append(calls, schemaQueryCall{cred, database})
			return query(ctx, cred, database)
		},
	}, &calls
}

func reuseEnvWith(db string) Env {
	env := Env{Project: "myapp", Env: "dev"}
	env.Neon = &Neon{Mode: NeonModeReuse, ProjectID: "fake-project-000000", Databases: []NeonDatabase{{Name: db}}}
	return env
}

func TestNeonSchemasListsTheDatabaseSchemasAsTheOwner(t *testing.T) {
	api := qaNeonAPIReady()
	deps, calls := schemaLookupDeps(t, api, func(context.Context, PGCredential, string) ([]string, error) {
		return []string{"public", "pg_toast", "app", "information_schema", "pg_catalog"}, nil
	})

	got := listNeonSchemas(context.Background(), deps, reuseEnvWith("app"), "app")

	if got.Note != "" {
		t.Fatalf("note = %q, want none", got.Note)
	}
	if !reflect.DeepEqual(got.Schemas, []string{"app", "public"}) {
		t.Fatalf("schemas = %q, want [app public]: system schemas left out, sorted", got.Schemas)
	}
	if len(*calls) != 1 {
		t.Fatalf("query ran %d times, want 1", len(*calls))
	}
	c := (*calls)[0]
	if c.database != "app" || c.cred.Host != "ep-fake.example.invalid" || c.cred.User != "neondb_owner" || c.cred.Password != "fake-owner-password-do-not-use" {
		t.Fatalf("query called with %+v on %q, want the owner on the read_write host of database app", c.cred, c.database)
	}
}

func TestNeonSchemasOfADatabaseNotInNeonYet(t *testing.T) {
	deps, calls := schemaLookupDeps(t, qaNeonAPIReady(), func(context.Context, PGCredential, string) ([]string, error) {
		t.Fatal("must not query a database that does not exist")
		return nil, nil
	})

	got := listNeonSchemas(context.Background(), deps, reuseEnvWith("reports"), "reports")

	if len(got.Schemas) != 0 || !strings.Contains(got.Note, "next deploy") {
		t.Fatalf("got %+v, want no schemas and a note that meroku creates the database at the next deploy", got)
	}
	if len(*calls) != 0 {
		t.Fatalf("query ran %d times", len(*calls))
	}
}

func TestNeonSchemasQueryFailureIsANoteWithoutThePassword(t *testing.T) {
	deps, _ := schemaLookupDeps(t, qaNeonAPIReady(), func(context.Context, PGCredential, string) ([]string, error) {
		return nil, errors.New("connection refused for neondb_owner:fake-owner-password-do-not-use")
	})

	got := listNeonSchemas(context.Background(), deps, reuseEnvWith("app"), "app")

	if len(got.Schemas) != 0 || got.Note == "" {
		t.Fatalf("got %+v, want no schemas and a note", got)
	}
	if strings.Contains(got.Note, "fake-owner-password-do-not-use") {
		t.Fatalf("note leaks the password: %q", got.Note)
	}
}

func TestNeonSchemasWithoutNeonInTheEnv(t *testing.T) {
	deps, _ := schemaLookupDeps(t, qaNeonAPIReady(), nil)
	got := listNeonSchemas(context.Background(), deps, Env{Project: "myapp", Env: "dev"}, "app")
	if len(got.Schemas) != 0 || got.Note == "" {
		t.Fatalf("got %+v, want no schemas and a note", got)
	}
}

func TestNeonSchemasJSONHasAnEmptyListNotNull(t *testing.T) {
	b, err := json.Marshal(neonSchemasResponse{Note: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"schemas":[]`) {
		t.Fatalf("json = %s, want schemas as []", b)
	}
}

func TestNeonSQLOverHTTPSendsTheQueryAndReadsTheRows(t *testing.T) {
	var gotConn, gotPath, gotMethod string
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotConn = r.Header.Get("Neon-Connection-String")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"command":"SELECT","rowCount":2,"rows":[{"nspname":"app"},{"nspname":"public"}],"fields":[{"name":"nspname"}]}`))
	}))
	defer server.Close()

	cred := PGCredential{Host: "ep-fake.example.invalid", User: "neondb_owner", Password: "x@y/z"}
	got, err := neonSchemasOverHTTP(context.Background(), server.Client(), server.URL, cred, "chekku-bridge-dev")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"app", "public"}) {
		t.Fatalf("schemas = %q", got)
	}
	if gotMethod != http.MethodPost || gotPath != "/sql" {
		t.Errorf("request = %s %s, want POST /sql", gotMethod, gotPath)
	}
	u, err := url.Parse(gotConn)
	if err != nil {
		t.Fatalf("Neon-Connection-String %q does not parse: %v", gotConn, err)
	}
	pw, _ := u.User.Password()
	if u.Scheme != "postgresql" || u.User.Username() != cred.User || pw != cred.Password ||
		u.Host != cred.Host || u.Path != "/chekku-bridge-dev" || u.RawQuery != "sslmode=require" {
		t.Errorf("Neon-Connection-String = %q, want the owner on the host, database chekku-bridge-dev, sslmode=require", gotConn)
	}
	if strings.Contains(gotConn, cred.Password) {
		t.Errorf("the password must be escaped in the connection string: %q", gotConn)
	}
	if q, _ := gotBody["query"].(string); !strings.Contains(q, "pg_namespace") {
		t.Errorf("query = %q, want a pg_namespace read", q)
	}
}

func TestNeonSQLOverHTTPReportsTheServerMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"password authentication failed"}`))
	}))
	defer server.Close()
	_, err := neonSchemasOverHTTP(context.Background(), server.Client(), server.URL, PGCredential{Host: "h", User: "u", Password: "p"}, "app")
	if err == nil || !strings.Contains(err.Error(), "password authentication failed") {
		t.Fatalf("err = %v, want the server's message", err)
	}
}

func TestNeonSchemasHandlerNeedsADatabase(t *testing.T) {
	old := neonLookupDeps
	t.Cleanup(func() { neonLookupDeps = old })
	neonLookupDeps = func() neonLookupDependencies {
		d, _ := schemaLookupDeps(t, qaNeonAPIReady(), nil)
		d.LoadEnv = func(string) (Env, error) { return reuseEnvWith("app"), nil }
		return d
	}
	req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/api/neon/schemas?env=dev", nil)
	req.RemoteAddr = "127.0.0.1:54000"
	w := httptest.NewRecorder()
	getNeonSchemas(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a missing database", w.Code)
	}
}
