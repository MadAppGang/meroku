package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"
)

// GET /api/neon/schemas?env=<env>&database=<db> lists the schemas of one
// database of the env's Neon project, so the web UI can offer them by name.
// It reads as the owner role through Neon's SQL-over-HTTP endpoint and runs
// one fixed read-only query. Anything that cannot be read becomes the note;
// the answer is still 200 with an empty list.

// neonSchemasResponse is the endpoint's answer.
type neonSchemasResponse struct {
	Schemas []string `json:"schemas"`
	Note    string   `json:"note"`
}

func (r neonSchemasResponse) MarshalJSON() ([]byte, error) {
	type plain neonSchemasResponse
	if r.Schemas == nil {
		r.Schemas = []string{}
	}
	return json.Marshal(plain(r))
}

// neonSchemaQueryTimeout bounds the SQL call; a suspended compute wakes in a
// few seconds.
const neonSchemaQueryTimeout = 20 * time.Second

func getNeonSchemas(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !neonLookupLoopback(r) {
		writeAPIError(w, http.StatusForbidden, "only the meroku web UI on this machine may call this")
		return
	}
	w.Header().Set("Cache-Control", "no-store")

	q := r.URL.Query()
	envName := q.Get("env")
	database := strings.TrimSpace(q.Get("database"))
	if envName == "" || database == "" {
		writeAPIError(w, http.StatusBadRequest, "env and database parameters are required")
		return
	}
	names, err := listEnvironmentNames()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, redactSecrets(err.Error()))
		return
	}
	if !slices.Contains(names, envName) {
		writeAPIError(w, http.StatusBadRequest, "unknown environment: "+envName)
		return
	}

	deps := neonLookupDeps()
	env, err := deps.LoadEnv(envName)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest,
			"failed to load environment "+envName+": "+redactSecrets(err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), neonLookupTimeout+neonSchemaQueryTimeout)
	defer cancel()
	writeJSON(w, http.StatusOK, listNeonSchemas(ctx, deps, env, database))
}

// listNeonSchemas finds the env's project, its default branch and database,
// and reads the database's schemas as the owner role.
func listNeonSchemas(ctx context.Context, deps neonLookupDependencies, env Env, database string) neonSchemasResponse {
	if env.Neon == nil {
		return neonSchemasResponse{Note: "This environment does not use Neon yet."}
	}
	tok, err := deps.Tokens.get(ctx, deps.Auth)
	if err != nil {
		return neonSchemasResponse{Note: fmt.Sprintf("Log in to Neon to see the schemas (%s login).", neonCLI())}
	}
	api := deps.NewAPI(tok.Value)
	lk := newNeonLookups(api)
	var notes neonLookupNotes

	project, found := neonResolveLookupProject(ctx, lk, env, neonLookupRequest{}, neonListedProjects{}, &notes)
	if !found {
		if notes.String() == "" {
			notes.add("The Neon project does not exist yet; meroku creates it and the database at the next deploy.")
		}
		return neonSchemasResponse{Note: notes.String()}
	}
	branch, err := lk.defaultBranch(ctx, project.ID)
	if err != nil {
		notes.failed("the default branch of project "+project.ID, err)
		return neonSchemasResponse{Note: notes.String()}
	}
	dbs, found, err := lk.databasesOn(ctx, project.ID, branch)
	if err != nil || !found {
		notes.failed("the databases of project "+project.ID, errOr(err, "the default branch is gone"))
		return neonSchemasResponse{Note: notes.String()}
	}
	if _, ok := neonFindDatabase(dbs, database); !ok {
		return neonSchemasResponse{Note: fmt.Sprintf("The database %s is not in Neon yet; meroku creates it at the next deploy.", database)}
	}
	host, err := lk.readWriteHost(ctx, project.ID, branch)
	if err != nil || host == "" {
		notes.failed("the database's address", errOr(err, "the default branch has no read_write endpoint"))
		return neonSchemasResponse{Note: notes.String()}
	}
	owner := normalizeNeon(env.Neon).OwnerRole
	password, err := api.RevealRolePassword(ctx, project.ID, branch, owner)
	if err != nil {
		notes.failed("the password of "+owner, err)
		return neonSchemasResponse{Note: notes.String()}
	}
	registerSecret(password)

	qctx, cancel := context.WithTimeout(ctx, neonSchemaQueryTimeout)
	defer cancel()
	all, err := deps.QuerySchemas(qctx, PGCredential{Host: host, User: owner, Password: password}, database)
	if err != nil {
		notes.failed("the schemas of "+database, err)
		return neonSchemasResponse{Note: notes.String()}
	}
	var out []string
	for _, s := range all {
		if strings.HasPrefix(s, "pg_") || s == "information_schema" {
			continue
		}
		out = append(out, s)
	}
	sort.Strings(out)
	return neonSchemasResponse{Schemas: out}
}

func errOr(err error, msg string) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("%s", msg)
}

// neonSchemasQuery is the read-only query listSchemas runs.
const neonSchemasQuery = "SELECT nspname FROM pg_catalog.pg_namespace ORDER BY nspname"

// queryNeonSchemas runs neonSchemasQuery on the database's host.
func queryNeonSchemas(ctx context.Context, cred PGCredential, database string) ([]string, error) {
	return neonSchemasOverHTTP(ctx, http.DefaultClient, "https://"+cred.Host, cred, database)
}

// neonSchemasOverHTTP posts neonSchemasQuery to baseURL/sql, Neon's
// SQL-over-HTTP endpoint, and returns the nspname column.
func neonSchemasOverHTTP(ctx context.Context, hc *http.Client, baseURL string, cred PGCredential, database string) ([]string, error) {
	conn := (&url.URL{
		Scheme:   "postgresql",
		User:     url.UserPassword(cred.User, cred.Password),
		Host:     cred.Host,
		Path:     "/" + database,
		RawQuery: "sslmode=require",
	}).String()
	body, err := json.Marshal(map[string]interface{}{"query": neonSchemasQuery, "params": []interface{}{}})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/sql", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Neon-Connection-String", conn)

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s", redactSecrets(err.Error()))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(neonErrorMessage(raw))
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf("%s", redactSecrets(msg))
	}
	var out struct {
		Rows []struct {
			Nspname string `json:"nspname"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("unreadable answer from Neon: %w", err)
	}
	names := make([]string, 0, len(out.Rows))
	for _, r := range out.Rows {
		names = append(names, r.Nspname)
	}
	return names, nil
}
