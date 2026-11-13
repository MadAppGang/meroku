package main

// GET /api/neon/lookup?env=<name>[&project_id=<id>][&org_id=<id>]
//
// Live Neon choices for the web UI's Neon panels: organizations, projects,
// the default branch's databases and roles. The body is neonLookupResponse,
// mirrored by web/src/types/neon.ts. It never carries the token, a password
// or a connection string, and nothing here logs.
//
// Loopback only: the route is registered without corsMiddleware, and a
// request gets 403 unless it arrived from a loopback address (RemoteAddr),
// its Host is localhost, 127.0.0.1 or [::1] on the port meroku serves, its
// Origin (when sent) is a loopback http origin, and its Sec-Fetch-Site (when
// sent) is same-origin or none. RemoteAddr refuses other machines on the
// network, which choose their own Host header; the Host and Origin rules
// refuse DNS-rebinding pages in a local browser, whose Host is their own
// name. The web UI's dev server proxies from loopback too. 4xx only for the caller's own mistakes; a Neon failure is a 200 with
// note set.
//
// The lookup never opens a browser and never starts a login: without a usable
// token it answers with auth.state and nothing else.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"
)

const (
	// neonLookupTimeout bounds one lookup, token resolution included.
	neonLookupTimeout = 15 * time.Second
	// neonctlMeInterval is the least time between two Neon CLI `me` runs
	// started by lookups.
	neonctlMeInterval = time.Minute
)

// neonAuthState values, as in web/src/types/neon.ts NeonAuthState.
const (
	neonAuthStateOK             = "ok"
	neonAuthStateNoCredentials  = "no_credentials"
	neonAuthStateNeonctlMissing = "neonctl_missing"
	neonAuthStateExpired        = "expired"
	neonAuthStateError          = "error"
)

type neonLookupAuth struct {
	State            string `json:"state"`
	Source           string `json:"source,omitempty"`
	ExpiresInSeconds *int64 `json:"expires_in_seconds,omitempty"`
	Message          string `json:"message,omitempty"`
}

type neonLookupOrg struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// neonLookupProject is one entry of projects. org_id is always present.
type neonLookupProject struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	RegionID  string `json:"region_id"`
	PGVersion int    `json:"pg_version"`
	OrgID     string `json:"org_id"`
}

// neonLookupProjectRef is the env's own project.
type neonLookupProjectRef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	RegionID  string `json:"region_id"`
	PGVersion int    `json:"pg_version"`
}

type neonLookupDatabase struct {
	Name  string `json:"name"`
	Owner string `json:"owner"`
}

type neonLookupRole struct {
	Name string `json:"name"`
}

type neonLookupResponse struct {
	Auth          neonLookupAuth        `json:"auth"`
	Organizations []neonLookupOrg       `json:"organizations,omitempty"`
	Projects      []neonLookupProject   `json:"projects,omitempty"`
	Project       *neonLookupProjectRef `json:"project,omitempty"`
	Databases     []neonLookupDatabase  `json:"databases,omitempty"`
	Roles         []neonLookupRole      `json:"roles,omitempty"`
	Note          string                `json:"note,omitempty"`
}

// neonLookupDependencies is the outside world a lookup touches.
type neonLookupDependencies struct {
	Auth    neonAuthDeps
	Tokens  *neonWebTokenCache
	NewAPI  func(token string) neonAPI
	LoadEnv func(name string) (Env, error)
}

func defaultNeonLookupDeps() neonLookupDependencies {
	return neonLookupDependencies{
		Auth:    defaultNeonAuthDeps(),
		Tokens:  neonWebTokens,
		NewAPI:  newNeonAPI,
		LoadEnv: loadEnv,
	}
}

// neonLookupDeps is the seam tests replace to keep the handler off Neon,
// the Neon CLI and the project directory.
var neonLookupDeps = defaultNeonLookupDeps

// ---------------------------------------------------------------------------
// Token cache
// ---------------------------------------------------------------------------

// neonWebTokenCache holds the token lookups use, with its expiry. One lookup
// at a time resolves it, so concurrent lookups never start two `<cli> me`
// runs, and `<cli> me` runs at most once per neonctlMeInterval. Between runs
// the Neon CLI login file is only re-read, so a fresh `<cli> login` shows up on
// the next Refresh.
type neonWebTokenCache struct {
	sem   chan struct{}
	tok   NeonToken
	ok    bool
	err   error
	ranAt time.Time
}

func newNeonWebTokenCache() *neonWebTokenCache {
	return &neonWebTokenCache{sem: make(chan struct{}, 1)}
}

var neonWebTokens = newNeonWebTokenCache()

// usable reports whether tok has at least the web minimum left.
func neonWebTokenUsable(tok NeonToken, now time.Time) bool {
	return tok.Value != "" && (tok.ExpiresAt.IsZero() || tok.ExpiresAt.Sub(now) >= neonMinWebLife)
}

// get returns a token with at least neonMinWebLife left, or the reason there
// is none. NEON_API_KEY is used as is and needs no cache.
func (c *neonWebTokenCache) get(ctx context.Context, d neonAuthDeps) (NeonToken, error) {
	if d.UserKey() != "" {
		return resolveNeonToken(ctx, d, neonMinWebLife)
	}

	select {
	case c.sem <- struct{}{}:
	case <-ctx.Done():
		return NeonToken{}, &NeonAuthError{Kind: neonAuthRefreshFailed, Err: ctx.Err()}
	}
	defer func() { <-c.sem }()

	now := d.Now()
	if c.ok && neonWebTokenUsable(c.tok, now) {
		return c.tok, nil
	}

	if !c.ranAt.IsZero() && now.Sub(c.ranAt) < neonctlMeInterval {
		if _, ok := findNeonCLI(d.LookPath); !ok {
			return NeonToken{}, &NeonAuthError{Kind: neonAuthNeonctlMissing}
		}
		tok, err := readNeonctlToken(d)
		if err == nil && neonWebTokenUsable(tok, now) {
			c.tok, c.ok, c.err = tok, true, nil
			return tok, nil
		}
		c.ok = false
		switch {
		case err != nil:
			return NeonToken{}, &NeonAuthError{Kind: neonAuthNoCredentials}
		case !tok.ExpiresAt.After(now):
			return NeonToken{}, &NeonAuthError{Kind: neonAuthExpired}
		default:
			return NeonToken{}, &NeonAuthError{Kind: neonAuthShort, Remaining: tok.ExpiresAt.Sub(now)}
		}
	}

	tok, err := resolveNeonToken(ctx, d, neonMinWebLife)
	c.ranAt = now
	c.tok, c.ok, c.err = tok, err == nil, err
	if err != nil {
		return NeonToken{}, err
	}
	return tok, nil
}

// forget drops a cached token Neon has refused, so the next lookup resolves
// the login again.
func (c *neonWebTokenCache) forget(tok NeonToken) {
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
		if c.tok.Value == tok.Value {
			c.tok, c.ok, c.ranAt = NeonToken{}, false, time.Time{}
		}
	default:
		// Another lookup is resolving the token right now.
	}
}

// neonLookupAuthFor turns the token resolution's outcome into auth. Messages
// are fixed sentences: they never quote Neon CLI output or an error string.
func neonLookupAuthFor(tok NeonToken, err error, now time.Time) neonLookupAuth {
	if err == nil {
		a := neonLookupAuth{State: neonAuthStateOK, Source: string(tok.Source)}
		if !tok.ExpiresAt.IsZero() {
			secs := int64(tok.ExpiresAt.Sub(now) / time.Second)
			a.ExpiresInSeconds = &secs
		}
		return a
	}
	var authErr *NeonAuthError
	if !errors.As(err, &authErr) {
		return neonLookupAuth{State: neonAuthStateError, Message: "meroku could not read your Neon login."}
	}
	switch authErr.Kind {
	case neonAuthNeonctlMissing:
		return neonLookupAuth{State: neonAuthStateNeonctlMissing,
			Message: "NEON_API_KEY is not set and the Neon CLI is not installed."}
	case neonAuthNoCredentials:
		return neonLookupAuth{State: neonAuthStateNoCredentials,
			Message: "You are not logged in to Neon."}
	case neonAuthExpired:
		return neonLookupAuth{State: neonAuthStateExpired,
			Message: "Your Neon login has expired and could not be refreshed."}
	case neonAuthShort:
		return neonLookupAuth{State: neonAuthStateExpired,
			Message: "Your Neon login expires in under a minute. Refresh again once it has expired."}
	default:
		return neonLookupAuth{State: neonAuthStateError,
			Message: "meroku could not refresh your Neon login."}
	}
}

// ---------------------------------------------------------------------------
// Handler
// ---------------------------------------------------------------------------

// neonLookupLoopback reports whether r was sent to meroku on this machine by
// a page meroku serves. See the file comment for the rules.
func neonLookupLoopback(r *http.Request) bool {
	remote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	if ip := net.ParseIP(remote); ip == nil || !ip.IsLoopback() {
		return false
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
	default:
		return false
	}
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil || !isLoopbackHostName(host) {
		return false
	}
	// The port the request reached, when the server recorded it. A Host on
	// another port names some other server.
	if addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		if _, localPort, err := net.SplitHostPort(addr.String()); err == nil && localPort != port {
			return false
		}
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	// The web UI's dev server proxies /api from its own loopback port, so an
	// Origin is checked for being loopback, not for matching Host's port.
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" {
		return false
	}
	return isLoopbackHostName(u.Hostname())
}

// isLoopbackHostName reports whether host (no port, no brackets) is a name
// that always means this machine.
func isLoopbackHostName(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

func getNeonLookup(w http.ResponseWriter, r *http.Request) {
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
	if envName == "" {
		writeAPIError(w, http.StatusBadRequest, "env parameter is required")
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

	ctx, cancel := context.WithTimeout(r.Context(), neonLookupTimeout)
	defer cancel()
	resp := lookupNeon(ctx, deps, env, neonLookupRequest{
		ProjectID: strings.TrimSpace(q.Get("project_id")),
		OrgID:     strings.TrimSpace(q.Get("org_id")),
	})
	writeJSON(w, http.StatusOK, resp)
}

// neonLookupRequest is the optional part of the query.
type neonLookupRequest struct {
	// ProjectID looks up this project instead of the env's.
	ProjectID string
	// OrgID lists this organization's projects.
	OrgID string
}

// neonLookupNotes collects the reasons parts of a response are missing.
type neonLookupNotes struct{ list []string }

func (n *neonLookupNotes) add(format string, args ...interface{}) {
	n.list = append(n.list, fmt.Sprintf(format, args...))
}

// failed records a Neon failure. A timeout is said in words; any other error
// text has already been stripped of the token by neon_api.go and is redacted
// again here.
func (n *neonLookupNotes) failed(what string, err error) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		n.add("Neon did not answer in time while reading %s.", what)
		return
	}
	n.add("Could not read %s: %s.", what, strings.TrimSuffix(err.Error(), "."))
}

func (n *neonLookupNotes) String() string {
	return redactSecrets(strings.Join(n.list, " "))
}

// neonUnauthorized reports whether Neon refused the token itself.
func neonUnauthorized(err error) bool {
	var apiErr *neonAPIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized
}

// lookupNeon builds the response for env. Every Neon failure becomes a note;
// the parts that could be read are still returned.
func lookupNeon(ctx context.Context, deps neonLookupDependencies, env Env, req neonLookupRequest) neonLookupResponse {
	tok, err := deps.Tokens.get(ctx, deps.Auth)
	resp := neonLookupResponse{Auth: neonLookupAuthFor(tok, err, deps.Auth.Now())}
	if err != nil {
		return resp
	}

	api := deps.NewAPI(tok.Value)
	var notes neonLookupNotes

	orgs, err := api.ListOrganizations(ctx)
	if neonUnauthorized(err) {
		deps.Tokens.forget(tok)
		return neonLookupResponse{Auth: neonLookupAuth{
			State:   neonAuthStateError,
			Message: fmt.Sprintf("Neon refused this login. Log in again with %s login, or check NEON_API_KEY.", neonCLI()),
		}}
	}
	if err != nil {
		notes.failed("your Neon organizations", err)
	}
	for _, o := range orgs {
		resp.Organizations = append(resp.Organizations, neonLookupOrg{ID: o.ID, Name: o.Name})
	}

	listed := neonListLookupProjects(ctx, api, env, req, orgs, &notes)
	for _, p := range listed.all {
		resp.Projects = append(resp.Projects, neonLookupProject{
			ID: p.ID, Name: p.Name, RegionID: p.RegionID, PGVersion: p.PGVersion, OrgID: p.OrgID,
		})
	}

	lk := newNeonLookups(api)
	project, found := neonResolveLookupProject(ctx, lk, env, req, listed, &notes)
	if found {
		resp.Project = &neonLookupProjectRef{
			ID: project.ID, Name: project.Name, RegionID: project.RegionID, PGVersion: project.PGVersion,
		}
		resp.Databases, resp.Roles = neonDefaultBranchContents(ctx, lk, api, project.ID, &notes)
	}

	resp.Note = notes.String()
	return resp
}

// neonListedProjects is what the project listing returned.
type neonListedProjects struct {
	all []NeonProjectInfo
	// byOrg holds every organization listed in full, "" for the personal
	// account.
	byOrg map[string][]NeonProjectInfo
}

// neonListLookupProjects lists projects: the org_id parameter's, else the
// env's create-mode organization's, else every organization the user belongs
// to (the personal account when there is none).
func neonListLookupProjects(ctx context.Context, api neonAPI, env Env, req neonLookupRequest, orgs []NeonOrg, notes *neonLookupNotes) neonListedProjects {
	var orgIDs []string
	switch {
	case req.OrgID != "":
		orgIDs = []string{req.OrgID}
	case neonEnvCreateProject(env) != nil && neonEnvCreateProject(env).OrgID != "":
		orgIDs = []string{neonEnvCreateProject(env).OrgID}
	case len(orgs) > 0:
		for _, o := range orgs {
			orgIDs = append(orgIDs, o.ID)
		}
	default:
		orgIDs = []string{""}
	}

	listed := neonListedProjects{byOrg: map[string][]NeonProjectInfo{}}
	for _, orgID := range orgIDs {
		projects, err := neonListOrgProjects(ctx, api, orgID)
		if err != nil {
			if orgID == "" {
				notes.failed("your Neon projects", err)
			} else {
				notes.failed("the projects of organization "+orgID, err)
			}
			continue
		}
		listed.byOrg[orgID] = projects
		listed.all = append(listed.all, projects...)
	}
	return listed
}

// neonListOrgProjects lists one organization's projects, each carrying the
// organization it was listed under when Neon leaves org_id out.
func neonListOrgProjects(ctx context.Context, api neonAPI, orgID string) ([]NeonProjectInfo, error) {
	projects, err := api.ListProjects(ctx, orgID)
	if err != nil {
		return nil, err
	}
	for i := range projects {
		if projects[i].OrgID == "" {
			projects[i].OrgID = orgID
		}
	}
	return projects, nil
}

// neonEnvCreateProject is the env's create-mode project block, or nil.
func neonEnvCreateProject(env Env) *NeonProject {
	if env.Neon == nil || env.Neon.Mode != NeonModeCreate {
		return nil
	}
	return env.Neon.Project
}

// neonResolveLookupProject finds the env's project: the project_id parameter;
// else the env's neon.project_id; else, in create mode, the one project in
// project.org_id named project.name. found is false when there is none or it
// cannot be told which; a reason that needs the user goes to notes.
func neonResolveLookupProject(ctx context.Context, lk *neonLookups, env Env, req neonLookupRequest, listed neonListedProjects, notes *neonLookupNotes) (NeonProjectInfo, bool) {
	id := req.ProjectID
	if id == "" && env.Neon != nil {
		id = env.Neon.ProjectID
	}
	if id != "" {
		info, found, err := lk.project(ctx, id)
		switch {
		case err != nil:
			notes.failed("project "+id, err)
		case !found:
			notes.add("Neon has no project %s, or this login cannot see it.", id)
		}
		return info, found && err == nil
	}

	want := neonEnvCreateProject(env)
	if want == nil || want.Name == "" || want.OrgID == "" {
		return NeonProjectInfo{}, false
	}
	projects, ok := listed.byOrg[want.OrgID]
	if !ok {
		var err error
		if projects, err = neonListOrgProjects(ctx, lk.api, want.OrgID); err != nil {
			notes.failed("the projects of organization "+want.OrgID, err)
			return NeonProjectInfo{}, false
		}
	}
	var named []NeonProjectInfo
	for _, p := range projects {
		if p.Name == want.Name {
			named = append(named, p)
		}
	}
	switch len(named) {
	case 0:
		// Not created yet.
		return NeonProjectInfo{}, false
	case 1:
		return named[0], true
	default:
		notes.add("%d projects in organization %s are named %s, so meroku cannot tell which one is this environment's.",
			len(named), want.OrgID, want.Name)
		return NeonProjectInfo{}, false
	}
}

// neonDefaultBranchContents lists the databases and roles of the project's
// default branch, sorted by name.
func neonDefaultBranchContents(ctx context.Context, lk *neonLookups, api neonAPI, projectID string, notes *neonLookupNotes) ([]neonLookupDatabase, []neonLookupRole) {
	branch, err := lk.defaultBranch(ctx, projectID)
	if err != nil {
		notes.failed("the default branch of project "+projectID, err)
		return nil, nil
	}

	var databases []neonLookupDatabase
	dbs, found, err := lk.databasesOn(ctx, projectID, branch)
	switch {
	case err != nil:
		notes.failed("the databases of project "+projectID, err)
	case !found:
		notes.add("Neon no longer has the default branch of project %s.", projectID)
	default:
		for _, d := range dbs {
			databases = append(databases, neonLookupDatabase{Name: d.Name, Owner: d.OwnerName})
		}
		sort.Slice(databases, func(i, j int) bool { return databases[i].Name < databases[j].Name })
	}

	var roles []neonLookupRole
	rs, err := api.ListRoles(ctx, projectID, branch)
	if err != nil {
		notes.failed("the roles of project "+projectID, err)
	} else {
		for _, r := range rs {
			roles = append(roles, neonLookupRole{Name: r.Name})
		}
		sort.Slice(roles, func(i, j int) bool { return roles[i].Name < roles[j].Name })
	}
	return databases, roles
}
