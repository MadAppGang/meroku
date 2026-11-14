package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"
)

// Write-only management of a vault's secret values from the web UI.
//
//	GET    /api/vaults/secrets?env=<env>&vault=<vault>              names, metadata, declared/set
//	PUT    /api/vaults/secrets?env=<env>&vault=<vault>  {name,value} set or replace a value
//	DELETE /api/vaults/secrets?env=<env>&vault=<vault>&name=<name>  delete a value
//
// The env YAML declares each vault's secret names (vaults[].secrets); this API
// only writes the values, which live in SSM and nowhere else. No route returns
// a value: listing uses ssm:DescribeParameters, which carries metadata only, and
// vaultSecretsSSMAPI has no GetParameter* method, so this file cannot read a
// value even by mistake. The client never sends a path: it is built here from
// the env, its project and a vault declared in the env YAML.

// vaultSecretsSSMAPI is the part of the SSM client the vault secrets API uses.
type vaultSecretsSSMAPI interface {
	DescribeParameters(ctx context.Context, in *ssm.DescribeParametersInput, optFns ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error)
	PutParameter(ctx context.Context, in *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
	DeleteParameter(ctx context.Context, in *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error)
}

// newVaultSecretsClient builds an SSM client for the environment's own AWS
// profile and region, never the shell's ambient credentials or the profile
// selected elsewhere in the UI: the value must land in the account the
// environment deploys to. A variable so tests can replace it.
var newVaultSecretsClient = func(ctx context.Context, e Env) (vaultSecretsSSMAPI, error) {
	var opts []func(*config.LoadOptions) error
	if e.AWSProfile != "" {
		opts = append(opts, config.WithSharedConfigProfile(e.AWSProfile))
	}
	if e.Region != "" {
		opts = append(opts, config.WithRegion(e.Region))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return ssm.NewFromConfig(cfg), nil
}

// loadEnvForVaultSecrets loads <env>.yaml without rewriting it. A variable so
// tests can replace it.
var loadEnvForVaultSecrets = loadEnvReadOnly

// vaultSecretDeletableNamePattern is what DELETE accepts. It is wider than
// vaultSecretNamePattern so a parameter written before secret names were
// declared (DATABASE_URL, say) can still be deleted from the tab; it is still a
// single path segment, never a separator or "..".
var vaultSecretDeletableNamePattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

// vaultSecretMaxValueLength is the standard-tier parameter value limit.
const vaultSecretMaxValueLength = 4096

// vaultSecretMaxBodyBytes caps a request body: a value of the maximum length in
// multi-byte characters, plus the name and JSON framing.
const vaultSecretMaxBodyBytes = 64 << 10

// vaultSecretStillListedWarning is added to a DELETE answer for a declared name.
const vaultSecretStillListedWarning = "still listed in the YAML; connected tasks will fail to start until it is set again or removed and applied"

// VaultSecret is one secret of a vault, without its value: declared in the
// YAML, present in SSM, or both. The metadata is there only when it is set.
type VaultSecret struct {
	Name         string `json:"name"`
	EnvName      string `json:"envName"`
	Declared     bool   `json:"declared"`
	Set          bool   `json:"set"`
	Type         string `json:"type,omitempty"`
	Version      int64  `json:"version,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
}

// VaultSecretsResponse is the GET answer.
type VaultSecretsResponse struct {
	Vault   string        `json:"vault"`
	Path    string        `json:"path"`
	Secrets []VaultSecret `json:"secrets"`
}

// vaultSecretWriteRequest is the PUT body.
type vaultSecretWriteRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type vaultSecretWriteResponse struct {
	Name    string `json:"name"`
	Version int64  `json:"version"`
}

type vaultSecretDeleteResponse struct {
	Name    string `json:"name"`
	Deleted bool   `json:"deleted"`
	Warning string `json:"warning,omitempty"`
}

func writeVaultSecretsJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeVaultSecretsError(w http.ResponseWriter, status int, msg string) {
	writeVaultSecretsJSON(w, status, ErrorResponse{Error: msg})
}

// vaultSecretsPrefix is the vault's path with a trailing slash.
func vaultSecretsPrefix(e Env, vault string) string {
	return fmt.Sprintf("/%s/%s/vault/%s/", e.Env, e.Project, vault)
}

// vaultSecretsTarget is a request's resolved env and vault.
type vaultSecretsTarget struct {
	env      Env
	vault    string
	declared []string // the vault's secrets list; nil when the YAML has none
}

func (t vaultSecretsTarget) isDeclared(name string) bool {
	for _, s := range t.declared {
		if s == name {
			return true
		}
	}
	return false
}

func (t vaultSecretsTarget) path(name string) string {
	return vaultSecretsPrefix(t.env, t.vault) + name
}

// resolveVaultSecretsTarget checks env and vault from the query and loads the
// env. It writes the error response itself and returns false on failure.
func resolveVaultSecretsTarget(w http.ResponseWriter, r *http.Request) (vaultSecretsTarget, bool) {
	q := r.URL.Query()
	envName := q.Get("env")
	vault := q.Get("vault")
	switch {
	case envName == "":
		writeVaultSecretsError(w, http.StatusBadRequest, "env is required")
		return vaultSecretsTarget{}, false
	case !validEventAPIEnvName(envName):
		writeVaultSecretsError(w, http.StatusBadRequest, "invalid env name: use letters, digits, hyphens and underscores")
		return vaultSecretsTarget{}, false
	case vault == "":
		writeVaultSecretsError(w, http.StatusBadRequest, "vault is required")
		return vaultSecretsTarget{}, false
	}

	e, err := loadEnvForVaultSecrets(envName)
	if err != nil {
		writeVaultSecretsError(w, http.StatusNotFound, fmt.Sprintf("environment %s not found: %v", envName, err))
		return vaultSecretsTarget{}, false
	}
	if e.Env == "" {
		e.Env = envName
	}
	// Only a vault the YAML declares, by its exact name: the name becomes part
	// of the path, and the declared names all passed the generate rules.
	for _, v := range e.Vaults {
		if v.Name == vault && vaultNamePattern.MatchString(v.Name) {
			return vaultSecretsTarget{env: e, vault: vault, declared: v.Secrets}, true
		}
	}
	writeVaultSecretsError(w, http.StatusNotFound, fmt.Sprintf("vault %q is not declared in %s.yaml", vault, envName))
	return vaultSecretsTarget{}, false
}

// writeVaultSecretsAWSError answers an AWS error. secret, when not empty, is
// removed from the message so that a value can never travel back.
func writeVaultSecretsAWSError(w http.ResponseWriter, err error, secret string) {
	msg := err.Error()
	status := http.StatusInternalServerError
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		if code == "AccessDeniedException" || code == "AccessDenied" {
			status = http.StatusForbidden
			if m := apiErr.ErrorMessage(); m != "" {
				msg = m
			}
		}
	}
	if secret != "" {
		msg = strings.ReplaceAll(msg, secret, "[redacted]")
	}
	writeVaultSecretsError(w, status, msg)
}

// handleVaultSecrets serves /api/vaults/secrets.
func handleVaultSecrets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodPut, http.MethodDelete:
	default:
		writeVaultSecretsError(w, http.StatusMethodNotAllowed, "method not allowed: use GET, PUT or DELETE")
		return
	}

	t, ok := resolveVaultSecretsTarget(w, r)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		listVaultSecrets(w, r, t)
	case http.MethodPut:
		putVaultSecret(w, r, t)
	case http.MethodDelete:
		deleteVaultSecret(w, r, t)
	}
}

func vaultSecretsClient(ctx context.Context, w http.ResponseWriter, e Env) (vaultSecretsSSMAPI, bool) {
	client, err := newVaultSecretsClient(ctx, e)
	if err != nil {
		writeVaultSecretsError(w, http.StatusInternalServerError,
			fmt.Sprintf("failed to load AWS config for profile %q: %v", e.AWSProfile, err))
		return nil, false
	}
	return client, true
}

// listVaultSecrets answers the union of the YAML's secrets list and the
// parameters directly under the vault's path, sorted by name.
func listVaultSecrets(w http.ResponseWriter, r *http.Request, t vaultSecretsTarget) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	client, ok := vaultSecretsClient(ctx, w, t.env)
	if !ok {
		return
	}

	byName := map[string]*VaultSecret{}
	for _, name := range t.declared {
		byName[name] = &VaultSecret{Name: name, EnvName: strings.ToUpper(name), Declared: true}
	}

	prefix := vaultSecretsPrefix(t.env, t.vault)
	paginator := ssm.NewDescribeParametersPaginator(client, &ssm.DescribeParametersInput{
		ParameterFilters: []ssmtypes.ParameterStringFilter{{
			Key:    aws.String("Path"),
			Option: aws.String("OneLevel"),
			Values: []string{strings.TrimSuffix(prefix, "/")},
		}},
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			writeVaultSecretsAWSError(w, err, "")
			return
		}
		for _, p := range page.Parameters {
			full := aws.ToString(p.Name)
			name := strings.TrimPrefix(full, prefix)
			// OneLevel already excludes deeper paths; this guards a response that
			// would not, since a deeper parameter is no workload's secret.
			if name == full || name == "" || strings.Contains(name, "/") {
				continue
			}
			s := byName[name]
			if s == nil {
				s = &VaultSecret{Name: name, EnvName: strings.ToUpper(name)}
				byName[name] = s
			}
			s.Set = true
			s.Type = string(p.Type)
			s.Version = p.Version
			if p.LastModifiedDate != nil {
				s.LastModified = p.LastModifiedDate.UTC().Format(time.RFC3339)
			}
		}
	}

	secrets := make([]VaultSecret, 0, len(byName))
	for _, s := range byName {
		secrets = append(secrets, *s)
	}
	sort.Slice(secrets, func(i, j int) bool { return secrets[i].Name < secrets[j].Name })

	writeVaultSecretsJSON(w, http.StatusOK, VaultSecretsResponse{Vault: t.vault, Path: prefix, Secrets: secrets})
}

// putVaultSecret sets or replaces the value of a secret the saved YAML declares.
func putVaultSecret(w http.ResponseWriter, r *http.Request, t vaultSecretsTarget) {
	var req vaultSecretWriteRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, vaultSecretMaxBodyBytes))
	// The decoder's message could quote part of the body, which holds the value,
	// so it is not passed on.
	if err := dec.Decode(&req); err != nil {
		writeVaultSecretsError(w, http.StatusBadRequest, "invalid request body: send JSON {\"name\": ..., \"value\": ...}")
		return
	}
	if !vaultSecretNamePattern.MatchString(req.Name) {
		writeVaultSecretsError(w, http.StatusBadRequest, "invalid secret name: "+vaultSecretNameRule)
		return
	}
	switch n := utf8.RuneCountInString(req.Value); {
	case n == 0:
		writeVaultSecretsError(w, http.StatusBadRequest, "value is required")
		return
	case n > vaultSecretMaxValueLength:
		writeVaultSecretsError(w, http.StatusBadRequest,
			fmt.Sprintf("value is %d characters; the limit is %d", n, vaultSecretMaxValueLength))
		return
	}
	if !t.isDeclared(req.Name) {
		writeVaultSecretsError(w, http.StatusConflict,
			fmt.Sprintf("%s is not in the vault's secrets list: save it to the YAML first", req.Name))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	client, ok := vaultSecretsClient(ctx, w, t.env)
	if !ok {
		return
	}

	out, err := client.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String(t.path(req.Name)),
		Value:     aws.String(req.Value),
		Type:      ssmtypes.ParameterTypeSecureString,
		Overwrite: aws.Bool(true),
	})
	if err != nil {
		writeVaultSecretsAWSError(w, err, req.Value)
		return
	}
	writeVaultSecretsJSON(w, http.StatusOK, vaultSecretWriteResponse{Name: req.Name, Version: out.Version})
}

// deleteVaultSecret deletes a value, declared or not. For a declared name the
// answer warns that connected tasks still list it.
func deleteVaultSecret(w http.ResponseWriter, r *http.Request, t vaultSecretsTarget) {
	name := r.URL.Query().Get("name")
	if !vaultSecretDeletableNamePattern.MatchString(name) {
		writeVaultSecretsError(w, http.StatusBadRequest,
			"invalid secret name: use up to 128 letters, digits, underscores, dots and hyphens")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	client, ok := vaultSecretsClient(ctx, w, t.env)
	if !ok {
		return
	}

	_, err := client.DeleteParameter(ctx, &ssm.DeleteParameterInput{Name: aws.String(t.path(name))})
	if err != nil {
		var notFound *ssmtypes.ParameterNotFound
		if errors.As(err, &notFound) {
			writeVaultSecretsError(w, http.StatusNotFound,
				fmt.Sprintf("%s has no value in vault %q", name, t.vault))
			return
		}
		writeVaultSecretsAWSError(w, err, "")
		return
	}
	resp := vaultSecretDeleteResponse{Name: name, Deleted: true}
	if t.isDeclared(name) {
		resp.Warning = vaultSecretStillListedWarning
	}
	writeVaultSecretsJSON(w, http.StatusOK, resp)
}
