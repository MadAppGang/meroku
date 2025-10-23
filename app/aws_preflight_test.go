package main

import (
	"strings"
	"testing"
)

func TestParseAWSCLIVersion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard AWS CLI v2 output",
			input:    "aws-cli/2.31.20 Python/3.11.6 Darwin/24.0.0 source/arm64",
			expected: "2.31.20",
		},
		{
			name:     "AWS CLI with different platform",
			input:    "aws-cli/2.35.0 Python/3.11.8 Linux/6.1.0-25-amd64 exe/x86_64.ubuntu.22",
			expected: "2.35.0",
		},
		{
			name:     "AWS CLI minimal output",
			input:    "aws-cli/2.31.20",
			expected: "2.31.20",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "malformed output",
			input:    "something-else/1.0.0",
			expected: "",
		},
		{
			name:     "no version number",
			input:    "aws-cli/",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAWSCLIVersion(tt.input)
			if result != tt.expected {
				t.Errorf("parseAWSCLIVersion(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseTerraformVersion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "standard Terraform output",
			input: `Terraform v1.13.4
on darwin_arm64`,
			expected: "1.13.4",
		},
		{
			name: "Terraform with additional info",
			input: `Terraform v1.15.0
on linux_amd64
+ provider registry.terraform.io/hashicorp/aws v5.0.0`,
			expected: "1.15.0",
		},
		{
			name:     "Terraform minimal output",
			input:    "Terraform v1.13.4",
			expected: "1.13.4",
		},
		{
			name: "Terraform with dev version",
			input: `Terraform v1.13.4-dev
on darwin_arm64`,
			expected: "1.13.4-dev",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "malformed output",
			input:    "version 1.0.0",
			expected: "",
		},
		{
			name:     "only one word",
			input:    "Terraform",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTerraformVersion(tt.input)
			if result != tt.expected {
				t.Errorf("parseTerraformVersion(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsVersionAtLeast(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		minimum  string
		expected bool
	}{
		// Equal versions
		{
			name:     "equal versions",
			current:  "2.31.20",
			minimum:  "2.31.20",
			expected: true,
		},
		// Current version higher
		{
			name:     "higher major version",
			current:  "3.0.0",
			minimum:  "2.31.20",
			expected: true,
		},
		{
			name:     "higher minor version",
			current:  "2.32.0",
			minimum:  "2.31.20",
			expected: true,
		},
		{
			name:     "higher patch version",
			current:  "2.31.21",
			minimum:  "2.31.20",
			expected: true,
		},
		// Current version lower
		{
			name:     "lower major version",
			current:  "1.31.20",
			minimum:  "2.31.20",
			expected: false,
		},
		{
			name:     "lower minor version",
			current:  "2.30.20",
			minimum:  "2.31.20",
			expected: false,
		},
		{
			name:     "lower patch version",
			current:  "2.31.19",
			minimum:  "2.31.20",
			expected: false,
		},
		// Terraform versions
		{
			name:     "Terraform exact match",
			current:  "1.13.4",
			minimum:  "1.13.4",
			expected: true,
		},
		{
			name:     "Terraform higher",
			current:  "1.15.0",
			minimum:  "1.13.4",
			expected: true,
		},
		{
			name:     "Terraform lower",
			current:  "1.12.0",
			minimum:  "1.13.4",
			expected: false,
		},
		// Edge cases
		{
			name:     "dev version suffix",
			current:  "1.13.4-dev",
			minimum:  "1.13.4",
			expected: true,
		},
		{
			name:     "version with only major",
			current:  "2",
			minimum:  "1.13.4",
			expected: true,
		},
		{
			name:     "version with major.minor",
			current:  "2.31",
			minimum:  "2.31.20",
			expected: false, // 2.31.0 < 2.31.20
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isVersionAtLeast(tt.current, tt.minimum)
			if result != tt.expected {
				t.Errorf("isVersionAtLeast(%q, %q) = %v, want %v", tt.current, tt.minimum, result, tt.expected)
			}
		})
	}
}

func TestParseVersionParts(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []int
	}{
		{
			name:     "standard semver",
			input:    "2.31.20",
			expected: []int{2, 31, 20},
		},
		{
			name:     "major.minor only",
			input:    "1.13",
			expected: []int{1, 13},
		},
		{
			name:     "major only",
			input:    "2",
			expected: []int{2},
		},
		{
			name:     "with dev suffix",
			input:    "1.13.4-dev",
			expected: []int{1, 13, 4},
		},
		{
			name:     "with rc suffix",
			input:    "2.0.0-rc1",
			expected: []int{2, 0, 0},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseVersionParts(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("parseVersionParts(%q) length = %d, want %d", tt.input, len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("parseVersionParts(%q)[%d] = %d, want %d", tt.input, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

// TestRunCommandWithOutput verifies that runCommandWithOutput correctly captures stdout
func TestRunCommandWithOutput(t *testing.T) {
	// Test with a simple command that outputs to stdout
	output, err := runCommandWithOutput("echo", "test output")
	if err != nil {
		t.Fatalf("runCommandWithOutput failed: %v", err)
	}

	// Trim whitespace from output
	output = strings.TrimSpace(output)
	if output != "test output" {
		t.Errorf("runCommandWithOutput() = %q, want %q", output, "test output")
	}
}
