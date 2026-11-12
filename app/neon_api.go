package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Neon HTTP API client.
//
// Read-only: meroku reads projects, branches, endpoints, databases and roles,
// and reveals the owner role's password. It never creates, changes or deletes
// anything through the API, so the client has no method that could.
//
// The token goes only into the Authorization header. It is registered for
// redaction when the client is built, and no error this file returns carries
// it, a password or a raw response body.

const (
	neonAPIBaseURL = "https://console.neon.tech/api/v2"
	// neonAPIRequestTimeout bounds one HTTP attempt.
	neonAPIRequestTimeout = 15 * time.Second
	// neonAPIAttempts is how many times a request is sent when Neon answers
	// 429 or 5xx, or the connection fails.
	neonAPIAttempts = 3
	// neonAPIMaxPages caps how many pages one List call follows.
	neonAPIMaxPages = 50
	// neonAPIPageLimit is the page size asked for on paged lists.
	neonAPIPageLimit = 100
	// neonAPIMaxErrorBody is how much of an error body is read for its
	// message.
	neonAPIMaxErrorBody = 64 << 10
)

// ErrNeonNotFound is returned (wrapped) when Neon answers 404: the object
// does not exist, or the token cannot see it.
var ErrNeonNotFound = errors.New("neon: not found")

// NeonProjectInfo is a Neon project as the API reports it.
type NeonProjectInfo struct {
	ID              string
	Name            string
	OrgID           string
	RegionID        string
	PGVersion       int
	DefaultBranchID string
}

// NeonOrg is an organization the token's user belongs to.
type NeonOrg struct {
	ID   string
	Name string
}

// NeonBranchInfo is one branch of a project.
type NeonBranchInfo struct {
	ID      string
	Name    string
	Default bool
}

// NeonEndpointInfo is one compute endpoint of a branch.
type NeonEndpointInfo struct {
	ID       string
	BranchID string
	Host     string
	// Type is "read_write" or "read_only".
	Type string
}

// NeonDatabaseInfo is one database on a branch.
type NeonDatabaseInfo struct {
	Name      string
	OwnerName string
	BranchID  string
}

// NeonRoleInfo is one role on a branch.
type NeonRoleInfo struct {
	Name      string
	Protected bool
}

// neonAPI is everything meroku asks Neon. There are no write or delete
// methods: meroku never changes Neon data through the API.
type neonAPI interface {
	GetProject(ctx context.Context, id string) (NeonProjectInfo, error)
	ListProjects(ctx context.Context, orgID string) ([]NeonProjectInfo, error)
	ListOrganizations(ctx context.Context) ([]NeonOrg, error)
	ListBranches(ctx context.Context, projectID string) ([]NeonBranchInfo, error)
	ListEndpoints(ctx context.Context, projectID, branchID string) ([]NeonEndpointInfo, error)
	ListDatabases(ctx context.Context, projectID, branchID string) ([]NeonDatabaseInfo, error)
	ListRoles(ctx context.Context, projectID, branchID string) ([]NeonRoleInfo, error)
	RevealRolePassword(ctx context.Context, projectID, branchID, role string) (string, error)
}

// neonHTTPClient implements neonAPI over net/http.
type neonHTTPClient struct {
	baseURL string
	token   string
	http    *http.Client
	// sleep waits between attempts; it returns early with ctx's error.
	sleep func(ctx context.Context, d time.Duration) error
}

// newNeonAPI is the client for the real Neon API.
func newNeonAPI(token string) neonAPI {
	return newNeonHTTPClient(neonAPIBaseURL, token, nil)
}

// newNeonHTTPClient builds a client for baseURL. A nil hc means a default
// client; each attempt is still bounded by neonAPIRequestTimeout.
func newNeonHTTPClient(baseURL, token string, hc *http.Client) *neonHTTPClient {
	registerSecret(token)
	if hc == nil {
		hc = &http.Client{}
	}
	return &neonHTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    hc,
		sleep:   sleepContext,
	}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// neonAPIError is a non-2xx answer from Neon. It holds the status and Neon's
// own message, redacted, never the raw body.
type neonAPIError struct {
	Method  string
	Path    string
	Status  int
	Message string
}

func (e *neonAPIError) Error() string {
	msg := fmt.Sprintf("neon API: %s %s: HTTP %d", e.Method, e.Path, e.Status)
	if e.Message != "" {
		msg += ": " + e.Message
	}
	return msg
}

func (e *neonAPIError) Unwrap() error {
	if e.Status == http.StatusNotFound {
		return ErrNeonNotFound
	}
	return nil
}

func neonRetryable(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

// get sends GET <base><path>?<query>, retrying 429, 5xx and transport
// failures, and decodes a 2xx JSON body into out.
func (c *neonHTTPClient) get(ctx context.Context, path string, query url.Values, out interface{}) error {
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	var lastErr error
	for attempt := 0; attempt < neonAPIAttempts; attempt++ {
		if attempt > 0 {
			if err := c.sleep(ctx, time.Duration(attempt)*time.Second); err != nil {
				return err
			}
		}
		retry, err := c.attempt(ctx, path, target, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retry {
			return err
		}
	}
	return lastErr
}

// attempt sends one request. retry reports whether a failure may succeed on
// another try.
func (c *neonHTTPClient) attempt(ctx context.Context, path, target string, out interface{}) (retry bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, neonAPIRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false, fmt.Errorf("neon API: GET %s: building the request failed", path)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// A transport error can quote the URL, never the header; redact anyway.
		return true, fmt.Errorf("neon API: GET %s: %s", path, redactSecrets(neonTransportReason(err)))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, neonAPIMaxErrorBody))
		apiErr := &neonAPIError{
			Method:  http.MethodGet,
			Path:    path,
			Status:  resp.StatusCode,
			Message: neonErrorMessage(body),
		}
		return neonRetryable(resp.StatusCode), apiErr
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return false, fmt.Errorf("neon API: GET %s: the response is not the JSON meroku expects", path)
	}
	return false, nil
}

// neonTransportReason describes a transport failure without the request URL.
func neonTransportReason(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timed out"
	}
	return err.Error()
}

// neonErrorMessage pulls Neon's `message` out of an error body, redacted and
// cut to one line.
func neonErrorMessage(body []byte) string {
	var parsed struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &parsed) != nil || parsed.Message == "" {
		return ""
	}
	msg := strings.Join(strings.Fields(parsed.Message), " ")
	if len(msg) > 300 {
		msg = msg[:300] + "…"
	}
	return redactSecrets(msg)
}

// neonPagination covers both page styles the API uses: `cursor` (projects)
// and `next` (branches).
type neonPagination struct {
	Cursor string `json:"cursor"`
	Next   string `json:"next"`
}

// listPaged follows a paged list. fetch requests one page with the given
// cursor ("" for the first) and returns how many items it held and the
// pagination block. It stops on an empty or short page, no or an unchanged
// cursor, and fails past neonAPIMaxPages.
func listPaged(path string, fetch func(cursor string) (int, *neonPagination, error)) error {
	cursor := ""
	for page := 0; page < neonAPIMaxPages; page++ {
		n, p, err := fetch(cursor)
		if err != nil {
			return err
		}
		if n == 0 || p == nil {
			return nil
		}
		next := p.Next
		if next == "" {
			next = p.Cursor
			// Cursor style returns a cursor on every page; a short page is the
			// last one.
			if n < neonAPIPageLimit {
				return nil
			}
		}
		if next == "" || next == cursor {
			return nil
		}
		cursor = next
	}
	return fmt.Errorf("neon API: GET %s: more than %d pages", path, neonAPIMaxPages)
}

// Wire types.

type neonWireProject struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	OrgID           string `json:"org_id"`
	RegionID        string `json:"region_id"`
	PGVersion       int    `json:"pg_version"`
	DefaultBranchID string `json:"default_branch_id"`
}

func (p neonWireProject) info() NeonProjectInfo {
	return NeonProjectInfo{
		ID: p.ID, Name: p.Name, OrgID: p.OrgID, RegionID: p.RegionID,
		PGVersion: p.PGVersion, DefaultBranchID: p.DefaultBranchID,
	}
}

func neonPathEscape(s string) string { return url.PathEscape(s) }

func neonBranchPath(projectID, branchID string) string {
	return "/projects/" + neonPathEscape(projectID) + "/branches/" + neonPathEscape(branchID)
}

// GetProject is GET /projects/{id}. A 404 wraps ErrNeonNotFound.
func (c *neonHTTPClient) GetProject(ctx context.Context, id string) (NeonProjectInfo, error) {
	var resp struct {
		Project neonWireProject `json:"project"`
	}
	if err := c.get(ctx, "/projects/"+neonPathEscape(id), nil, &resp); err != nil {
		return NeonProjectInfo{}, err
	}
	return resp.Project.info(), nil
}

// ListProjects is GET /projects?org_id=, every page. An empty orgID lists the
// projects of the user's personal account.
func (c *neonHTTPClient) ListProjects(ctx context.Context, orgID string) ([]NeonProjectInfo, error) {
	var out []NeonProjectInfo
	err := listPaged("/projects", func(cursor string) (int, *neonPagination, error) {
		q := url.Values{}
		if orgID != "" {
			q.Set("org_id", orgID)
		}
		q.Set("limit", strconv.Itoa(neonAPIPageLimit))
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var resp struct {
			Projects   []neonWireProject `json:"projects"`
			Pagination *neonPagination   `json:"pagination"`
		}
		if err := c.get(ctx, "/projects", q, &resp); err != nil {
			return 0, nil, err
		}
		for _, p := range resp.Projects {
			out = append(out, p.info())
		}
		return len(resp.Projects), resp.Pagination, nil
	})
	return out, err
}

// ListOrganizations is GET /users/me/organizations.
func (c *neonHTTPClient) ListOrganizations(ctx context.Context) ([]NeonOrg, error) {
	var resp struct {
		Organizations []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"organizations"`
	}
	if err := c.get(ctx, "/users/me/organizations", nil, &resp); err != nil {
		return nil, err
	}
	out := make([]NeonOrg, 0, len(resp.Organizations))
	for _, o := range resp.Organizations {
		out = append(out, NeonOrg{ID: o.ID, Name: o.Name})
	}
	return out, nil
}

// ListBranches is GET /projects/{id}/branches, every page. Default is set
// from `default`, or from the older `primary` when no branch has `default`.
func (c *neonHTTPClient) ListBranches(ctx context.Context, projectID string) ([]NeonBranchInfo, error) {
	type wireBranch struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Default bool   `json:"default"`
		Primary bool   `json:"primary"`
	}
	var all []wireBranch
	path := "/projects/" + neonPathEscape(projectID) + "/branches"
	err := listPaged(path, func(cursor string) (int, *neonPagination, error) {
		q := url.Values{}
		q.Set("limit", strconv.Itoa(neonAPIPageLimit))
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var resp struct {
			Branches   []wireBranch    `json:"branches"`
			Pagination *neonPagination `json:"pagination"`
		}
		if err := c.get(ctx, path, q, &resp); err != nil {
			return 0, nil, err
		}
		all = append(all, resp.Branches...)
		return len(resp.Branches), resp.Pagination, nil
	})
	if err != nil {
		return nil, err
	}

	anyDefault := false
	for _, b := range all {
		anyDefault = anyDefault || b.Default
	}
	out := make([]NeonBranchInfo, 0, len(all))
	for _, b := range all {
		def := b.Default
		if !anyDefault {
			def = b.Primary
		}
		out = append(out, NeonBranchInfo{ID: b.ID, Name: b.Name, Default: def})
	}
	return out, nil
}

// ListEndpoints is GET /projects/{id}/branches/{bid}/endpoints.
func (c *neonHTTPClient) ListEndpoints(ctx context.Context, projectID, branchID string) ([]NeonEndpointInfo, error) {
	var resp struct {
		Endpoints []struct {
			ID       string `json:"id"`
			BranchID string `json:"branch_id"`
			Host     string `json:"host"`
			Type     string `json:"type"`
		} `json:"endpoints"`
	}
	if err := c.get(ctx, neonBranchPath(projectID, branchID)+"/endpoints", nil, &resp); err != nil {
		return nil, err
	}
	out := make([]NeonEndpointInfo, 0, len(resp.Endpoints))
	for _, e := range resp.Endpoints {
		out = append(out, NeonEndpointInfo{ID: e.ID, BranchID: e.BranchID, Host: e.Host, Type: e.Type})
	}
	return out, nil
}

// ListDatabases is GET /projects/{id}/branches/{bid}/databases.
func (c *neonHTTPClient) ListDatabases(ctx context.Context, projectID, branchID string) ([]NeonDatabaseInfo, error) {
	var resp struct {
		Databases []struct {
			Name      string `json:"name"`
			OwnerName string `json:"owner_name"`
			BranchID  string `json:"branch_id"`
		} `json:"databases"`
	}
	if err := c.get(ctx, neonBranchPath(projectID, branchID)+"/databases", nil, &resp); err != nil {
		return nil, err
	}
	out := make([]NeonDatabaseInfo, 0, len(resp.Databases))
	for _, d := range resp.Databases {
		out = append(out, NeonDatabaseInfo{Name: d.Name, OwnerName: d.OwnerName, BranchID: d.BranchID})
	}
	return out, nil
}

// ListRoles is GET /projects/{id}/branches/{bid}/roles.
func (c *neonHTTPClient) ListRoles(ctx context.Context, projectID, branchID string) ([]NeonRoleInfo, error) {
	var resp struct {
		Roles []struct {
			Name      string `json:"name"`
			Protected bool   `json:"protected"`
		} `json:"roles"`
	}
	if err := c.get(ctx, neonBranchPath(projectID, branchID)+"/roles", nil, &resp); err != nil {
		return nil, err
	}
	out := make([]NeonRoleInfo, 0, len(resp.Roles))
	for _, r := range resp.Roles {
		out = append(out, NeonRoleInfo{Name: r.Name, Protected: r.Protected})
	}
	return out, nil
}

// RevealRolePassword is GET …/roles/{role}/reveal_password. The password is
// registered for redaction before it is returned.
func (c *neonHTTPClient) RevealRolePassword(ctx context.Context, projectID, branchID, role string) (string, error) {
	var resp struct {
		Password string `json:"password"`
	}
	path := neonBranchPath(projectID, branchID) + "/roles/" + neonPathEscape(role) + "/reveal_password"
	if err := c.get(ctx, path, nil, &resp); err != nil {
		return "", err
	}
	if resp.Password == "" {
		return "", fmt.Errorf("neon API: GET %s: no password in the response", path)
	}
	registerSecret(resp.Password)
	return resp.Password, nil
}
