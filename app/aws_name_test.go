package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The Go half of the naming cascade, checked against the same vector table
// modules/naming/tests/cascade.tftest.hcl mirrors. See app/aws_name.go for why
// the algorithm exists in two places at all.

type nameVector struct {
	Name                  string   `json:"name"`
	Project               string   `json:"project"`
	Env                   string   `json:"env"`
	Parts                 []string `json:"parts"`
	Limit                 int      `json:"limit"`
	Separator             string   `json:"separator"`
	Suffix                string   `json:"suffix"`
	LegacyOverride        string   `json:"legacy_override"`
	Want                  string   `json:"want"`
	WantMaxLen            int      `json:"want_max_len"`
	WantPrefix            string   `json:"want_prefix"`
	WantSuffix            string   `json:"want_suffix"`
	WantForm              int      `json:"want_form"`
	WantNoDoubleSeparator bool     `json:"want_no_double_separator"`
}

type nameVectorFile struct {
	Vectors []nameVector `json:"vectors"`

	Distinctness struct {
		Project    string   `json:"project"`
		Env        string   `json:"env"`
		Limit      int      `json:"limit"`
		Separator  string   `json:"separator"`
		Identities []string `json:"identities"`
	} `json:"distinctness"`

	CrossEnv struct {
		Project   string   `json:"project"`
		Envs      []string `json:"envs"`
		Identity  string   `json:"identity"`
		Limit     int      `json:"limit"`
		Separator string   `json:"separator"`
	} `json:"cross_env"`
}

func loadNameVectors(t *testing.T) nameVectorFile {
	t.Helper()
	raw, err := os.ReadFile("testdata/aws_name_vectors.json")
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var f nameVectorFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parse vectors: %v", err)
	}
	if len(f.Vectors) == 0 {
		t.Fatal("vector file has no vectors")
	}
	return f
}

func TestAWSNameVectors(t *testing.T) {
	f := loadNameVectors(t)

	for _, v := range f.Vectors {
		t.Run(v.Name, func(t *testing.T) {
			legacy := v.LegacyOverride
			if legacy == "" {
				legacy = legacyName(v.Project, v.Env, v.Parts, v.Separator, v.Suffix)
			}
			got := awsName(v.Project, v.Env, v.Parts, v.Limit, v.Separator, v.Suffix, legacy)

			if v.Want != "" && got != v.Want {
				t.Errorf("awsName = %q, want %q", got, v.Want)
			}
			if len(got) > v.Limit {
				t.Errorf("awsName = %q (%d chars), exceeds limit %d", got, len(got), v.Limit)
			}
			if v.WantMaxLen > 0 && len(got) > v.WantMaxLen {
				t.Errorf("awsName = %q (%d chars), want <= %d", got, len(got), v.WantMaxLen)
			}
			if v.WantPrefix != "" && !strings.HasPrefix(got, v.WantPrefix) {
				t.Errorf("awsName = %q, want prefix %q: the identity must lead the name", got, v.WantPrefix)
			}
			if v.WantSuffix != "" && !strings.HasSuffix(got, v.WantSuffix) {
				t.Errorf("awsName = %q, want suffix %q: AWS rejects a FIFO queue without it", got, v.WantSuffix)
			}
			if v.WantNoDoubleSeparator {
				if strings.Contains(got, v.Separator+v.Separator) {
					t.Errorf("awsName = %q contains a doubled separator", got)
				}
				if strings.HasSuffix(got, v.Separator) || strings.HasPrefix(got, v.Separator) {
					t.Errorf("awsName = %q starts or ends on the separator", got)
				}
			}
		})
	}
}

// The legacy form must come back byte-identical whenever it fits. This is the
// property that keeps a migration from renaming, and therefore destroying and
// recreating, resources that are already deployed.
func TestAWSNameLeavesDeployedNamesAlone(t *testing.T) {
	cases := []struct {
		project, env string
		parts        []string
		limit        int
		suffix       string
	}{
		{"myapp", "dev", []string{"orders"}, 80, ""},
		{"myapp", "dev", []string{"orders"}, 80, ".fifo"},
		{"myapp", "dev", []string{"orders", "dlq"}, 80, ".fifo"},
		{"myapp", "prod", []string{"notifications"}, 256, ""},
		{"myapp", "staging", []string{"events"}, 80, ""},
	}

	for _, c := range cases {
		legacy := legacyName(c.project, c.env, c.parts, "-", c.suffix)
		got := awsName(c.project, c.env, c.parts, c.limit, "-", c.suffix, legacy)
		if got != legacy {
			t.Errorf("awsName(%s/%s/%v) = %q, want the deployed name %q unchanged",
				c.project, c.env, c.parts, got, legacy)
		}
	}
}

// Truncation must never be what decides uniqueness. These identities differ
// only past the point the readable head is cut.
func TestAWSNameTruncatedIdentitiesStayDistinct(t *testing.T) {
	f := loadNameVectors(t)
	d := f.Distinctness

	seen := map[string]string{}
	for _, id := range d.Identities {
		got := awsName(d.Project, d.Env, []string{id}, d.Limit, d.Separator, "", "")
		if prev, dup := seen[got]; dup {
			t.Fatalf("%q and %q both produced %q; the digest is supposed to prevent this", prev, id, got)
		}
		seen[got] = id
		if len(got) > d.Limit {
			t.Errorf("%q produced %q (%d chars), over limit %d", id, got, len(got), d.Limit)
		}
	}
}

// Two environments in one AWS account must not produce the same name. This is
// why the digest covers env and not just the identity.
func TestAWSNameEnvironmentsDoNotCollide(t *testing.T) {
	f := loadNameVectors(t)
	c := f.CrossEnv

	seen := map[string]string{}
	for _, env := range c.Envs {
		got := awsName(c.Project, env, []string{c.Identity}, c.Limit, c.Separator, "", "")
		if prev, dup := seen[got]; dup {
			t.Fatalf("envs %q and %q both produced %q", prev, env, got)
		}
		seen[got] = env
	}
}

func TestAWSNameSuffixHelper(t *testing.T) {
	if got := awsNameSuffix(true); got != ".fifo" {
		t.Errorf("awsNameSuffix(true) = %q, want .fifo", got)
	}
	for _, falsey := range []interface{}{false, nil, "", 0} {
		if got := awsNameSuffix(falsey); got != "" {
			t.Errorf("awsNameSuffix(%v) = %q, want empty", falsey, got)
		}
	}
}

func TestAWSNamePartsDropsEmpty(t *testing.T) {
	got := awsNameParts("orders", "")
	if len(got) != 1 || got[0] != "orders" {
		t.Errorf("awsNameParts(orders, \"\") = %v, want [orders]", got)
	}
	got = awsNameParts("orders", "dlq")
	if len(got) != 2 {
		t.Errorf("awsNameParts(orders, dlq) = %v, want two parts", got)
	}
	if got := awsNameParts("  ", "dlq"); len(got) != 1 {
		t.Errorf("whitespace-only part should be dropped, got %v", got)
	}
}

func TestAWSNameLimitForRejectsUnknownKind(t *testing.T) {
	if _, err := awsNameLimitFor("sqs_queue"); err != nil {
		t.Errorf("sqs_queue should be known: %v", err)
	}
	if _, err := awsNameLimitFor("not_a_real_resource"); err == nil {
		t.Error("expected an error for an unknown kind; a template must not be able to invent a cap")
	}
}
