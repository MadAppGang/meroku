package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"
)

// fakeVaultSSM records every call and answers from a map of existing parameters.
type fakeVaultSSM struct {
	params map[string]ssmtypes.ParameterMetadata

	describeInputs []*ssm.DescribeParametersInput
	putInputs      []*ssm.PutParameterInput
	deleteInputs   []*ssm.DeleteParameterInput

	// pages, when set, is what successive DescribeParameters calls return.
	pages [][]ssmtypes.ParameterMetadata

	describeErr error
	putErr      error
	deleteErr   error
	putVersion  int64
}

func (f *fakeVaultSSM) DescribeParameters(_ context.Context, in *ssm.DescribeParametersInput, _ ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error) {
	f.describeInputs = append(f.describeInputs, in)
	if f.describeErr != nil {
		return nil, f.describeErr
	}
	i := len(f.describeInputs) - 1
	if i >= len(f.pages) {
		return &ssm.DescribeParametersOutput{}, nil
	}
	out := &ssm.DescribeParametersOutput{Parameters: f.pages[i]}
	if i+1 < len(f.pages) {
		out.NextToken = aws.String("page-" + string(rune('1'+i)))
	}
	return out, nil
}

func (f *fakeVaultSSM) PutParameter(_ context.Context, in *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
	f.putInputs = append(f.putInputs, in)
	if f.putErr != nil {
		return nil, f.putErr
	}
	return &ssm.PutParameterOutput{Version: f.putVersion}, nil
}

func (f *fakeVaultSSM) DeleteParameter(_ context.Context, in *ssm.DeleteParameterInput, _ ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
	f.deleteInputs = append(f.deleteInputs, in)
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	if _, ok := f.params[aws.ToString(in.Name)]; !ok {
		return nil, &ssmtypes.ParameterNotFound{Message: aws.String("not found")}
	}
	return &ssm.DeleteParameterOutput{}, nil
}

const testVaultSecretValue = "postgres://example:fake-key-do-not-use@db.example.com/app"

// vaultTestEnv is a synthetic environment. payments declares database_url and
// api_key; shared declares token; legacy has no secrets list.
func vaultTestEnv() Env {
	var e Env
	e.Project = "myapp"
	e.Env = "dev"
	e.Region = "us-east-1"
	e.AWSProfile = "example-dev"
	e.AccountID = "000000000000"
	e.Vaults = []Vault{
		{Name: "shared", Secrets: []string{"token"}},
		{Name: "payments", Secrets: []string{"database_url", "api_key"}},
		{Name: "legacy"},
	}
	e.Workload.Vaults = []VaultConnection{{Name: "shared", Prefix: "SH"}}
	e.Services = []Service{{Name: "api", Vaults: []VaultConnection{{Name: "payments"}}}}
	return e
}

// setupVaultSecrets replaces the loader and the client constructor for one test.
func setupVaultSecrets(t *testing.T, fake *fakeVaultSSM) {
	t.Helper()
	env := vaultTestEnv()
	prevLoad, prevClient := loadEnvForVaultSecrets, newVaultSecretsClient
	loadEnvForVaultSecrets = func(name string) (Env, error) {
		if name != "dev" {
			return Env{}, errors.New("no such file")
		}
		return env, nil
	}
	newVaultSecretsClient = func(_ context.Context, e Env) (vaultSecretsSSMAPI, error) {
		if e.AWSProfile != "example-dev" || e.Region != "us-east-1" {
			t.Errorf("client built for profile %q region %q, want the env's own", e.AWSProfile, e.Region)
		}
		return fake, nil
	}
	t.Cleanup(func() { loadEnvForVaultSecrets, newVaultSecretsClient = prevLoad, prevClient })
}

func callVaultSecrets(method, query, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/api/vaults/secrets?"+query, strings.NewReader(body))
	handleVaultSecrets(rec, req)
	return rec
}

func vaultWriteBody(name, value string) string {
	b, _ := json.Marshal(map[string]string{"name": name, "value": value})
	return string(b)
}

func assertNoVaultValue(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if strings.Contains(rec.Body.String(), testVaultSecretValue) || strings.Contains(rec.Body.String(), "fake-key-do-not-use") {
		t.Errorf("response body contains the secret value: %s", rec.Body.String())
	}
}

func vaultErrorOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e.Error == "" {
		t.Fatalf("want a JSON {\"error\"} body, got %q", rec.Body.String())
	}
	return e.Error
}

// The SSM interface must not be able to read a value at all.
func TestVaultSecretsAPI_SSMInterfaceHasNoReadMethod(t *testing.T) {
	typ := reflect.TypeOf((*vaultSecretsSSMAPI)(nil)).Elem()
	var names []string
	for i := 0; i < typ.NumMethod(); i++ {
		names = append(names, typ.Method(i).Name)
	}
	sort.Strings(names)
	want := []string{"DeleteParameter", "DescribeParameters", "PutParameter"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("interface methods %v, want exactly %v", names, want)
	}
}

func TestVaultSecretsAPI_ListIsTheUnionOfYAMLAndSSM(t *testing.T) {
	modified := time.Date(2026, 11, 14, 3, 0, 0, 0, time.UTC)
	fake := &fakeVaultSSM{pages: [][]ssmtypes.ParameterMetadata{
		{
			{Name: aws.String("/dev/myapp/vault/payments/old_token"), Type: ssmtypes.ParameterTypeSecureString, Version: 1, LastModifiedDate: &modified},
		},
		{
			{Name: aws.String("/dev/myapp/vault/payments/database_url"), Type: ssmtypes.ParameterTypeSecureString, Version: 3, LastModifiedDate: &modified},
			{Name: aws.String("/dev/myapp/vault/payments/old/nested"), Type: ssmtypes.ParameterTypeSecureString, Version: 1},
		},
	}}
	setupVaultSecrets(t, fake)

	rec := callVaultSecrets(http.MethodGet, "env=dev&vault=payments", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", rec.Code, rec.Body.String())
	}

	if len(fake.describeInputs) != 2 {
		t.Fatalf("DescribeParameters called %d times, want 2 (paginated)", len(fake.describeInputs))
	}
	if got := aws.ToString(fake.describeInputs[1].NextToken); got != "page-1" {
		t.Errorf("second page token %q, want page-1", got)
	}
	f := fake.describeInputs[0].ParameterFilters
	if len(f) != 1 || aws.ToString(f[0].Key) != "Path" || aws.ToString(f[0].Option) != "OneLevel" ||
		!reflect.DeepEqual(f[0].Values, []string{"/dev/myapp/vault/payments"}) {
		t.Errorf("filter %+v, want Path OneLevel /dev/myapp/vault/payments", f)
	}

	var got VaultSecretsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Vault != "payments" || got.Path != "/dev/myapp/vault/payments/" {
		t.Errorf("vault %q path %q", got.Vault, got.Path)
	}
	want := []VaultSecret{
		{Name: "api_key", EnvName: "API_KEY", Declared: true, Set: false},
		{Name: "database_url", EnvName: "DATABASE_URL", Declared: true, Set: true, Type: "SecureString", Version: 3, LastModified: "2026-11-14T03:00:00Z"},
		{Name: "old_token", EnvName: "OLD_TOKEN", Declared: false, Set: true, Type: "SecureString", Version: 1, LastModified: "2026-11-14T03:00:00Z"},
	}
	if !reflect.DeepEqual(got.Secrets, want) {
		t.Errorf("secrets %+v\nwant %+v", got.Secrets, want)
	}
	// The wire form of a declared secret without a value carries no metadata.
	if !strings.Contains(rec.Body.String(), `{"name":"api_key","envName":"API_KEY","declared":true,"set":false}`) {
		t.Errorf("unset secret form: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"value"`) {
		t.Errorf("list response has a value field: %s", rec.Body.String())
	}
}

func TestVaultSecretsAPI_ListEmptyIsArray(t *testing.T) {
	setupVaultSecrets(t, &fakeVaultSSM{})
	rec := callVaultSecrets(http.MethodGet, "env=dev&vault=legacy", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"secrets":[]`) {
		t.Errorf("want secrets: [], got %s", rec.Body.String())
	}
}

// A vault without a secrets list lists what SSM has, all undeclared, so the
// tab can offer to import the names.
func TestVaultSecretsAPI_ListOfAVaultWithoutAList(t *testing.T) {
	setupVaultSecrets(t, &fakeVaultSSM{pages: [][]ssmtypes.ParameterMetadata{{
		{Name: aws.String("/dev/myapp/vault/legacy/DATABASE_URL"), Type: ssmtypes.ParameterTypeSecureString, Version: 2},
	}}})
	rec := callVaultSecrets(http.MethodGet, "env=dev&vault=legacy", "")
	var got VaultSecretsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := []VaultSecret{{Name: "DATABASE_URL", EnvName: "DATABASE_URL", Set: true, Type: "SecureString", Version: 2}}
	if !reflect.DeepEqual(got.Secrets, want) {
		t.Errorf("secrets %+v, want %+v", got.Secrets, want)
	}
}

func TestVaultSecretsAPI_PutSetsADeclaredSecret(t *testing.T) {
	fake := &fakeVaultSSM{putVersion: 4}
	setupVaultSecrets(t, fake)

	rec := callVaultSecrets(http.MethodPut, "env=dev&vault=payments", vaultWriteBody("database_url", testVaultSecretValue))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	assertNoVaultValue(t, rec)
	if rec.Body.String() != "{\"name\":\"database_url\",\"version\":4}\n" {
		t.Errorf("body %s", rec.Body.String())
	}
	if len(fake.describeInputs) != 0 {
		t.Errorf("PUT made %d DescribeParameters calls, want none", len(fake.describeInputs))
	}
	if len(fake.putInputs) != 1 {
		t.Fatalf("PutParameter called %d times", len(fake.putInputs))
	}
	in := fake.putInputs[0]
	if aws.ToString(in.Name) != "/dev/myapp/vault/payments/database_url" {
		t.Errorf("path %q", aws.ToString(in.Name))
	}
	if in.Type != ssmtypes.ParameterTypeSecureString || !aws.ToBool(in.Overwrite) || in.KeyId != nil {
		t.Errorf("want SecureString, default key, Overwrite=true; got %s overwrite=%v key=%v", in.Type, aws.ToBool(in.Overwrite), in.KeyId)
	}
	if aws.ToString(in.Value) != testVaultSecretValue {
		t.Errorf("value not passed through")
	}
}

func TestVaultSecretsAPI_PutOfAnUndeclaredNameIs409(t *testing.T) {
	fake := &fakeVaultSSM{}
	setupVaultSecrets(t, fake)
	for _, tc := range []struct{ vault, name string }{
		{"payments", "stripe_key"},
		{"payments", "token"}, // declared in another vault
		{"legacy", "database_url"},
	} {
		rec := callVaultSecrets(http.MethodPut, "env=dev&vault="+tc.vault, vaultWriteBody(tc.name, testVaultSecretValue))
		if rec.Code != http.StatusConflict {
			t.Errorf("%s/%s: status %d, want 409", tc.vault, tc.name, rec.Code)
			continue
		}
		if msg := vaultErrorOf(t, rec); msg != tc.name+" is not in the vault's secrets list: save it to the YAML first" {
			t.Errorf("error %q", msg)
		}
		assertNoVaultValue(t, rec)
	}
	if len(fake.putInputs) != 0 {
		t.Errorf("PutParameter called for an undeclared name")
	}
}

func TestVaultSecretsAPI_DeleteDeclaredWarns(t *testing.T) {
	path := "/dev/myapp/vault/payments/database_url"
	fake := &fakeVaultSSM{params: map[string]ssmtypes.ParameterMetadata{path: {Name: aws.String(path)}}}
	setupVaultSecrets(t, fake)

	rec := callVaultSecrets(http.MethodDelete, "env=dev&vault=payments&name=database_url", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	want := `{"name":"database_url","deleted":true,"warning":"still listed in the YAML; connected tasks will fail to start until it is set again or removed and applied"}` + "\n"
	if rec.Body.String() != want {
		t.Errorf("body %s", rec.Body.String())
	}
	if aws.ToString(fake.deleteInputs[0].Name) != path {
		t.Errorf("deleted %q", aws.ToString(fake.deleteInputs[0].Name))
	}
}

func TestVaultSecretsAPI_DeleteUndeclaredHasNoWarning(t *testing.T) {
	paths := []string{"/dev/myapp/vault/payments/old_token", "/dev/myapp/vault/legacy/DATABASE_URL"}
	fake := &fakeVaultSSM{params: map[string]ssmtypes.ParameterMetadata{}}
	for _, p := range paths {
		fake.params[p] = ssmtypes.ParameterMetadata{Name: aws.String(p)}
	}
	setupVaultSecrets(t, fake)

	rec := callVaultSecrets(http.MethodDelete, "env=dev&vault=payments&name=old_token", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "{\"name\":\"old_token\",\"deleted\":true}\n" {
		t.Errorf("status %d body %s", rec.Code, rec.Body.String())
	}
	// A name from before secret names were declared can still be deleted.
	rec = callVaultSecrets(http.MethodDelete, "env=dev&vault=legacy&name=DATABASE_URL", "")
	if rec.Code != http.StatusOK {
		t.Errorf("legacy upper-case name: status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestVaultSecretsAPI_DeleteMissingIs404(t *testing.T) {
	setupVaultSecrets(t, &fakeVaultSSM{})
	rec := callVaultSecrets(http.MethodDelete, "env=dev&vault=shared&name=token", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", rec.Code)
	}
	vaultErrorOf(t, rec)
}

func TestVaultSecretsAPI_UndeclaredVaultIs404(t *testing.T) {
	fake := &fakeVaultSSM{}
	setupVaultSecrets(t, fake)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		rec := callVaultSecrets(method, "env=dev&vault=other&name=token", vaultWriteBody("token", testVaultSecretValue))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", method, rec.Code)
			continue
		}
		if msg := vaultErrorOf(t, rec); msg != `vault "other" is not declared in dev.yaml` {
			t.Errorf("%s: error %q", method, msg)
		}
	}
	// A path-like vault name is never a declared one.
	rec := callVaultSecrets(http.MethodGet, "env=dev&vault="+url.QueryEscape("shared/../payments"), "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("path-like vault: status %d, want 404", rec.Code)
	}
	if len(fake.describeInputs)+len(fake.putInputs)+len(fake.deleteInputs) != 0 {
		t.Errorf("AWS was called for an undeclared vault")
	}
}

func TestVaultSecretsAPI_UnknownEnvIs404(t *testing.T) {
	setupVaultSecrets(t, &fakeVaultSSM{})
	rec := callVaultSecrets(http.MethodGet, "env=prod&vault=shared", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", rec.Code)
	}
	vaultErrorOf(t, rec)
}

func TestVaultSecretsAPI_BadEnvIs400(t *testing.T) {
	loaded := 0
	setupVaultSecrets(t, &fakeVaultSSM{})
	prev := loadEnvForVaultSecrets
	loadEnvForVaultSecrets = func(n string) (Env, error) { loaded++; return prev(n) }
	t.Cleanup(func() { loadEnvForVaultSecrets = prev })

	for _, name := range []string{"", "../x", "a/b", "..", `a\b`, "dev.yaml"} {
		rec := callVaultSecrets(http.MethodGet, "env="+url.QueryEscape(name)+"&vault=shared", "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("env %q: status %d, want 400", name, rec.Code)
		}
	}
	if loaded != 0 {
		t.Errorf("the loader ran %d times for refused env names", loaded)
	}
	rec := callVaultSecrets(http.MethodGet, "env=dev", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing vault: status %d, want 400", rec.Code)
	}
}

func TestVaultSecretsAPI_BadNameIs400(t *testing.T) {
	fake := &fakeVaultSSM{}
	setupVaultSecrets(t, fake)
	for _, name := range []string{"", "1abc", "Database_url", "DATABASE_URL", "has-dash", "a/b", "../x", "with space", "a" + strings.Repeat("b", 63)} {
		rec := callVaultSecrets(http.MethodPut, "env=dev&vault=payments", vaultWriteBody(name, testVaultSecretValue))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("PUT name %q: status %d, want 400", name, rec.Code)
		}
		assertNoVaultValue(t, rec)
	}
	for _, name := range []string{"", "a/b", "../x", "..", ".hidden", "with space", strings.Repeat("a", 129)} {
		rec := callVaultSecrets(http.MethodDelete, "env=dev&vault=payments&name="+url.QueryEscape(name), "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("DELETE name %q: status %d, want 400", name, rec.Code)
		}
	}
	if len(fake.putInputs)+len(fake.deleteInputs) != 0 {
		t.Errorf("AWS was written for a bad name")
	}
}

func TestVaultSecretsAPI_BadValueIs400(t *testing.T) {
	fake := &fakeVaultSSM{}
	setupVaultSecrets(t, fake)
	for _, value := range []string{"", strings.Repeat("x", 4097), strings.Repeat("é", 4097)} {
		rec := callVaultSecrets(http.MethodPut, "env=dev&vault=payments", vaultWriteBody("api_key", value))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("value of %d bytes: status %d, want 400", len(value), rec.Code)
		}
	}
	if len(fake.putInputs) != 0 {
		t.Errorf("PutParameter called for a bad value")
	}
	rec := callVaultSecrets(http.MethodPut, "env=dev&vault=payments", vaultWriteBody("api_key", strings.Repeat("é", 4096)))
	if rec.Code != http.StatusOK {
		t.Errorf("4096 characters: status %d, want 200", rec.Code)
	}
	rec = callVaultSecrets(http.MethodPut, "env=dev&vault=payments", `{"name":"api_key","value":"`+testVaultSecretValue)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("broken JSON: status %d, want 400", rec.Code)
	}
	assertNoVaultValue(t, rec)
}

func TestVaultSecretsAPI_AccessDeniedIs403(t *testing.T) {
	denied := &smithy.GenericAPIError{
		Code:    "AccessDeniedException",
		Message: "User: arn:aws:iam::000000000000:user/example is not authorized to perform: ssm:PutParameter",
	}
	cases := []struct {
		name   string
		fake   *fakeVaultSSM
		method string
		query  string
	}{
		{"list", &fakeVaultSSM{describeErr: denied}, http.MethodGet, "env=dev&vault=shared"},
		{"put", &fakeVaultSSM{putErr: denied}, http.MethodPut, "env=dev&vault=shared"},
		{"delete", &fakeVaultSSM{deleteErr: denied}, http.MethodDelete, "env=dev&vault=shared&name=token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupVaultSecrets(t, tc.fake)
			rec := callVaultSecrets(tc.method, tc.query, vaultWriteBody("token", testVaultSecretValue))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status %d, want 403: %s", rec.Code, rec.Body.String())
			}
			if msg := vaultErrorOf(t, rec); msg != denied.Message {
				t.Errorf("error %q, want the AWS message", msg)
			}
			assertNoVaultValue(t, rec)
		})
	}
}

// An AWS error that quotes the value must not carry it back to the browser.
func TestVaultSecretsAPI_AWSErrorNeverEchoesValue(t *testing.T) {
	setupVaultSecrets(t, &fakeVaultSSM{putErr: &smithy.GenericAPIError{
		Code:    "ValidationException",
		Message: "Value '" + testVaultSecretValue + "' failed to satisfy constraint",
	}})
	rec := callVaultSecrets(http.MethodPut, "env=dev&vault=shared", vaultWriteBody("token", testVaultSecretValue))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", rec.Code)
	}
	assertNoVaultValue(t, rec)
}

func TestVaultSecretsAPI_MethodNotAllowed(t *testing.T) {
	setupVaultSecrets(t, &fakeVaultSSM{})
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		rec := callVaultSecrets(method, "env=dev&vault=shared", vaultWriteBody("token", testVaultSecretValue))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status %d, want 405", method, rec.Code)
		}
		vaultErrorOf(t, rec)
	}
}

func TestVaultSecretsAPI_ClientErrorIs500(t *testing.T) {
	setupVaultSecrets(t, &fakeVaultSSM{})
	prev := newVaultSecretsClient
	newVaultSecretsClient = func(context.Context, Env) (vaultSecretsSSMAPI, error) {
		return nil, errors.New("profile example-dev not found")
	}
	t.Cleanup(func() { newVaultSecretsClient = prev })
	rec := callVaultSecrets(http.MethodGet, "env=dev&vault=shared", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", rec.Code)
	}
	vaultErrorOf(t, rec)
}

// The route is registered in mainRouter behind the CORS middleware, and reaches
// the handler.
func TestVaultSecretsAPI_RouteIsRegistered(t *testing.T) {
	setupVaultSecrets(t, &fakeVaultSSM{})
	router := mainRouter()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/api/vaults/secrets", nil))
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Errorf("preflight: status %d headers %v", rec.Code, rec.Header())
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/vaults/secrets?env=dev&vault=shared", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"vault":"shared"`) {
		t.Errorf("GET through the router: status %d body %s", rec.Code, rec.Body.String())
	}
}
