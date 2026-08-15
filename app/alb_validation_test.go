package main

import (
	"strings"
	"testing"
)

// Found on real AWS, not in a test: alb.enabled with domain.enabled false plans
// perfectly and then fails mid-apply with "A certificate must be specified for
// HTTPS listeners", after the VPC, cluster, ALB and ~130 other resources exist.
//
// It plans clean because certificate_arn is simply an empty string, which is
// valid HCL. Only AWS knows it is not a valid listener. So the check has to be
// here, before anything is created.

func albValidationConfig(albEnabled, domainEnabled bool) map[string]interface{} {
	return map[string]interface{}{
		"alb":    map[string]interface{}{"enabled": albEnabled},
		"domain": map[string]interface{}{"enabled": domainEnabled},
	}
}

func TestValidateALBRequiresACertificate(t *testing.T) {
	tests := []struct {
		name      string
		config    map[string]interface{}
		wantError bool
	}{
		{
			name:      "alb on with a domain is fine",
			config:    albValidationConfig(true, true),
			wantError: false,
		},
		{
			name:      "alb on without a domain is refused",
			config:    albValidationConfig(true, false),
			wantError: true,
		},
		{
			name:      "alb off without a domain is fine",
			config:    albValidationConfig(false, false),
			wantError: false,
		},
		{
			name:      "alb off with a domain is fine",
			config:    albValidationConfig(false, true),
			wantError: false,
		},
		{
			name:      "no alb block at all is fine",
			config:    map[string]interface{}{},
			wantError: false,
		},
		{
			// The domain block being absent means the same as disabled, and
			// must not be read as "unknown, allow it".
			name:      "alb on with no domain block is refused",
			config:    map[string]interface{}{"alb": map[string]interface{}{"enabled": true}},
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateALBConfigMap(tc.config)

			if tc.wantError && err == nil {
				t.Fatal("expected an error, got none")
			}
			if !tc.wantError && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		})
	}
}

// The message has to name both keys and both ways out, because the AWS error it
// replaces names neither and arrives too late to act on.
func TestValidateALBErrorNamesTheFixes(t *testing.T) {
	err := validateALBConfigMap(albValidationConfig(true, false))
	if err == nil {
		t.Fatal("expected an error")
	}

	for _, want := range []string{
		"alb.enabled",
		"domain.enabled",
		"certificate",
		"API Gateway",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message does not mention %q:\n%s", want, err)
		}
	}
}
