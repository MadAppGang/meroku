package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

// Shared synthetic fixtures only. Reuse the supplied envYAML, chdir and inTempDir
// helpers; in particular, do not replace the real sibling reader or YAML loader.
func cidrQARand(seed uint64) func(int) int {
	return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)).IntN
}

func cidrQAEnv(t *testing.T, fields map[string]interface{}) Env {
	t.Helper()
	m := map[string]interface{}{
		"schema_version":  CurrentSchemaVersion,
		"project":         "qa-project",
		"env":             "yaml-dev",
		"region":          "us-east-1",
		"account_id":      "",
		"aws_profile":     "",
		"use_default_vpc": false,
	}
	for k, v := range fields {
		m[k] = v
	}
	body, err := yaml.Marshal(m)
	if err != nil {
		t.Fatalf("marshal synthetic environment: %v", err)
	}
	var env Env
	if err := yaml.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode synthetic environment: %v", err)
	}
	return env
}

func cidrQAEnvMap(t *testing.T, env Env) map[string]interface{} {
	t.Helper()
	body, err := yaml.Marshal(env)
	if err != nil {
		t.Fatalf("marshal environment: %v", err)
	}
	var m map[string]interface{}
	if err := yaml.Unmarshal(body, &m); err != nil {
		t.Fatalf("decode environment map: %v", err)
	}
	return m
}

func cidrQAWriteEnv(t *testing.T, dir, file, declaredEnv, account, extra string) string {
	t.Helper()
	envYAML(t, dir, file, "qa-project", declaredEnv, account)
	path := filepath.Join(dir, file)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open fixture %s: %v", file, err)
	}
	if _, err := f.WriteString(extra); err != nil {
		_ = f.Close()
		t.Fatalf("append synthetic network settings: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}
	return path
}

func cidrQAReadBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read synthetic fixture %s: %v", path, err)
	}
	return b
}

func cidrQAReadMap(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := yaml.Unmarshal(cidrQAReadBytes(t, path), &m); err != nil {
		t.Fatalf("decode synthetic fixture %s: %v", path, err)
	}
	return m
}

// Include directories so creating even an empty backup directory is observable.
func cidrQASnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			snapshot[rel] = "directory"
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snapshot[rel] = "file:" + string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot synthetic directory: %v", err)
	}
	return snapshot
}

func cidrQAAssertUnchanged(t *testing.T, dir string, before map[string]string) {
	t.Helper()
	if after := cidrQASnapshot(t, dir); !reflect.DeepEqual(after, before) {
		t.Errorf("read-only operation changed synthetic YAML or backup entries\nbefore: %v\nafter: %v", before, after)
	}
}

func cidrQADeps(uses []vpcCIDRUse) vpcCIDRDeps {
	return vpcCIDRDeps{
		siblings: func(_, _ string) ([]vpcCIDRUse, error) {
			return append([]vpcCIDRUse(nil), uses...), nil
		},
		aws: func(context.Context, Env) (vpcCIDRAWS, error) {
			return vpcCIDRAWS{}, errors.New("synthetic AWS target unavailable")
		},
		intn: cidrQARand(0x5eed),
	}
}

func cidrQAFakeAWS(vpcs []awsVPC, listingErr error) func(context.Context, Env) (vpcCIDRAWS, error) {
	return func(context.Context, Env) (vpcCIDRAWS, error) {
		return vpcCIDRAWS{
			ListVPCs: func(ctx context.Context) ([]awsVPC, error) {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				return vpcs, listingErr
			},
			// remoteStateLookup's signature is not supplied. Nil deliberately
			// exercises the documented "unknown", not a fabricated state API.
		}, nil
	}
}

// The expected pool comes from R2, independently of vpcCIDRPool and the picker.
// A used /16 for every other pool member makes a single remaining value exact.
func cidrQAPoolUsesExcept(free ...int) []vpcCIDRUse {
	var used []vpcCIDRUse
	for n := 2; n <= 255; n++ {
		if n == 99 {
			continue
		}
		keep := false
		for _, f := range free {
			if f == n {
				keep = true
			}
		}
		if !keep {
			used = append(used, vpcCIDRUse{
				Source: "synthetic sibling",
				Name:   fmt.Sprintf("synthetic-%d", n),
				Prefix: netip.MustParsePrefix(fmt.Sprintf("10.%d.0.0/16", n)),
			})
		}
	}
	return used
}

func cidrQAAssertCurated(t *testing.T, p netip.Prefix) {
	t.Helper()
	if !p.IsValid() || !p.Addr().Is4() {
		t.Fatalf("suggestion %v is not an IPv4 network", p)
	}
	a := p.Addr().As4()
	if p.Bits() != 16 || p != p.Masked() || a[0] != 10 || a[1] < 2 || a[1] == 99 {
		t.Errorf("suggestion %s is outside R2's curated pool", p)
	}
}

func cidrQAAssertSuggestions(t *testing.T, values []string, used []vpcCIDRUse) {
	t.Helper()
	seen := map[netip.Prefix]bool{}
	for _, value := range values {
		p, err := netip.ParsePrefix(value)
		if err != nil {
			t.Fatalf("suggestion %q is not a range: %v", value, err)
		}
		cidrQAAssertCurated(t, p)
		if seen[p] {
			t.Errorf("duplicate suggestion %s", p)
		}
		seen[p] = true
		for _, u := range used {
			if p.Overlaps(u.Prefix) {
				t.Errorf("suggestion %s overlaps %s (%s)", p, u.Prefix, u.Source)
			}
		}
	}
}

var cidrQARangeInHint = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+/[0-9]+`)
var cidrQAJargonInHint = regexp.MustCompile(`(?i)\b(mask|cidr|prefix)[a-z]*\b`)

func cidrQAAssertPlainHint(t *testing.T, hint string) {
	t.Helper()
	if strings.TrimSpace(hint) == "" {
		t.Error("plain hint is empty")
	}
	// A range value may contain a slash; hint prose may not. Monospace
	// rendering belongs to the web UI and is not asserted by Go text tests.
	prose := cidrQARangeInHint.ReplaceAllString(hint, "")
	if strings.Contains(prose, "/") || cidrQAJargonInHint.MatchString(prose) {
		t.Errorf("hint contains technical prose: %q", hint)
	}
}

func cidrQACaptureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	previous := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = previous
		_ = w.Close()
	}()
	type readResult struct {
		body []byte
		err  error
	}
	done := make(chan readResult, 1)
	go func() {
		body, err := io.ReadAll(r)
		_ = r.Close()
		done <- readResult{body, err}
	}()
	callErr := fn()
	os.Stdout = previous
	if err := w.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	result := <-done
	if result.err != nil {
		t.Fatalf("read captured stdout: %v", result.err)
	}
	return string(result.body), callErr
}
